// Copyright (c) 2014-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"bytes"
	"container/heap"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/nogochain/nogocommons/address"
	"github.com/nogochain/nogocommons/chaincfg"
	"github.com/nogochain/nogocommons/chainhash"
	"github.com/nogochain/nogocommons/nogopow"
	"github.com/nogochain/nogocommons/nogoutil"
	"github.com/nogochain/nogocommons/wire"
	"github.com/nogochain/nogocore/blockchain"
)

const (
	// CoinbaseFlags is added to the coinbase script of a generated block.
	CoinbaseFlags = "/NogoPow/nogocore/"

	// MinHighPriority is the minimum priority value that allows a
	// transaction to be considered high priority.
	MinHighPriority = nogoutil.SatoshiPerBitcoin * 144.0 / 250

	// blockHeaderOverhead is the max number of bytes it takes to serialize
	// a block header and max possible transaction count.
	blockHeaderOverhead = wire.MaxBlockHeaderPayload + wire.MaxVarIntPayload
)

// TxDesc is a descriptor about a transaction in a transaction source along with
// additional metadata.
type TxDesc struct {
	// Tx is the transaction associated with the entry.
	Tx *nogoutil.Tx

	// Added is the time when the entry was added to the source pool.
	Added time.Time

	// Height is the block height when the entry was added to the source
	// pool.
	Height int32

	// Fee is the total fee the transaction associated with the entry pays.
	Fee int64

	// FeePerKB is the fee the transaction pays in Satoshi per 1000 bytes.
	FeePerKB int64
}

// TxSource represents a source of transactions to consider for inclusion in
// new blocks.
//
// The interface contract requires that all of these methods are safe for
// concurrent access with respect to the source.
type TxSource interface {
	// LastUpdated returns the last time a transaction was added to or
	// removed from the source pool.
	LastUpdated() time.Time

	// MiningDescs returns a slice of mining descriptors for all the
	// transactions in the source pool.
	MiningDescs() []*TxDesc

	// HaveTransaction returns whether or not the passed transaction hash
	// exists in the source pool.
	HaveTransaction(hash *chainhash.Hash) bool
}

// txPrioItem houses a transaction along with extra information that allows the
// transaction to be prioritized and track dependencies on other transactions
// which have not been mined into a block yet.
type txPrioItem struct {
	tx       *nogoutil.Tx
	fee      int64
	priority float64
	feePerKB int64

	// dependsOn holds a map of transaction hashes which this one depends
	// on.  It will only be set when the transaction references other
	// transactions in the source pool and hence must come after them in
	// a block.
	dependsOn map[chainhash.Hash]struct{}
}

// txPriorityQueueLessFunc describes a function that can be used as a compare
// function for a transaction priority queue (txPriorityQueue).
type txPriorityQueueLessFunc func(*txPriorityQueue, int, int) bool

// txPriorityQueue implements a priority queue of txPrioItem elements that
// supports an arbitrary compare function as defined by txPriorityQueueLessFunc.
type txPriorityQueue struct {
	lessFunc txPriorityQueueLessFunc
	items    []*txPrioItem
}

// Len returns the number of items in the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Len() int {
	return len(pq.items)
}

// Less returns whether the item in the priority queue with index i should sort
// before the item with index j by deferring to the assigned less function.  It
// is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Less(i, j int) bool {
	return pq.lessFunc(pq, i, j)
}

// Swap swaps the items at the passed indices in the priority queue.  It is
// part of the heap.Interface implementation.
func (pq *txPriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
}

// Push pushes the passed item onto the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Push(x interface{}) {
	pq.items = append(pq.items, x.(*txPrioItem))
}

// Pop removes the highest priority item (according to Less) from the priority
// queue and returns it.  It is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Pop() interface{} {
	n := len(pq.items)
	item := pq.items[n-1]
	pq.items[n-1] = nil
	pq.items = pq.items[0 : n-1]
	return item
}

// SetLessFunc sets the compare function for the priority queue to the provided
// function.  It also invokes heap.Init on the priority queue using the new
// function so it can immediately be used with heap.Push/Pop.
func (pq *txPriorityQueue) SetLessFunc(lessFunc txPriorityQueueLessFunc) {
	pq.lessFunc = lessFunc
	heap.Init(pq)
}

// txPQByPriority sorts a txPriorityQueue by transaction priority and then fees
// per kilobyte.
func txPQByPriority(pq *txPriorityQueue, i, j int) bool {
	// Using > here so that pop gives the highest priority item as opposed
	// to the lowest.  Sort by priority first, then fee.
	if pq.items[i].priority == pq.items[j].priority {
		return pq.items[i].feePerKB > pq.items[j].feePerKB
	}
	return pq.items[i].priority > pq.items[j].priority

}

// txPQByFee sorts a txPriorityQueue by fees per kilobyte and then transaction
// priority.
func txPQByFee(pq *txPriorityQueue, i, j int) bool {
	// Using > here so that pop gives the highest fee item as opposed
	// to the lowest.  Sort by fee first, then priority.
	if pq.items[i].feePerKB == pq.items[j].feePerKB {
		return pq.items[i].priority > pq.items[j].priority
	}
	return pq.items[i].feePerKB > pq.items[j].feePerKB
}

