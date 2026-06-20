// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

// Package genesis provides the genesis block generator for NogoCore.
// The genesis block contains a coinbase transaction with the full
// pre-allocation (1,000,000 NOGO on mainnet by default).
package genesis

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/nogochain/nogocommons/chaincfg"
	"github.com/nogochain/nogocommons/nogopow"
)

const (
	// genesisCoinbaseData is the coinbase script signature for the genesis block.
	genesisCoinbaseData = "NogoCore Genesis Block"

	// defaultGenesisTimestamp is the genesis block timestamp (Unix).
	// 2026-06-19T00:00:00Z
	defaultGenesisTimestamp = 1750262400
)

// GenesisBlockSpec describes the genesis block parameters before mining.
type GenesisBlockSpec struct {
	Timestamp    int64  `json:"timestamp"`
	ParentHash   string `json:"parentHash"`
	Coinbase     string `json:"coinbase"`
	StateRoot    string `json:"stateRoot"`
	MerkleRoot   string `json:"merkleRoot"`
	Difficulty   uint32 `json:"difficulty"`
	GasLimit     uint64 `json:"gasLimit"`
	ExtraData    string `json:"extraData"`
	PreAlloc     int64  `json:"preAllocation"`
	GenesisAddr  string `json:"genesisAddress"`
	ShareAddr    string `json:"shareAddress"`
}

// Generate creates a genesis block specification for the given network parameters.
// Uses nogopow ModeFake for fast deterministic genesis creation.
//
// The genesis block coinbase output contains the full pre-allocation
// to the genesis address.
func Generate(params *chaincfg.Params) (*GenesisBlockSpec, error) {
	if params == nil {
		params = &chaincfg.MainNetParams
	}

	spec := &GenesisBlockSpec{
		Timestamp:   defaultGenesisTimestamp,
		ParentHash:  "0000000000000000000000000000000000000000000000000000000000000000",
		Coinbase:    "0000000000000000000000000000000000000000",
		StateRoot:   "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
		MerkleRoot:  "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
		Difficulty:  params.GenesisDifficultyBits,
		GasLimit:    8_000_000,
		ExtraData:   genesisCoinbaseData,
		PreAlloc:    params.PreAllocation,
		GenesisAddr: params.GenesisAddress,
		ShareAddr:   params.ShareAddress,
	}

	return spec, nil
}

// GenerateToFile creates a genesis block specification and writes it to a JSON file.
func GenerateToFile(params *chaincfg.Params, filePath string) error {
	spec, err := Generate(params)
	if err != nil {
		return fmt.Errorf("failed to generate genesis spec: %w", err)
	}

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal genesis spec: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write genesis file: %w", err)
	}

	return nil
}

// ComputeGenesisHash computes the double-SHA256 hash of the genesis block data.
// This provides a deterministic genesis hash for checkpoint verification.
func ComputeGenesisHash(spec *GenesisBlockSpec) []byte {
	data := serializeGenesisForHash(spec)
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second[:]
}

// serializeGenesisForHash serializes genesis spec fields for hash computation.
func serializeGenesisForHash(spec *GenesisBlockSpec) []byte {
	buf := make([]byte, 0, 256)

	// Timestamp (8 bytes, big-endian).
	ts := make([]byte, 8)
	for i := 0; i < 8; i++ {
		ts[7-i] = byte(spec.Timestamp >> (8 * i))
	}
	buf = append(buf, ts...)

	// PreAlloc (8 bytes, big-endian).
	pa := make([]byte, 8)
	for i := 0; i < 8; i++ {
		pa[7-i] = byte(spec.PreAlloc >> (8 * i))
	}
	buf = append(buf, pa...)

	// Difficulty (4 bytes, big-endian).
	diff := make([]byte, 4)
	for i := 0; i < 4; i++ {
		diff[3-i] = byte(spec.Difficulty >> (8 * i))
	}
	buf = append(buf, diff...)

	// Extra data as raw bytes.
	buf = append(buf, []byte(spec.ExtraData)...)

	return buf
}

// CreateProtoHeader creates a nogopow.Header for the genesis block.
// Uses ModeFake: the PoW is not verified for genesis, only the header structure matters.
func CreateProtoHeader(spec *GenesisBlockSpec) (*nogopow.Header, error) {
	parentHash := nogopow.Hash{}
	coinbase, err := nogopow.StringToAddress(spec.Coinbase)
	if err != nil {
		return nil, fmt.Errorf("invalid genesis coinbase: %w", err)
	}

	difficulty := nogopow.CompactToBig(spec.Difficulty)

	header := &nogopow.Header{
		ParentHash: parentHash,
		Coinbase:   coinbase,
		Root:       nogopow.Hash{},
		TxHash:     nogopow.Hash{},
		Number:     new(big.Int).SetInt64(0),
		GasLimit:   spec.GasLimit,
		Time:       uint64(spec.Timestamp),
		Extra:      []byte(spec.ExtraData),
		Nonce:      nogopow.BlockNonce{},
		Difficulty: difficulty,
	}

	return header, nil
}

// DefaultGenesisSpec returns the hardcoded mainnet genesis specification.
func DefaultGenesisSpec() *GenesisBlockSpec {
	return &GenesisBlockSpec{
		Timestamp:   defaultGenesisTimestamp,
		ParentHash:  "0000000000000000000000000000000000000000000000000000000000000000",
		Coinbase:    "0000000000000000000000000000000000000000",
		StateRoot:   "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
		MerkleRoot:  "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
		Difficulty:  0x207fffff,
		GasLimit:    8_000_000,
		ExtraData:   genesisCoinbaseData,
		PreAlloc:    1_000_000 * 100_000_000,
		GenesisAddr: "",
		ShareAddr:   "",
	}
}

// GetGenesisTimestamp returns the real current time for the first non-genesis block,
// or the default timestamp for the genesis block itself.
func GetGenesisTimestamp() int64 {
	return time.Now().Unix()
}
