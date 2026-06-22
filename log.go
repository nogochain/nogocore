// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/btcsuite/btclog"
	"github.com/nogochain/nogocommons/addrmgr"
	"github.com/nogochain/nogocommons/connmgr"
	"github.com/nogochain/nogocommons/database"
	"github.com/nogochain/nogocommons/peer"
	"github.com/nogochain/nogocommons/v2transport"
	"github.com/nogochain/nogocore/blockchain"
	"github.com/nogochain/nogocore/blockchain/indexers"
	"github.com/nogochain/nogocore/mempool"
	"github.com/nogochain/nogocore/mining"
	"github.com/nogochain/nogocore/netsync"
)

// loggers for each subsystem.
var (
	backendLog = btclog.NewBackend(os.Stdout)
	adxrLog    = backendLog.Logger("ADXR")
	amgrLog    = backendLog.Logger("AMGR")
	cmgrLog    = backendLog.Logger("CMGR")
	bcdbLog    = backendLog.Logger("BCDB")
	btcdLog    = backendLog.Logger("NogoCore")
	chanLog    = backendLog.Logger("CHAN")
	discLog    = backendLog.Logger("DISC")
	indxLog    = backendLog.Logger("INDX")
	minrLog    = backendLog.Logger("MINR")
	peerLog    = backendLog.Logger("PEER")
	rpcsLog    = backendLog.Logger("RPCS")
	srvrLog    = backendLog.Logger("SRVR")
	syncLog    = backendLog.Logger("SYNC")
	txmpLog    = backendLog.Logger("TXMP")
	v2trLog    = backendLog.Logger(v2transport.Subsystem)
)

// Initialize package-global logger variables.
func init() {
	addrmgr.UseLogger(amgrLog)
	connmgr.UseLogger(cmgrLog)
	database.UseLogger(bcdbLog)
	blockchain.UseLogger(chanLog)
	indexers.UseLogger(indxLog)
	mining.UseLogger(minrLog)
	peer.UseLogger(peerLog)
	netsync.UseLogger(syncLog)
	mempool.UseLogger(txmpLog)
	v2transport.UseLogger(v2trLog)
}

// subsystemLoggers maps each subsystem identifier to its logger.
var subsystemLoggers = map[string]btclog.Logger{
	"ADXR":                adxrLog,
	"AMGR":                amgrLog,
	"CMGR":                cmgrLog,
	"BCDB":                bcdbLog,
	"CHAN":                chanLog,
	"DISC":                discLog,
	"INDX":                indxLog,
	"MINR":                minrLog,
	"PEER":                peerLog,
	"RPCS":                rpcsLog,
	"SRVR":                srvrLog,
	"SYNC":                syncLog,
	"TXMP":                txmpLog,
	"MAIN":                btcdLog,
}

// setLogLevel sets the logging level for provided subsystem. Invalid
// subsystems are ignored.
func setLogLevel(subsystemID string, logLevel string) {
	logger, ok := subsystemLoggers[subsystemID]
	if !ok {
		return
	}

	level, ok := btclog.LevelFromString(logLevel)
	if !ok {
		return
	}

	logger.SetLevel(level)
}

// setLogLevels sets the log level for all subsystem loggers to the passed
// level.
func setLogLevels(logLevel string) {
	for subsystemID := range subsystemLoggers {
		setLogLevel(subsystemID, logLevel)
	}
}

// directionString is a helper that returns a string that represents the
// direction of a connection (inbound or outbound).
func directionString(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}