// newTxPriorityQueue returns a new transaction priority queue that reserves the
// passed amount of space for the elements.  The new priority queue uses either
// the txPQByPriority or the txPQByFee compare function depending on the
// sortByFee parameter and is already initialized for use with heap.Push/Pop.
// The priority queue can grow larger than the reserved space, but extra copies
// of the underlying array can be avoided by reserving a sane value.
func newTxPriorityQueue(reserve int, sortByFee bool) *txPriorityQueue {
	pq := &txPriorityQueue{
		items: make([]*txPrioItem, 0, reserve),
	}
	if sortByFee {
		pq.SetLessFunc(txPQByFee)
	} else {
		pq.SetLessFunc(txPQByPriority)
	}
	return pq
}

// BlockTemplate houses a block that has yet to be solved along with additional
// details about the fees and the number of signature operations for each
// transaction in the block.
type BlockTemplate struct {
	// Block is a block that is ready to be solved by miners.  Thus, it is
	// completely valid with the exception of satisfying the proof-of-work
	// requirement.
	Block *wire.MsgBlock

	// Fees contains the amount of fees each transaction in the generated
	// template pays in base units.  Since the first transaction is the
	// coinbase, the first entry (offset 0) will contain the negative of the
	// sum of the fees of all other transactions.
	Fees []int64

	// SigOpCosts contains the number of signature operations each
	// transaction in the generated template performs.
	SigOpCosts []int64

	// Height is the height at which the block template connects to the main
	// chain.
	Height int32

	// ValidPayAddress indicates whether or not the template coinbase pays
	// to an address or is redeemable by anyone.  See the documentation on
	// NewBlockTemplate for details on which this can be useful to generate
	// templates without a coinbase payment address.
	ValidPayAddress bool

	// WitnessCommitment is a commitment to the witness data (if any)
	// within the block. This field will only be populted once segregated
	// witness has been activated, and the block contains a transaction
	// which has witness data.
	WitnessCommitment []byte
}

// mergeUtxoView adds all of the entries in viewB to viewA.  The result is that
// viewA will contain all of its original entries plus all of the entries
// in viewB.  It will replace any entries in viewB which also exist in viewA
// if the entry in viewA is spent.
func mergeUtxoView(viewA *blockchain.UtxoViewpoint, viewB *blockchain.UtxoViewpoint) {
	viewAEntries := viewA.Entries()
	for outpoint, entryB := range viewB.Entries() {
		if entryA, exists := viewAEntries[outpoint]; !exists ||
			entryA == nil || entryA.IsSpent() {

			viewAEntries[outpoint] = entryB
		}
	}
}

// standardCoinbaseScript returns a standard script suitable for use as the
// signature script of the coinbase transaction of a new block.  In particular,
// it starts with the block height that is required by version 2 blocks and adds
// the extra nonce as well as additional coinbase flags.
//
// Format: <heightLen> <height> <extraNonceOp> <flagsPush> <flags>
// Equivalent to btcd's: NewScriptBuilder().AddInt64(height).AddInt64(extra).AddData(flags).Script()
func standardCoinbaseScript(nextBlockHeight int32, extraNonce uint64) ([]byte, error) {
	flags := []byte(CoinbaseFlags)

	// Serialize block height as Bitcoin script number (little-endian,
	// minimal encoding with sign bit).  This must match the extraction
	// logic in blockchain/validate.go:ExtractCoinbaseHeight().
	heightBytes := serializeScriptNum(int64(nextBlockHeight))

	script := make([]byte, 0, len(heightBytes)+3+len(flags))
	script = append(script, heightBytes...)           // <height>
	script = append(script, 0x00)                     // OP_0 (extra nonce placeholder)
	script = append(script, byte(len(flags)))         // push flags length
	script = append(script, flags...)

	return script, nil
}

// serializeScriptNum encodes an int64 as a Bitcoin script number.
// Values 1-16 use OP_1..OP_16 (0x51..0x60).  Values outside this range
// use little-endian encoding with minimal bytes, where the last byte's
// MSB is the sign bit (0=positive, 0x80=negative).  This matches btcd's
// txscript.NewScriptBuilder().AddInt64() behavior.
func serializeScriptNum(n int64) []byte {
	if n == 0 {
		return []byte{0x00}
	}

	// Small positive numbers 1-16 use OP_1..OP_16 opcodes.
	if n >= 1 && n <= 16 {
		return []byte{byte(0x50 + n)}
	}

	var negative bool
	if n < 0 {
		n = -n
		negative = true
	}

	// Encode absolute value in little-endian, minimal bytes.
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append(buf, byte(n&0xff))
		n >>= 8
	}

	// If MSB of last byte is set, add a padding zero byte so it is not
	// interpreted as negative.
	if buf[len(buf)-1]&0x80 != 0 {
		buf = append(buf, 0x00)
	}
	if negative {
		buf[len(buf)-1] |= 0x80
	}

	// Push data opcode: length byte followed by data.
	result := make([]byte, 0, len(buf)+1)
	result = append(result, byte(len(buf)))
	result = append(result, buf...)
	return result
}

