// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/nogochain/nogocommons/chaincfg"
	"github.com/nogochain/nogocommons/wire"
)

// params is used to group parameters for various networks such as the main
// network and test networks.
type params struct {
	*chaincfg.Params
	rpcPort string
}

// mainNetParams contains parameters specific to the main network.
var mainNetParams = params{
	Params:  &chaincfg.MainNetParams,
	rpcPort: "19445",
}

// testNet3Params contains parameters specific to the test network (version 3).
var testNet3Params = params{
	Params:  &chaincfg.TestNet3Params,
	rpcPort: "19556",
}

// simNetParams contains parameters specific to the simulation test network.
var simNetParams = params{
	Params:  &chaincfg.SimNetParams,
	rpcPort: "19556",
}

// regressionNetParams contains parameters specific to the regression test
// network.
var regressionNetParams = params{
	Params:  &chaincfg.RegressionNetParams,
	rpcPort: "19666",
}

// sigNetParams contains parameters specific to the Signet network.
var sigNetParams = params{
	Params:  &chaincfg.SigNetParams,
	rpcPort: "19557",
}

// netName returns the name used when referring to a NogoCore network. Test
// networks may use a different directory name than their chaincfg.Name field.
func netName(chainParams *params) string {
	switch chainParams.Net {
	case wire.TestNet3:
		return "testnet"
	default:
		return chainParams.Name
	}
}
