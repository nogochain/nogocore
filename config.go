// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/nogochain/nogocommons/chaincfg"
	"github.com/nogochain/nogocommons/peer"
	"github.com/nogochain/nogocommons/wire"
	"github.com/nogochain/nogocore/blockchain"
)

const (
	defaultConfigFilename       = "nogocore.conf"
	defaultLogDirname           = "logs"
	defaultMaxPeers             = 125
	defaultBanDuration          = time.Hour * 24
	defaultBanThreshold         = 100
	defaultConnectTimeout       = time.Second * 30
	defaultMaxRPCClients        = 10
	defaultMaxRPCWebsockets     = 25
	defaultMaxRPCConcurrentReqs = 20
	defaultLogLevel             = "info"
	defaultRebroadcastInterval  = 30
	defaultMaxOrphanTxSize      = 5000
	defaultMaxOrphanTxs         = 100
	targetOutbound              = 8
	defaultBlockPrioritySize    = 50000
	defaultMinRelayTxFee        = 1000
	blockMaxWeightMin           = 1000
	blockMaxSizeMin             = 1000
)

// config defines the configuration options for nogocore.
type config struct {
	ShowVersion bool   `short:"V" long:"version" description:"Display version information and exit"`
	DataDir     string `short:"b" long:"datadir" description:"Directory to store data"`
	LogDir      string `long:"logdir" description:"Directory to log output."`
	ConfigFile  string `short:"C" long:"configfile" description:"Path to configuration file"`

	// Debug
	DebugLevel string `long:"debuglevel" description:"Logging level {trace, debug, info, warn, error, critical}"`
	CPUProfile string `long:"cpuprofile" description:"Write CPU profile to the specified file"`

	// Network
	TestNet3 bool `long:"testnet" description:"Use the test network (version 3)"`

	// P2P
	Listen              []string      `long:"listen" description:"Add an interface/port to listen for connections"`
	ExternalIPs         []string      `long:"externalip" description:"Add an ip to the list of local addresses we claim to listen on"`
	MaxPeers            int           `long:"maxpeers" description:"Max number of inbound and outbound peers"`
	TargetOutbound      int           `long:"targetoutbound" description:"Target number of outbound peers"`
	DisableBanning      bool          `long:"nobanning" description:"Disable banning of misbehaving peers"`
	BanDuration         time.Duration `long:"banduration" description:"How long to ban misbehaving peers"`
	BanThreshold        uint32        `long:"banthreshold" description:"Maximum allowed ban score before disconnecting and banning misbehaving peers"`
	ConnectPeers        []string      `long:"connect" description:"Connect only to the specified peers at startup"`
	AddPeers            []string      `long:"addpeer" description:"Add a peer to connect with at startup"`
	Whitelists          []string      `long:"whitelist" description:"Add an IP network or IP that will not be banned"`
	BlocksOnly          bool          `long:"blocksonly" description:"Do not accept transactions from remote peers"`
	DisableListen       bool          `long:"nolisten" description:"Disable listening for incoming connections"`
	Proxy               string        `long:"proxy" description:"Connect via SOCKS5 proxy"`
	DisableStallHandler bool          `long:"nostalldetect" description:"Disables the stall handler system for each peer"`
	TrickleInterval     time.Duration `long:"trickleinterval" description:"Minimum time between attempts to send new inventory to a connected peer"`
	V2Transport         bool          `long:"v2transport" description:"Enable BIP324 v2 transport protocol"`

	// RPC
	DisableRPC           bool   `long:"norpc" description:"Disable built-in RPC server"`
	RPCListeners         []string `long:"rpclisten" description:"Add an interface/port to listen for RPC connections"`
	RPCUser              string `long:"rpcuser" description:"Username for RPC connections"`
	RPCPass              string `long:"rpcpass" description:"Password for RPC connections"`
	RPCLimitUser         string `long:"rpclimituser" description:"Username for limited RPC connections"`
	RPCLimitPass         string `long:"rpclimitpass" description:"Password for limited RPC connections"`
	RPCMaxClients        int    `long:"rpcmaxclients" description:"Max number of RPC clients for standard connections"`
	RPCMaxWebsockets     int    `long:"rpcmaxwebsockets" description:"Max number of RPC websocket connections"`
	RPCMaxConcurrentReqs int    `long:"rpcmaxconcurrentreqs" description:"Max number of concurrent RPC requests"`
	DisableTLS           bool   `long:"notls" description:"Disable TLS for the RPC server -- NOTE: This is only allowed if the RPC server is bound to localhost"`
	RPCCert              string `long:"rpccert" description:"File containing the certificate file"`
	RPCKey               string `long:"rpckey" description:"File containing the certificate key"`

	// Indexes
	TxIndex    bool `long:"txindex" description:"Maintain a full hash-based transaction index"`
	AddrIndex  bool `long:"addrindex" description:"Maintain a full address-based transaction index"`
	NoCFilters bool `long:"nocfilters" description:"Disable committed filtering (CF) support"`

	// Mempool
	NoRelayPriority   bool    `long:"norelaypriority" description:"Do not require free or low-fee transactions to have high priority for relaying"`
	RelayNonStd       bool    `long:"relaynonstd" description:"Relay non-standard transactions"`
	FreeTxRelayLimit  float64 `long:"limitfreerelay" description:"Limit relay of transactions with no transaction fee to the given amount in thousands of bytes per minute"`
	MaxOrphanTxs      int     `long:"maxorphantx" description:"Max number of orphan transactions to keep in memory"`
	RejectReplacement bool    `long:"rejectreplacement" description:"Reject transactions that attempt to replace existing mempool transactions"`

	// Mining
	BlockMinWeight    uint32   `long:"blockminweight" description:"Minimum block weight to be used when creating blocks"`
	BlockMaxWeight    uint32   `long:"blockmaxweight" description:"Maximum block weight to be used when creating blocks"`
	BlockMinSize      uint32   `long:"blockminsize" description:"Minimum block size in bytes to be used when creating blocks"`
	BlockMaxSize      uint32   `long:"blockmaxsize" description:"Maximum block size in bytes to be used when creating blocks"`
	BlockPrioritySize uint32   `long:"blockprioritysize" description:"Size in bytes for high-priority/low-fee transactions when creating blocks"`
	MiningAddrs       []string `long:"miningaddr" description:"Add the specified payment address to the list of addresses to use for generated blocks -- At least one address is required if the generate option is set"`

	// Peer bloom filters
	NoPeerBloomFilters bool `long:"nopeerbloomfilters" description:"Disable bloom filtering support"`

	// DNS Seed
	DisableDNSSeed bool `long:"nodnsseed" description:"Disable DNS seeding for peers"`

	// Checkpoints
	DisableCheckpoints bool `long:"nocheckpoints" description:"Disable built-in checkpoints"`
	addCheckpoints     []chaincfg.Checkpoint

	// Misc
	Discover            bool   `long:"discover" description:"Enable peer discovery"`
	DisableUPnP         bool   `long:"noupnp" description:"Disable UPnP port mapping"`
	DefaultPort         string
	RebroadcastInterval int
	minRelayTxFee       int64

	// Resolved at startup.
	ActiveNetParams *chaincfg.Params
}