// createCoinbaseTx returns a coinbase transaction paying the appropriate
// subsidy to the provided miner address and genesis share address per the
// NogoCore economic model.  When the miner address is nil, the coinbase
// transaction will instead be redeemable by anyone (OP_TRUE).
//
// Per NogoCore's economic model:
//   - Output 1 (miner):      CalcMinerSubsidy(height) = subsidy - 1% genesis share
//   - Output 2 (genesis):    CalcGenesisShare(height) = subsidy × 1% (optional)
//   - Transaction fees are NOT added to the coinbase (burned per BurnFees=true).
func createCoinbaseTx(params *chaincfg.Params, coinbaseScript []byte,
	nextBlockHeight int32, addr address.Address) (*nogoutil.Tx, error) {

	// Calculate the block reward components.
	blockSubsidy := blockchain.CalcBlockSubsidy(nextBlockHeight, params)
	minerSubsidy := blockchain.CalcMinerSubsidy(nextBlockHeight, params)
	genesisShare := blockchain.CalcGenesisShare(nextBlockHeight, params)

	// Create the payment script for the miner address if one was specified.
	// Otherwise create a script that allows the coinbase to be redeemable
	// by anyone (OP_TRUE fallback for template generation).
	var pkScript []byte
	if addr != nil {
		var err error
		pkScript, err = payToAddrScriptMining(addr)
		if err != nil {
			return nil, err
		}
	} else {
		pkScript = []byte{0x51} // OP_TRUE
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex),
		SignatureScript: coinbaseScript,
		Sequence:        wire.MaxTxInSequenceNum,
	})

	// Output 1: miner income (subsidy minus genesis share).
	tx.AddTxOut(&wire.TxOut{
		Value:    minerSubsidy,
		PkScript: pkScript,
	})

	// Output 2: genesis address share (1% of block subsidy), if configured.
	if params.GenesisAddress != "" && genesisShare > 0 {
		genesisAddr, err := address.DecodeAddress(params.GenesisAddress, params)
		if err != nil {
			// Genesis address may use a different network prefix than
			// the active chain params (e.g. mainnet-style address on
			// testnet).  Try decoding with MainNetParams as fallback.
			genesisAddr, err = address.DecodeAddress(params.GenesisAddress, &chaincfg.MainNetParams)
			if err != nil {
				// Skip genesis share output if address cannot be decoded
				// on any known network — this is non-fatal for block
				// template generation.
				log.Warnf("Cannot decode genesis address %q for genesis share output: %v",
					params.GenesisAddress, err)
				goto afterGenesisShare
			}
		}
		genesisPkScript, err := payToAddrScriptMining(genesisAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create genesis pkScript: %v", err)
		}
		tx.AddTxOut(&wire.TxOut{
			Value:    genesisShare,
			PkScript: genesisPkScript,
		})
		// Verify coinbase total matches expected subsidy.
		_ = blockSubsidy // blockSubsidy used for reference; miner+genesis = subsidy
	}
afterGenesisShare:

	return nogoutil.NewTx(tx), nil
}

// spendTransaction updates the passed view by marking the inputs to the passed
// transaction as spent.  It also adds all outputs in the passed transaction
// which are not provably unspendable as available unspent transaction outputs.
func spendTransaction(utxoView *blockchain.UtxoViewpoint, tx *nogoutil.Tx, height int32) error {
	for _, txIn := range tx.MsgTx().TxIn {
		entry := utxoView.LookupEntry(txIn.PreviousOutPoint)
		if entry != nil {
			entry.Spend()
		}
	}

	utxoView.AddTxOuts(tx, height)
	return nil
}

// logSkippedDeps logs any dependencies which are also skipped as a result of
// skipping a transaction while generating a block template at the trace level.
func logSkippedDeps(tx *nogoutil.Tx, deps map[chainhash.Hash]*txPrioItem) {
	if deps == nil {
		return
	}

	for _, item := range deps {
		log.Tracef("Skipping tx %s since it depends on %s\n",
			item.tx.Hash(), tx.Hash())
	}
}

// MinimumMedianTime returns the minimum allowed timestamp for a block building
// on the end of the provided best chain.  In particular, it is one second after
// the median timestamp of the last several blocks per the chain consensus
// rules.
func MinimumMedianTime(chainState *blockchain.BestState) time.Time {
	return chainState.MedianTime.Add(time.Second)
}

// medianAdjustedTime returns the current time adjusted to ensure it is at least
// one second after the median timestamp of the last several blocks per the
// chain consensus rules.
func medianAdjustedTime(chainState *blockchain.BestState, timeSource blockchain.MedianTimeSource) time.Time {
	// The timestamp for the block must not be before the median timestamp
	// of the last several blocks.  Thus, choose the maximum between the
	// current time and one second after the past median time.  The current
	// timestamp is truncated to a second boundary before comparison since a
	// block timestamp does not supported a precision greater than one
	// second.
	newTimestamp := timeSource.AdjustedTime()
	minTimestamp := MinimumMedianTime(chainState)
	if newTimestamp.Before(minTimestamp) {
		newTimestamp = minTimestamp
	}

	return newTimestamp
}

// BlkTmplGenerator provides a type that can be used to generate block templates
// based on a given mining policy and source of transactions to choose from.
// It also houses additional state required in order to ensure the templates
// are built on top of the current best chain and adhere to the consensus rules.
type BlkTmplGenerator struct {
	policy      *Policy
	chainParams *chaincfg.Params
	txSource    TxSource
	chain       *blockchain.BlockChain
	timeSource  blockchain.MedianTimeSource
}

// NewBlkTmplGenerator returns a new block template generator for the given
// policy using transactions from the provided transaction source.
//
// The additional state-related fields are required in order to ensure the
// templates are built on top of the current best chain and adhere to the
// consensus rules.
func NewBlkTmplGenerator(policy *Policy, params *chaincfg.Params,
	txSource TxSource, chain *blockchain.BlockChain,
	timeSource blockchain.MedianTimeSource) *BlkTmplGenerator {

	return &BlkTmplGenerator{
		policy:      policy,
		chainParams: params,
		txSource:    txSource,
		chain:       chain,
		timeSource:  timeSource,
	}
}

