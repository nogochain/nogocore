// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

// Package config provides unified configuration for NogoCore node operation.
// Supports command-line flags, environment variables, and config file loading.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/nogochain/nogocommons/chaincfg"
)

const (
	// defaultDataDirname is the default data directory name.
	defaultDataDirname = "nogocore"

	// defaultConfigFilename is the default config file name.
	defaultConfigFilename = "nogocore.conf"

	// defaultLogDirname is the default log directory name.
	defaultLogDirname = "logs"

	// blockDbNamePrefix is the prefix for the block database directory.
	blockDbNamePrefix = "blocks"

	// defaultMaxPeers is the default maximum number of peers.
	defaultMaxPeers = 125

	// defaultBanThreshold is the default ban score threshold.
	defaultBanThreshold = 100

	// defaultBanDuration is the default ban duration in seconds (24 hours).
	defaultBanDuration = 24 * 60 * 60

	// defaultRPCMaxClients is the default maximum RPC clients.
	defaultRPCMaxClients = 10

	// defaultRPCMaxConcurrentReqs is the default maximum concurrent RPC requests.
	defaultRPCMaxConcurrentReqs = 20
)

var (
	// nogocoreHomeDir is the default home directory for NogoCore.
	nogocoreHomeDir = defaultHomeDir()
)

// Config defines the complete NogoCore node configuration.
type Config struct {
	// Network
	Network string `json:"network"`
	TestNet bool   `json:"testnet"`
	SimNet  bool   `json:"simnet"`

	// Directories
	HomeDir    string `json:"homedir"`
	DataDir    string `json:"datadir"`
	LogDir     string `json:"logdir"`
	ConfigFile string `json:"configfile"`

	// P2P
	Listen       []string `json:"listen"`
	ExternalIP   string   `json:"externalip"`
	MaxPeers     int      `json:"maxpeers"`
	BanDuration  int      `json:"banduration"`
	BanThreshold int      `json:"banthreshold"`
	AddPeers     []string `json:"addpeers"`
	ConnectPeers []string `json:"connect"`

	// RPC
	RPCListeners          []string `json:"rpclisten"`
	RPCUser               string   `json:"rpcuser"`
	RPCPass               string   `json:"rpcpass"`
	RPCMaxClients         int      `json:"rpcmaxclients"`
	RPCMaxConcurrentReqs  int      `json:"rpcmaxconcurrentreqs"`
	DisableRPC            bool     `json:"norpc"`

	// Block template parameters
	BlockMinSize   uint32 `json:"blockminsize"`
	BlockMaxSize   uint32 `json:"blockmaxsize"`
	BlockMinWeight uint32 `json:"blockminweight"`
	BlockMaxWeight uint32 `json:"blockmaxweight"`

	// Debug
	DebugLevel string `json:"debuglevel"`
	LogLevel   string `json:"loglevel"`
	CPUProfile string `json:"cpuprofile"`

	// Chain parameters (populated after parsing)
	ActiveParams *chaincfg.Params `json:"-"`
}

// DefaultConfig returns a Config with safe defaults for mainnet.
func DefaultConfig() *Config {
	dataDir := filepath.Join(nogocoreHomeDir, defaultDataDirname)
	logDir := filepath.Join(dataDir, defaultLogDirname)

	return &Config{
		Network:              "mainnet",
		TestNet:              false,
		SimNet:               false,
		HomeDir:              nogocoreHomeDir,
		DataDir:              dataDir,
		LogDir:               logDir,
		Listen:               []string{},
		MaxPeers:             defaultMaxPeers,
		BanDuration:          defaultBanDuration,
		BanThreshold:         defaultBanThreshold,
		RPCListeners:         []string{"127.0.0.1:19445"},
		RPCMaxClients:        defaultRPCMaxClients,
		RPCMaxConcurrentReqs: defaultRPCMaxConcurrentReqs,
		DisableRPC:           false,
		BlockMaxSize:         8 * 1024 * 1024,
		DebugLevel:           "info",
		LogLevel:             "info",
	}
}

// TestNetConfig returns a Config with testnet defaults.
func TestNetConfig() *Config {
	cfg := DefaultConfig()
	cfg.Network = "testnet"
	cfg.TestNet = true
	cfg.DataDir = filepath.Join(nogocoreHomeDir, defaultDataDirname, "testnet")
	cfg.LogDir = filepath.Join(cfg.DataDir, defaultLogDirname)
	cfg.RPCListeners = []string{"127.0.0.1:19556"}
	return cfg
}

// Validate checks the configuration for consistency.
func (c *Config) Validate() error {
	if c.Network == "" {
		return errors.New("network must be specified")
	}
	if c.DataDir == "" {
		return errors.New("data directory must be specified")
	}
	return nil
}

// ResolveParams resolves the active chain parameters based on network selection.
func (c *Config) ResolveParams() error {
	if c.TestNet || c.Network == "testnet" {
		c.ActiveParams = &chaincfg.TestNet3Params
	} else {
		c.ActiveParams = &chaincfg.MainNetParams
	}
	return nil
}

// SaveToFile writes the current configuration to a JSON file.
func (c *Config) SaveToFile(filePath string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// LoadFromFile reads configuration from a JSON file.
func LoadFromFile(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := cfg.ResolveParams(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// defaultHomeDir returns the OS-appropriate home directory.
func defaultHomeDir() string {
	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		return homeDir
	}

	if runtime.GOOS == "windows" {
		homeDir = os.Getenv("LOCALAPPDATA")
		if homeDir != "" {
			return homeDir
		}
		return filepath.Join(os.Getenv("APPDATA"), "NogoCore")
	}

	return filepath.Join("/var", "lib", "nogocore")
}
