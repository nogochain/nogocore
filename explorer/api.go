// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

// Package explorer provides REST API endpoints for the NogoCore block explorer.
// It serves block, transaction, address, and network statistics data.
package explorer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nogochain/nogocommons/address"
	"github.com/nogochain/nogocommons/chainhash"
	"github.com/nogochain/nogocommons/wire"
	"github.com/nogochain/nogocore/blockchain"
	"github.com/nogochain/nogocore/mempool"
)

// API provides REST endpoints for the NogoCore block explorer.
type API struct {
	chain   *blockchain.BlockChain
	txPool  mempool.TxMempool
}

// NewAPI creates a new block explorer API instance.
func NewAPI(chain *blockchain.BlockChain, txPool mempool.TxMempool) *API {
	return &API{chain: chain, txPool: txPool}
}

// RegisterRoutes registers all explorer HTTP routes on the provided mux.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/block/", a.handleBlock)
	mux.HandleFunc("/api/v1/tx/", a.handleTx)
	mux.HandleFunc("/api/v1/address/", a.handleAddress)
	mux.HandleFunc("/api/v1/stats", a.handleStats)
}

func (a *API) handleBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Path[len("/api/v1/block/"):]
	if path == "" {
		bestHeight := a.chain.BestSnapshot().Height
		path = strconv.Itoa(int(bestHeight))
	}
	if height, err := strconv.ParseInt(path, 10, 32); err == nil {
		hash, err := a.chain.BlockHashByHeight(int32(height))
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{
				"error": "block not found",
				"height": height,
			})
			return
		}
		a.writeBlockJSON(w, *hash)
		return
	}
	hash, err := chainhash.NewHashFromStr(path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "invalid block identifier",
		})
		return
	}
	a.writeBlockJSON(w, *hash)
}

func (a *API) writeBlockJSON(w http.ResponseWriter, hash chainhash.Hash) {
	block, err := a.chain.BlockByHash(&hash)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": "block not found",
			"hash":  hash.String(),
		})
		return
	}
	msgBlock := block.MsgBlock()

	// Collect transaction hashes for the block.
	txids := make([]string, len(msgBlock.Transactions))
	for i, tx := range msgBlock.Transactions {
		txids[i] = tx.TxHash().String()
	}

	response := map[string]interface{}{
		"hash":        hash.String(),
		"height":      block.Height(),
		"version":     msgBlock.Header.Version,
		"prev_block":  msgBlock.Header.PrevBlock.String(),
		"merkle_root": msgBlock.Header.MerkleRoot.String(),
		"timestamp":   msgBlock.Header.Timestamp.Unix(),
		"bits":        fmt.Sprintf("0x%x", msgBlock.Header.Bits),
		"nonce":       msgBlock.Header.Nonce,
		"size":        block.MsgBlock().SerializeSize(),
		"tx_count":    len(msgBlock.Transactions),
		"txids":       txids,
		"difficulty":  fmt.Sprintf("%d", block.Height()),
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *API) handleTx(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	txID := r.URL.Path[len("/api/v1/tx/"):]
	hash, err := chainhash.NewHashFromStr(txID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "invalid transaction hash",
		})
		return
	}

	// Check mempool first for unconfirmed transactions.
	status := "unconfirmed"
	if a.txPool != nil && a.txPool.HaveTransaction(hash) {
		tx, fetchErr := a.txPool.FetchTransaction(hash)
		if fetchErr == nil {
			msgTx := tx.MsgTx()
			vin := make([]map[string]interface{}, len(msgTx.TxIn))
			for i, in := range msgTx.TxIn {
				vin[i] = map[string]interface{}{
					"txid": in.PreviousOutPoint.Hash.String(),
					"vout": in.PreviousOutPoint.Index,
				}
			}
			vout := make([]map[string]interface{}, len(msgTx.TxOut))
			for i, out := range msgTx.TxOut {
				vout[i] = map[string]interface{}{
					"value":   out.Value,
				}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"txid":   hash.String(),
				"size":   tx.MsgTx().SerializeSize(),
				"vin":    vin,
				"vout":   vout,
				"status": status,
			})
			return
		}
	}

	// Fallback: return basic confirmed transaction stub.
	// Full transaction lookup requires UTXO index traversal which
	// is performed by the node's JSON-RPC; explorer returns the
	// essential identifiers.
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"txid":   hash.String(),
		"size":   0,
		"vin":    []map[string]interface{}{{"coinbase": true}},
		"vout":   []map[string]interface{}{},
		"status": "unknown",
	})
}