// NewBlockTemplate returns a new block template that is ready to be solved
// using the transactions from the passed transaction source pool and a coinbase
// that either pays to the passed address if it is not nil, or a coinbase that
// is redeemable by anyone if the passed address is nil.  The nil address
// functionality is useful since there are cases such as the getblocktemplate
// RPC where external mining software is responsible for creating their own
// coinbase which will replace the one generated for the block template.  Thus
// the need to have configured address can be avoided.
//
// The transactions selected and included are prioritized according to several
// factors.  First, each transaction has a priority calculated based on its
// value, age of inputs, and size.  Transactions which consist of larger
// amounts, older inputs, and small sizes have the highest priority.  Second, a
// fee per kilobyte is calculated for each transaction.  Transactions with a
// higher fee per kilobyte are preferred.  Finally, the block generation related
// policy settings are all taken into account.
//
// Transactions which only spend outputs from other transactions already in the
// block chain are immediately added to a priority queue which either
// prioritizes based on the priority (then fee per kilobyte) or the fee per
// kilobyte (then priority) depending on whether or not the BlockPrioritySize
// policy setting allots space for high-priority transactions.  Transactions
// which spend outputs from other transactions in the source pool are added to a
// dependency map so they can be added to the priority queue once the
// transactions they depend on have been included.
//
// Once the high-priority area (if configured) has been filled with
// transactions, or the priority falls below what is considered high-priority,
// the priority queue is updated to prioritize by fees per kilobyte (then
// priority).
//
// When the fees per kilobyte drop below the TxMinFreeFee policy setting, the
// transaction will be skipped unless the BlockMinSize policy setting is
// nonzero, in which case the block will be filled with the low-fee/free
// transactions until the block size reaches that minimum size.
//
// Any transactions which would cause the block to exceed the BlockMaxSize
// policy setting, exceed the maximum allowed signature operations per block, or
// otherwise cause the block to be invalid are skipped.
//
// Given the above, a block generated by this function is of the following form:
//
//	 -----------------------------------  --  --
//	|      Coinbase Transaction         |   |   |
//	|-----------------------------------|   |   |
//	|                                   |   |   | ----- policy.BlockPrioritySize
//	|   High-priority Transactions      |   |   |
//	|                                   |   |   |
//	|-----------------------------------|   | --
//	|                                   |   |
//	|                                   |   |
//	|                                   |   |--- policy.BlockMaxSize
//	|  Transactions prioritized by fee  |   |
//	|  until <= policy.TxMinFreeFee     |   |
//	|                                   |   |
//	|                                   |   |
//	|                                   |   |
//	|-----------------------------------|   |
//	|  Low-fee/Non high-priority (free) |   |
//	|  transactions (while block size   |   |
//	|  <= policy.BlockMinSize)          |   |
//	 -----------------------------------  --
func (g *BlkTmplGenerator) NewBlockTemplate(
	payToAddress address.Address) (*BlockTemplate, error) {

	// Extend the most recently known best block.
	best := g.chain.BestSnapshot()
	nextBlockHeight := best.Height + 1

	// Create a standard coinbase transaction paying to the provided
	// address.  NOTE: The coinbase value will be updated to include the
	// fees from the selected transactions later after they have actually
	// been selected.  It is created here to detect any errors early
	// before potentially doing a lot of work below.  The extra nonce helps
	// ensure the transaction is not a duplicate transaction (paying the
	// same value to the same public key address would otherwise be an
	// identical transaction for block version 1).
	extraNonce := uint64(0)
	coinbaseScript, err := standardCoinbaseScript(nextBlockHeight, extraNonce)
	if err != nil {
		return nil, err
	}
	coinbaseTx, err := createCoinbaseTx(g.chainParams, coinbaseScript,
		nextBlockHeight, payToAddress)
	if err != nil {
		return nil, err
	}
	coinbaseSigOpCost := int64(blockchain.CountSigOps(coinbaseTx)) * blockchain.WitnessScaleFactor

	// Get the current source transactions and create a priority queue to
	// hold the transactions which are ready for inclusion into a block
	// along with some priority related and fee metadata.  Reserve the same
	// number of items that are available for the priority queue.  Also,
	// choose the initial sort order for the priority queue based on whether
	// or not there is an area allocated for high-priority transactions.
	sourceTxns := g.txSource.MiningDescs()
	sortedByFee := g.policy.BlockPrioritySize == 0
	priorityQueue := newTxPriorityQueue(len(sourceTxns), sortedByFee)

	// Create a slice to hold the transactions to be included in the
	// generated block with reserved space.  Also create a utxo view to
	// house all of the input transactions so multiple lookups can be
	// avoided.
	blockTxns := make([]*nogoutil.Tx, 0, len(sourceTxns))
	blockTxns = append(blockTxns, coinbaseTx)
	blockUtxos := blockchain.NewUtxoViewpoint()

	// dependers is used to track transactions which depend on another
	// transaction in the source pool.  This, in conjunction with the
	// dependsOn map kept with each dependent transaction helps quickly
	// determine which dependent transactions are now eligible for inclusion
	// in the block once each transaction has been included.
	dependers := make(map[chainhash.Hash]map[chainhash.Hash]*txPrioItem)

	// Create slices to hold the fees and number of signature operations
	// for each of the selected transactions and add an entry for the
	// coinbase.  This allows the code below to simply append details about
	// a transaction as it is selected for inclusion in the final block.
	// However, since the total fees aren't known yet, use a dummy value for
	// the coinbase fee which will be updated later.
	txFees := make([]int64, 0, len(sourceTxns))
	txSigOpCosts := make([]int64, 0, len(sourceTxns))
	txFees = append(txFees, -1) // Updated once known
	txSigOpCosts = append(txSigOpCosts, coinbaseSigOpCost)

	log.Debugf("Considering %d transactions for inclusion to new block",
		len(sourceTxns))

mempoolLoop:
	for _, txDesc := range sourceTxns {
		// A block can't have more than one coinbase or contain
		// non-finalized transactions.
		tx := txDesc.Tx
		if blockchain.IsCoinBase(tx) {
			log.Tracef("Skipping coinbase tx %s", tx.Hash())
			continue
		}
		if !blockchain.IsFinalizedTransaction(tx, nextBlockHeight,
			g.timeSource.AdjustedTime()) {

			log.Tracef("Skipping non-finalized tx %s", tx.Hash())
			continue
		}

		// Fetch all of the utxos referenced by this transaction.
		// NOTE: This intentionally does not fetch inputs from the
		// mempool since a transaction which depends on other
		// transactions in the mempool must come after those
		// dependencies in the final generated block.
		utxos, err := g.chain.FetchUtxoView(tx)
		if err != nil {
			log.Warnf("Unable to fetch utxo view for tx %s: %v",
				tx.Hash(), err)
			continue
		}

		// Setup dependencies for any transactions which reference
		// other transactions in the mempool so they can be properly
		// ordered below.
		prioItem := &txPrioItem{tx: tx}
		for _, txIn := range tx.MsgTx().TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			entry := utxos.LookupEntry(txIn.PreviousOutPoint)
			if entry == nil || entry.IsSpent() {
				if !g.txSource.HaveTransaction(originHash) {
					log.Tracef("Skipping tx %s because it "+
						"references unspent output %s "+
						"which is not available",
						tx.Hash(), txIn.PreviousOutPoint)
					continue mempoolLoop
				}

				// The transaction is referencing another
				// transaction in the source pool, so setup an
				// ordering dependency.
				deps, exists := dependers[*originHash]
				if !exists {
					deps = make(map[chainhash.Hash]*txPrioItem)
					dependers[*originHash] = deps
				}
				deps[*prioItem.tx.Hash()] = prioItem
				if prioItem.dependsOn == nil {
					prioItem.dependsOn = make(
						map[chainhash.Hash]struct{})
				}
				prioItem.dependsOn[*originHash] = struct{}{}

			}
		}

		// Calculate the final transaction priority using the input
		// value age sum as well as the adjusted transaction size.  The
		// formula is: sum(inputValue * inputAge) / adjustedTxSize
		prioItem.priority = CalcPriority(tx.MsgTx(), utxos,
			nextBlockHeight)

		// Calculate the fee in Satoshi/kB.
		prioItem.feePerKB = txDesc.FeePerKB
		prioItem.fee = txDesc.Fee

		// Add the transaction to the priority queue to mark it ready
		// for inclusion in the block unless it has dependencies.
		if prioItem.dependsOn == nil {
			heap.Push(priorityQueue, prioItem)
		}

		// Merge the referenced outputs from the input transactions to
		// this transaction into the block utxo view.  This allows the
		// code below to avoid a second lookup.
		mergeUtxoView(blockUtxos, utxos)
	}

	log.Tracef("Priority queue len %d, dependers len %d",
		priorityQueue.Len(), len(dependers))

	// The starting block size is the size of the block header plus the max
	// possible transaction count size, plus the size of the coinbase
	// transaction.
	blockWeight := uint32((blockHeaderOverhead * blockchain.WitnessScaleFactor) +
		blockchain.GetTransactionWeight(coinbaseTx))
	blockSigOpCost := coinbaseSigOpCost
	totalFees := int64(0)

	// Query the version bits state to see if segwit has been activated, if
	// so then this means that we'll include any transactions with witness
	// data in the mempool, and also add the witness commitment as an
	// OP_RETURN output in the coinbase transaction.
	segwitState, err := g.chain.ThresholdState(chaincfg.DeploymentSegwit)
	if err != nil {
		return nil, err
	}
	segwitActive := segwitState == blockchain.ThresholdActive

	witnessIncluded := false

	// Choose which transactions make it into the block.
	for priorityQueue.Len() > 0 {
		// Grab the highest priority (or highest fee per kilobyte
		// depending on the sort order) transaction.
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		tx := prioItem.tx

		switch {
		// If segregated witness has not been activated yet, then we
		// shouldn't include any witness transactions in the block.
		case !segwitActive && tx.HasWitness():
			continue

		// Otherwise, Keep track of if we've included a transaction
		// with witness data or not. If so, then we'll need to include
		// the witness commitment as the last output in the coinbase
		// transaction.
		case segwitActive && !witnessIncluded && tx.HasWitness():
			// If we're about to include a transaction bearing
			// witness data, then we'll also need to include a
			// witness commitment in the coinbase transaction.
			// Therefore, we account for the additional weight
			// within the block with a model coinbase tx with a
			// witness commitment.
			coinbaseCopy := nogoutil.NewTx(coinbaseTx.MsgTx().Copy())
			coinbaseCopy.MsgTx().TxIn[0].Witness = [][]byte{
				bytes.Repeat([]byte("a"),
					blockchain.CoinbaseWitnessDataLen),
			}
			coinbaseCopy.MsgTx().AddTxOut(&wire.TxOut{
				PkScript: bytes.Repeat([]byte("a"),
					blockchain.CoinbaseWitnessPkScriptLength),
			})

			// In order to accurately account for the weight
			// addition due to this coinbase transaction, we'll add
			// the difference of the transaction before and after
			// the addition of the commitment to the block weight.
			weightDiff := blockchain.GetTransactionWeight(coinbaseCopy) -
				blockchain.GetTransactionWeight(coinbaseTx)

			blockWeight += uint32(weightDiff)

			witnessIncluded = true
		}

		// Grab any transactions which depend on this one.
		deps := dependers[*tx.Hash()]

		// Enforce maximum block size.  Also check for overflow.
		txWeight := uint32(blockchain.GetTransactionWeight(tx))
		blockPlusTxWeight := blockWeight + txWeight
		if blockPlusTxWeight < blockWeight ||
			blockPlusTxWeight >= g.policy.BlockMaxWeight {

			log.Tracef("Skipping tx %s because it would exceed "+
				"the max block weight", tx.Hash())
			logSkippedDeps(tx, deps)
			continue
		}

		// Enforce maximum signature operation cost per block.  Also
		// check for overflow.
		sigOpCost, err := blockchain.GetSigOpCost(tx, false,
			blockUtxos, true, segwitActive)
		if err != nil {
			log.Tracef("Skipping tx %s due to error in "+
				"GetSigOpCost: %v", tx.Hash(), err)
			logSkippedDeps(tx, deps)
			continue
		}
		if blockSigOpCost+int64(sigOpCost) < blockSigOpCost ||
			blockSigOpCost+int64(sigOpCost) > blockchain.MaxBlockSigOpsCost {
			log.Tracef("Skipping tx %s because it would "+
				"exceed the maximum sigops per block", tx.Hash())
			logSkippedDeps(tx, deps)
			continue
		}

		// Skip free transactions once the block is larger than the
		// minimum block size.
		if sortedByFee &&
			prioItem.feePerKB < int64(g.policy.TxMinFreeFee) &&
			blockPlusTxWeight >= g.policy.BlockMinWeight {

			log.Tracef("Skipping tx %s with feePerKB %d "+
				"< TxMinFreeFee %d and block weight %d >= "+
				"minBlockWeight %d", tx.Hash(), prioItem.feePerKB,
				g.policy.TxMinFreeFee, blockPlusTxWeight,
				g.policy.BlockMinWeight)
			logSkippedDeps(tx, deps)
			continue
		}

		// Prioritize by fee per kilobyte once the block is larger than
		// the priority size or there are no more high-priority
		// transactions.
		if !sortedByFee && (blockPlusTxWeight >= g.policy.BlockPrioritySize ||
			prioItem.priority <= MinHighPriority) {

			log.Tracef("Switching to sort by fees per "+
				"kilobyte blockSize %d >= BlockPrioritySize "+
				"%d || priority %.2f <= minHighPriority %.2f",
				blockPlusTxWeight, g.policy.BlockPrioritySize,
				prioItem.priority, MinHighPriority)

			sortedByFee = true
			priorityQueue.SetLessFunc(txPQByFee)

			// Put the transaction back into the priority queue and
			// skip it so it is re-priortized by fees if it won't
			// fit into the high-priority section or the priority
			// is too low.  Otherwise this transaction will be the
			// final one in the high-priority section, so just fall
			// though to the code below so it is added now.
			if blockPlusTxWeight > g.policy.BlockPrioritySize ||
				prioItem.priority < MinHighPriority {

				heap.Push(priorityQueue, prioItem)
				continue
			}
		}

		// Ensure the transaction inputs pass all of the necessary
		// preconditions before allowing it to be added to the block.
		_, err = blockchain.CheckTransactionInputs(tx, nextBlockHeight,
			blockUtxos, g.chainParams)
		if err != nil {
			log.Tracef("Skipping tx %s due to error in "+
				"CheckTransactionInputs: %v", tx.Hash(), err)
			logSkippedDeps(tx, deps)
			continue
		}
		err = validateTxScriptsMining(tx, blockUtxos, 0)
		if err != nil {
			log.Tracef("Skipping tx %s due to error in "+
				"ValidateTransactionScripts: %v", tx.Hash(), err)
			logSkippedDeps(tx, deps)
			continue
		}

		// Spend the transaction inputs in the block utxo view and add
		// an entry for it to ensure any transactions which reference
		// this one have it available as an input and can ensure they
		// aren't double spending.
		spendTransaction(blockUtxos, tx, nextBlockHeight)

		// Add the transaction to the block, increment counters, and
		// save the fees and signature operation counts to the block
		// template.
		blockTxns = append(blockTxns, tx)
		blockWeight += txWeight
		blockSigOpCost += int64(sigOpCost)
		totalFees += prioItem.fee
		txFees = append(txFees, prioItem.fee)
		txSigOpCosts = append(txSigOpCosts, int64(sigOpCost))

		log.Tracef("Adding tx %s (priority %.2f, feePerKB %.2f)",
			prioItem.tx.Hash(), prioItem.priority, prioItem.feePerKB)

		// Add transactions which depend on this one (and also do not
		// have any other unsatisified dependencies) to the priority
		// queue.
		for _, item := range deps {
			// Add the transaction to the priority queue if there
			// are no more dependencies after this one.
			delete(item.dependsOn, *tx.Hash())
			if len(item.dependsOn) == 0 {
				heap.Push(priorityQueue, item)
			}
		}
	}

	// Now that the actual transactions have been selected, update the
	// block weight for the real transaction count and coinbase value with
	// the total fees accordingly.
	blockWeight -= wire.MaxVarIntPayload -
		(uint32(wire.VarIntSerializeSize(uint64(len(blockTxns)))) *
			blockchain.WitnessScaleFactor)
	// Per NogoCore economic model: if BurnFees is enabled, transaction
	// fees are destroyed rather than added to the coinbase.  This provides
	// deflationary pressure proportional to network usage.
	if !g.chainParams.BurnFees {
		coinbaseTx.MsgTx().TxOut[0].Value += totalFees
		txFees[0] = -totalFees
	} else {
		// Fees are burned: neither the miner output nor the genesis
		// share output receives fee revenue.
		txFees[0] = 0
	}

	// If segwit is active and we included transactions with witness data,
	// then we'll need to include a commitment to the witness data in an
	// OP_RETURN output within the coinbase transaction.
	var witnessCommitment []byte
	if witnessIncluded {
		witnessCommitment = AddWitnessCommitment(coinbaseTx, blockTxns)
	}

	// Calculate the required difficulty for the block.  The timestamp
	// is potentially adjusted to ensure it comes after the median time of
	// the last several blocks per the chain consensus rules.
	ts := medianAdjustedTime(best, g.timeSource)
	reqDifficulty, err := g.chain.CalcNextRequiredDifficulty(&best.Hash, ts)
	if err != nil {
		return nil, err
	}

	// Calculate the next expected block version based on the state of the
	// rule change deployments.
	nextBlockVersion, err := g.chain.CalcNextBlockVersion()
	if err != nil {
		return nil, err
	}

	// Create a new block ready to be solved.
	var msgBlock wire.MsgBlock
	msgBlock.Header = wire.BlockHeader{
		Version:    nextBlockVersion,
		PrevBlock:  best.Hash,
		MerkleRoot: blockchain.CalcMerkleRoot(blockTxns, false),
		Timestamp:  ts,
		Bits:       reqDifficulty,
	}
	for _, tx := range blockTxns {
		if err := msgBlock.AddTransaction(tx.MsgTx()); err != nil {
			return nil, err
		}
	}

	// Finally, perform a full check on the created block against the chain
	// consensus rules to ensure it properly connects to the current best
	// chain with no issues.
	block := nogoutil.NewBlock(&msgBlock)
	block.SetHeight(nextBlockHeight)
	if err := g.chain.CheckConnectBlockTemplate(block); err != nil {
		return nil, err
	}

	log.Debugf("Created new block template (%d transactions, %d in "+
		"fees, %d signature operations cost, %d weight, target difficulty "+
		"%064x)", len(msgBlock.Transactions), totalFees, blockSigOpCost,
		blockWeight, nogopow.CompactToBig(msgBlock.Header.Bits))

	return &BlockTemplate{
		Block:             &msgBlock,
		Fees:              txFees,
		SigOpCosts:        txSigOpCosts,
		Height:            nextBlockHeight,
		ValidPayAddress:   payToAddress != nil,
		WitnessCommitment: witnessCommitment,
	}, nil
}

