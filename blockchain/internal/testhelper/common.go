package testhelper

import (
	"encoding/binary"
	"math"
	"math/big"
	"runtime"

	"github.com/nogochain/nogocommons/nogoutil"
	"github.com/nogochain/nogocommons/chainhash"
	"github.com/nogochain/nogocommons/wire"
)

const (
	// OP_TRUE is the opcode for OP_TRUE (0x51).
	OP_TRUE = 0x51
	// OP_RETURN is the opcode for OP_RETURN (0x6a).
	OP_RETURN = 0x6a
)

var (
	// OpTrueScript is simply a public key script that contains the OP_TRUE
	// opcode.  It is defined here to reduce garbage creation.
	OpTrueScript = []byte{OP_TRUE}

	// LowFee is a single satoshi and exists to make the test code more
	// readable.
	LowFee = nogoutil.Amount(1)
)

// CreateCoinbaseTx returns a coinbase transaction paying an appropriate
// subsidy based on the passed block height and the block subsidy.  The
// coinbase signature script conforms to the requirements of version 2 blocks.
func CreateCoinbaseTx(blockHeight int32, blockSubsidy int64) *wire.MsgTx {
	extraNonce := uint64(0)
	coinbaseScript, err := StandardCoinbaseScript(blockHeight, extraNonce)
	if err != nil {
		panic(err)
	}

	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex),
		Sequence:        wire.MaxTxInSequenceNum,
		SignatureScript: coinbaseScript,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    blockSubsidy,
		PkScript: OpTrueScript,
	})
	return tx
}

// StandardCoinbaseScript returns a standard script suitable for use as the
// signature script of the coinbase transaction of a new block.  In particular,
// it starts with the block height that is required by version 2 blocks.
func StandardCoinbaseScript(blockHeight int32, extraNonce uint64) ([]byte, error) {
	return scriptBuilderAddInt64(int64(blockHeight), int64(extraNonce)), nil
}

// OpReturnScript returns a provably-pruneable OP_RETURN script with the
// provided data.
func OpReturnScript(data []byte) ([]byte, error) {
	script := make([]byte, 0, len(data)+2)
	script = append(script, OP_RETURN)
	script = append(script, byte(len(data)))
	script = append(script, data...)
	return script, nil
}

// UniqueOpReturnScript returns a standard provably-pruneable OP_RETURN script
// with a random uint64 encoded as the data.
func UniqueOpReturnScript() ([]byte, error) {
	rand, err := wire.RandomUint64()
	if err != nil {
		return nil, err
	}

	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, rand)
	return OpReturnScript(data)
}

// solveBlockStub attempts to find a nonce that makes the passed block header hash
// less than the target difficulty.
func solveBlockStub(header *wire.BlockHeader, targetDifficulty *big.Int) bool {
	for i := uint64(0); i < math.MaxUint64; i++ {
		header.Nonce = uint32(i)
		hash := header.BlockHash()
		hashNum := HashToBigStub(&hash)
		if hashNum.Cmp(targetDifficulty) <= 0 {
			return true
		}
		if i%1000000 == 0 {
			runtime.Gosched()
		}
	}
	return false
}

// scriptBuilderAddInt64 creates a script pushing two int64 values.
func scriptBuilderAddInt64(a, b int64) []byte {
	script := make([]byte, 0, 18)
	script = append(script, encodeScriptNum(a)...)
	script = append(script, encodeScriptNum(b)...)
	return script
}

// encodeScriptNum encodes an int64 as a script number (minimal encoding).
func encodeScriptNum(n int64) []byte {
	if n == 0 {
		return []byte{0x00}
	}
	absN := n
	negative := false
	if n < 0 {
		absN = -n
		negative = true
	}
	result := make([]byte, 0, 8)
	for absN > 0 {
		result = append(result, byte(absN&0xff))
		absN >>= 8
	}
	if result[len(result)-1]&0x80 != 0 {
		if negative {
			result = append(result, 0x80)
		} else {
			result = append(result, 0x00)
		}
	} else if negative {
		result[len(result)-1] |= 0x80
	}
	result = append([]byte{byte(len(result))}, result...)
	return result
}

type SpendableOut struct { PrevOut wire.OutPoint; Amount nogoutil.Amount }

// MakeSpendableOutForTx creates a spendable output from the first output of a transaction.
func MakeSpendableOutForTx(tx *wire.MsgTx, index uint32) SpendableOut {
	return SpendableOut{
		PrevOut: wire.OutPoint{Hash: tx.TxHash(), Index: index},
		Amount:  nogoutil.Amount(tx.TxOut[index].Value),
	}
}

// CreateSpendTx creates a transaction that spends from the provided spendable output.
func CreateSpendTx(spend *SpendableOut, fee nogoutil.Amount) *wire.MsgTx {
	spendTx := wire.NewMsgTx(1)
	spendTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: spend.PrevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		SignatureScript:  nil,
	})
	spendTx.AddTxOut(wire.NewTxOut(int64(spend.Amount-fee),
		OpTrueScript))
	opRetScript, err := UniqueOpReturnScript()
	if err != nil {
		panic(err)
	}
	spendTx.AddTxOut(wire.NewTxOut(0, opRetScript))

	return spendTx
}

func HashToBigStub(hash *chainhash.Hash) *big.Int { return new(big.Int).SetBytes(hash[:]) }

// SolveBlock attempts to find a nonce for the given block header.
func SolveBlock(header *wire.BlockHeader, targetDifficulty *big.Int) bool {
	for i := uint64(0); i < math.MaxUint64; i++ {
		header.Nonce = uint32(i)
		hash := header.BlockHash()
		hashNum := new(big.Int).SetBytes(hash[:])
		if hashNum.Cmp(targetDifficulty) <= 0 {
			return true
		}
		if i%1000000 == 0 {
			runtime.Gosched()
		}
	}
	return false
}

// MakeSpendableOut creates a spendable output from a transaction.
func MakeSpendableOut(tx *wire.MsgTx, index uint32, amount nogoutil.Amount) *SpendableOut {
	return &SpendableOut{
		PrevOut: wire.OutPoint{Hash: tx.TxHash(), Index: index},
		Amount:  amount,
	}
}
