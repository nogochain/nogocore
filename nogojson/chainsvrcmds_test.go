// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package nogojson_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/nogochain/nogocore/nogojson"
	"github.com/nogochain/nogocommons/chainhash"
	"github.com/nogochain/nogocommons/wire"
)

// TestChainSvrCmds tests all of the chain server commands marshal and unmarshal
// into valid results include handling of optional fields being omitted in the
// marshalled command, while optional fields with defaults have the default
// assigned on unmarshalled commands.
func TestChainSvrCmds(t *testing.T) {
	t.Parallel()

	testID := int(1)
	tests := []struct {
		name         string
		newCmd       func() (interface{}, error)
		staticCmd    func() interface{}
		marshalled   string
		unmarshalled interface{}
	}{
		{
			name: "addnode",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("addnode", "127.0.0.1", nogojson.ANRemove)
			},
			staticCmd: func() interface{} {
				return nogojson.NewAddNodeCmd("127.0.0.1", nogojson.ANRemove)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"addnode","params":["127.0.0.1","remove"],"id":1}`,
			unmarshalled: &nogojson.AddNodeCmd{Addr: "127.0.0.1", SubCmd: nogojson.ANRemove},
		},
		{
			name: "createrawtransaction",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("createrawtransaction", `[{"txid":"123","vout":1}]`,
					`{"456":0.0123}`)
			},
			staticCmd: func() interface{} {
				txInputs := []nogojson.TransactionInput{
					{Txid: "123", Vout: 1},
				}
				amounts := map[string]float64{"456": .0123}
				return nogojson.NewCreateRawTransactionCmd(txInputs, amounts, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createrawtransaction","params":[[{"txid":"123","vout":1}],{"456":0.0123}],"id":1}`,
			unmarshalled: &nogojson.CreateRawTransactionCmd{
				Inputs:  []nogojson.TransactionInput{{Txid: "123", Vout: 1}},
				Amounts: map[string]float64{"456": .0123},
			},
		},
		{
			name: "createrawtransaction - no inputs",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("createrawtransaction", `[]`, `{"456":0.0123}`)
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"456": .0123}
				return nogojson.NewCreateRawTransactionCmd(nil, amounts, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createrawtransaction","params":[[],{"456":0.0123}],"id":1}`,
			unmarshalled: &nogojson.CreateRawTransactionCmd{
				Inputs:  []nogojson.TransactionInput{},
				Amounts: map[string]float64{"456": .0123},
			},
		},
		{
			name: "createrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("createrawtransaction", `[{"txid":"123","vout":1}]`,
					`{"456":0.0123}`, int64(12312333333))
			},
			staticCmd: func() interface{} {
				txInputs := []nogojson.TransactionInput{
					{Txid: "123", Vout: 1},
				}
				amounts := map[string]float64{"456": .0123}
				return nogojson.NewCreateRawTransactionCmd(txInputs, amounts, nogojson.Int64(12312333333))
			},
			marshalled: `{"jsonrpc":"1.0","method":"createrawtransaction","params":[[{"txid":"123","vout":1}],{"456":0.0123},12312333333],"id":1}`,
			unmarshalled: &nogojson.CreateRawTransactionCmd{
				Inputs:   []nogojson.TransactionInput{{Txid: "123", Vout: 1}},
				Amounts:  map[string]float64{"456": .0123},
				LockTime: nogojson.Int64(12312333333),
			},
		},
		{
			name: "fundrawtransaction - empty opts",
			newCmd: func() (i interface{}, e error) {
				return nogojson.NewCmd("fundrawtransaction", "deadbeef", "{}")
			},
			staticCmd: func() interface{} {
				deadbeef, err := hex.DecodeString("deadbeef")
				if err != nil {
					panic(err)
				}
				return nogojson.NewFundRawTransactionCmd(deadbeef, nogojson.FundRawTransactionOpts{}, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"fundrawtransaction","params":["deadbeef",{}],"id":1}`,
			unmarshalled: &nogojson.FundRawTransactionCmd{
				HexTx:     "deadbeef",
				Options:   nogojson.FundRawTransactionOpts{},
				IsWitness: nil,
			},
		},
		{
			name: "fundrawtransaction - full opts",
			newCmd: func() (i interface{}, e error) {
				return nogojson.NewCmd("fundrawtransaction", "deadbeef", `{"changeAddress":"bcrt1qeeuctq9wutlcl5zatge7rjgx0k45228cxez655","changePosition":1,"change_type":"legacy","includeWatching":true,"lockUnspents":true,"feeRate":0.7,"subtractFeeFromOutputs":[0],"replaceable":true,"conf_target":8,"estimate_mode":"ECONOMICAL"}`)
			},
			staticCmd: func() interface{} {
				deadbeef, err := hex.DecodeString("deadbeef")
				if err != nil {
					panic(err)
				}
				changeAddress := "bcrt1qeeuctq9wutlcl5zatge7rjgx0k45228cxez655"
				change := 1
				changeType := nogojson.ChangeTypeLegacy
				watching := true
				lockUnspents := true
				feeRate := 0.7
				replaceable := true
				confTarget := 8

				return nogojson.NewFundRawTransactionCmd(deadbeef, nogojson.FundRawTransactionOpts{
					ChangeAddress:          &changeAddress,
					ChangePosition:         &change,
					ChangeType:             &changeType,
					IncludeWatching:        &watching,
					LockUnspents:           &lockUnspents,
					FeeRate:                &feeRate,
					SubtractFeeFromOutputs: []int{0},
					Replaceable:            &replaceable,
					ConfTarget:             &confTarget,
					EstimateMode:           &nogojson.EstimateModeEconomical,
				}, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"fundrawtransaction","params":["deadbeef",{"changeAddress":"bcrt1qeeuctq9wutlcl5zatge7rjgx0k45228cxez655","changePosition":1,"change_type":"legacy","includeWatching":true,"lockUnspents":true,"feeRate":0.7,"subtractFeeFromOutputs":[0],"replaceable":true,"conf_target":8,"estimate_mode":"ECONOMICAL"}],"id":1}`,
			unmarshalled: func() interface{} {
				changeAddress := "bcrt1qeeuctq9wutlcl5zatge7rjgx0k45228cxez655"
				change := 1
				changeType := nogojson.ChangeTypeLegacy
				watching := true
				lockUnspents := true
				feeRate := 0.7
				replaceable := true
				confTarget := 8
				return &nogojson.FundRawTransactionCmd{
					HexTx: "deadbeef",
					Options: nogojson.FundRawTransactionOpts{
						ChangeAddress:          &changeAddress,
						ChangePosition:         &change,
						ChangeType:             &changeType,
						IncludeWatching:        &watching,
						LockUnspents:           &lockUnspents,
						FeeRate:                &feeRate,
						SubtractFeeFromOutputs: []int{0},
						Replaceable:            &replaceable,
						ConfTarget:             &confTarget,
						EstimateMode:           &nogojson.EstimateModeEconomical,
					},
					IsWitness: nil,
				}
			}(),
		},
		{
			name: "fundrawtransaction - iswitness",
			newCmd: func() (i interface{}, e error) {
				return nogojson.NewCmd("fundrawtransaction", "deadbeef", "{}", true)
			},
			staticCmd: func() interface{} {
				deadbeef, err := hex.DecodeString("deadbeef")
				if err != nil {
					panic(err)
				}
				t := true
				return nogojson.NewFundRawTransactionCmd(deadbeef, nogojson.FundRawTransactionOpts{}, &t)
			},
			marshalled: `{"jsonrpc":"1.0","method":"fundrawtransaction","params":["deadbeef",{},true],"id":1}`,
			unmarshalled: &nogojson.FundRawTransactionCmd{
				HexTx:   "deadbeef",
				Options: nogojson.FundRawTransactionOpts{},
				IsWitness: func() *bool {
					t := true
					return &t
				}(),
			},
		},
		{
			name: "decoderawtransaction",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("decoderawtransaction", "123")
			},
			staticCmd: func() interface{} {
				return nogojson.NewDecodeRawTransactionCmd("123")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"decoderawtransaction","params":["123"],"id":1}`,
			unmarshalled: &nogojson.DecodeRawTransactionCmd{HexTx: "123"},
		},
		{
			name: "decodescript",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("decodescript", "00")
			},
			staticCmd: func() interface{} {
				return nogojson.NewDecodeScriptCmd("00")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"decodescript","params":["00"],"id":1}`,
			unmarshalled: &nogojson.DecodeScriptCmd{HexScript: "00"},
		},
		{
			name: "deriveaddresses no range",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("deriveaddresses", "00")
			},
			staticCmd: func() interface{} {
				return nogojson.NewDeriveAddressesCmd("00", nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"deriveaddresses","params":["00"],"id":1}`,
			unmarshalled: &nogojson.DeriveAddressesCmd{Descriptor: "00"},
		},
		{
			name: "deriveaddresses int range",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"deriveaddresses", "00", nogojson.DescriptorRange{Value: 2})
			},
			staticCmd: func() interface{} {
				return nogojson.NewDeriveAddressesCmd(
					"00", &nogojson.DescriptorRange{Value: 2})
			},
			marshalled: `{"jsonrpc":"1.0","method":"deriveaddresses","params":["00",2],"id":1}`,
			unmarshalled: &nogojson.DeriveAddressesCmd{
				Descriptor: "00",
				Range:      &nogojson.DescriptorRange{Value: 2},
			},
		},
		{
			name: "deriveaddresses slice range",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"deriveaddresses", "00",
					nogojson.DescriptorRange{Value: []int{0, 2}},
				)
			},
			staticCmd: func() interface{} {
				return nogojson.NewDeriveAddressesCmd(
					"00", &nogojson.DescriptorRange{Value: []int{0, 2}})
			},
			marshalled: `{"jsonrpc":"1.0","method":"deriveaddresses","params":["00",[0,2]],"id":1}`,
			unmarshalled: &nogojson.DeriveAddressesCmd{
				Descriptor: "00",
				Range:      &nogojson.DescriptorRange{Value: []int{0, 2}},
			},
		},
		{
			name: "getaddednodeinfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getaddednodeinfo", true)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetAddedNodeInfoCmd(true, nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getaddednodeinfo","params":[true],"id":1}`,
			unmarshalled: &nogojson.GetAddedNodeInfoCmd{DNS: true, Node: nil},
		},
		{
			name: "getaddednodeinfo optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getaddednodeinfo", true, "127.0.0.1")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetAddedNodeInfoCmd(true, nogojson.String("127.0.0.1"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaddednodeinfo","params":[true,"127.0.0.1"],"id":1}`,
			unmarshalled: &nogojson.GetAddedNodeInfoCmd{
				DNS:  true,
				Node: nogojson.String("127.0.0.1"),
			},
		},
		{
			name: "getbestblockhash",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getbestblockhash")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBestBlockHashCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getbestblockhash","params":[],"id":1}`,
			unmarshalled: &nogojson.GetBestBlockHashCmd{},
		},
		{
			name: "getblock",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblock", "123", nogojson.Int(0))
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockCmd("123", nogojson.Int(0))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123",0],"id":1}`,
			unmarshalled: &nogojson.GetBlockCmd{
				Hash:      "123",
				Verbosity: nogojson.Int(0),
			},
		},
		{
			name: "getblock default verbosity",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblock", "123")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123"],"id":1}`,
			unmarshalled: &nogojson.GetBlockCmd{
				Hash:      "123",
				Verbosity: nogojson.Int(1),
			},
		},
		{
			name: "getblock required optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblock", "123", nogojson.Int(1))
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockCmd("123", nogojson.Int(1))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123",1],"id":1}`,
			unmarshalled: &nogojson.GetBlockCmd{
				Hash:      "123",
				Verbosity: nogojson.Int(1),
			},
		},
		{
			name: "getblock required optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblock", "123", nogojson.Int(2))
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockCmd("123", nogojson.Int(2))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123",2],"id":1}`,
			unmarshalled: &nogojson.GetBlockCmd{
				Hash:      "123",
				Verbosity: nogojson.Int(2),
			},
		},
		{
			name: "getblockchaininfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblockchaininfo")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockChainInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockchaininfo","params":[],"id":1}`,
			unmarshalled: &nogojson.GetBlockChainInfoCmd{},
		},
		{
			name: "getblockcount",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblockcount")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockCountCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockcount","params":[],"id":1}`,
			unmarshalled: &nogojson.GetBlockCountCmd{},
		},
		{
			name: "getblockfilter",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblockfilter", "0000afaf")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockFilterCmd("0000afaf", nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockfilter","params":["0000afaf"],"id":1}`,
			unmarshalled: &nogojson.GetBlockFilterCmd{"0000afaf", nil},
		},
		{
			name: "getblockfilter optional filtertype",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblockfilter", "0000afaf", "basic")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockFilterCmd("0000afaf", nogojson.NewFilterTypeName(nogojson.FilterTypeBasic))
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockfilter","params":["0000afaf","basic"],"id":1}`,
			unmarshalled: &nogojson.GetBlockFilterCmd{"0000afaf", nogojson.NewFilterTypeName(nogojson.FilterTypeBasic)},
		},
		{
			name: "getblockhash",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblockhash", 123)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockHashCmd(123)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockhash","params":[123],"id":1}`,
			unmarshalled: &nogojson.GetBlockHashCmd{Index: 123},
		},
		{
			name: "getblockheader",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblockheader", "123")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockHeaderCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockheader","params":["123"],"id":1}`,
			unmarshalled: &nogojson.GetBlockHeaderCmd{
				Hash:    "123",
				Verbose: nogojson.Bool(true),
			},
		},
		{
			name: "getblockstats height",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblockstats", nogojson.HashOrHeight{Value: 123})
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockStatsCmd(nogojson.HashOrHeight{Value: 123}, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockstats","params":[123],"id":1}`,
			unmarshalled: &nogojson.GetBlockStatsCmd{
				HashOrHeight: nogojson.HashOrHeight{Value: 123},
			},
		},
		{
			name: "getblockstats hash",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblockstats", nogojson.HashOrHeight{Value: "deadbeef"})
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockStatsCmd(nogojson.HashOrHeight{Value: "deadbeef"}, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockstats","params":["deadbeef"],"id":1}`,
			unmarshalled: &nogojson.GetBlockStatsCmd{
				HashOrHeight: nogojson.HashOrHeight{Value: "deadbeef"},
			},
		},
		{
			name: "getblockstats height optional stats",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblockstats", nogojson.HashOrHeight{Value: 123}, []string{"avgfee", "maxfee"})
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockStatsCmd(nogojson.HashOrHeight{Value: 123}, &[]string{"avgfee", "maxfee"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockstats","params":[123,["avgfee","maxfee"]],"id":1}`,
			unmarshalled: &nogojson.GetBlockStatsCmd{
				HashOrHeight: nogojson.HashOrHeight{Value: 123},
				Stats:        &[]string{"avgfee", "maxfee"},
			},
		},
		{
			name: "getblockstats hash optional stats",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblockstats", nogojson.HashOrHeight{Value: "deadbeef"}, []string{"avgfee", "maxfee"})
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockStatsCmd(nogojson.HashOrHeight{Value: "deadbeef"}, &[]string{"avgfee", "maxfee"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockstats","params":["deadbeef",["avgfee","maxfee"]],"id":1}`,
			unmarshalled: &nogojson.GetBlockStatsCmd{
				HashOrHeight: nogojson.HashOrHeight{Value: "deadbeef"},
				Stats:        &[]string{"avgfee", "maxfee"},
			},
		},
		{
			name: "getblocktemplate",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblocktemplate")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBlockTemplateCmd(nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblocktemplate","params":[],"id":1}`,
			unmarshalled: &nogojson.GetBlockTemplateCmd{Request: nil},
		},
		{
			name: "getblocktemplate optional - template request",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblocktemplate", `{"mode":"template","capabilities":["longpoll","coinbasetxn"]}`)
			},
			staticCmd: func() interface{} {
				template := nogojson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
				}
				return nogojson.NewGetBlockTemplateCmd(&template)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocktemplate","params":[{"mode":"template","capabilities":["longpoll","coinbasetxn"]}],"id":1}`,
			unmarshalled: &nogojson.GetBlockTemplateCmd{
				Request: &nogojson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
				},
			},
		},
		{
			name: "getblocktemplate optional - template request with tweaks",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblocktemplate", `{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":500,"sizelimit":100000000,"maxversion":2}`)
			},
			staticCmd: func() interface{} {
				template := nogojson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   500,
					SizeLimit:    100000000,
					MaxVersion:   2,
				}
				return nogojson.NewGetBlockTemplateCmd(&template)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocktemplate","params":[{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":500,"sizelimit":100000000,"maxversion":2}],"id":1}`,
			unmarshalled: &nogojson.GetBlockTemplateCmd{
				Request: &nogojson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   int64(500),
					SizeLimit:    int64(100000000),
					MaxVersion:   2,
				},
			},
		},
		{
			name: "getblocktemplate optional - template request with tweaks 2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getblocktemplate", `{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":true,"sizelimit":100000000,"maxversion":2}`)
			},
			staticCmd: func() interface{} {
				template := nogojson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   true,
					SizeLimit:    100000000,
					MaxVersion:   2,
				}
				return nogojson.NewGetBlockTemplateCmd(&template)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocktemplate","params":[{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":true,"sizelimit":100000000,"maxversion":2}],"id":1}`,
			unmarshalled: &nogojson.GetBlockTemplateCmd{
				Request: &nogojson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   true,
					SizeLimit:    int64(100000000),
					MaxVersion:   2,
				},
			},
		},
		{
			name: "getcfilter",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getcfilter", "123",
					wire.GCSFilterRegular)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetCFilterCmd("123",
					wire.GCSFilterRegular)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getcfilter","params":["123",0],"id":1}`,
			unmarshalled: &nogojson.GetCFilterCmd{
				Hash:       "123",
				FilterType: wire.GCSFilterRegular,
			},
		},
		{
			name: "getcfilterheader",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getcfilterheader", "123",
					wire.GCSFilterRegular)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetCFilterHeaderCmd("123",
					wire.GCSFilterRegular)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getcfilterheader","params":["123",0],"id":1}`,
			unmarshalled: &nogojson.GetCFilterHeaderCmd{
				Hash:       "123",
				FilterType: wire.GCSFilterRegular,
			},
		},
		{
			name: "getchaintips",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getchaintips")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetChainTipsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getchaintips","params":[],"id":1}`,
			unmarshalled: &nogojson.GetChainTipsCmd{},
		},
		{
			name: "getchaintxstats",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getchaintxstats")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetChainTxStatsCmd(nil, nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getchaintxstats","params":[],"id":1}`,
			unmarshalled: &nogojson.GetChainTxStatsCmd{},
		},
		{
			name: "getchaintxstats optional nblocks",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getchaintxstats", nogojson.Int32(1000))
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetChainTxStatsCmd(nogojson.Int32(1000), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getchaintxstats","params":[1000],"id":1}`,
			unmarshalled: &nogojson.GetChainTxStatsCmd{
				NBlocks: nogojson.Int32(1000),
			},
		},
		{
			name: "getchaintxstats optional nblocks and blockhash",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getchaintxstats", nogojson.Int32(1000), nogojson.String("0000afaf"))
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetChainTxStatsCmd(nogojson.Int32(1000), nogojson.String("0000afaf"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getchaintxstats","params":[1000,"0000afaf"],"id":1}`,
			unmarshalled: &nogojson.GetChainTxStatsCmd{
				NBlocks:   nogojson.Int32(1000),
				BlockHash: nogojson.String("0000afaf"),
			},
		},
		{
			name: "getconnectioncount",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getconnectioncount")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetConnectionCountCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getconnectioncount","params":[],"id":1}`,
			unmarshalled: &nogojson.GetConnectionCountCmd{},
		},
		{
			name: "getdifficulty",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getdifficulty")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetDifficultyCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getdifficulty","params":[],"id":1}`,
			unmarshalled: &nogojson.GetDifficultyCmd{},
		},
		{
			name: "getgenerate",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getgenerate")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetGenerateCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getgenerate","params":[],"id":1}`,
			unmarshalled: &nogojson.GetGenerateCmd{},
		},
		{
			name: "gethashespersec",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("gethashespersec")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetHashesPerSecCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"gethashespersec","params":[],"id":1}`,
			unmarshalled: &nogojson.GetHashesPerSecCmd{},
		},
		{
			name: "getinfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getinfo")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getinfo","params":[],"id":1}`,
			unmarshalled: &nogojson.GetInfoCmd{},
		},
		{
			name: "getmempoolentry",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getmempoolentry", "txhash")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetMempoolEntryCmd("txhash")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getmempoolentry","params":["txhash"],"id":1}`,
			unmarshalled: &nogojson.GetMempoolEntryCmd{
				TxID: "txhash",
			},
		},
		{
			name: "getmempoolinfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getmempoolinfo")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetMempoolInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getmempoolinfo","params":[],"id":1}`,
			unmarshalled: &nogojson.GetMempoolInfoCmd{},
		},
		{
			name: "getmininginfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getmininginfo")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetMiningInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getmininginfo","params":[],"id":1}`,
			unmarshalled: &nogojson.GetMiningInfoCmd{},
		},
		{
			name: "getnetworkinfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getnetworkinfo")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetNetworkInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getnetworkinfo","params":[],"id":1}`,
			unmarshalled: &nogojson.GetNetworkInfoCmd{},
		},
		{
			name: "getnettotals",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getnettotals")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetNetTotalsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getnettotals","params":[],"id":1}`,
			unmarshalled: &nogojson.GetNetTotalsCmd{},
		},
		{
			name: "getnetworkhashps",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getnetworkhashps")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetNetworkHashPSCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[],"id":1}`,
			unmarshalled: &nogojson.GetNetworkHashPSCmd{
				Blocks: nogojson.Int(120),
				Height: nogojson.Int(-1),
			},
		},
		{
			name: "getnetworkhashps optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getnetworkhashps", 200)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetNetworkHashPSCmd(nogojson.Int(200), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[200],"id":1}`,
			unmarshalled: &nogojson.GetNetworkHashPSCmd{
				Blocks: nogojson.Int(200),
				Height: nogojson.Int(-1),
			},
		},
		{
			name: "getnetworkhashps optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getnetworkhashps", 200, 123)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetNetworkHashPSCmd(nogojson.Int(200), nogojson.Int(123))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[200,123],"id":1}`,
			unmarshalled: &nogojson.GetNetworkHashPSCmd{
				Blocks: nogojson.Int(200),
				Height: nogojson.Int(123),
			},
		},
		{
			name: "getnodeaddresses",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getnodeaddresses")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetNodeAddressesCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnodeaddresses","params":[],"id":1}`,
			unmarshalled: &nogojson.GetNodeAddressesCmd{
				Count: nogojson.Int32(1),
			},
		},
		{
			name: "getnodeaddresses optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getnodeaddresses", 10)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetNodeAddressesCmd(nogojson.Int32(10))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnodeaddresses","params":[10],"id":1}`,
			unmarshalled: &nogojson.GetNodeAddressesCmd{
				Count: nogojson.Int32(10),
			},
		},
		{
			name: "getpeerinfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getpeerinfo")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetPeerInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getpeerinfo","params":[],"id":1}`,
			unmarshalled: &nogojson.GetPeerInfoCmd{},
		},
		{
			name: "getrawmempool",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getrawmempool")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetRawMempoolCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawmempool","params":[],"id":1}`,
			unmarshalled: &nogojson.GetRawMempoolCmd{
				Verbose: nogojson.Bool(false),
			},
		},
		{
			name: "getrawmempool optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getrawmempool", false)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetRawMempoolCmd(nogojson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawmempool","params":[false],"id":1}`,
			unmarshalled: &nogojson.GetRawMempoolCmd{
				Verbose: nogojson.Bool(false),
			},
		},
		{
			name: "getrawtransaction",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getrawtransaction", "123")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetRawTransactionCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawtransaction","params":["123"],"id":1}`,
			unmarshalled: &nogojson.GetRawTransactionCmd{
				Txid:    "123",
				Verbose: nogojson.Int(0),
			},
		},
		{
			name: "getrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getrawtransaction", "123", 1)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetRawTransactionCmd("123", nogojson.Int(1))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawtransaction","params":["123",1],"id":1}`,
			unmarshalled: &nogojson.GetRawTransactionCmd{
				Txid:    "123",
				Verbose: nogojson.Int(1),
			},
		},
		{
			name: "gettxout",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("gettxout", "123", 1)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetTxOutCmd("123", 1, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxout","params":["123",1],"id":1}`,
			unmarshalled: &nogojson.GetTxOutCmd{
				Txid:           "123",
				Vout:           1,
				IncludeMempool: nogojson.Bool(true),
			},
		},
		{
			name: "gettxout optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("gettxout", "123", 1, true)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetTxOutCmd("123", 1, nogojson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxout","params":["123",1,true],"id":1}`,
			unmarshalled: &nogojson.GetTxOutCmd{
				Txid:           "123",
				Vout:           1,
				IncludeMempool: nogojson.Bool(true),
			},
		},
		{
			name: "gettxoutproof",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("gettxoutproof", []string{"123", "456"})
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetTxOutProofCmd([]string{"123", "456"}, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxoutproof","params":[["123","456"]],"id":1}`,
			unmarshalled: &nogojson.GetTxOutProofCmd{
				TxIDs: []string{"123", "456"},
			},
		},
		{
			name: "gettxoutproof optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("gettxoutproof", []string{"123", "456"},
					nogojson.String("000000000000034a7dedef4a161fa058a2d67a173a90155f3a2fe6fc132e0ebf"))
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetTxOutProofCmd([]string{"123", "456"},
					nogojson.String("000000000000034a7dedef4a161fa058a2d67a173a90155f3a2fe6fc132e0ebf"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxoutproof","params":[["123","456"],` +
				`"000000000000034a7dedef4a161fa058a2d67a173a90155f3a2fe6fc132e0ebf"],"id":1}`,
			unmarshalled: &nogojson.GetTxOutProofCmd{
				TxIDs:     []string{"123", "456"},
				BlockHash: nogojson.String("000000000000034a7dedef4a161fa058a2d67a173a90155f3a2fe6fc132e0ebf"),
			},
		},
		{
			name: "gettxoutsetinfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("gettxoutsetinfo")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetTxOutSetInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"gettxoutsetinfo","params":[],"id":1}`,
			unmarshalled: &nogojson.GetTxOutSetInfoCmd{},
		},
		{
			name: "getwork",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getwork")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetWorkCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getwork","params":[],"id":1}`,
			unmarshalled: &nogojson.GetWorkCmd{
				Data: nil,
			},
		},
		{
			name: "getwork optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getwork", "00112233")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetWorkCmd(nogojson.String("00112233"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getwork","params":["00112233"],"id":1}`,
			unmarshalled: &nogojson.GetWorkCmd{
				Data: nogojson.String("00112233"),
			},
		},
		{
			name: "help",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("help")
			},
			staticCmd: func() interface{} {
				return nogojson.NewHelpCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"help","params":[],"id":1}`,
			unmarshalled: &nogojson.HelpCmd{
				Command: nil,
			},
		},
		{
			name: "help optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("help", "getblock")
			},
			staticCmd: func() interface{} {
				return nogojson.NewHelpCmd(nogojson.String("getblock"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"help","params":["getblock"],"id":1}`,
			unmarshalled: &nogojson.HelpCmd{
				Command: nogojson.String("getblock"),
			},
		},
		{
			name: "invalidateblock",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("invalidateblock", "123")
			},
			staticCmd: func() interface{} {
				return nogojson.NewInvalidateBlockCmd("123")
			},
			marshalled: `{"jsonrpc":"1.0","method":"invalidateblock","params":["123"],"id":1}`,
			unmarshalled: &nogojson.InvalidateBlockCmd{
				BlockHash: "123",
			},
		},
		{
			name: "ping",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("ping")
			},
			staticCmd: func() interface{} {
				return nogojson.NewPingCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"ping","params":[],"id":1}`,
			unmarshalled: &nogojson.PingCmd{},
		},
		{
			name: "preciousblock",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("preciousblock", "0123")
			},
			staticCmd: func() interface{} {
				return nogojson.NewPreciousBlockCmd("0123")
			},
			marshalled: `{"jsonrpc":"1.0","method":"preciousblock","params":["0123"],"id":1}`,
			unmarshalled: &nogojson.PreciousBlockCmd{
				BlockHash: "0123",
			},
		},
		{
			name: "reconsiderblock",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("reconsiderblock", "123")
			},
			staticCmd: func() interface{} {
				return nogojson.NewReconsiderBlockCmd("123")
			},
			marshalled: `{"jsonrpc":"1.0","method":"reconsiderblock","params":["123"],"id":1}`,
			unmarshalled: &nogojson.ReconsiderBlockCmd{
				BlockHash: "123",
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("searchrawtransactions", "1Address")
			},
			staticCmd: func() interface{} {
				return nogojson.NewSearchRawTransactionsCmd("1Address", nil, nil, nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address"],"id":1}`,
			unmarshalled: &nogojson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     nogojson.Int(1),
				Skip:        nogojson.Int(0),
				Count:       nogojson.Int(100),
				VinExtra:    nogojson.Int(0),
				Reverse:     nogojson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("searchrawtransactions", "1Address", 0)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSearchRawTransactionsCmd("1Address",
					nogojson.Int(0), nil, nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0],"id":1}`,
			unmarshalled: &nogojson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     nogojson.Int(0),
				Skip:        nogojson.Int(0),
				Count:       nogojson.Int(100),
				VinExtra:    nogojson.Int(0),
				Reverse:     nogojson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("searchrawtransactions", "1Address", 0, 5)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSearchRawTransactionsCmd("1Address",
					nogojson.Int(0), nogojson.Int(5), nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5],"id":1}`,
			unmarshalled: &nogojson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     nogojson.Int(0),
				Skip:        nogojson.Int(5),
				Count:       nogojson.Int(100),
				VinExtra:    nogojson.Int(0),
				Reverse:     nogojson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSearchRawTransactionsCmd("1Address",
					nogojson.Int(0), nogojson.Int(5), nogojson.Int(10), nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10],"id":1}`,
			unmarshalled: &nogojson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     nogojson.Int(0),
				Skip:        nogojson.Int(5),
				Count:       nogojson.Int(10),
				VinExtra:    nogojson.Int(0),
				Reverse:     nogojson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, 1)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSearchRawTransactionsCmd("1Address",
					nogojson.Int(0), nogojson.Int(5), nogojson.Int(10), nogojson.Int(1), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,1],"id":1}`,
			unmarshalled: &nogojson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     nogojson.Int(0),
				Skip:        nogojson.Int(5),
				Count:       nogojson.Int(10),
				VinExtra:    nogojson.Int(1),
				Reverse:     nogojson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, 1, true)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSearchRawTransactionsCmd("1Address",
					nogojson.Int(0), nogojson.Int(5), nogojson.Int(10), nogojson.Int(1), nogojson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,1,true],"id":1}`,
			unmarshalled: &nogojson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     nogojson.Int(0),
				Skip:        nogojson.Int(5),
				Count:       nogojson.Int(10),
				VinExtra:    nogojson.Int(1),
				Reverse:     nogojson.Bool(true),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, 1, true, []string{"1Address"})
			},
			staticCmd: func() interface{} {
				return nogojson.NewSearchRawTransactionsCmd("1Address",
					nogojson.Int(0), nogojson.Int(5), nogojson.Int(10), nogojson.Int(1), nogojson.Bool(true), &[]string{"1Address"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,1,true,["1Address"]],"id":1}`,
			unmarshalled: &nogojson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     nogojson.Int(0),
				Skip:        nogojson.Int(5),
				Count:       nogojson.Int(10),
				VinExtra:    nogojson.Int(1),
				Reverse:     nogojson.Bool(true),
				FilterAddrs: &[]string{"1Address"},
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, "null", true, []string{"1Address"})
			},
			staticCmd: func() interface{} {
				return nogojson.NewSearchRawTransactionsCmd("1Address",
					nogojson.Int(0), nogojson.Int(5), nogojson.Int(10), nil, nogojson.Bool(true), &[]string{"1Address"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,null,true,["1Address"]],"id":1}`,
			unmarshalled: &nogojson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     nogojson.Int(0),
				Skip:        nogojson.Int(5),
				Count:       nogojson.Int(10),
				VinExtra:    nil,
				Reverse:     nogojson.Bool(true),
				FilterAddrs: &[]string{"1Address"},
			},
		},
		{
			name: "sendrawtransaction",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendrawtransaction", "1122", &nogojson.AllowHighFeesOrMaxFeeRate{})
			},
			staticCmd: func() interface{} {
				return nogojson.NewSendRawTransactionCmd("1122", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawtransaction","params":["1122",false],"id":1}`,
			unmarshalled: &nogojson.SendRawTransactionCmd{
				HexTx: "1122",
				FeeSetting: &nogojson.AllowHighFeesOrMaxFeeRate{
					Value: nogojson.Bool(false),
				},
			},
		},
		{
			name: "sendrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendrawtransaction", "1122", &nogojson.AllowHighFeesOrMaxFeeRate{Value: nogojson.Bool(false)})
			},
			staticCmd: func() interface{} {
				return nogojson.NewSendRawTransactionCmd("1122", nogojson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawtransaction","params":["1122",false],"id":1}`,
			unmarshalled: &nogojson.SendRawTransactionCmd{
				HexTx: "1122",
				FeeSetting: &nogojson.AllowHighFeesOrMaxFeeRate{
					Value: nogojson.Bool(false),
				},
			},
		},
		{
			name: "sendrawtransaction optional, bitcoind >= 0.19.0",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendrawtransaction", "1122", &nogojson.AllowHighFeesOrMaxFeeRate{Value: nogojson.Float64(0.1234)})
			},
			staticCmd: func() interface{} {
				return nogojson.NewBitcoindSendRawTransactionCmd("1122", 0.1234)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawtransaction","params":["1122",0.1234],"id":1}`,
			unmarshalled: &nogojson.SendRawTransactionCmd{
				HexTx: "1122",
				FeeSetting: &nogojson.AllowHighFeesOrMaxFeeRate{
					Value: nogojson.Float64(0.1234),
				},
			},
		},
		{
			name: "setgenerate",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("setgenerate", true)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSetGenerateCmd(true, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"setgenerate","params":[true],"id":1}`,
			unmarshalled: &nogojson.SetGenerateCmd{
				Generate:     true,
				GenProcLimit: nogojson.Int(-1),
			},
		},
		{
			name: "setgenerate optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("setgenerate", true, 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSetGenerateCmd(true, nogojson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"setgenerate","params":[true,6],"id":1}`,
			unmarshalled: &nogojson.SetGenerateCmd{
				Generate:     true,
				GenProcLimit: nogojson.Int(6),
			},
		},
		{
			name: "signmessagewithprivkey",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("signmessagewithprivkey", "5Hue", "Hey")
			},
			staticCmd: func() interface{} {
				return nogojson.NewSignMessageWithPrivKey("5Hue", "Hey")
			},
			marshalled: `{"jsonrpc":"1.0","method":"signmessagewithprivkey","params":["5Hue","Hey"],"id":1}`,
			unmarshalled: &nogojson.SignMessageWithPrivKeyCmd{
				PrivKey: "5Hue",
				Message: "Hey",
			},
		},
		{
			name: "stop",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("stop")
			},
			staticCmd: func() interface{} {
				return nogojson.NewStopCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"stop","params":[],"id":1}`,
			unmarshalled: &nogojson.StopCmd{},
		},
		{
			name: "submitblock",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("submitblock", "112233")
			},
			staticCmd: func() interface{} {
				return nogojson.NewSubmitBlockCmd("112233", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitblock","params":["112233"],"id":1}`,
			unmarshalled: &nogojson.SubmitBlockCmd{
				HexBlock: "112233",
				Options:  nil,
			},
		},
		{
			name: "submitblock optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("submitblock", "112233", `{"workid":"12345"}`)
			},
			staticCmd: func() interface{} {
				options := nogojson.SubmitBlockOptions{
					WorkID: "12345",
				}
				return nogojson.NewSubmitBlockCmd("112233", &options)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitblock","params":["112233",{"workid":"12345"}],"id":1}`,
			unmarshalled: &nogojson.SubmitBlockCmd{
				HexBlock: "112233",
				Options: &nogojson.SubmitBlockOptions{
					WorkID: "12345",
				},
			},
		},
		{
			name: "uptime",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("uptime")
			},
			staticCmd: func() interface{} {
				return nogojson.NewUptimeCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"uptime","params":[],"id":1}`,
			unmarshalled: &nogojson.UptimeCmd{},
		},
		{
			name: "validateaddress",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("validateaddress", "1Address")
			},
			staticCmd: func() interface{} {
				return nogojson.NewValidateAddressCmd("1Address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"validateaddress","params":["1Address"],"id":1}`,
			unmarshalled: &nogojson.ValidateAddressCmd{
				Address: "1Address",
			},
		},
		{
			name: "verifychain",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("verifychain")
			},
			staticCmd: func() interface{} {
				return nogojson.NewVerifyChainCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[],"id":1}`,
			unmarshalled: &nogojson.VerifyChainCmd{
				CheckLevel: nogojson.Int32(3),
				CheckDepth: nogojson.Int32(288),
			},
		},
		{
			name: "verifychain optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("verifychain", 2)
			},
			staticCmd: func() interface{} {
				return nogojson.NewVerifyChainCmd(nogojson.Int32(2), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[2],"id":1}`,
			unmarshalled: &nogojson.VerifyChainCmd{
				CheckLevel: nogojson.Int32(2),
				CheckDepth: nogojson.Int32(288),
			},
		},
		{
			name: "verifychain optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("verifychain", 2, 500)
			},
			staticCmd: func() interface{} {
				return nogojson.NewVerifyChainCmd(nogojson.Int32(2), nogojson.Int32(500))
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[2,500],"id":1}`,
			unmarshalled: &nogojson.VerifyChainCmd{
				CheckLevel: nogojson.Int32(2),
				CheckDepth: nogojson.Int32(500),
			},
		},
		{
			name: "verifymessage",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("verifymessage", "1Address", "301234", "test")
			},
			staticCmd: func() interface{} {
				return nogojson.NewVerifyMessageCmd("1Address", "301234", "test")
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifymessage","params":["1Address","301234","test"],"id":1}`,
			unmarshalled: &nogojson.VerifyMessageCmd{
				Address:   "1Address",
				Signature: "301234",
				Message:   "test",
			},
		},
		{
			name: "verifytxoutproof",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("verifytxoutproof", "test")
			},
			staticCmd: func() interface{} {
				return nogojson.NewVerifyTxOutProofCmd("test")
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifytxoutproof","params":["test"],"id":1}`,
			unmarshalled: &nogojson.VerifyTxOutProofCmd{
				Proof: "test",
			},
		},
		{
			name: "getdescriptorinfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getdescriptorinfo", "123")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetDescriptorInfoCmd("123")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getdescriptorinfo","params":["123"],"id":1}`,
			unmarshalled: &nogojson.GetDescriptorInfoCmd{Descriptor: "123"},
		},
		{
			name: "getzmqnotifications",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getzmqnotifications")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetZmqNotificationsCmd()
			},

			marshalled:   `{"jsonrpc":"1.0","method":"getzmqnotifications","params":[],"id":1}`,
			unmarshalled: &nogojson.GetZmqNotificationsCmd{},
		},
		{
			name: "testmempoolaccept",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("testmempoolaccept", []string{"rawhex"}, 0.1)
			},
			staticCmd: func() interface{} {
				return nogojson.NewTestMempoolAcceptCmd([]string{"rawhex"}, 0.1)
			},
			marshalled: `{"jsonrpc":"1.0","method":"testmempoolaccept","params":[["rawhex"],0.1],"id":1}`,
			unmarshalled: &nogojson.TestMempoolAcceptCmd{
				RawTxns:    []string{"rawhex"},
				MaxFeeRate: 0.1,
			},
		},
		{
			name: "testmempoolaccept with maxfeerate",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("testmempoolaccept", []string{"rawhex"}, 0.01)
			},
			staticCmd: func() interface{} {
				return nogojson.NewTestMempoolAcceptCmd([]string{"rawhex"}, 0.01)
			},
			marshalled: `{"jsonrpc":"1.0","method":"testmempoolaccept","params":[["rawhex"],0.01],"id":1}`,
			unmarshalled: &nogojson.TestMempoolAcceptCmd{
				RawTxns:    []string{"rawhex"},
				MaxFeeRate: 0.01,
			},
		},
		{
			name: "gettxspendingprevout",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"gettxspendingprevout",
					[]*nogojson.GetTxSpendingPrevOutCmdOutput{
						{Txid: "0000000000000000000000000000000000000000000000000000000000000001", Vout: 0},
					})
			},
			staticCmd: func() interface{} {
				outputs := []wire.OutPoint{
					{Hash: chainhash.Hash{1}, Index: 0},
				}
				return nogojson.NewGetTxSpendingPrevOutCmd(outputs)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxspendingprevout","params":[[{"txid":"0000000000000000000000000000000000000000000000000000000000000001","vout":0}]],"id":1}`,
			unmarshalled: &nogojson.GetTxSpendingPrevOutCmd{
				Outputs: []*nogojson.GetTxSpendingPrevOutCmdOutput{{
					Txid: "0000000000000000000000000000000000000000000000000000000000000001",
					Vout: 0,
				}},
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Marshal the command as created by the new static command
		// creation function.
		marshalled, err := nogojson.MarshalCmd(nogojson.RpcVersion1, testID, test.staticCmd())
		if err != nil {
			t.Errorf("MarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !bytes.Equal(marshalled, []byte(test.marshalled)) {
			t.Errorf("Test #%d (%s) unexpected marshalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.marshalled)
			t.Errorf("\n%s\n%s", marshalled, test.marshalled)
			continue
		}

		// Ensure the command is created without error via the generic
		// new command creation function.
		cmd, err := test.newCmd()
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected NewCmd error: %v ",
				i, test.name, err)
		}

		// Marshal the command as created by the generic new command
		// creation function.
		marshalled, err = nogojson.MarshalCmd(nogojson.RpcVersion1, testID, cmd)
		if err != nil {
			t.Errorf("MarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !bytes.Equal(marshalled, []byte(test.marshalled)) {
			t.Errorf("Test #%d (%s) unexpected marshalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.marshalled)
			continue
		}

		var request nogojson.Request
		if err := json.Unmarshal(marshalled, &request); err != nil {
			t.Errorf("Test #%d (%s) unexpected error while "+
				"unmarshalling JSON-RPC request: %v", i,
				test.name, err)
			continue
		}

		cmd, err = nogojson.UnmarshalCmd(&request)
		if err != nil {
			t.Errorf("UnmarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !reflect.DeepEqual(cmd, test.unmarshalled) {
			t.Errorf("Test #%d (%s) unexpected unmarshalled command "+
				"- got %s, want %s", i, test.name,
				fmt.Sprintf("(%T) %+[1]v", cmd),
				fmt.Sprintf("(%T) %+[1]v\n", test.unmarshalled))
			continue
		}
	}
}

// TestChainSvrCmdErrors ensures any errors that occur in the command during
// custom mashal and unmarshal are as expected.
func TestChainSvrCmdErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		result     interface{}
		marshalled string
		err        error
	}{
		{
			name:       "template request with invalid type",
			result:     &nogojson.TemplateRequest{},
			marshalled: `{"mode":1}`,
			err:        &json.UnmarshalTypeError{},
		},
		{
			name:       "invalid template request sigoplimit field",
			result:     &nogojson.TemplateRequest{},
			marshalled: `{"sigoplimit":"invalid"}`,
			err:        nogojson.Error{ErrorCode: nogojson.ErrInvalidType},
		},
		{
			name:       "invalid template request sizelimit field",
			result:     &nogojson.TemplateRequest{},
			marshalled: `{"sizelimit":"invalid"}`,
			err:        nogojson.Error{ErrorCode: nogojson.ErrInvalidType},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		err := json.Unmarshal([]byte(test.marshalled), &test.result)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error - got %T (%v), "+
				"want %T", i, test.name, err, err, test.err)
			continue
		}

		if terr, ok := test.err.(nogojson.Error); ok {
			gotErrorCode := err.(nogojson.Error).ErrorCode
			if gotErrorCode != terr.ErrorCode {
				t.Errorf("Test #%d (%s) mismatched error code "+
					"- got %v (%v), want %v", i, test.name,
					gotErrorCode, terr, terr.ErrorCode)
				continue
			}
		}
	}
}