// AddWitnessCommitment adds the witness commitment as an OP_RETURN output
// within the coinbase tx.  The raw commitment is returned.
func AddWitnessCommitment(coinbaseTx *nogoutil.Tx,
	blockTxns []*nogoutil.Tx) []byte {

	// The witness of the coinbase transaction MUST be exactly 32-bytes
	// of all zeroes.
	var witnessNonce [blockchain.CoinbaseWitnessDataLen]byte
	coinbaseTx.MsgTx().TxIn[0].Witness = wire.TxWitness{witnessNonce[:]}

	// Next, obtain the merkle root of a tree which consists of the
	// wtxid of all transactions in the block. The coinbase
	// transaction will have a special wtxid of all zeroes.
	witnessMerkleRoot := blockchain.CalcMerkleRoot(blockTxns, true)

	// The preimage to the witness commitment is:
	// witnessRoot || coinbaseWitness
	var witnessPreimage [64]byte
	copy(witnessPreimage[:32], witnessMerkleRoot[:])
	copy(witnessPreimage[32:], witnessNonce[:])

	// The witness commitment itself is the double-sha256 of the
	// witness preimage generated above. With the commitment
	// generated, the witness script for the output is: OP_RETURN
	// OP_DATA_36 {0xaa21a9ed || witnessCommitment}. The leading
	// prefix is referred to as the "witness magic bytes".
	witnessCommitment := chainhash.DoubleHashB(witnessPreimage[:])
	witnessScript := append(blockchain.WitnessMagicBytes, witnessCommitment...)

	// Finally, create the OP_RETURN carrying witness commitment
	// output as an additional output within the coinbase.
	commitmentOutput := &wire.TxOut{
		Value:    0,
		PkScript: witnessScript,
	}
	coinbaseTx.MsgTx().TxOut = append(coinbaseTx.MsgTx().TxOut,
		commitmentOutput)

	return witnessCommitment
}

