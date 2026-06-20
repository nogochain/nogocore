// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

package tests

import (
	"math"
	"testing"

	"github.com/nogochain/nogocommons/chaincfg"
	"github.com/nogochain/nogocore/blockchain"
)

// TestCalcBlockSubsidy verifies the block reward calculation at key epochs.
// Reference: NogoCore Implementation Plan Section 1.3 and 3.2.
func TestCalcBlockSubsidy(t *testing.T) {
	params := &chaincfg.MainNetParams

	tests := []struct {
		name     string
		height   int32
		expected int64 // in atomic units (no)
	}{
		{"genesis block", 0, 0},
		{"first block", 1, 800_000_000},
		{"last block year 1", 525_600, 800_000_000},
		{"first block year 2", 525_601, 720_000_000},
		{"last block year 2", 1_051_200, 720_000_000},
		{"first block year 3", 1_051_201, 648_000_000},
		{"first block year 10", 4_730_401, int64(8.0 * math.Pow(0.9, 9) * 1e8)},
		{"first block year 11", 5_256_001, int64(8.0 * math.Pow(0.9, 10) * 1e8)},
		{"at floor trigger", 18_396_001, 20_000_000},
		{"deep past floor", 100_000_000, 20_000_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := blockchain.CalcBlockSubsidy(tt.height, params)
			if result != tt.expected {
				t.Errorf("CalcBlockSubsidy(%d) = %d, want %d",
					tt.height, result, tt.expected)
			}
		})
	}
}

// TestCalcGenesisShare verifies the 1% genesis address share computation.
func TestCalcGenesisShare(t *testing.T) {
	params := &chaincfg.MainNetParams

	tests := []struct {
		name     string
		height   int32
		expected int64
	}{
		{"year 1 share", 1, 8_000_000},
		{"year 2 share", 525_601, 7_200_000},
		{"year 3 share", 1_051_201, 6_480_000},
		{"floor share", 18_396_001, 200_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := blockchain.CalcGenesisShare(tt.height, params)
			if result != tt.expected {
				t.Errorf("CalcGenesisShare(%d) = %d, want %d",
					tt.height, result, tt.expected)
			}
		})
	}
}

// TestCalcMinerSubsidy verifies miner income = subsidy - genesis share.
func TestCalcMinerSubsidy(t *testing.T) {
	params := &chaincfg.MainNetParams

	subsidy := blockchain.CalcBlockSubsidy(1, params)
	share := blockchain.CalcGenesisShare(1, params)
	miner := blockchain.CalcMinerSubsidy(1, params)

	if miner != subsidy-share {
		t.Errorf("CalcMinerSubsidy(1) = %d, want %d (subsidy=%d - share=%d)",
			miner, subsidy-share, subsidy, share)
	}
	if miner != 792_000_000 {
		t.Errorf("CalcMinerSubsidy(1) = %d, want 792_000_000 (99%% of 8 NOGO)",
			miner)
	}
}

// TestSubsidyDeterminism verifies that CalcBlockSubsidy is deterministic.
func TestSubsidyDeterminism(t *testing.T) {
	params := &chaincfg.MainNetParams
	// Run the same calculation 1000 times and verify consistency.
	var first int64
	for i := 0; i < 1000; i++ {
		result := blockchain.CalcBlockSubsidy(525_601, params)
		if i == 0 {
			first = result
		}
		if result != first {
			t.Fatalf("CalcBlockSubsidy is non-deterministic: run %d gave %d, first=%d",
				i, result, first)
		}
	}
}

// TestSubsidyMonotonicDecreasing verifies reward never increases.
func TestSubsidyMonotonicDecreasing(t *testing.T) {
	params := &chaincfg.MainNetParams
	prev := int64(math.MaxInt64)
	for h := int32(1); h <= 20_000_000; h += 525_600 {
		curr := blockchain.CalcBlockSubsidy(h, params)
		if curr > prev {
			t.Fatalf("Subsidy increased at height %d: prev=%d, curr=%d",
				h, prev, curr)
		}
		prev = curr
	}
}

// TestAnnualSupply verifies approximate annual supply figures.
func TestAnnualSupply(t *testing.T) {
	params := &chaincfg.MainNetParams

	// Year 1 total supply verification (blocks 1 through 525_600).
	var year1Total int64
	for h := int32(1); h <= 525_600; h++ {
		year1Total += blockchain.CalcBlockSubsidy(h, params)
	}

	expectedYear1 := int64(4_204_800 * 1e8)
	tolerance := int64(1e8) // 1 NOGO tolerance

	if abs(year1Total-expectedYear1) > tolerance {
		t.Errorf("Year 1 total supply = %d (%f NOGO), expected ~%d (%f NOGO)",
			year1Total, float64(year1Total)/1e8,
			expectedYear1, float64(expectedYear1)/1e8)
	}
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