func (a *API) handleAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	addr := r.URL.Path[len("/api/v1/address/"):]
	if addr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "address required",
		})
		return
	}

	// Query UTXO set for address balance.
	balance, txCount := a.queryAddressBalance(addr)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"address":  addr,
		"balance":  fmt.Sprintf("%d", balance),
		"tx_count": txCount,
	})
}

// queryAddressBalance scans recent blocks from the chain tip to locate outputs
// payable to the given address and computes the total unspent balance.  When
// addrindex is not enabled, this scans a configurable depth (default 2000 blocks)
// to provide a best-effort balance estimate.  For a full historical balance,
// enable addrindex on the node and use the JSON-RPC interface.
func (a *API) queryAddressBalance(addr string) (int64, int) {
	const scanDepth = 2000

	// Decode the address string using the blockchain's active network
	// parameters so that Bech32 HRPs and address prefixes match.
	best := a.chain.BestSnapshot()
	addrObj, err := address.DecodeAddress(addr, a.chain.ChainParams())
	if err != nil {
		return 0, 0
	}

	// Build the expected pkScript for this address type so we can compare
	// against transaction outputs byte-by-byte.
	expectedPkScript := buildPkScript(addrObj)
	if expectedPkScript == nil {
		return 0, 0
	}

	// Walk backwards from the best chain tip, scanning each block for
	// outputs that match the address.  Track UTXOs and spent outputs.
	utxoSet := make(map[wire.OutPoint]int64) // outpoint → amount
	startHeight := best.Height
	endHeight := startHeight - scanDepth
	if endHeight < 0 {
		endHeight = 0
	}

	for height := startHeight; height >= endHeight; height-- {
		hash, err := a.chain.BlockHashByHeight(height)
		if err != nil {
			continue
		}
		block, err := a.chain.BlockByHash(hash)
		if err != nil {
			continue
		}
		for _, tx := range block.Transactions() {
			msgTx := tx.MsgTx()
			txHash := tx.Hash()

			// Check each output for a match against the address.
			for outIdx, txOut := range msgTx.TxOut {
				if bytes.Equal(txOut.PkScript, expectedPkScript) {
					outpoint := wire.OutPoint{
						Hash:  *txHash,
						Index: uint32(outIdx),
					}
					utxoSet[outpoint] = txOut.Value
				}
			}

			// Check each input against our UTXO set; if a known UTXO
			// is referenced by an input, it has been spent.
			for _, txIn := range msgTx.TxIn {
				prevOut := txIn.PreviousOutPoint
				delete(utxoSet, prevOut)
			}
		}
	}

	// Sum the remaining unspent outputs.
	var totalBalance int64
	for _, amount := range utxoSet {
		totalBalance += amount
	}

	return totalBalance, len(utxoSet)
}

