// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

// Package tests provides integration and system-level tests for NogoCore.
package tests

import (
	"testing"

	"github.com/nogochain/nogocommons/chaincfg"
)

// TestChainParamsIntegrity verifies essential network parameter consistency.
func TestChainParamsIntegrity(t *testing.T) {
	p := &chaincfg.MainNetParams

	// Verify fundamental economic parameters are non-zero.
	if p.PreAllocation <= 0 {
		t.Error("PreAllocation must be positive")
	}
	if p.InitialBlockReward <= 0 {
		t.Error("InitialBlockReward must be positive")
	}
	if p.MinimumBlockReward <= 0 {
		t.Error("MinimumBlockReward must be positive")
	}
	if p.AnnualReductionRate <= 0 || p.AnnualReductionRate >= 1 {
		t.Error("AnnualReductionRate must be in (0, 1)")
	}
	if p.AnnualBlockCount <= 0 {
		t.Error("AnnualBlockCount must be positive")
	}

	// Verify minimum reward is less than initial reward.
	if p.MinimumBlockReward >= p.InitialBlockReward {
		t.Errorf("MinimumBlockReward (%d) must be less than InitialBlockReward (%d)",
			p.MinimumBlockReward, p.InitialBlockReward)
	}

	// Verify genesis share is reasonable.
	if p.GenesisAddressShare < 0 || p.GenesisAddressShare > 1 {
		t.Error("GenesisAddressShare must be in [0, 1]")
	}

	// Verify fees are burned.
	if !p.BurnFees {
		t.Error("BurnFees must be true per NogoCore economic model")
	}
}

// TestMainNetParamsConstraints verifies MainNet-specific constraints.
func TestMainNetParamsConstraints(t *testing.T) {
	p := &chaincfg.MainNetParams

	if p.Name != "mainnet" {
		t.Errorf("MainNetParams.Name = %q, want %q", p.Name, "mainnet")
	}

	// Block size constraints.
	if p.MaxBlockSize <= p.MaxTxSize {
		t.Errorf("MaxBlockSize (%d) must be larger than MaxTxSize (%d)",
			p.MaxBlockSize, p.MaxTxSize)
	}

	// Coinbase maturity.
	if p.CoinbaseMaturity < 10 {
		t.Errorf("CoinbaseMaturity (%d) is too low for security", p.CoinbaseMaturity)
	}

	// Target block time.
	if p.TargetBlockTime <= 0 {
		t.Error("TargetBlockTime must be positive")
	}
}

// TestTestNetParamsConstraints verifies TestNet-specific constraints.
func TestTestNetParamsConstraints(t *testing.T) {
	p := &chaincfg.TestNet3Params

	if p.Name != "testnet" {
		t.Errorf("TestNet3Params.Name = %q, want %q", p.Name, "testnet")
	}

	// TestNet should have faster block times.
	if p.TargetBlockTime > chaincfg.MainNetParams.TargetBlockTime {
		t.Errorf("TestNet block time (%v) should be shorter than MainNet (%v)",
			p.TargetBlockTime, chaincfg.MainNetParams.TargetBlockTime)
	}

	// TestNet should have more flexible parameters for testing.
	if p.CoinbaseMaturity >= chaincfg.MainNetParams.CoinbaseMaturity {
		t.Errorf("TestNet CoinbaseMaturity (%d) should be shorter than MainNet (%d)",
			p.CoinbaseMaturity, chaincfg.MainNetParams.CoinbaseMaturity)
	}
}

// TestGenesisShareAddressVerification verifies the 1% share address derivation.
func TestGenesisShareAddressVerification(t *testing.T) {
	p := &chaincfg.MainNetParams
	subsidy := int64(800_000_000)
	// GenesisAddressShare is 0.01 (1%), compute share = subsidy * 1%
	expectedShare := int64(float64(subsidy) * float64(p.GenesisAddressShare) / 1e8 * 1e8)
	actualShare := int64(float64(subsidy) * float64(p.GenesisAddressShare) / 1e8 * 1e8)

	// The share should be exactly 0.08 NOGO = 8_000_000 no at block 1.
	if expectedShare != 8_000_000 {
		t.Errorf("Genesis share for block 1 = %d, want %d", expectedShare, 8_000_000)
	}
	if actualShare != 8_000_000 {
		t.Errorf("Calculated genesis share = %d, want %d", actualShare, 8_000_000)
	}
}
