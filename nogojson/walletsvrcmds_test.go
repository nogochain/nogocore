// Copyright (c) 2014-2020 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package nogojson_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/nogochain/nogocore/nogojson"
	"github.com/nogochain/nogocommons/nogoutil"
)

// TestWalletSvrCmds tests all of the wallet server commands marshal and
// unmarshal into valid results include handling of optional fields being
// omitted in the marshalled command, while optional fields with defaults have
// the default assigned on unmarshalled commands.
func TestWalletSvrCmds(t *testing.T) {
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
			name: "addmultisigaddress",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("addmultisigaddress", 2, []string{"031234", "035678"})
			},
			staticCmd: func() interface{} {
				keys := []string{"031234", "035678"}
				return nogojson.NewAddMultisigAddressCmd(2, keys, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"addmultisigaddress","params":[2,["031234","035678"]],"id":1}`,
			unmarshalled: &nogojson.AddMultisigAddressCmd{
				NRequired: 2,
				Keys:      []string{"031234", "035678"},
				Account:   nil,
			},
		},
		{
			name: "addmultisigaddress optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("addmultisigaddress", 2, []string{"031234", "035678"}, "test")
			},
			staticCmd: func() interface{} {
				keys := []string{"031234", "035678"}
				return nogojson.NewAddMultisigAddressCmd(2, keys, nogojson.String("test"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"addmultisigaddress","params":[2,["031234","035678"],"test"],"id":1}`,
			unmarshalled: &nogojson.AddMultisigAddressCmd{
				NRequired: 2,
				Keys:      []string{"031234", "035678"},
				Account:   nogojson.String("test"),
			},
		},
		{
			name: "createwallet",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("createwallet", "mywallet", true, true, "secret", true)
			},
			staticCmd: func() interface{} {
				return nogojson.NewCreateWalletCmd("mywallet",
					nogojson.Bool(true), nogojson.Bool(true),
					nogojson.String("secret"), nogojson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"createwallet","params":["mywallet",true,true,"secret",true],"id":1}`,
			unmarshalled: &nogojson.CreateWalletCmd{
				WalletName:         "mywallet",
				DisablePrivateKeys: nogojson.Bool(true),
				Blank:              nogojson.Bool(true),
				Passphrase:         nogojson.String("secret"),
				AvoidReuse:         nogojson.Bool(true),
			},
		},
		{
			name: "createwallet - optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("createwallet", "mywallet")
			},
			staticCmd: func() interface{} {
				return nogojson.NewCreateWalletCmd("mywallet",
					nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createwallet","params":["mywallet"],"id":1}`,
			unmarshalled: &nogojson.CreateWalletCmd{
				WalletName:         "mywallet",
				DisablePrivateKeys: nogojson.Bool(false),
				Blank:              nogojson.Bool(false),
				Passphrase:         nogojson.String(""),
				AvoidReuse:         nogojson.Bool(false),
			},
		},
		{
			name: "createwallet - optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("createwallet", "mywallet", "null", "null", "secret")
			},
			staticCmd: func() interface{} {
				return nogojson.NewCreateWalletCmd("mywallet",
					nil, nil, nogojson.String("secret"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createwallet","params":["mywallet",null,null,"secret"],"id":1}`,
			unmarshalled: &nogojson.CreateWalletCmd{
				WalletName:         "mywallet",
				DisablePrivateKeys: nil,
				Blank:              nil,
				Passphrase:         nogojson.String("secret"),
				AvoidReuse:         nogojson.Bool(false),
			},
		},
		{
			name: "addwitnessaddress",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("addwitnessaddress", "1address")
			},
			staticCmd: func() interface{} {
				return nogojson.NewAddWitnessAddressCmd("1address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"addwitnessaddress","params":["1address"],"id":1}`,
			unmarshalled: &nogojson.AddWitnessAddressCmd{
				Address: "1address",
			},
		},
		{
			name: "backupwallet",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("backupwallet", "backup.dat")
			},
			staticCmd: func() interface{} {
				return nogojson.NewBackupWalletCmd("backup.dat")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"backupwallet","params":["backup.dat"],"id":1}`,
			unmarshalled: &nogojson.BackupWalletCmd{Destination: "backup.dat"},
		},
		{
			name: "loadwallet",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("loadwallet", "wallet.dat")
			},
			staticCmd: func() interface{} {
				return nogojson.NewLoadWalletCmd("wallet.dat")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"loadwallet","params":["wallet.dat"],"id":1}`,
			unmarshalled: &nogojson.LoadWalletCmd{WalletName: "wallet.dat"},
		},
		{
			name: "unloadwallet",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("unloadwallet", "default")
			},
			staticCmd: func() interface{} {
				return nogojson.NewUnloadWalletCmd(nogojson.String("default"))
			},
			marshalled:   `{"jsonrpc":"1.0","method":"unloadwallet","params":["default"],"id":1}`,
			unmarshalled: &nogojson.UnloadWalletCmd{WalletName: nogojson.String("default")},
		},
		{name: "unloadwallet - nil arg",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("unloadwallet")
			},
			staticCmd: func() interface{} {
				return nogojson.NewUnloadWalletCmd(nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"unloadwallet","params":[],"id":1}`,
			unmarshalled: &nogojson.UnloadWalletCmd{WalletName: nil},
		},
		{
			name: "createmultisig",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("createmultisig", 2, []string{"031234", "035678"})
			},
			staticCmd: func() interface{} {
				keys := []string{"031234", "035678"}
				return nogojson.NewCreateMultisigCmd(2, keys)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createmultisig","params":[2,["031234","035678"]],"id":1}`,
			unmarshalled: &nogojson.CreateMultisigCmd{
				NRequired: 2,
				Keys:      []string{"031234", "035678"},
			},
		},
		{
			name: "dumpprivkey",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("dumpprivkey", "1Address")
			},
			staticCmd: func() interface{} {
				return nogojson.NewDumpPrivKeyCmd("1Address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"dumpprivkey","params":["1Address"],"id":1}`,
			unmarshalled: &nogojson.DumpPrivKeyCmd{
				Address: "1Address",
			},
		},
		{
			name: "encryptwallet",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("encryptwallet", "pass")
			},
			staticCmd: func() interface{} {
				return nogojson.NewEncryptWalletCmd("pass")
			},
			marshalled: `{"jsonrpc":"1.0","method":"encryptwallet","params":["pass"],"id":1}`,
			unmarshalled: &nogojson.EncryptWalletCmd{
				Passphrase: "pass",
			},
		},
		{
			name: "estimatefee",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("estimatefee", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewEstimateFeeCmd(6)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatefee","params":[6],"id":1}`,
			unmarshalled: &nogojson.EstimateFeeCmd{
				NumBlocks: 6,
			},
		},
		{
			name: "estimatesmartfee - no mode",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("estimatesmartfee", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewEstimateSmartFeeCmd(6, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatesmartfee","params":[6],"id":1}`,
			unmarshalled: &nogojson.EstimateSmartFeeCmd{
				ConfTarget:   6,
				EstimateMode: &nogojson.EstimateModeConservative,
			},
		},
		{
			name: "estimatesmartfee - economical mode",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("estimatesmartfee", 6, nogojson.EstimateModeEconomical)
			},
			staticCmd: func() interface{} {
				return nogojson.NewEstimateSmartFeeCmd(6, &nogojson.EstimateModeEconomical)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatesmartfee","params":[6,"ECONOMICAL"],"id":1}`,
			unmarshalled: &nogojson.EstimateSmartFeeCmd{
				ConfTarget:   6,
				EstimateMode: &nogojson.EstimateModeEconomical,
			},
		},
		{
			name: "estimatepriority",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("estimatepriority", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewEstimatePriorityCmd(6)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatepriority","params":[6],"id":1}`,
			unmarshalled: &nogojson.EstimatePriorityCmd{
				NumBlocks: 6,
			},
		},
		{
			name: "getaccount",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getaccount", "1Address")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetAccountCmd("1Address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaccount","params":["1Address"],"id":1}`,
			unmarshalled: &nogojson.GetAccountCmd{
				Address: "1Address",
			},
		},
		{
			name: "getaccountaddress",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getaccountaddress", "acct")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetAccountAddressCmd("acct")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaccountaddress","params":["acct"],"id":1}`,
			unmarshalled: &nogojson.GetAccountAddressCmd{
				Account: "acct",
			},
		},
		{
			name: "getaddressesbyaccount",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getaddressesbyaccount", "acct")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetAddressesByAccountCmd("acct")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaddressesbyaccount","params":["acct"],"id":1}`,
			unmarshalled: &nogojson.GetAddressesByAccountCmd{
				Account: "acct",
			},
		},
		{
			name: "getaddressinfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getaddressinfo", "1234")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetAddressInfoCmd("1234")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaddressinfo","params":["1234"],"id":1}`,
			unmarshalled: &nogojson.GetAddressInfoCmd{
				Address: "1234",
			},
		},
		{
			name: "getbalance",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getbalance")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBalanceCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getbalance","params":[],"id":1}`,
			unmarshalled: &nogojson.GetBalanceCmd{
				Account: nil,
				MinConf: nogojson.Int(1),
			},
		},
		{
			name: "getbalance optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getbalance", "acct")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBalanceCmd(nogojson.String("acct"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getbalance","params":["acct"],"id":1}`,
			unmarshalled: &nogojson.GetBalanceCmd{
				Account: nogojson.String("acct"),
				MinConf: nogojson.Int(1),
			},
		},
		{
			name: "getbalance optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getbalance", "acct", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBalanceCmd(nogojson.String("acct"), nogojson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getbalance","params":["acct",6],"id":1}`,
			unmarshalled: &nogojson.GetBalanceCmd{
				Account: nogojson.String("acct"),
				MinConf: nogojson.Int(6),
			},
		},
		{
			name: "getbalances",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getbalances")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetBalancesCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getbalances","params":[],"id":1}`,
			unmarshalled: &nogojson.GetBalancesCmd{},
		},
		{
			name: "getnewaddress",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getnewaddress")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetNewAddressCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnewaddress","params":[],"id":1}`,
			unmarshalled: &nogojson.GetNewAddressCmd{
				Account:     nil,
				AddressType: nil,
			},
		},
		{
			name: "getnewaddress optional acct",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getnewaddress", "acct")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetNewAddressCmd(nogojson.String("acct"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnewaddress","params":["acct"],"id":1}`,
			unmarshalled: &nogojson.GetNewAddressCmd{
				Account:     nogojson.String("acct"),
				AddressType: nil,
			},
		},
		{
			name: "getnewaddress optional acct and type",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getnewaddress", "acct", "legacy")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetNewAddressCmd(nogojson.String("acct"), nogojson.String("legacy"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnewaddress","params":["acct","legacy"],"id":1}`,
			unmarshalled: &nogojson.GetNewAddressCmd{
				Account:     nogojson.String("acct"),
				AddressType: nogojson.String("legacy"),
			},
		},
		{
			name: "getrawchangeaddress",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getrawchangeaddress")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetRawChangeAddressCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawchangeaddress","params":[],"id":1}`,
			unmarshalled: &nogojson.GetRawChangeAddressCmd{
				Account:     nil,
				AddressType: nil,
			},
		},
		{
			name: "getrawchangeaddress optional acct",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getrawchangeaddress", "acct")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetRawChangeAddressCmd(nogojson.String("acct"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawchangeaddress","params":["acct"],"id":1}`,
			unmarshalled: &nogojson.GetRawChangeAddressCmd{
				Account:     nogojson.String("acct"),
				AddressType: nil,
			},
		},
		{
			name: "getrawchangeaddress optional acct and type",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getrawchangeaddress", "acct", "legacy")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetRawChangeAddressCmd(nogojson.String("acct"), nogojson.String("legacy"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawchangeaddress","params":["acct","legacy"],"id":1}`,
			unmarshalled: &nogojson.GetRawChangeAddressCmd{
				Account:     nogojson.String("acct"),
				AddressType: nogojson.String("legacy"),
			},
		},
		{
			name: "getreceivedbyaccount",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getreceivedbyaccount", "acct")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetReceivedByAccountCmd("acct", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaccount","params":["acct"],"id":1}`,
			unmarshalled: &nogojson.GetReceivedByAccountCmd{
				Account: "acct",
				MinConf: nogojson.Int(1),
			},
		},
		{
			name: "getreceivedbyaccount optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getreceivedbyaccount", "acct", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetReceivedByAccountCmd("acct", nogojson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaccount","params":["acct",6],"id":1}`,
			unmarshalled: &nogojson.GetReceivedByAccountCmd{
				Account: "acct",
				MinConf: nogojson.Int(6),
			},
		},
		{
			name: "getreceivedbyaddress",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getreceivedbyaddress", "1Address")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetReceivedByAddressCmd("1Address", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaddress","params":["1Address"],"id":1}`,
			unmarshalled: &nogojson.GetReceivedByAddressCmd{
				Address: "1Address",
				MinConf: nogojson.Int(1),
			},
		},
		{
			name: "getreceivedbyaddress optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getreceivedbyaddress", "1Address", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetReceivedByAddressCmd("1Address", nogojson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaddress","params":["1Address",6],"id":1}`,
			unmarshalled: &nogojson.GetReceivedByAddressCmd{
				Address: "1Address",
				MinConf: nogojson.Int(6),
			},
		},
		{
			name: "gettransaction",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("gettransaction", "123")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetTransactionCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettransaction","params":["123"],"id":1}`,
			unmarshalled: &nogojson.GetTransactionCmd{
				Txid:             "123",
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "gettransaction optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("gettransaction", "123", true)
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetTransactionCmd("123", nogojson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettransaction","params":["123",true],"id":1}`,
			unmarshalled: &nogojson.GetTransactionCmd{
				Txid:             "123",
				IncludeWatchOnly: nogojson.Bool(true),
			},
		},
		{
			name: "getwalletinfo",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("getwalletinfo")
			},
			staticCmd: func() interface{} {
				return nogojson.NewGetWalletInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getwalletinfo","params":[],"id":1}`,
			unmarshalled: &nogojson.GetWalletInfoCmd{},
		},
		{
			name: "importprivkey",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("importprivkey", "abc")
			},
			staticCmd: func() interface{} {
				return nogojson.NewImportPrivKeyCmd("abc", nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importprivkey","params":["abc"],"id":1}`,
			unmarshalled: &nogojson.ImportPrivKeyCmd{
				PrivKey: "abc",
				Label:   nil,
				Rescan:  nogojson.Bool(true),
			},
		},
		{
			name: "importprivkey optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("importprivkey", "abc", "label")
			},
			staticCmd: func() interface{} {
				return nogojson.NewImportPrivKeyCmd("abc", nogojson.String("label"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importprivkey","params":["abc","label"],"id":1}`,
			unmarshalled: &nogojson.ImportPrivKeyCmd{
				PrivKey: "abc",
				Label:   nogojson.String("label"),
				Rescan:  nogojson.Bool(true),
			},
		},
		{
			name: "importprivkey optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("importprivkey", "abc", "label", false)
			},
			staticCmd: func() interface{} {
				return nogojson.NewImportPrivKeyCmd("abc", nogojson.String("label"), nogojson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"importprivkey","params":["abc","label",false],"id":1}`,
			unmarshalled: &nogojson.ImportPrivKeyCmd{
				PrivKey: "abc",
				Label:   nogojson.String("label"),
				Rescan:  nogojson.Bool(false),
			},
		},
		{
			name: "keypoolrefill",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("keypoolrefill")
			},
			staticCmd: func() interface{} {
				return nogojson.NewKeyPoolRefillCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"keypoolrefill","params":[],"id":1}`,
			unmarshalled: &nogojson.KeyPoolRefillCmd{
				NewSize: nogojson.Uint(100),
			},
		},
		{
			name: "keypoolrefill optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("keypoolrefill", 200)
			},
			staticCmd: func() interface{} {
				return nogojson.NewKeyPoolRefillCmd(nogojson.Uint(200))
			},
			marshalled: `{"jsonrpc":"1.0","method":"keypoolrefill","params":[200],"id":1}`,
			unmarshalled: &nogojson.KeyPoolRefillCmd{
				NewSize: nogojson.Uint(200),
			},
		},
		{
			name: "listaccounts",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listaccounts")
			},
			staticCmd: func() interface{} {
				return nogojson.NewListAccountsCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listaccounts","params":[],"id":1}`,
			unmarshalled: &nogojson.ListAccountsCmd{
				MinConf: nogojson.Int(1),
			},
		},
		{
			name: "listaccounts optional",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listaccounts", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListAccountsCmd(nogojson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listaccounts","params":[6],"id":1}`,
			unmarshalled: &nogojson.ListAccountsCmd{
				MinConf: nogojson.Int(6),
			},
		},
		{
			name: "listaddressgroupings",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listaddressgroupings")
			},
			staticCmd: func() interface{} {
				return nogojson.NewListAddressGroupingsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"listaddressgroupings","params":[],"id":1}`,
			unmarshalled: &nogojson.ListAddressGroupingsCmd{},
		},
		{
			name: "listlockunspent",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listlockunspent")
			},
			staticCmd: func() interface{} {
				return nogojson.NewListLockUnspentCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"listlockunspent","params":[],"id":1}`,
			unmarshalled: &nogojson.ListLockUnspentCmd{},
		},
		{
			name: "listreceivedbyaccount",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listreceivedbyaccount")
			},
			staticCmd: func() interface{} {
				return nogojson.NewListReceivedByAccountCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[],"id":1}`,
			unmarshalled: &nogojson.ListReceivedByAccountCmd{
				MinConf:          nogojson.Int(1),
				IncludeEmpty:     nogojson.Bool(false),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaccount optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listreceivedbyaccount", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListReceivedByAccountCmd(nogojson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[6],"id":1}`,
			unmarshalled: &nogojson.ListReceivedByAccountCmd{
				MinConf:          nogojson.Int(6),
				IncludeEmpty:     nogojson.Bool(false),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaccount optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listreceivedbyaccount", 6, true)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListReceivedByAccountCmd(nogojson.Int(6), nogojson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[6,true],"id":1}`,
			unmarshalled: &nogojson.ListReceivedByAccountCmd{
				MinConf:          nogojson.Int(6),
				IncludeEmpty:     nogojson.Bool(true),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaccount optional3",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listreceivedbyaccount", 6, true, false)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListReceivedByAccountCmd(nogojson.Int(6), nogojson.Bool(true), nogojson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[6,true,false],"id":1}`,
			unmarshalled: &nogojson.ListReceivedByAccountCmd{
				MinConf:          nogojson.Int(6),
				IncludeEmpty:     nogojson.Bool(true),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listreceivedbyaddress")
			},
			staticCmd: func() interface{} {
				return nogojson.NewListReceivedByAddressCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[],"id":1}`,
			unmarshalled: &nogojson.ListReceivedByAddressCmd{
				MinConf:          nogojson.Int(1),
				IncludeEmpty:     nogojson.Bool(false),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listreceivedbyaddress", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListReceivedByAddressCmd(nogojson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[6],"id":1}`,
			unmarshalled: &nogojson.ListReceivedByAddressCmd{
				MinConf:          nogojson.Int(6),
				IncludeEmpty:     nogojson.Bool(false),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listreceivedbyaddress", 6, true)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListReceivedByAddressCmd(nogojson.Int(6), nogojson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[6,true],"id":1}`,
			unmarshalled: &nogojson.ListReceivedByAddressCmd{
				MinConf:          nogojson.Int(6),
				IncludeEmpty:     nogojson.Bool(true),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress optional3",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listreceivedbyaddress", 6, true, false)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListReceivedByAddressCmd(nogojson.Int(6), nogojson.Bool(true), nogojson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[6,true,false],"id":1}`,
			unmarshalled: &nogojson.ListReceivedByAddressCmd{
				MinConf:          nogojson.Int(6),
				IncludeEmpty:     nogojson.Bool(true),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listsinceblock",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listsinceblock")
			},
			staticCmd: func() interface{} {
				return nogojson.NewListSinceBlockCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":[],"id":1}`,
			unmarshalled: &nogojson.ListSinceBlockCmd{
				BlockHash:           nil,
				TargetConfirmations: nogojson.Int(1),
				IncludeWatchOnly:    nogojson.Bool(false),
			},
		},
		{
			name: "listsinceblock optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listsinceblock", "123")
			},
			staticCmd: func() interface{} {
				return nogojson.NewListSinceBlockCmd(nogojson.String("123"), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":["123"],"id":1}`,
			unmarshalled: &nogojson.ListSinceBlockCmd{
				BlockHash:           nogojson.String("123"),
				TargetConfirmations: nogojson.Int(1),
				IncludeWatchOnly:    nogojson.Bool(false),
			},
		},
		{
			name: "listsinceblock optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listsinceblock", "123", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListSinceBlockCmd(nogojson.String("123"), nogojson.Int(6), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":["123",6],"id":1}`,
			unmarshalled: &nogojson.ListSinceBlockCmd{
				BlockHash:           nogojson.String("123"),
				TargetConfirmations: nogojson.Int(6),
				IncludeWatchOnly:    nogojson.Bool(false),
			},
		},
		{
			name: "listsinceblock optional3",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listsinceblock", "123", 6, true)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListSinceBlockCmd(nogojson.String("123"), nogojson.Int(6), nogojson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":["123",6,true],"id":1}`,
			unmarshalled: &nogojson.ListSinceBlockCmd{
				BlockHash:           nogojson.String("123"),
				TargetConfirmations: nogojson.Int(6),
				IncludeWatchOnly:    nogojson.Bool(true),
			},
		},
		{
			name: "listsinceblock pad null",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listsinceblock", "null", 1, false)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListSinceBlockCmd(nil, nogojson.Int(1), nogojson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":[null,1,false],"id":1}`,
			unmarshalled: &nogojson.ListSinceBlockCmd{
				BlockHash:           nil,
				TargetConfirmations: nogojson.Int(1),
				IncludeWatchOnly:    nogojson.Bool(false),
			},
		},
		{
			name: "listtransactions",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listtransactions")
			},
			staticCmd: func() interface{} {
				return nogojson.NewListTransactionsCmd(nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":[],"id":1}`,
			unmarshalled: &nogojson.ListTransactionsCmd{
				Account:          nil,
				Count:            nogojson.Int(10),
				From:             nogojson.Int(0),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listtransactions optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listtransactions", "acct")
			},
			staticCmd: func() interface{} {
				return nogojson.NewListTransactionsCmd(nogojson.String("acct"), nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct"],"id":1}`,
			unmarshalled: &nogojson.ListTransactionsCmd{
				Account:          nogojson.String("acct"),
				Count:            nogojson.Int(10),
				From:             nogojson.Int(0),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listtransactions optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listtransactions", "acct", 20)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListTransactionsCmd(nogojson.String("acct"), nogojson.Int(20), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct",20],"id":1}`,
			unmarshalled: &nogojson.ListTransactionsCmd{
				Account:          nogojson.String("acct"),
				Count:            nogojson.Int(20),
				From:             nogojson.Int(0),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listtransactions optional3",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listtransactions", "acct", 20, 1)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListTransactionsCmd(nogojson.String("acct"), nogojson.Int(20),
					nogojson.Int(1), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct",20,1],"id":1}`,
			unmarshalled: &nogojson.ListTransactionsCmd{
				Account:          nogojson.String("acct"),
				Count:            nogojson.Int(20),
				From:             nogojson.Int(1),
				IncludeWatchOnly: nogojson.Bool(false),
			},
		},
		{
			name: "listtransactions optional4",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listtransactions", "acct", 20, 1, true)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListTransactionsCmd(nogojson.String("acct"), nogojson.Int(20),
					nogojson.Int(1), nogojson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct",20,1,true],"id":1}`,
			unmarshalled: &nogojson.ListTransactionsCmd{
				Account:          nogojson.String("acct"),
				Count:            nogojson.Int(20),
				From:             nogojson.Int(1),
				IncludeWatchOnly: nogojson.Bool(true),
			},
		},
		{
			name: "listunspent",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listunspent")
			},
			staticCmd: func() interface{} {
				return nogojson.NewListUnspentCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[],"id":1}`,
			unmarshalled: &nogojson.ListUnspentCmd{
				MinConf:   nogojson.Int(1),
				MaxConf:   nogojson.Int(9999999),
				Addresses: nil,
			},
		},
		{
			name: "listunspent optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listunspent", 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListUnspentCmd(nogojson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[6],"id":1}`,
			unmarshalled: &nogojson.ListUnspentCmd{
				MinConf:   nogojson.Int(6),
				MaxConf:   nogojson.Int(9999999),
				Addresses: nil,
			},
		},
		{
			name: "listunspent optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listunspent", 6, 100)
			},
			staticCmd: func() interface{} {
				return nogojson.NewListUnspentCmd(nogojson.Int(6), nogojson.Int(100), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[6,100],"id":1}`,
			unmarshalled: &nogojson.ListUnspentCmd{
				MinConf:   nogojson.Int(6),
				MaxConf:   nogojson.Int(100),
				Addresses: nil,
			},
		},
		{
			name: "listunspent optional3",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("listunspent", 6, 100, []string{"1Address", "1Address2"})
			},
			staticCmd: func() interface{} {
				return nogojson.NewListUnspentCmd(nogojson.Int(6), nogojson.Int(100),
					&[]string{"1Address", "1Address2"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[6,100,["1Address","1Address2"]],"id":1}`,
			unmarshalled: &nogojson.ListUnspentCmd{
				MinConf:   nogojson.Int(6),
				MaxConf:   nogojson.Int(100),
				Addresses: &[]string{"1Address", "1Address2"},
			},
		},
		{
			name: "lockunspent",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("lockunspent", true, `[{"txid":"123","vout":1}]`)
			},
			staticCmd: func() interface{} {
				txInputs := []nogojson.TransactionInput{
					{Txid: "123", Vout: 1},
				}
				return nogojson.NewLockUnspentCmd(true, txInputs)
			},
			marshalled: `{"jsonrpc":"1.0","method":"lockunspent","params":[true,[{"txid":"123","vout":1}]],"id":1}`,
			unmarshalled: &nogojson.LockUnspentCmd{
				Unlock: true,
				Transactions: []nogojson.TransactionInput{
					{Txid: "123", Vout: 1},
				},
			},
		},
		{
			name: "move",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("move", "from", "to", 0.5)
			},
			staticCmd: func() interface{} {
				return nogojson.NewMoveCmd("from", "to", 0.5, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"move","params":["from","to",0.5],"id":1}`,
			unmarshalled: &nogojson.MoveCmd{
				FromAccount: "from",
				ToAccount:   "to",
				Amount:      0.5,
				MinConf:     nogojson.Int(1),
				Comment:     nil,
			},
		},
		{
			name: "move optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("move", "from", "to", 0.5, 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewMoveCmd("from", "to", 0.5, nogojson.Int(6), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"move","params":["from","to",0.5,6],"id":1}`,
			unmarshalled: &nogojson.MoveCmd{
				FromAccount: "from",
				ToAccount:   "to",
				Amount:      0.5,
				MinConf:     nogojson.Int(6),
				Comment:     nil,
			},
		},
		{
			name: "move optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("move", "from", "to", 0.5, 6, "comment")
			},
			staticCmd: func() interface{} {
				return nogojson.NewMoveCmd("from", "to", 0.5, nogojson.Int(6), nogojson.String("comment"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"move","params":["from","to",0.5,6,"comment"],"id":1}`,
			unmarshalled: &nogojson.MoveCmd{
				FromAccount: "from",
				ToAccount:   "to",
				Amount:      0.5,
				MinConf:     nogojson.Int(6),
				Comment:     nogojson.String("comment"),
			},
		},
		{
			name: "sendfrom",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendfrom", "from", "1Address", 0.5)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSendFromCmd("from", "1Address", 0.5, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5],"id":1}`,
			unmarshalled: &nogojson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     nogojson.Int(1),
				Comment:     nil,
				CommentTo:   nil,
			},
		},
		{
			name: "sendfrom optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendfrom", "from", "1Address", 0.5, 6)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSendFromCmd("from", "1Address", 0.5, nogojson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5,6],"id":1}`,
			unmarshalled: &nogojson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     nogojson.Int(6),
				Comment:     nil,
				CommentTo:   nil,
			},
		},
		{
			name: "sendfrom optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendfrom", "from", "1Address", 0.5, 6, "comment")
			},
			staticCmd: func() interface{} {
				return nogojson.NewSendFromCmd("from", "1Address", 0.5, nogojson.Int(6),
					nogojson.String("comment"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5,6,"comment"],"id":1}`,
			unmarshalled: &nogojson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     nogojson.Int(6),
				Comment:     nogojson.String("comment"),
				CommentTo:   nil,
			},
		},
		{
			name: "sendfrom optional3",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendfrom", "from", "1Address", 0.5, 6, "comment", "commentto")
			},
			staticCmd: func() interface{} {
				return nogojson.NewSendFromCmd("from", "1Address", 0.5, nogojson.Int(6),
					nogojson.String("comment"), nogojson.String("commentto"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5,6,"comment","commentto"],"id":1}`,
			unmarshalled: &nogojson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     nogojson.Int(6),
				Comment:     nogojson.String("comment"),
				CommentTo:   nogojson.String("commentto"),
			},
		},
		{
			name: "sendmany",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendmany", "from", `{"1Address":0.5}`)
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"1Address": 0.5}
				return nogojson.NewSendManyCmd("from", amounts, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendmany","params":["from",{"1Address":0.5}],"id":1}`,
			unmarshalled: &nogojson.SendManyCmd{
				FromAccount: "from",
				Amounts:     map[string]float64{"1Address": 0.5},
				MinConf:     nogojson.Int(1),
				Comment:     nil,
			},
		},
		{
			name: "sendmany optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendmany", "from", `{"1Address":0.5}`, 6)
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"1Address": 0.5}
				return nogojson.NewSendManyCmd("from", amounts, nogojson.Int(6), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendmany","params":["from",{"1Address":0.5},6],"id":1}`,
			unmarshalled: &nogojson.SendManyCmd{
				FromAccount: "from",
				Amounts:     map[string]float64{"1Address": 0.5},
				MinConf:     nogojson.Int(6),
				Comment:     nil,
			},
		},
		{
			name: "sendmany optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendmany", "from", `{"1Address":0.5}`, 6, "comment")
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"1Address": 0.5}
				return nogojson.NewSendManyCmd("from", amounts, nogojson.Int(6), nogojson.String("comment"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendmany","params":["from",{"1Address":0.5},6,"comment"],"id":1}`,
			unmarshalled: &nogojson.SendManyCmd{
				FromAccount: "from",
				Amounts:     map[string]float64{"1Address": 0.5},
				MinConf:     nogojson.Int(6),
				Comment:     nogojson.String("comment"),
			},
		},
		{
			name: "sendtoaddress",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendtoaddress", "1Address", 0.5)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSendToAddressCmd("1Address", 0.5, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendtoaddress","params":["1Address",0.5],"id":1}`,
			unmarshalled: &nogojson.SendToAddressCmd{
				Address:   "1Address",
				Amount:    0.5,
				Comment:   nil,
				CommentTo: nil,
			},
		},
		{
			name: "sendtoaddress optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("sendtoaddress", "1Address", 0.5, "comment", "commentto")
			},
			staticCmd: func() interface{} {
				return nogojson.NewSendToAddressCmd("1Address", 0.5, nogojson.String("comment"),
					nogojson.String("commentto"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendtoaddress","params":["1Address",0.5,"comment","commentto"],"id":1}`,
			unmarshalled: &nogojson.SendToAddressCmd{
				Address:   "1Address",
				Amount:    0.5,
				Comment:   nogojson.String("comment"),
				CommentTo: nogojson.String("commentto"),
			},
		},
		{
			name: "setaccount",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("setaccount", "1Address", "acct")
			},
			staticCmd: func() interface{} {
				return nogojson.NewSetAccountCmd("1Address", "acct")
			},
			marshalled: `{"jsonrpc":"1.0","method":"setaccount","params":["1Address","acct"],"id":1}`,
			unmarshalled: &nogojson.SetAccountCmd{
				Address: "1Address",
				Account: "acct",
			},
		},
		{
			name: "settxfee",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("settxfee", 0.0001)
			},
			staticCmd: func() interface{} {
				return nogojson.NewSetTxFeeCmd(0.0001)
			},
			marshalled: `{"jsonrpc":"1.0","method":"settxfee","params":[0.0001],"id":1}`,
			unmarshalled: &nogojson.SetTxFeeCmd{
				Amount: 0.0001,
			},
		},
		{
			name: "signmessage",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("signmessage", "1Address", "message")
			},
			staticCmd: func() interface{} {
				return nogojson.NewSignMessageCmd("1Address", "message")
			},
			marshalled: `{"jsonrpc":"1.0","method":"signmessage","params":["1Address","message"],"id":1}`,
			unmarshalled: &nogojson.SignMessageCmd{
				Address: "1Address",
				Message: "message",
			},
		},
		{
			name: "signrawtransaction",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("signrawtransaction", "001122")
			},
			staticCmd: func() interface{} {
				return nogojson.NewSignRawTransactionCmd("001122", nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122"],"id":1}`,
			unmarshalled: &nogojson.SignRawTransactionCmd{
				RawTx:    "001122",
				Inputs:   nil,
				PrivKeys: nil,
				Flags:    nogojson.String("ALL"),
			},
		},
		{
			name: "signrawtransaction optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("signrawtransaction", "001122", `[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01"}]`)
			},
			staticCmd: func() interface{} {
				txInputs := []nogojson.RawTxInput{
					{
						Txid:         "123",
						Vout:         1,
						ScriptPubKey: "00",
						RedeemScript: "01",
					},
				}

				return nogojson.NewSignRawTransactionCmd("001122", &txInputs, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122",[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01"}]],"id":1}`,
			unmarshalled: &nogojson.SignRawTransactionCmd{
				RawTx: "001122",
				Inputs: &[]nogojson.RawTxInput{
					{
						Txid:         "123",
						Vout:         1,
						ScriptPubKey: "00",
						RedeemScript: "01",
					},
				},
				PrivKeys: nil,
				Flags:    nogojson.String("ALL"),
			},
		},
		{
			name: "signrawtransaction optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("signrawtransaction", "001122", `[]`, `["abc"]`)
			},
			staticCmd: func() interface{} {
				txInputs := []nogojson.RawTxInput{}
				privKeys := []string{"abc"}
				return nogojson.NewSignRawTransactionCmd("001122", &txInputs, &privKeys, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122",[],["abc"]],"id":1}`,
			unmarshalled: &nogojson.SignRawTransactionCmd{
				RawTx:    "001122",
				Inputs:   &[]nogojson.RawTxInput{},
				PrivKeys: &[]string{"abc"},
				Flags:    nogojson.String("ALL"),
			},
		},
		{
			name: "signrawtransaction optional3",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("signrawtransaction", "001122", `[]`, `[]`, "ALL")
			},
			staticCmd: func() interface{} {
				txInputs := []nogojson.RawTxInput{}
				privKeys := []string{}
				return nogojson.NewSignRawTransactionCmd("001122", &txInputs, &privKeys,
					nogojson.String("ALL"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122",[],[],"ALL"],"id":1}`,
			unmarshalled: &nogojson.SignRawTransactionCmd{
				RawTx:    "001122",
				Inputs:   &[]nogojson.RawTxInput{},
				PrivKeys: &[]string{},
				Flags:    nogojson.String("ALL"),
			},
		},
		{
			name: "signrawtransactionwithwallet",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("signrawtransactionwithwallet", "001122")
			},
			staticCmd: func() interface{} {
				return nogojson.NewSignRawTransactionWithWalletCmd("001122", nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransactionwithwallet","params":["001122"],"id":1}`,
			unmarshalled: &nogojson.SignRawTransactionWithWalletCmd{
				RawTx:       "001122",
				Inputs:      nil,
				SigHashType: nogojson.String("ALL"),
			},
		},
		{
			name: "signrawtransactionwithwallet optional1",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("signrawtransactionwithwallet", "001122", `[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01","witnessScript":"02","amount":1.5}]`)
			},
			staticCmd: func() interface{} {
				txInputs := []nogojson.RawTxWitnessInput{
					{
						Txid:          "123",
						Vout:          1,
						ScriptPubKey:  "00",
						RedeemScript:  nogojson.String("01"),
						WitnessScript: nogojson.String("02"),
						Amount:        nogojson.Float64(1.5),
					},
				}

				return nogojson.NewSignRawTransactionWithWalletCmd("001122", &txInputs, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransactionwithwallet","params":["001122",[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01","witnessScript":"02","amount":1.5}]],"id":1}`,
			unmarshalled: &nogojson.SignRawTransactionWithWalletCmd{
				RawTx: "001122",
				Inputs: &[]nogojson.RawTxWitnessInput{
					{
						Txid:          "123",
						Vout:          1,
						ScriptPubKey:  "00",
						RedeemScript:  nogojson.String("01"),
						WitnessScript: nogojson.String("02"),
						Amount:        nogojson.Float64(1.5),
					},
				},
				SigHashType: nogojson.String("ALL"),
			},
		},
		{
			name: "signrawtransactionwithwallet optional1 with blank fields in input",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("signrawtransactionwithwallet", "001122", `[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01"}]`)
			},
			staticCmd: func() interface{} {
				txInputs := []nogojson.RawTxWitnessInput{
					{
						Txid:         "123",
						Vout:         1,
						ScriptPubKey: "00",
						RedeemScript: nogojson.String("01"),
					},
				}

				return nogojson.NewSignRawTransactionWithWalletCmd("001122", &txInputs, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransactionwithwallet","params":["001122",[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01"}]],"id":1}`,
			unmarshalled: &nogojson.SignRawTransactionWithWalletCmd{
				RawTx: "001122",
				Inputs: &[]nogojson.RawTxWitnessInput{
					{
						Txid:         "123",
						Vout:         1,
						ScriptPubKey: "00",
						RedeemScript: nogojson.String("01"),
					},
				},
				SigHashType: nogojson.String("ALL"),
			},
		},
		{
			name: "signrawtransactionwithwallet optional2",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("signrawtransactionwithwallet", "001122", `[]`, "ALL")
			},
			staticCmd: func() interface{} {
				txInputs := []nogojson.RawTxWitnessInput{}
				return nogojson.NewSignRawTransactionWithWalletCmd("001122", &txInputs, nogojson.String("ALL"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransactionwithwallet","params":["001122",[],"ALL"],"id":1}`,
			unmarshalled: &nogojson.SignRawTransactionWithWalletCmd{
				RawTx:       "001122",
				Inputs:      &[]nogojson.RawTxWitnessInput{},
				SigHashType: nogojson.String("ALL"),
			},
		},
		{
			name: "walletlock",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("walletlock")
			},
			staticCmd: func() interface{} {
				return nogojson.NewWalletLockCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"walletlock","params":[],"id":1}`,
			unmarshalled: &nogojson.WalletLockCmd{},
		},
		{
			name: "walletpassphrase",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("walletpassphrase", "pass", 60)
			},
			staticCmd: func() interface{} {
				return nogojson.NewWalletPassphraseCmd("pass", 60)
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletpassphrase","params":["pass",60],"id":1}`,
			unmarshalled: &nogojson.WalletPassphraseCmd{
				Passphrase: "pass",
				Timeout:    60,
			},
		},
		{
			name: "walletpassphrasechange",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd("walletpassphrasechange", "old", "new")
			},
			staticCmd: func() interface{} {
				return nogojson.NewWalletPassphraseChangeCmd("old", "new")
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletpassphrasechange","params":["old","new"],"id":1}`,
			unmarshalled: &nogojson.WalletPassphraseChangeCmd{
				OldPassphrase: "old",
				NewPassphrase: "new",
			},
		},
		{
			name: "importmulti with descriptor + options",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp.
					[]nogojson.ImportMultiRequest{
						{Descriptor: nogojson.String("123"), Timestamp: nogojson.TimestampOrNow{Value: 0}},
					},
					`{"rescan": true}`,
				)
			},
			staticCmd: func() interface{} {
				requests := []nogojson.ImportMultiRequest{
					{Descriptor: nogojson.String("123"), Timestamp: nogojson.TimestampOrNow{Value: 0}},
				}
				options := nogojson.ImportMultiOptions{Rescan: true}
				return nogojson.NewImportMultiCmd(requests, &options)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"desc":"123","timestamp":0}],{"rescan":true}],"id":1}`,
			unmarshalled: &nogojson.ImportMultiCmd{
				Requests: []nogojson.ImportMultiRequest{
					{
						Descriptor: nogojson.String("123"),
						Timestamp:  nogojson.TimestampOrNow{Value: 0},
					},
				},
				Options: &nogojson.ImportMultiOptions{Rescan: true},
			},
		},
		{
			name: "importmulti with descriptor + no options",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp.
					[]nogojson.ImportMultiRequest{
						{
							Descriptor: nogojson.String("123"),
							Timestamp:  nogojson.TimestampOrNow{Value: 0},
							WatchOnly:  nogojson.Bool(false),
							Internal:   nogojson.Bool(true),
							Label:      nogojson.String("aaa"),
							KeyPool:    nogojson.Bool(false),
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []nogojson.ImportMultiRequest{
					{
						Descriptor: nogojson.String("123"),
						Timestamp:  nogojson.TimestampOrNow{Value: 0},
						WatchOnly:  nogojson.Bool(false),
						Internal:   nogojson.Bool(true),
						Label:      nogojson.String("aaa"),
						KeyPool:    nogojson.Bool(false),
					},
				}
				return nogojson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"desc":"123","timestamp":0,"internal":true,"watchonly":false,"label":"aaa","keypool":false}]],"id":1}`,
			unmarshalled: &nogojson.ImportMultiCmd{
				Requests: []nogojson.ImportMultiRequest{
					{
						Descriptor: nogojson.String("123"),
						Timestamp:  nogojson.TimestampOrNow{Value: 0},
						WatchOnly:  nogojson.Bool(false),
						Internal:   nogojson.Bool(true),
						Label:      nogojson.String("aaa"),
						KeyPool:    nogojson.Bool(false),
					},
				},
			},
		},
		{
			name: "importmulti with descriptor + string timestamp",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp.
					[]nogojson.ImportMultiRequest{
						{
							Descriptor: nogojson.String("123"),
							Timestamp:  nogojson.TimestampOrNow{Value: "now"},
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []nogojson.ImportMultiRequest{
					{Descriptor: nogojson.String("123"), Timestamp: nogojson.TimestampOrNow{Value: "now"}},
				}
				return nogojson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"desc":"123","timestamp":"now"}]],"id":1}`,
			unmarshalled: &nogojson.ImportMultiCmd{
				Requests: []nogojson.ImportMultiRequest{
					{Descriptor: nogojson.String("123"), Timestamp: nogojson.TimestampOrNow{Value: "now"}},
				},
			},
		},
		{
			name: "importmulti with scriptPubKey script",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp and scriptPubKey
					[]nogojson.ImportMultiRequest{
						{
							ScriptPubKey: &nogojson.ScriptPubKey{Value: "script"},
							RedeemScript: nogojson.String("123"),
							Timestamp:    nogojson.TimestampOrNow{Value: 0},
							PubKeys:      &[]string{"aaa"},
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []nogojson.ImportMultiRequest{
					{
						ScriptPubKey: &nogojson.ScriptPubKey{Value: "script"},
						RedeemScript: nogojson.String("123"),
						Timestamp:    nogojson.TimestampOrNow{Value: 0},
						PubKeys:      &[]string{"aaa"},
					},
				}
				return nogojson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"scriptPubKey":"script","timestamp":0,"redeemscript":"123","pubkeys":["aaa"]}]],"id":1}`,
			unmarshalled: &nogojson.ImportMultiCmd{
				Requests: []nogojson.ImportMultiRequest{
					{
						ScriptPubKey: &nogojson.ScriptPubKey{Value: "script"},
						RedeemScript: nogojson.String("123"),
						Timestamp:    nogojson.TimestampOrNow{Value: 0},
						PubKeys:      &[]string{"aaa"},
					},
				},
			},
		},
		{
			name: "importmulti with scriptPubKey address",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp and scriptPubKey
					[]nogojson.ImportMultiRequest{
						{
							ScriptPubKey:  &nogojson.ScriptPubKey{Value: nogojson.ScriptPubKeyAddress{Address: "addr"}},
							WitnessScript: nogojson.String("123"),
							Timestamp:     nogojson.TimestampOrNow{Value: 0},
							Keys:          &[]string{"aaa"},
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []nogojson.ImportMultiRequest{
					{
						ScriptPubKey:  &nogojson.ScriptPubKey{Value: nogojson.ScriptPubKeyAddress{Address: "addr"}},
						WitnessScript: nogojson.String("123"),
						Timestamp:     nogojson.TimestampOrNow{Value: 0},
						Keys:          &[]string{"aaa"},
					},
				}
				return nogojson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"scriptPubKey":{"address":"addr"},"timestamp":0,"witnessscript":"123","keys":["aaa"]}]],"id":1}`,
			unmarshalled: &nogojson.ImportMultiCmd{
				Requests: []nogojson.ImportMultiRequest{
					{
						ScriptPubKey:  &nogojson.ScriptPubKey{Value: nogojson.ScriptPubKeyAddress{Address: "addr"}},
						WitnessScript: nogojson.String("123"),
						Timestamp:     nogojson.TimestampOrNow{Value: 0},
						Keys:          &[]string{"aaa"},
					},
				},
			},
		},
		{
			name: "importmulti with ranged (int) descriptor",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp.
					[]nogojson.ImportMultiRequest{
						{
							Descriptor: nogojson.String("123"),
							Timestamp:  nogojson.TimestampOrNow{Value: 0},
							Range:      &nogojson.DescriptorRange{Value: 7},
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []nogojson.ImportMultiRequest{
					{
						Descriptor: nogojson.String("123"),
						Timestamp:  nogojson.TimestampOrNow{Value: 0},
						Range:      &nogojson.DescriptorRange{Value: 7},
					},
				}
				return nogojson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"desc":"123","timestamp":0,"range":7}]],"id":1}`,
			unmarshalled: &nogojson.ImportMultiCmd{
				Requests: []nogojson.ImportMultiRequest{
					{
						Descriptor: nogojson.String("123"),
						Timestamp:  nogojson.TimestampOrNow{Value: 0},
						Range:      &nogojson.DescriptorRange{Value: 7},
					},
				},
			},
		},
		{
			name: "importmulti with ranged (slice) descriptor",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp.
					[]nogojson.ImportMultiRequest{
						{
							Descriptor: nogojson.String("123"),
							Timestamp:  nogojson.TimestampOrNow{Value: 0},
							Range:      &nogojson.DescriptorRange{Value: []int{1, 7}},
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []nogojson.ImportMultiRequest{
					{
						Descriptor: nogojson.String("123"),
						Timestamp:  nogojson.TimestampOrNow{Value: 0},
						Range:      &nogojson.DescriptorRange{Value: []int{1, 7}},
					},
				}
				return nogojson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"desc":"123","timestamp":0,"range":[1,7]}]],"id":1}`,
			unmarshalled: &nogojson.ImportMultiCmd{
				Requests: []nogojson.ImportMultiRequest{
					{
						Descriptor: nogojson.String("123"),
						Timestamp:  nogojson.TimestampOrNow{Value: 0},
						Range:      &nogojson.DescriptorRange{Value: []int{1, 7}},
					},
				},
			},
		},
		{
			name: "walletcreatefundedpsbt",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"walletcreatefundedpsbt",
					[]nogojson.PsbtInput{
						{
							Txid:     "1234",
							Vout:     0,
							Sequence: 0,
						},
					},
					[]nogojson.PsbtOutput{
						nogojson.NewPsbtOutput("1234", nogoutil.Amount(1234)),
						nogojson.NewPsbtDataOutput([]byte{1, 2, 3, 4}),
					},
					nogojson.Uint32(1),
					nogojson.WalletCreateFundedPsbtOpts{},
					nogojson.Bool(true),
				)
			},
			staticCmd: func() interface{} {
				return nogojson.NewWalletCreateFundedPsbtCmd(
					[]nogojson.PsbtInput{
						{
							Txid:     "1234",
							Vout:     0,
							Sequence: 0,
						},
					},
					[]nogojson.PsbtOutput{
						nogojson.NewPsbtOutput("1234", nogoutil.Amount(1234)),
						nogojson.NewPsbtDataOutput([]byte{1, 2, 3, 4}),
					},
					nogojson.Uint32(1),
					&nogojson.WalletCreateFundedPsbtOpts{},
					nogojson.Bool(true),
				)
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletcreatefundedpsbt","params":[[{"txid":"1234","vout":0,"sequence":0}],[{"1234":0.00001234},{"data":"01020304"}],1,{},true],"id":1}`,
			unmarshalled: &nogojson.WalletCreateFundedPsbtCmd{
				Inputs: []nogojson.PsbtInput{
					{
						Txid:     "1234",
						Vout:     0,
						Sequence: 0,
					},
				},
				Outputs: []nogojson.PsbtOutput{
					nogojson.NewPsbtOutput("1234", nogoutil.Amount(1234)),
					nogojson.NewPsbtDataOutput([]byte{1, 2, 3, 4}),
				},
				Locktime:    nogojson.Uint32(1),
				Options:     &nogojson.WalletCreateFundedPsbtOpts{},
				Bip32Derivs: nogojson.Bool(true),
			},
		},
		{
			name: "walletprocesspsbt",
			newCmd: func() (interface{}, error) {
				return nogojson.NewCmd(
					"walletprocesspsbt", "1234", nogojson.Bool(true), nogojson.String("ALL"), nogojson.Bool(true))
			},
			staticCmd: func() interface{} {
				return nogojson.NewWalletProcessPsbtCmd(
					"1234", nogojson.Bool(true), nogojson.String("ALL"), nogojson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletprocesspsbt","params":["1234",true,"ALL",true],"id":1}`,
			unmarshalled: &nogojson.WalletProcessPsbtCmd{
				Psbt:        "1234",
				Sign:        nogojson.Bool(true),
				SighashType: nogojson.String("ALL"),
				Bip32Derivs: nogojson.Bool(true),
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