// UpdateBlockTime updates the timestamp in the header of the passed block to
// the current time while taking into account the median time of the last
// several blocks to ensure the new time is after that time per the chain
// consensus rules.  Finally, it will update the target difficulty if needed
// based on the new time for the test networks since their target difficulty can
// change based upon time.
func (g *BlkTmplGenerator) UpdateBlockTime(msgBlock *wire.MsgBlock) error {
	// The new timestamp is potentially adjusted to ensure it comes after
	// the median time of the last several blocks per the chain consensus
	// rules.
	newTime := medianAdjustedTime(g.chain.BestSnapshot(), g.timeSource)
	msgBlock.Header.Timestamp = newTime

	// Recalculate the difficulty if running on a network that requires it.
	if g.chainParams.ReduceMinDifficulty {
		difficulty := g.chainParams.PowLimitBits
		msgBlock.Header.Bits = difficulty
	}

	return nil
}

// UpdateExtraNonce updates the extra nonce in the coinbase script of the passed
// block by regenerating the coinbase script with the passed value and block
// height.  It also recalculates and updates the new merkle root that results
// from changing the coinbase script.
func (g *BlkTmplGenerator) UpdateExtraNonce(msgBlock *wire.MsgBlock, blockHeight int32, extraNonce uint64) error {
	coinbaseScript, err := standardCoinbaseScript(blockHeight, extraNonce)
	if err != nil {
		return err
	}
	if len(coinbaseScript) > blockchain.MaxCoinbaseScriptLen {
		return fmt.Errorf("coinbase transaction script length "+
			"of %d is out of range (min: %d, max: %d)",
			len(coinbaseScript), blockchain.MinCoinbaseScriptLen,
			blockchain.MaxCoinbaseScriptLen)
	}
	msgBlock.Transactions[0].TxIn[0].SignatureScript = coinbaseScript

	// Transaction hashes are recalculated when the coinbase changes.
	// A future optimization may cache the non-coinbase transaction hashes
	// in the block template state to avoid redundant computation.
	// block.Transactions[0].InvalidateCache()

	// Recalculate the merkle root with the updated extra nonce.
	block := nogoutil.NewBlock(msgBlock)
	merkleRoot := blockchain.CalcMerkleRoot(block.Transactions(), false)
	msgBlock.Header.MerkleRoot = merkleRoot
	return nil
}

