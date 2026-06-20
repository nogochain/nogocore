// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

// NogoCore is a full-node blockchain implementation using NogoPow consensus
// and the UTXO model, built on btcd's battle-tested P2P and infrastructure.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/jessevdk/go-flags"

	"github.com/nogochain/nogocommons/chaincfg"
	"github.com/nogochain/nogocommons/database"
	_ "github.com/nogochain/nogocommons/database/ffldb"
	"github.com/nogochain/nogocore/api"
	"github.com/nogochain/nogocore/blockchain"
	"github.com/nogochain/nogocore/config"
)

const (
	appName           = "nogocore"
	appVersion        = "0.3.0"
	blockDbNamePrefix = "blocks"
	defaultLogDirname = "logs"
)

// nogoCoreConfig aggregates command-line flags and derived configuration.
type nogoCoreConfig struct {
	TestNet    bool   `long:"testnet" description:"Use the test network"`
	MainNet    bool   `long:"mainnet" description:"Use the main network (default)"`
	SimNet     bool   `long:"simnet" description:"Use the simulation test network"`
	DataDir    string `long:"datadir" description:"Directory to store data"`
	ConfigFile string `long:"configfile" description:"Path to configuration file"`

	Listen      []string `long:"listen" description:"Add an interface/port to listen for connections"`
	ExternalIP  string   `long:"externalip" description:"Add an ip to the list of local addresses we claim to listen on"`
	MaxPeers    int      `long:"maxpeers" description:"Max number of inbound and outbound peers"`
	Connect     string   `long:"connect" description:"Connect only to the specified peer(s) at startup"`
	AddPeer     []string `long:"addpeer" description:"Add a peer to connect with at startup"`
	NoOnion     bool     `long:"noonion" description:"Disable connecting to tor hidden services"`

	RPCListeners []string `long:"rpclisten" description:"Add an interface/port to listen for RPC connections"`
	RPCUser      string   `long:"rpcuser" description:"Username for RPC connections"`
	RPCPass      string   `long:"rpcpass" description:"Password for RPC connections"`
	DisableRPC   bool     `long:"norpc" description:"Disable built-in RPC server"`

	DebugLevel string `long:"debuglevel" description:"Logging level: trace, debug, info, warn, error, critical"`
	CPUProfile string `long:"cpuprofile" description:"Write CPU profile to the specified file"`

	ShowVersion bool `short:"V" long:"version" description:"Display version information and exit"`
}

func main() {
	cfg := &nogoCoreConfig{
		MaxPeers: 125,
	}
	parser := flags.NewParser(cfg, flags.HelpFlag|flags.PassDoubleDash)
	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error parsing arguments: %v\n", err)
		os.Exit(1)
	}

	if cfg.ShowVersion {
		fmt.Printf("NogoCore Node v%s\n", appVersion)
		fmt.Printf("Go version: %s\n", runtime.Version())
		os.Exit(0)
	}

	// Resolve data directory.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	if cfg.DataDir == "" {
		defaultDataDir := filepath.Join(homeDir, appName)
		if cfg.TestNet {
			defaultDataDir = filepath.Join(defaultDataDir, "testnet")
		}
		cfg.DataDir = defaultDataDir
	}
	logDir := filepath.Join(cfg.DataDir, defaultLogDirname)

	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create data directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(logDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
		os.Exit(1)
	}

	// Determine active chain parameters.
	var activeParams *chaincfg.Params
	switch {
	case cfg.TestNet:
		activeParams = &chaincfg.TestNet3Params
	case cfg.SimNet:
		fmt.Fprintln(os.Stderr, "SimNet is not yet supported.")
		os.Exit(1)
	default:
		activeParams = &chaincfg.MainNetParams
	}

	printBanner(activeParams, cfg.DataDir)

	// Initialize database.
	dbPath := filepath.Join(cfg.DataDir, blockDbNamePrefix)
	db, err := database.Create("ffldb", dbPath, activeParams.Net)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Printf("Database: %s (%s)\n", db.Type(), dbPath)

	// Initialize blockchain engine.
	chain, err := blockchain.New(&blockchain.Config{
		DB:          db,
		ChainParams: activeParams,
		Checkpoints: activeParams.Checkpoints,
		TimeSource:  blockchain.NewMedianTime(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize blockchain: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		fmt.Println("Flushing blockchain state...")
		chain.FlushUtxoCache(blockchain.FlushRequired)
	}()

	best := chain.BestSnapshot()
	fmt.Printf("Chain height: %d\n", best.Height)
	fmt.Printf("Best block:   %s\n", best.Hash)

	// Initialize node services via config.
	nodeCfg := config.DefaultConfig()
	nodeCfg.ActiveParams = activeParams
	nodeCfg.DataDir = cfg.DataDir
	_ = nodeCfg

	// Start REST API + Block Explorer server.
	restServer := api.NewServer(nodeCfg, chain, nil)
	go func() {
		fmt.Printf("Block Explorer: http://%s\n", restServer.Addr())
		if err := restServer.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "REST server error: %v\n", err)
		}
	}()

	// Setup graceful shutdown.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n=== NogoCore Node Started ===")
	fmt.Printf("Network:     %s\n", activeParams.Name)
	fmt.Printf("P2P Port:    %s\n", activeParams.DefaultPort)
	fmt.Printf("Block Time:  %s\n", activeParams.TargetTimePerBlock)
	fmt.Println()
	fmt.Println("Node is running. Press Ctrl+C to stop.")

	// Wait for shutdown signal.
	<-interrupt
	fmt.Println("\nShutting down NogoCore node...")
	restServer.Shutdown(5 * time.Second)
	fmt.Println("Node shutdown complete.")
}

func printBanner(params *chaincfg.Params, dataDir string) {
	fmt.Println("===========================================")
	fmt.Printf("  NogoCore Node v%s\n", appVersion)
	fmt.Println("  NogoPow Consensus + UTXO Model")
	fmt.Println("  ISC License")
	fmt.Println("===========================================")
	fmt.Printf("Network:  %s\n", params.Name)
	fmt.Printf("Data Dir: %s\n", dataDir)
	fmt.Println("===========================================")
	fmt.Println()
}