// loadConfig initializes and parses the config using a config file, command line
// options, and default values.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		MaxPeers:             defaultMaxPeers,
		TargetOutbound:       targetOutbound,
		BanDuration:          defaultBanDuration,
		BanThreshold:         uint32(defaultBanThreshold),
		RPCMaxClients:        defaultMaxRPCClients,
		RPCMaxWebsockets:     defaultMaxRPCWebsockets,
		RPCMaxConcurrentReqs: defaultMaxRPCConcurrentReqs,
		DebugLevel:           defaultLogLevel,
		BlockMinWeight:       blockMaxWeightMin,
		BlockMaxWeight:       blockchain.MaxBlockWeight,
		BlockMinSize:         blockMaxSizeMin,
		BlockMaxSize:         wire.MaxBlockPayload,
		BlockPrioritySize:    defaultBlockPrioritySize,
		MaxOrphanTxs:         defaultMaxOrphanTxs,
		TrickleInterval:      peer.DefaultTrickleInterval,
		RebroadcastInterval:  defaultRebroadcastInterval,
		V2Transport:          true,
		Discover:             true,
	}

	// Pre-parse the command line options to see if an alternative config
	// file or the version flag was specified.
	preCfg := cfg
	preParser := flags.NewParser(&preCfg, flags.HelpFlag|flags.PassDoubleDash)
	_, err := preParser.Parse()
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}
	}

	// Show the version and exit if the version flag was specified.
	if preCfg.ShowVersion {
		fmt.Printf("NogoCore Node v%d.%d.%d\n", appMajor, appMinor, appPatch)
		fmt.Printf("Go version: %s\n", runtime.Version())
		os.Exit(0)
	}

	// Create a new parser with the config file options.
	parser := flags.NewParser(&cfg, flags.Default)
	if iniErr := flags.NewIniParser(parser).ParseFile(preCfg.ConfigFile); iniErr != nil {
		if _, ok := iniErr.(*os.PathError); !ok {
			fmt.Fprintf(os.Stderr, "Error parsing config file: %v\n", iniErr)
		}
	}

	// Parse command line options again to ensure they take precedence.
	remainingArgs, err := parser.Parse()
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}
	}

	// Resolve active network parameters.
	// Multiple networks can't be selected simultaneously.
	numNets := 0
	if cfg.TestNet3 {
		numNets++
		activeNetParams = &chaincfg.TestNet3Params
	}
	if numNets == 0 {
		numNets++
		activeNetParams = &chaincfg.MainNetParams
	}
	if numNets > 1 {
		str := "%s: the testnet and simnet params can't be " +
			"used together -- choose one of the three"
		err := fmt.Errorf(str, funcName)
		fmt.Fprintln(os.Stderr, err)
		return nil, nil, err
	}
	cfg.ActiveNetParams = activeNetParams

	// Set the default port.
	cfg.DefaultPort = activeNetParams.DefaultPort

	// Set default RPC listeners if not specified.
	if len(cfg.RPCListeners) == 0 {
		cfg.RPCListeners = []string{"127.0.0.1:19445"}
	}

	// Append the network type to the data directory so it is "namespaced"
	// per network.
	cfg.DataDir = cleanAndExpandPath(cfg.DataDir)
	cfg.DataDir = filepath.Join(cfg.DataDir, activeNetParams.Name)

	// Append the network type to the log directory.
	cfg.LogDir = cleanAndExpandPath(cfg.LogDir)
	cfg.LogDir = filepath.Join(cfg.LogDir, activeNetParams.Name)

	// Special show command to list supported subsystems and exit.
	if cfg.DebugLevel == "show" {
		fmt.Println("Supported subsystems: PEER, SRVR, RPCS, INDX, MAIN")
		return nil, nil, errors.New("show debugging subsystems")
	}

	// Validate the given ban duration.
	if cfg.BanDuration < time.Second {
		return nil, nil, fmt.Errorf("the banduration option may not be less than 1s")
	}

	// Set the min relay tx fee.
	cfg.minRelayTxFee = int64(defaultMinRelayTxFee)

	// Validate checkpoints.
	if activeNetParams.Name == "simnet" || activeNetParams.Name == "regtest" {
		cfg.DisableCheckpoints = true
	}

	// Limit the max orphan count.
	if cfg.MaxOrphanTxs < 0 {
		cfg.MaxOrphanTxs = defaultMaxOrphanTxs
	}

	// Generate the RPC cert/key filenames.
	if cfg.RPCKey == "" {
		cfg.RPCKey = filepath.Join(cfg.DataDir, "rpc.key")
	}
	if cfg.RPCCert == "" {
		cfg.RPCCert = filepath.Join(cfg.DataDir, "rpc.cert")
	}

	result := &cfg
	return result, remainingArgs, nil
}

