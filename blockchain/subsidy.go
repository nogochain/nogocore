// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC license.

package blockchain

import (
	"math"

	"github.com/nogochain/nogocommons/chaincfg"
)

const (
	// AtomsPerNogo is the number of atomic units (no) in one NOGO.
	AtomsPerNogo = 100_000_000

	// subsidyReductionRate is the annual reduction multiplier (1.0 - 10%).
	subsidyReductionRate = 0.9

	// minimumSubsidyAtoms is the floor block reward in atomic units (0.2 NOGO).
	minimumSubsidyAtoms int64 = 20_000_000

	// genesisShareNumerator/Denominator is the 1% genesis share per block.
	genesisShareNumerator   = 1
	genesisShareDenominator = 100
)

// CalcBlockSubsidy computes the total block subsidy in atomic units (no)
// for the given block height.
func CalcBlockSubsidy(height int32, params *chaincfg.Params) int64 {
	if height <= 0 {
		return 0
	}

	annualBlocks := params.AnnualBlockCount
	if annualBlocks <= 0 {
		annualBlocks = 525_600
	}

	initialReward := params.InitialBlockReward
	if initialReward <= 0 {
		initialReward = 8 * AtomsPerNogo
	}

	years := float64(int64(height-1) / annualBlocks)
	reward := float64(initialReward) * math.Pow(subsidyReductionRate, years)

	minReward := float64(minimumSubsidyAtoms)
	if params.MinimumBlockReward > 0 {
		minReward = float64(params.MinimumBlockReward)
	}
	if reward < minReward {
		reward = minReward
	}

	return int64(reward)
}

// CalcGenesisShare computes the genesis address share (1% of block subsidy).
func CalcGenesisShare(height int32, params *chaincfg.Params) int64 {
	subsidy := CalcBlockSubsidy(height, params)
	share := params.GenesisAddressShare
	if share <= 0 {
		share = genesisShareNumerator
	}
	return subsidy * int64(share) / genesisShareDenominator
}

// CalcMinerSubsidy computes the miner's revenue after genesis share deduction.
// Fees are NOT included — they are burned per NogoCore economic model.
func CalcMinerSubsidy(height int32, params *chaincfg.Params) int64 {
	return CalcBlockSubsidy(height, params) - CalcGenesisShare(height, params)
}
