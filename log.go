// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/btcsuite/btclog"
)

// loggers for each subsystem.
var (
	backendLog = btclog.NewBackend(os.Stdout)
	peerLog    = backendLog.Logger("PEER")
	srvrLog    = backendLog.Logger("SRVR")
	rpcsLog    = backendLog.Logger("RPCS")
	indxLog    = backendLog.Logger("INDX")
	btcdLog    = backendLog.Logger("NogoCore")
)

// subsystemLoggers maps each subsystem identifier to its logger.
var subsystemLoggers = map[string]btclog.Logger{
	"PEER": peerLog,
	"SRVR": srvrLog,
	"RPCS": rpcsLog,
	"INDX": indxLog,
	"MAIN": btcdLog,
}

// setLogLevel sets the logging level for provided subsystem. Invalid
// subsystems are ignored.
func setLogLevel(subsystemID string, logLevel string) {
	// Ignore invalid subsystems.
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