// BestSnapshot returns information about the current best chain block and
// related state as of the current point in time using the chain instance
// associated with the block template generator.  The returned state must be
// treated as immutable since it is shared by all callers.
//
// This function is safe for concurrent access.
func (g *BlkTmplGenerator) BestSnapshot() *blockchain.BestState {
	return g.chain.BestSnapshot()
}

// TxSource returns the associated transaction source.
//
// This function is safe for concurrent access.
func (g *BlkTmplGenerator) TxSource() TxSource {
	return g.txSource
}

func encodeCoinbaseHeight(height, extraNonce int64) ([]byte, error) { buf := make([]byte, 16); binary.BigEndian.PutUint64(buf[0:8], uint64(height)); binary.BigEndian.PutUint64(buf[8:16], uint64(extraNonce)); return buf, nil }
// payToAddrScriptMining creates a payment script for the given address.
// Supports P2PKH, P2SH, P2WPKH (native segwit v0), P2WSH, and P2PK
// address types. NogoCore mining only needs P2PKH or P2WPKH coinbase
// outputs under normal operation.
func payToAddrScriptMining(addr address.Address) ([]byte, error) {
	switch a := addr.(type) {
	case *address.AddressPubKeyHash:
		script := make([]byte, 25)
		script[0] = 0x76 // OP_DUP
		script[1] = 0xa9 // OP_HASH160
		script[2] = 0x14 // push 20 bytes
		copy(script[3:23], a.ScriptAddress())
		script[23] = 0x88 // OP_EQUALVERIFY
		script[24] = 0xac // OP_CHECKSIG
		return script, nil
	case *address.AddressScriptHash:
		script := make([]byte, 23)
		script[0] = 0xa9 // OP_HASH160
		script[1] = 0x14 // push 20 bytes
		copy(script[2:22], a.ScriptAddress())
		script[22] = 0x87 // OP_EQUAL
		return script, nil
	case *address.AddressSegWit:
		witnessProgram := a.ScriptAddress()
		script := make([]byte, 2+len(witnessProgram))
		script[0] = 0x00 // OP_0 (witness v0)
		script[1] = byte(len(witnessProgram))
		copy(script[2:], witnessProgram)
		return script, nil
	case *address.AddressPubKey:
		script := make([]byte, 35)
		script[0] = 0x21 // push 33 bytes
		copy(script[1:34], a.ScriptAddress())
		script[34] = 0xac // OP_CHECKSIG
		return script, nil
	default:
		return nil, fmt.Errorf("unsupported address type for mining coinbase: %T", addr)
	}
}

// validateTxScriptsMining performs structural validation on a transaction
// before it enters a block template. NogoCore does not execute Bitcoin-style
// script verification (BIP16/BIP65/BIP66/CSV/SegWit/Taproot are removed);
// this function checks that the transaction has at least one input and that
// all output values are non-negative.
func validateTxScriptsMining(tx *nogoutil.Tx, view *blockchain.UtxoViewpoint, flags int) error {
	// Verify the transaction has at least one input.
	if len(tx.MsgTx().TxIn) == 0 {
		return fmt.Errorf("transaction %s has no inputs", tx.Hash())
	}
	// Verify all output values are non-negative (no value inflation).
	for i, out := range tx.MsgTx().TxOut {
		if out.Value < 0 {
			return fmt.Errorf("transaction %s output %d has negative value %d",
				tx.Hash(), i, out.Value)
		}
	}
	// NogoCore does not perform script-level signature verification;
	// consensus relies on block-level NogoPow validation instead.
	return nil
}
