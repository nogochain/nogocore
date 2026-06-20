// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	appName    = "nogoctl"
	appVersion = "0.1.0"
)

// config holds the CLI configuration parsed from flags.
type config struct {
	TestNet   bool
	RPCURL    string
	RPCUser   string
	RPCPass   string
}

// rpcClient performs JSON-RPC 2.0 calls against a nogocore node.
type rpcClient struct {
	url     string
	user    string
	pass    string
	http    *http.Client
}

func newRPCClient(cfg config) *rpcClient {
	return &rpcClient{
		url:  cfg.RPCURL,
		user: cfg.RPCUser,
		pass: cfg.RPCPass,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *rpcClient) call(method string, params interface{}, result interface{}) error {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequest("POST", c.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.user != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("rpc call: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if result != nil {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}
	return nil
}

func main() {
	cfg := config{}
	flag.BoolVar(&cfg.TestNet, "testnet", false, "Use the test network")
	flag.StringVar(&cfg.RPCURL, "rpcconnect", "http://127.0.0.1:19445", "RPC server URL")
	flag.StringVar(&cfg.RPCUser, "rpcuser", "", "RPC username")
	flag.StringVar(&cfg.RPCPass, "rpcpass", "", "RPC password")

	showVersion := flag.Bool("version", false, "Display version information")
	showHelp := flag.Bool("help", false, "Display help information")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <command> [command-args...]\n\n", appName)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  getinfo                            Show network and node status\n")
		fmt.Fprintf(os.Stderr, "  getblock <height>                  Get block by height\n")
		fmt.Fprintf(os.Stderr, "  getblockhash <height>              Get block hash by height\n")
		fmt.Fprintf(os.Stderr, "  getblockbyhash <hash>              Get block by hash\n")
		fmt.Fprintf(os.Stderr, "  gettx <txid>                       Get transaction by ID\n")
		fmt.Fprintf(os.Stderr, "  getbalance <address>               Get balance for address\n")
		fmt.Fprintf(os.Stderr, "  getbestblockhash                   Get best block hash\n")
		fmt.Fprintf(os.Stderr, "  getblockcount                      Get current block height\n")
		fmt.Fprintf(os.Stderr, "  getpeerinfo                        Show connected peer information\n")
		fmt.Fprintf(os.Stderr, "  getblocksubsidy <height>           Get block subsidy at height\n")
		fmt.Fprintf(os.Stderr, "  getgenesisinfo                     Show genesis block information\n")
		fmt.Fprintf(os.Stderr, "  sendrawtransaction <hex>            Broadcast a raw transaction\n")
		fmt.Fprintf(os.Stderr, "  getnetworkinfo                     Show network parameters\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s --testnet getinfo\n", appName)
		fmt.Fprintf(os.Stderr, "  %s getblock 1000\n", appName)
		fmt.Fprintf(os.Stderr, "  %s getbalance NGxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n", appName)
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("%s v%s (Go %s)\n", appName, appVersion, runtime.Version())
		os.Exit(0)
	}
	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	if cfg.TestNet {
		cfg.RPCURL = strings.Replace(cfg.RPCURL, "19445", "19556", 1)
	}

	client := newRPCClient(cfg)
	cmd := args[0]

	switch cmd {
	case "getinfo":
		cmdGetInfo(client)
	case "getblock":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: getblock <height>\n")
			os.Exit(1)
		}
		cmdGetBlock(client, args[1])
	case "getblockhash":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: getblockhash <height>\n")
			os.Exit(1)
		}
		cmdGetBlockHash(client, args[1])
	case "getblockbyhash":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: getblockbyhash <hash>\n")
			os.Exit(1)
		}
		cmdGetBlockByHash(client, args[1])
	case "gettx":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: gettx <txid>\n")
			os.Exit(1)
		}
		cmdGetTx(client, args[1])
	case "getbalance":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: getbalance <address>\n")
			os.Exit(1)
		}
		cmdGetBalance(client, args[1])
	case "getbestblockhash":
		cmdGetBestBlockHash(client)
	case "getblockcount":
		cmdGetBlockCount(client)
	case "getpeerinfo":
		cmdGetPeerInfo(client)
	case "getblocksubsidy":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: getblocksubsidy <height>\n")
			os.Exit(1)
		}
		cmdGetBlockSubsidy(client, args[1])
	case "getgenesisinfo":
		cmdGetGenesisInfo(client)
	case "getnetworkinfo":
		cmdGetNetworkInfo(client)
	case "sendrawtransaction":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: sendrawtransaction <hex>\n")
			os.Exit(1)
		}
		cmdSendRawTransaction(client, args[1])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		flag.Usage()
		os.Exit(1)
	}
}

// ---- Command implementations ----

func cmdGetInfo(c *rpcClient) {
	var result map[string]interface{}
	if err := c.call("getinfo", nil, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdGetBlock(c *rpcClient, heightStr string) {
	var height int64
	if _, err := fmt.Sscanf(heightStr, "%d", &height); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid height: %s\n", heightStr)
		os.Exit(1)
	}

	// First get the block hash at this height.
	var hash string
	if err := c.call("getblockhash", []interface{}{height}, &hash); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Then get the block by hash.
	var result map[string]interface{}
	verbosity := 2
	if err := c.call("getblock", []interface{}{hash, verbosity}, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdGetBlockHash(c *rpcClient, heightStr string) {
	var height int64
	if _, err := fmt.Sscanf(heightStr, "%d", &height); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid height: %s\n", heightStr)
		os.Exit(1)
	}
	var hash string
	if err := c.call("getblockhash", []interface{}{height}, &hash); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}

func cmdGetBlockByHash(c *rpcClient, hashStr string) {
	var result map[string]interface{}
	verbosity := 2
	if err := c.call("getblock", []interface{}{hashStr, verbosity}, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdGetTx(c *rpcClient, txid string) {
	var result map[string]interface{}
	verbosity := 1
	if err := c.call("getrawtransaction", []interface{}{txid, verbosity}, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdGetBalance(c *rpcClient, addr string) {
	var result float64
	if err := c.call("getbalance", []interface{}{addr}, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Balance: %.8f NOGO\n", result)
}

func cmdGetBestBlockHash(c *rpcClient) {
	var hash string
	if err := c.call("getbestblockhash", nil, &hash); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}

func cmdGetBlockCount(c *rpcClient) {
	var count int64
	if err := c.call("getblockcount", nil, &count); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Block count: %d\n", count)
}

func cmdGetPeerInfo(c *rpcClient) {
	var result interface{}
	if err := c.call("getpeerinfo", nil, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdGetBlockSubsidy(c *rpcClient, heightStr string) {
	var height int64
	if _, err := fmt.Sscanf(heightStr, "%d", &height); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid height: %s\n", heightStr)
		os.Exit(1)
	}
	var result map[string]interface{}
	if err := c.call("getblocksubsidy", []interface{}{height}, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdGetGenesisInfo(c *rpcClient) {
	var result map[string]interface{}
	if err := c.call("getgenesisinfo", nil, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdGetNetworkInfo(c *rpcClient) {
	var result map[string]interface{}
	if err := c.call("getnetworkinfo", nil, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdSendRawTransaction(c *rpcClient, hex string) {
	var txid string
	if err := c.call("sendrawtransaction", []interface{}{hex}, &txid); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Transaction broadcast: %s\n", txid)
}

func printJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
	}
}