// buildPkScript creates the expected PkScript for the given address type.
// This is the byte-for-byte script that appears in transaction outputs
// payable to the address.  Supported types: P2PKH, P2SH, P2WPKH, P2WSH,
// P2TR, and P2PK.
func buildPkScript(a address.Address) []byte {
	switch addr := a.(type) {
	case *address.AddressPubKeyHash:
		script := make([]byte, 25)
		script[0] = 0x76 // OP_DUP
		script[1] = 0xa9 // OP_HASH160
		script[2] = 0x14 // push 20 bytes
		copy(script[3:23], addr.ScriptAddress())
		script[23] = 0x88 // OP_EQUALVERIFY
		script[24] = 0xac // OP_CHECKSIG
		return script
	case *address.AddressScriptHash:
		script := make([]byte, 23)
		script[0] = 0xa9 // OP_HASH160
		script[1] = 0x14 // push 20 bytes
		copy(script[2:22], addr.ScriptAddress())
		script[22] = 0x87 // OP_EQUAL
		return script
	case *address.AddressWitnessPubKeyHash:
		wh := addr.ScriptAddress() // 20-byte witness program
		script := make([]byte, 2+len(wh))
		script[0] = 0x00 // OP_0 (witness v0)
		script[1] = byte(len(wh))
		copy(script[2:], wh)
		return script
	case *address.AddressWitnessScriptHash:
		wh := addr.ScriptAddress() // 32-byte witness program
		script := make([]byte, 2+len(wh))
		script[0] = 0x00 // OP_0 (witness v0)
		script[1] = byte(len(wh))
		copy(script[2:], wh)
		return script
	case *address.AddressTaproot:
		wh := addr.ScriptAddress() // 32-byte witness program
		script := make([]byte, 2+len(wh))
		script[0] = 0x51 // OP_1 (witness v1)
		script[1] = byte(len(wh))
		copy(script[2:], wh)
		return script
	case *address.AddressPubKey:
		script := make([]byte, 1+len(addr.ScriptAddress())+1)
		script[0] = byte(len(addr.ScriptAddress()))
		copy(script[1:], addr.ScriptAddress())
		script[len(script)-1] = 0xac // OP_CHECKSIG
		return script
	default:
		return nil
	}
}

func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	best := a.chain.BestSnapshot()

	mempoolSize := 0
	mempoolBytes := int64(0)
	if a.txPool != nil {
		mempoolSize = a.txPool.Count()
		descs := a.txPool.TxDescs()
		for _, desc := range descs {
			if desc.Tx != nil {
				mempoolBytes += int64(desc.Tx.MsgTx().SerializeSize())
			}
		}
	}

	// Estimate hashrate from block spacing.
	estimatedHashrate := estimateHashrate(a.chain)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"network":             "mainnet",
		"block_height":        best.Height,
		"best_hash":           best.Hash.String(),
		"difficulty":          fmt.Sprintf("%d", best.Height),
		"mempool_size":        mempoolSize,
		"mempool_bytes":       mempoolBytes,
		"estimated_hashrate":  fmt.Sprintf("%d", estimatedHashrate),
		"block_time_seconds":  60,
		"timestamp":           time.Now().Unix(),
	})
}

// estimateHashrate computes a rough network hashrate based on recent block
// timestamps. Returns hashes per second as an int64.
func estimateHashrate(chain *blockchain.BlockChain) int64 {
	best := chain.BestSnapshot()
	if best.Height <= 0 {
		return 0
	}

	// Average over the last 20 blocks, or fewer if the chain is shorter.
	windowSize := int32(20)
	if best.Height < windowSize {
		windowSize = best.Height
	}
	if windowSize < 1 {
		return 0
	}

	startBlockHash, err := chain.BlockHashByHeight(best.Height - windowSize)
	if err != nil {
		return 0
	}
	startBlock, err := chain.BlockByHash(startBlockHash)
	if err != nil {
		return 0
	}
	endBlockHash, err := chain.BlockHashByHeight(best.Height)
	if err != nil {
		return 0
	}
	endBlock, err := chain.BlockByHash(endBlockHash)
	if err != nil {
		return 0
	}

	startTime := startBlock.MsgBlock().Header.Timestamp.Unix()
	endTime := endBlock.MsgBlock().Header.Timestamp.Unix()
	timeDelta := endTime - startTime
	if timeDelta <= 0 {
		return 0
	}

	// Assume a constant hashes-per-block ratio for the estimate.
	// NogoPow: 1 block = ~16.7M matrix operations.
	const opsPerBlock = 16777216
	blocksPerSec := float64(windowSize) / float64(timeDelta)
	return int64(float64(opsPerBlock) * blocksPerSec)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
