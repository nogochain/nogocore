// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

// Package api provides the HTTP REST API and JSON-RPC mining server.
// The JSON-RPC endpoint handles getblocktemplate, submitblock, and related
// mining commands that follow the btcd architecture: the miner builds a
// complete block and the node validates it as a self-contained unit.
package api

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/nogochain/nogocommons/address"
	"github.com/nogochain/nogocommons/chaincfg"
	"github.com/nogochain/nogocommons/chainhash"
	"github.com/nogochain/nogocommons/nogopow"
	"github.com/nogochain/nogocommons/nogoutil"
	"github.com/nogochain/nogocore/blockchain"
	"github.com/nogochain/nogocore/mining"
)

// rpcRequest represents a JSON-RPC 2.0 request.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

// rpcResponse represents a JSON-RPC 2.0 response.
type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// rpcError represents a JSON-RPC 2.0 error.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcMiningHandler handles JSON-RPC mining requests (getblocktemplate, submitblock).
// It wraps a blockchain instance and an optional block template generator,
// serving the core mining protocol between the node and solo miner.
type rpcMiningHandler struct {
	chain     *blockchain.BlockChain
	params    *chaincfg.Params
	generator *mining.BlkTmplGenerator

	rpcUser   string
	rpcPass   string
	authHash  [sha256.Size]byte

	// Template cache for long-poll (simplified: single cached template).
	mu           sync.Mutex
	cachedTemplate *mining.BlockTemplate
}

// newRPCMiningHandler creates a JSON-RPC mining handler with basic auth.
// If rpcUser is empty, authentication is disabled (insecure — dev/test only).
func newRPCMiningHandler(chain *blockchain.BlockChain, params *chaincfg.Params,
	generator *mining.BlkTmplGenerator, rpcUser, rpcPass string) *rpcMiningHandler {

	h := &rpcMiningHandler{
		chain:     chain,
		params:    params,
		generator: generator,
		rpcUser:   rpcUser,
		rpcPass:   rpcPass,
	}
	if rpcUser != "" {
		auth := "Basic " + hex.EncodeToString([]byte(rpcUser+":"+rpcPass))
		h.authHash = sha256.Sum256([]byte(auth))
	}
	return h
}

// checkAuth verifies HTTP Basic authentication in constant time.
func (h *rpcMiningHandler) checkAuth(r *http.Request) bool {
	if h.rpcUser == "" {
		return true // auth disabled
	}
	_, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	auth := "Basic " + hex.EncodeToString([]byte(h.rpcUser+":"+pass))
	authHash := sha256.Sum256([]byte(auth))
	return subtle.ConstantTimeCompare(authHash[:], h.authHash[:]) == 1
}