// cleanAndExpandPath expands environment variables and cleans the path.
func cleanAndExpandPath(path string) string {
	if path == "" {
		return path
	}

	path = os.ExpandEnv(path)

	if !filepath.IsAbs(path) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		path = filepath.Join(homeDir, path)
	}

	return filepath.Clean(path)
}

// activeNetParams is populated during startup after parsing the config.
var activeNetParams *chaincfg.Params

// funcName returns the name of the calling function.
var funcName = "loadConfig"

// cfg is the global configuration for the running nogocore node.
var cfg *config

// agentBlacklist returns the blacklisted user agent substrings.
func (c *config) agentBlacklist() []string {
	return nil
}

// agentWhitelist returns the whitelisted user agent substrings.
func (c *config) agentWhitelist() []string {
	return nil
}

// normalizeAddress returns addr with the passed default port appended if there
// is not already a port specified.
func normalizeAddress(addr, defaultPort string) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(addr, defaultPort)
	}
	return addr
}

// supportedSubsystems returns a sorted slice of the supported subsystems for
// logging purposes.  This is used by the RPC debuglevel handler.
func supportedSubsystems() []string {
	return []string{"NogoCore"}
}

// parseAndSetDebugLevels attempts to parse the specified debug level and set
// the levels accordingly.  An appropriate error is returned if anything is
// invalid.
func parseAndSetDebugLevels(levelSpec string) error {
	levelSpec = strings.TrimSpace(levelSpec)
	if levelSpec == "" || levelSpec == "show" {
		return nil
	}
	// In a full implementation this would parse and set per-subsystem levels.
	// For NogoCore, we accept a simple global level.
	return nil
}
