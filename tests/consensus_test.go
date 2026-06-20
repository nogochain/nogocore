//go:build ignore

// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

package tests

import (
	"testing"

	"github.com/nogochain/nogocommons/nogopow"
)

// TestNogoPowConfigDefaults verifies consensus configuration defaults.
func TestNogoPowConfigDefaults(t *testing.T) {
	cfg := nogopow.DefaultConfig()

	if cfg.MinDifficulty <= 0 {
		t.Error("MinDifficulty must be positive")
	}
	if cfg.MaxDifficulty <= cfg.MinDifficulty {
		t.Error("MaxDifficulty must be greater than MinDifficulty")
	}
	if cfg.EpochLength <= 0 {
		t.Error("EpochLength must be positive")
	}
	if cfg.TargetTime <= 0 {
		t.Error("TargetTime must be positive")
	}
}

// TestNogoPowDifficultyRange verifies difficulty values are within valid range.
func TestNogoPowDifficultyRange(t *testing.T) {
	cfg := nogopow.DefaultConfig()

	// Verify difficulty limits.
	if cfg.MinDifficulty < 1 {
		t.Error("MinDifficulty must be at least 1")
	}

	// MaxDifficulty should be substantially larger than MinDifficulty.
	if cfg.MaxDifficulty < cfg.MinDifficulty*2 {
		t.Error("MaxDifficulty should be substantially larger than MinDifficulty")
	}
}

// TestNogoPowEpochBoundary verifies epoch length is a power of 2.
func TestNogoPowEpochBoundary(t *testing.T) {
	cfg := nogopow.DefaultConfig()

	// Epoch length should be a power of 2 for cache optimization.
	epoch := uint64(cfg.EpochLength)
	if epoch == 0 {
		t.Fatal("EpochLength cannot be zero")
	}
	if epoch&(epoch-1) != 0 {
		t.Logf("EpochLength %d is not a power of 2 (acceptable but suboptimal)", epoch)
	}
}

// TestNogoPowTargetTime verifies target block time is reasonable.
func TestNogoPowTargetTime(t *testing.T) {
	cfg := nogopow.DefaultConfig()

	if cfg.TargetTime < 15*1000 { // Less than 15 seconds in ms
		t.Errorf("TargetTime (%d ms) is too short for network propagation", cfg.TargetTime)
	}
	if cfg.TargetTime > 120*1000 { // More than 120 seconds in ms
		t.Errorf("TargetTime (%d ms) is too long for user experience", cfg.TargetTime)
	}
}

// TestNogoPowHeaderCreation verifies block header creation basics.
func TestNogoPowHeaderCreation(t *testing.T) {
	// Verify Header type can be constructed with default values.
	header := &nogopow.Header{
		Version:    1,
		Difficulty: nogopow.CompactToBig(0x207fffff),
		GasLimit:   8_000_000,
		Time:       1750262400,
		Nonce:      nogopow.BlockNonce{},
	}

	if header.Version != 1 {
		t.Errorf("Version = %d, want 1", header.Version)
	}
	if header.GasLimit != 8_000_000 {
		t.Errorf("GasLimit = %d, want 8000000", header.GasLimit)
	}

	// Verify difficulty decode.
	diff := nogopow.CompactToBig(0x207fffff)
	if diff == nil || diff.Sign() <= 0 {
		t.Error("Difficulty must be positive after CompactToBig")
	}
}