// ServeHTTP implements http.Handler for JSON-RPC mining requests.
func (h *rpcMiningHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.checkAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="nogocore"`)
		http.Error(w, "401 Unauthorized.", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, nil, -32700, "Parse error: "+err.Error())
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeError(w, nil, -32700, "Parse error: "+err.Error())
		return
	}

	var result interface{}
	var rpcErr *rpcError

	switch req.Method {
	case "getblocktemplate":
		result, rpcErr = h.handleGetBlockTemplate(body)
	case "submitblock":
		result, rpcErr = h.handleSubmitBlock(body)
	case "getbestblock":
		result, rpcErr = h.handleGetBestBlock()
	case "getblockcount":
		result, rpcErr = h.handleGetBlockCount()
	case "getblockchaininfo":
		result, rpcErr = h.handleGetBlockchainInfo()
	default:
		rpcErr = &rpcError{Code: -32601, Message: "Method not found: " + req.Method}
	}

	resp := rpcResponse{
		JSONRPC: "2.0",
		Result:  result,
		Error:   rpcErr,
		ID:      req.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[RPC] Failed to encode response: %v", err)
	}
}

// writeError writes a JSON-RPC error response.
func (h *rpcMiningHandler) writeError(w http.ResponseWriter, id interface{}, code int, msg string) {
	resp := rpcResponse{
		JSONRPC: "2.0",
		Error:   &rpcError{Code: code, Message: msg},
		ID:      id,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// gbtRequest is the parsed getblocktemplate request.
type gbtRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []struct {
		Capabilities  []string `json:"capabilities"`
		Rules         []string `json:"rules"`
		CoinbaseAddr  string   `json:"coinbaseaddr"`
		MinerAddr     string   `json:"coinbasetxn"`
	} `json:"params"`
	ID interface{} `json:"id"`
}

// handleGetBlockTemplate generates a block template for mining.
// Returns a complete template including coinbase and transaction data
// so the miner can build a self-contained block for submitblock.
func (h *rpcMiningHandler) handleGetBlockTemplate(rawBody []byte) (interface{}, *rpcError) {
	var gbtReq gbtRequest
	if err := json.Unmarshal(rawBody, &gbtReq); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params: " + err.Error()}
	}

	if h.generator == nil {
		return nil, &rpcError{Code: -32603, Message: "Block template generator not available"}
	}

	// Decode the miner reward address from the request (if provided).
	// Falls back to OP_TRUE (anyone-can-spend) when no address is given.
	var minerAddr address.Address
	if len(gbtReq.Params) > 0 {
		addrStr := gbtReq.Params[0].CoinbaseAddr
		if addrStr == "" {
			addrStr = gbtReq.Params[0].MinerAddr
		}
		if addrStr != "" {
			var err error
			minerAddr, err = address.DecodeAddress(addrStr, h.params)
			if err != nil {
				// Try MainNet params as fallback for cross-network addresses.
				minerAddr, err = address.DecodeAddress(addrStr, &chaincfg.MainNetParams)
				if err != nil {
					log.Printf("[RPC] getblocktemplate: cannot decode miner address %q: %v", addrStr, err)
				}
			}
		}
	}

	template, err := h.generator.NewBlockTemplate(minerAddr)
	if err != nil {
		return nil, &rpcError{Code: -32603, Message: "Failed to generate template: " + err.Error()}
	}

	// Cache the latest template.
	h.mu.Lock()
	h.cachedTemplate = template
	h.mu.Unlock()

	// Serialize the block template into the GBT JSON format.
	// The miner needs: header fields, target, coinbase tx data, and
	// all non-coinbase transaction data to build a complete block.
	header := &template.Block.Header
	result := map[string]interface{}{
		"version":           header.Version,
		"previousblockhash": header.PrevBlock.String(),
		"curtime":           header.Timestamp.Unix(),
		"bits":              fmt.Sprintf("%x", header.Bits),
		"height":            int64(template.Height),
		"target":            fmt.Sprintf("%x", nogopow.CompactToBig(header.Bits)),
		"merkleroot":        header.MerkleRoot.String(),
	}

	// Add coinbase transaction data (hex-encoded serialized tx).
	if len(template.Block.Transactions) > 0 {
		cbTx := template.Block.Transactions[0]
		var cbBuf bytes.Buffer
		if err := cbTx.Serialize(&cbBuf); err == nil {
			result["coinbasetxn"] = map[string]interface{}{
				"data": hex.EncodeToString(cbBuf.Bytes()),
			}
		}
	}

	// Add non-coinbase transactions (hex-encoded serialized tx each).
	txList := make([]map[string]interface{}, 0, len(template.Block.Transactions)-1)
	for i := 1; i < len(template.Block.Transactions); i++ {
		tx := template.Block.Transactions[i]
		var txBuf bytes.Buffer
		if err := tx.Serialize(&txBuf); err != nil {
			continue
		}
		txList = append(txList, map[string]interface{}{
			"data": hex.EncodeToString(txBuf.Bytes()),
			"txid": tx.TxHash().String(),
		})
	}
	result["transactions"] = txList

	return result, nil
}

// submitBlockRequest is the parsed submitblock request.
type submitBlockRequest struct {
	JSONRPC string   `json:"jsonrpc"`
	Method  string   `json:"method"`
	Params  []string `json:"params"`
	ID      interface{} `json:"id"`
}

// handleSubmitBlock processes a submitted block.
// The block must be a complete, serialized wire.MsgBlock (header + all
// transactions).  The node deserializes and validates it as a self-contained
// unit — there is no cached template involved in validation.  This follows
// the btcd architecture where the block stands on its own.
func (h *rpcMiningHandler) handleSubmitBlock(rawBody []byte) (interface{}, *rpcError) {
	var subReq submitBlockRequest
	if err := json.Unmarshal(rawBody, &subReq); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params: " + err.Error()}
	}

	if len(subReq.Params) < 1 {
		return nil, &rpcError{Code: -32602, Message: "Missing hex block parameter"}
	}

	hexStr := subReq.Params[0]
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	serializedBlock, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid hex block: " + err.Error()}
	}

	block, err := nogoutil.NewBlockFromBytes(serializedBlock)
	if err != nil {
		return nil, &rpcError{Code: -32602, Message: "Block decode failed: " + err.Error()}
	}

	// Process the block through the blockchain consensus engine.
	// This includes PoW verification, transaction validation, and
	// connecting the block to the active chain.
	_, _, err = h.chain.ProcessBlock(block, blockchain.BFNone)
	if err != nil {
		// Return "rejected" (not an RPC error) — matches btcd behavior
		// where rejected blocks are informational results, not errors.
		return fmt.Sprintf("rejected: %s", err.Error()), nil
	}

	best := h.chain.BestSnapshot()
	log.Printf("[RPC] Accepted block %s at height %d via submitblock", block.Hash(), best.Height)
	return nil, nil
}

// handleGetBestBlock returns the current best block hash and height.
func (h *rpcMiningHandler) handleGetBestBlock() (interface{}, *rpcError) {
	best := h.chain.BestSnapshot()
	return map[string]interface{}{
		"hash":   best.Hash.String(),
		"height": int64(best.Height),
	}, nil
}

// handleGetBlockCount returns the current block count.
func (h *rpcMiningHandler) handleGetBlockCount() (interface{}, *rpcError) {
	best := h.chain.BestSnapshot()
	return int64(best.Height), nil
}

// handleGetBlockchainInfo returns blockchain state information.
func (h *rpcMiningHandler) handleGetBlockchainInfo() (interface{}, *rpcError) {
	best := h.chain.BestSnapshot()
	return map[string]interface{}{
		"chain":                 h.params.Name,
		"blocks":                int64(best.Height),
		"bestblockhash":         best.Hash.String(),
		"difficulty":            fmt.Sprintf("%x", best.Bits),
		"mediantime":            best.MedianTime.Unix(),
		"size":                  int64(h.params.MaxBlockSize),
		"pruned":                false,
		"verificationprogress":  1.0,
	}, nil
}

// StartRPCServer starts a standalone JSON-RPC HTTP server on the given address.
// It handles mining-related RPC methods (getblocktemplate, submitblock, etc.)
// and returns the http.Server for lifecycle management.
//
// It defaults to HTTPS+TLS with a self-signed certificate (same as btcd).
// If certFile/keyFile are provided but the files don't exist, they are
// auto-generated.  Miners connect with InsecureSkipVerify.
func StartRPCServer(addr string, chain *blockchain.BlockChain, params *chaincfg.Params,
	generator *mining.BlkTmplGenerator, rpcUser, rpcPass, certFile, keyFile string) *http.Server {

	handler := newRPCMiningHandler(chain, params, generator, rpcUser, rpcPass)

	mux := http.NewServeMux()
	mux.Handle("/", handler)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Generate self-signed cert if the paths are set but files don't exist.
	if certFile != "" && keyFile != "" && (!fileExists(certFile) || !fileExists(keyFile)) {
		if err := genCertPair(certFile, keyFile); err != nil {
			log.Printf("[RPC] Failed to generate TLS certificate: %v — falling back to HTTP", err)
			certFile = ""
			keyFile = ""
		}
	}

	useTLS := certFile != "" && keyFile != "" && fileExists(certFile) && fileExists(keyFile)

	go func() {
		if useTLS {
			log.Printf("[RPC] JSON-RPC mining server listening on %s (TLS)", addr)
		} else {
			log.Printf("[RPC] JSON-RPC mining server listening on %s (HTTP)", addr)
		}

		var err error
		if useTLS {
			err = srv.ListenAndServeTLS(certFile, keyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Printf("[RPC] JSON-RPC server error: %v", err)
		}
	}()

	return srv
}

// fileExists reports whether the named file exists.
func fileExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// genCertPair generates a key/cert pair to the paths provided.
func genCertPair(certFile, keyFile string) error {
	if runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		return fmt.Errorf("TLS certificate generation not available on this platform")
	}

	org := "nogocore autogenerated cert"
	validUntil := time.Now().Add(10 * 365 * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{org},
		},
		NotBefore:             time.Now(),
		NotAfter:              validUntil,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add localhost and hostname as DNS names.
	host, err := os.Hostname()
	if err == nil {
		template.DNSNames = append(template.DNSNames, host)
	}
	template.DNSNames = append(template.DNSNames, "localhost")

	// Generate ECDSA P-256 private key.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	var certBuf bytes.Buffer
	pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return err
	}

	var keyBuf bytes.Buffer
	pem.Encode(&keyBuf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	// Write cert and key files.
	if err = os.WriteFile(certFile, certBuf.Bytes(), 0644); err != nil {
		return err
	}
	if err = os.WriteFile(keyFile, keyBuf.Bytes(), 0600); err != nil {
		os.Remove(certFile)
		return err
	}

	return nil
}

// EmptyTxSource returns a minimal TxSource for BlkTmplGenerator when no
// mempool is available (produces blocks with coinbase transactions only).
func EmptyTxSource() mining.TxSource { return emptyTxSource{} }

// emptyTxSource is a minimal TxSource for BlkTmplGenerator when no mempool
// is available (produces blocks with coinbase transactions only).
type emptyTxSource struct{}

func (e emptyTxSource) LastUpdated() time.Time                          { return time.Now() }
func (e emptyTxSource) MiningDescs() []*mining.TxDesc                   { return nil }
func (e emptyTxSource) HaveTransaction(*chainhash.Hash) bool            { return false }

// SimpleMiningPolicy returns a default mining policy suitable for testnet.
func SimpleMiningPolicy() *mining.Policy {
	return &mining.Policy{
		BlockMinWeight:    400000,
		BlockMaxWeight:    4000000,
		BlockMinSize:      800000,
		BlockMaxSize:      8000000,
		BlockPrioritySize: 0,
		TxMinFreeFee:      1000,
	}
}
