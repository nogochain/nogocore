// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/lru"
	"github.com/nogochain/nogocommons/addrmgr"
	"github.com/nogochain/nogocommons/chaincfg"
	"github.com/nogochain/nogocommons/chainhash"
	"github.com/nogochain/nogocommons/connmgr"
	"github.com/nogochain/nogocommons/database"
	"github.com/nogochain/nogocommons/nogoutil"
	"github.com/nogochain/nogocommons/nogoutil/bloom"
	"github.com/nogochain/nogocommons/peer"
	"github.com/nogochain/nogocommons/wire"
	"github.com/nogochain/nogocore/blockchain"
	"github.com/nogochain/nogocore/blockchain/indexers"
	"github.com/nogochain/nogocore/mempool"
	"github.com/nogochain/nogocore/mining"
	"github.com/nogochain/nogocore/netsync"
)

const (
	// defaultServices describes the default services that are supported by
	// the server.
	defaultServices = wire.SFNodeNetwork | wire.SFNodeNetworkLimited |
		wire.SFNodeBloom | wire.SFNodeWitness | wire.SFNodeCF | wire.SFNodeP2PV2

	// defaultRequiredServices describes the default services that are
	// required to be supported by outbound peers.
	defaultRequiredServices = wire.SFNodeNetwork

	// defaultTargetOutbound is the default number of outbound peers to target.
	defaultTargetOutbound = 8

	// connectionRetryInterval is the base amount of time to wait in between
	// retries when connecting to persistent peers. It is adjusted by the
	// number of retries such that there is a retry backoff.
	connectionRetryInterval = time.Second * 5
)

var (
	// userAgentName is the user agent name and is used to help identify
	// ourselves to other NogoCore peers.
	userAgentName = "nogocore"

	// userAgentVersion is the user agent version and is used to help
	// identify ourselves to other NogoCore peers.
	userAgentVersion = fmt.Sprintf("%d.%d.%d", appMajor, appMinor, appPatch)
)

// zeroHash is the zero value hash (all zeros). It is defined as a convenience.
var zeroHash chainhash.Hash

// onionAddr implements the net.Addr interface and represents a tor address.
type onionAddr struct {
	addr string
}

// String returns the onion address.
func (oa *onionAddr) String() string {
	return oa.addr
}

// Network returns "onion".
func (oa *onionAddr) Network() string {
	return "onion"
}

// Ensure onionAddr implements the net.Addr interface.
var _ net.Addr = (*onionAddr)(nil)

// simpleAddr implements the net.Addr interface with two struct fields.
type simpleAddr struct {
	net, addr string
}

// String returns the address.
func (a simpleAddr) String() string {
	return a.addr
}

// Network returns the network.
func (a simpleAddr) Network() string {
	return a.net
}

// Ensure simpleAddr implements the net.Addr interface.
var _ net.Addr = simpleAddr{}

// broadcastMsg provides the ability to house a message to be broadcast
// to all connected peers except specified excluded peers.
type broadcastMsg struct {
	message      wire.Message
	excludePeers []*serverPeer
}

// broadcastInventoryAdd is a type used to declare that the InvVect it contains
// needs to be added to the rebroadcast map.
type broadcastInventoryAdd relayMsg

// broadcastInventoryDel is a type used to declare that the InvVect it contains
// needs to be removed from the rebroadcast map.
type broadcastInventoryDel *wire.InvVect

// relayMsg packages an inventory vector along with the newly discovered
// inventory so the relay has access to that information.
type relayMsg struct {
	invVect *wire.InvVect
	data    interface{}
}

// updatePeerHeightsMsg is a message sent from the blockmanager to the server
// after a new block has been accepted. The purpose of the message is to update
// the heights of peers that were known to announce the block before we
// connected it to the main chain or recognized it as an orphan.
type updatePeerHeightsMsg struct {
	newHash    *chainhash.Hash
	newHeight  int32
	originPeer *peer.Peer
}

// peerLifecycleAction describes the type of peer lifecycle event.
type peerLifecycleAction uint8

const (
	peerAdd peerLifecycleAction = iota
	peerDone
)

// peerLifecycleEvent represents a peer connection or disconnection event.
type peerLifecycleEvent struct {
	action peerLifecycleAction
	sp     *serverPeer
}

// peerState maintains state of inbound, persistent, outbound peers as well
// as banned peers and outbound groups.
type peerState struct {
	inboundPeers    map[int32]*serverPeer
	outboundPeers   map[int32]*serverPeer
	persistentPeers map[int32]*serverPeer
	banned          map[string]time.Time
	outboundGroups  map[string]int
}

// Count returns the count of all known peers.
func (ps *peerState) Count() int {
	return len(ps.inboundPeers) + len(ps.outboundPeers) +
		len(ps.persistentPeers)
}

// forAllOutboundPeers is a helper function that runs closure on all outbound
// peers known to peerState.
func (ps *peerState) forAllOutboundPeers(closure func(sp *serverPeer)) {
	for _, e := range ps.outboundPeers {
		closure(e)
	}
	for _, e := range ps.persistentPeers {
		closure(e)
	}
}

// forAllPeers is a helper function that runs closure on all peers known to
// peerState.
func (ps *peerState) forAllPeers(closure func(sp *serverPeer)) {
	for _, e := range ps.inboundPeers {
		closure(e)
	}
	ps.forAllOutboundPeers(closure)
}

// cfHeaderKV is a tuple of a filter header and its associated block hash.
type cfHeaderKV struct {
	blockHash    chainhash.Hash
	filterHeader chainhash.Hash
}

// server provides a NogoCore server for handling communications to and from
// NogoCore peers.
type server struct {
	// The following variables must only be used atomically.
	// Putting the uint64s first makes them 64-bit aligned for 32-bit systems.
	bytesReceived uint64 // Total bytes received from all peers since start.
	bytesSent     uint64 // Total bytes sent by all peers since start.
	started       int32
	shutdown      int32
	shutdownSched int32
	startupTime   int64

	chainParams          *chaincfg.Params
	addrManager          *addrmgr.AddrManager
	connManager          *connmgr.ConnManager
	// rpcServer is set during RPC server initialization.
	syncManager          *netsync.SyncManager
	chain                *blockchain.BlockChain
	txMemPool            *mempool.TxPool
	modifyRebroadcastInv chan interface{}
	p2pDowngrader        *peer.P2PDowngrader
	peerLifecycle        chan peerLifecycleEvent
	banPeers             chan *serverPeer
	query                chan interface{}
	relayInv             chan relayMsg
	broadcast            chan broadcastMsg
	peerHeightsUpdate    chan updatePeerHeightsMsg
	wg                   sync.WaitGroup
	quit                 chan struct{}
	nat                  NAT
	db                   database.DB
	timeSource           blockchain.MedianTimeSource
	services             wire.ServiceFlag

	// Optional indexes. They will be nil if the associated index is not
	// enabled. These fields are set during initial creation of the server
	// and never changed afterwards, so they do not need to be protected
	// for concurrent access.
	txIndex   *indexers.TxIndex
	addrIndex *indexers.AddrIndex
	cfIndex   *indexers.CfIndex

	// The fee estimator keeps track of how long transactions are left in
	// the mempool before they are mined into blocks.
	feeEstimator *mempool.FeeEstimator

	// cfCheckptCaches stores a cached slice of filter headers for cfcheckpt
	// messages for each filter type.
	cfCheckptCaches    map[wire.FilterType][]cfHeaderKV
	cfCheckptCachesMtx sync.RWMutex

	// agentBlacklist is a list of blacklisted substrings by which to filter
	// user agents.
	agentBlacklist []string

	// agentWhitelist is a list of whitelisted user agent substrings.
	agentWhitelist []string
}

// serverPeer extends the peer to maintain state shared by the server.
type serverPeer struct {
	// The following variables must only be used atomically.
	feeFilter int64

	*peer.Peer

	connReq       *connmgr.ConnReq
	server        *server
	persistent    bool
	continueHash  *chainhash.Hash
	relayMtx      sync.Mutex
	disableRelayTx bool
	sentAddrs     bool
	isWhitelisted bool
	filter        *bloom.Filter
	addressesMtx  sync.RWMutex
	knownAddresses lru.Cache
	banScore      connmgr.DynamicBanScore
	quit          chan struct{}

	// Closed by verAckOnce when OnVerAck fires.
	verAckCh   chan struct{}
	verAckOnce sync.Once

	// peerAdded is set by peerLifecycleHandler after a peerAdd event
	// has been enqueued on s.peerLifecycle.
	peerAdded atomic.Bool

	// The following chans are used to sync blockmanager and server.
	txProcessed    chan struct{}
	blockProcessed chan struct{}
}

// newServerPeer returns a new serverPeer instance. The peer needs to be set by
// the caller.
func newServerPeer(s *server, isPersistent bool) *serverPeer {
	return &serverPeer{
		server:         s,
		persistent:     isPersistent,
		filter:         bloom.LoadFilter(nil),
		knownAddresses: lru.NewCache(5000),
		quit:           make(chan struct{}),
		txProcessed:    make(chan struct{}, 1),
		blockProcessed: make(chan struct{}, 1),
		verAckCh:       make(chan struct{}),
	}
}

// addKnownAddresses adds the given addresses to the set of known addresses for
// the peer to prevent sending duplicate addresses.
func (sp *serverPeer) addKnownAddresses(addresses []*wire.NetAddressV2) {
	sp.addressesMtx.Lock()
	defer sp.addressesMtx.Unlock()
	for _, na := range addresses {
		sp.knownAddresses.Add(na.Addr.String())
	}
}

// addressKnown true if the given address is already known to the peer.
func (sp *serverPeer) addressKnown(na *wire.NetAddressV2) bool {
	sp.addressesMtx.RLock()
	defer sp.addressesMtx.RUnlock()
	return sp.knownAddresses.Contains(na.Addr.String())
}

// relayTxDisabled returns whether or not relaying of transactions is disabled
// for the peer.
func (sp *serverPeer) relayTxDisabled() bool {
	sp.relayMtx.Lock()
	defer sp.relayMtx.Unlock()
	return sp.disableRelayTx
}

// setDisableRelayTx sets the relaying of transactions on or off for the peer.
func (sp *serverPeer) setDisableRelayTx(disable bool) {
	sp.relayMtx.Lock()
	defer sp.relayMtx.Unlock()
	sp.disableRelayTx = disable
}

// pushAddrMsg sends an addrv2 message to the connected peer using the
// provided addresses.
func (sp *serverPeer) pushAddrMsg(addresses []*wire.NetAddressV2) {
	// Nothing to send.
	if len(addresses) == 0 {
		return
	}

	addrsLen := len(addresses)

	// If we have more addresses than the max allowed, then use a shuffled
	// subset.
	if addrsLen > wire.MaxV2AddrPerMsg {
		shuffleAddrsV2(addresses)
		addresses = addresses[:wire.MaxV2AddrPerMsg]
	}

	// Send the addrv2 message.
	addrv2 := wire.NewMsgAddrV2()
	addrv2.AddrList = addresses
	sp.QueueMessage(addrv2, nil)
}

// addBanScore increases the persistent and decaying ban score fields by the
// values passed as parameters. If the resulting score exceeds half of the ban
// threshold, a warning is logged. If the score is above the ban threshold,
// the peer will be banned and disconnected.
func (sp *serverPeer) addBanScore(persistent, transient uint32, reason string) bool {
	// No warning is logged and no score is calculated if banning is disabled.
	if cfg.DisableBanning {
		return false
	}
	if sp.isWhitelisted {
		peerLog.Debugf("Misbehaving whitelisted peer %s: %s", sp, reason)
		return false
	}

	warnThreshold := cfg.BanThreshold >> 1
	if transient == 0 && persistent == 0 {
		score := sp.banScore.Int()
		if score > warnThreshold {
			peerLog.Warnf("Misbehaving peer %s: %s -- ban score is %d, "+
				"it was not increased this time", sp, reason, score)
		}
		return false
	}
	score := sp.banScore.Increase(persistent, transient)
	if score > warnThreshold {
		peerLog.Warnf("Misbehaving peer %s: %s -- ban score increased to %d",
			sp, reason, score)
		if score > cfg.BanThreshold {
			peerLog.Warnf("Misbehaving peer %s -- banning and disconnecting",
				sp)
			sp.server.BanPeer(sp)
			sp.Disconnect()
			return true
		}
	}
	return false
}

// hasServices returns whether or not the provided advertised service flags have
// all of the provided desired service flags set.
func hasServices(advertised, desired wire.ServiceFlag) bool {
	return advertised&desired == desired
}

// OnVersion is invoked when a peer receives a version message.
func (sp *serverPeer) OnVersion(_ *peer.Peer, msg *wire.MsgVersion) *wire.MsgReject {
	// Update the address manager with the advertised services for outbound
	// connections in case they have changed. This is not done for inbound
	// connections to help prevent malicious behavior.
	isInbound := sp.Inbound()
	remoteAddr := sp.NA()
	addrManager := sp.server.addrManager
	if activeNetParams.Name != "simnet" && !isInbound {
		addrManager.SetServices(remoteAddr, msg.Services)
	}

	// Ignore peers that have a protocol version that is too old.
	if msg.ProtocolVersion < int32(peer.MinAcceptableProtocolVersion) {
		return nil
	}

	// Reject outbound peers that are not full nodes.
	wantServices := wire.SFNodeNetwork
	if !isInbound && !hasServices(msg.Services, wantServices) {
		missingServices := wantServices & ^msg.Services
		srvrLog.Debugf("Rejecting peer %s with services %v due to not "+
			"providing desired services %v", sp.Peer, msg.Services,
			missingServices)
		reason := fmt.Sprintf("required services %#x not offered",
			uint64(missingServices))
		return wire.NewMsgReject(msg.Command(), wire.RejectNonstandard, reason)
	}

	// Add the remote peer time as a sample for time offset calculation.
	sp.server.timeSource.AddTimeSample(sp.Addr(), msg.Timestamp)

	// Choose whether or not to relay transactions before a filter command
	// is received.
	sp.setDisableRelayTx(msg.DisableRelayTx)

	return nil
}

// OnVerAck is invoked when a peer receives a verack message.
func (sp *serverPeer) OnVerAck(_ *peer.Peer, _ *wire.MsgVerAck) {
	sp.verAckOnce.Do(func() { close(sp.verAckCh) })
}

// OnMemPool is invoked when a peer receives a mempool message.
func (sp *serverPeer) OnMemPool(_ *peer.Peer, msg *wire.MsgMemPool) {
	// Only allow mempool requests if the server has bloom filtering enabled.
	if sp.server.services&wire.SFNodeBloom != wire.SFNodeBloom {
		peerLog.Debugf("peer %v sent mempool request with bloom "+
			"filtering disabled -- disconnecting", sp)
		sp.Disconnect()
		return
	}

	if sp.addBanScore(0, 33, "mempool") {
		return
	}

	// Generate inventory message with the available transactions in the
	// transaction memory pool. Limit it to the max allowed inventory per
	// message. The NewMsgInvSizeHint function automatically limits the
	// passed hint to the maximum allowed, so it's safe to pass the total
	// count.
	txDescs := sp.server.txMemPool.TxDescs()
	invMsg := wire.NewMsgInvSizeHint(uint(len(txDescs)))

	// If the peer has a bloom filter loaded, only relay transactions that
	// match it.
	for _, txDesc := range txDescs {
		if sp.filter.IsLoaded() {
			if !sp.filter.MatchTxAndUpdate(txDesc.Tx) {
				continue
			}
		}

		iv := wire.NewInvVect(wire.InvTypeTx, txDesc.Tx.Hash())
		if err := invMsg.AddInvVect(iv); err != nil {
			break
		}
	}

	if len(invMsg.InvList) > 0 {
		sp.QueueMessage(invMsg, nil)
	}
}

// OnTx is invoked when a peer receives a tx message.
func (sp *serverPeer) OnTx(_ *peer.Peer, msg *wire.MsgTx) {
	if cfg.BlocksOnly {
		txHash := msg.TxHash()
		peerLog.Tracef("Ignoring tx %v from %v -- blocksonly enabled",
			txHash, sp)
		return
	}

	// Add the transaction to the known inventory for the peer.
	txHash := msg.TxHash()
	iv := wire.NewInvVect(wire.InvTypeTx, &txHash)
	sp.AddKnownInventory(iv)

	// Wrap the wire transaction in a btcutil.Tx for mempool processing.
	tx := nogoutil.NewTx(msg)

	// Process the transaction through the memory pool.
	acceptedTxs, err := sp.server.txMemPool.ProcessTransaction(
		tx, true, true, mempool.Tag(sp.ID()),
	)

	if err != nil {
		var txRuleErr mempool.TxRuleError
		if errors.As(err, &txRuleErr) {
			peerLog.Debugf("Rejected tx %v from %v: %v", txHash,
				sp, err)
		} else {
			peerLog.Debugf("Failed to process tx %v: %v", txHash, err)
		}
		sp.addBanScore(0, 1, "rejected tx")
		return
	}

	// When accepted, notify the sync manager via a done channel for the
	// first accepted transaction.
	if len(acceptedTxs) > 0 && acceptedTxs[0] != nil {
		sp.server.syncManager.QueueTx(acceptedTxs[0].Tx, sp.Peer, sp.txProcessed)
	}
}

// OnBlock is invoked when a peer receives a block message.
func (sp *serverPeer) OnBlock(_ *peer.Peer, msg *wire.MsgBlock, buf []byte) {
	// Convert the raw MsgBlock to a btcutil.Block which includes some
	// additional information such as whether or not it's part of the main
	// chain.
	block := nogoutil.NewBlockFromBlockAndBytes(msg, buf)

	// Add the block to the known inventory for the peer.
	hash := block.Hash()
	iv := wire.NewInvVect(wire.InvTypeBlock, hash)
	sp.AddKnownInventory(iv)

	// Queue the block up to be handled by the sync manager.
	sp.server.syncManager.QueueBlock(block, sp.Peer, sp.blockProcessed)
}

// OnInv is invoked when a peer receives an inv message.
func (sp *serverPeer) OnInv(_ *peer.Peer, msg *wire.MsgInv) {
	if len(msg.InvList) > 0 {
		// If the peer has bloom filtering disabled, don't allow them
		// to send inventory to prevent wasting bandwidth.
		if sp.server.services&wire.SFNodeBloom != wire.SFNodeBloom {
			peerLog.Debugf("peer %v sent inv with bloom filtering "+
				"disabled -- disconnecting", sp)
			sp.Disconnect()
			return
		}
		sp.server.syncManager.QueueInv(msg, sp.Peer)
	}
}

// OnHeaders is invoked when a peer receives a headers message.
func (sp *serverPeer) OnHeaders(_ *peer.Peer, msg *wire.MsgHeaders) {
	sp.server.syncManager.QueueHeaders(msg, sp.Peer)
}

// OnGetData is invoked when a peer receives a getdata message.
func (sp *serverPeer) OnGetData(_ *peer.Peer, msg *wire.MsgGetData) {
	failedMsg := wire.NewMsgNotFound()

	length := len(msg.InvList)
	if sp.addBanScore(0, uint32(length)*99/wire.MaxInvPerMsg, "getdata") {
		return
	}

	const numBuffered = 5
	doneChans := make([]chan struct{}, 0, numBuffered)

	for i, iv := range msg.InvList {
		doneChan := make(chan struct{}, 1)
		doneChans = append(doneChans, doneChan)

		err := sp.server.pushInventory(sp, iv, doneChan)
		if err != nil {
			_ = failedMsg.AddInvVect(iv)
		}

		if (i+1)%numBuffered != 0 {
			continue
		}

		// Empty all the slots.
		for _, dc := range doneChans {
			select {
			case <-dc:
			case <-sp.quit:
				peerLog.Debug("Server shutting down in OnGetData")
				return
			}
		}

		doneChans = make([]chan struct{}, 0, numBuffered)
	}

	if len(failedMsg.InvList) != 0 {
		doneChan := make(chan struct{}, 1)
		doneChans = append(doneChans, doneChan)
		sp.QueueMessage(failedMsg, doneChan)
	}

	for _, dc := range doneChans {
		select {
		case <-dc:
		case <-sp.quit:
			peerLog.Debug("Server shutting down in OnGetData")
			return
		}
	}
}

// pushInventory sends the requested inventory to the given peer.
func (s *server) pushInventory(sp *serverPeer, iv *wire.InvVect,
	doneChan chan<- struct{}) error {

	switch iv.Type {
	case wire.InvTypeWitnessTx:
		return s.pushTxMsg(sp, &iv.Hash, doneChan, wire.WitnessEncoding)
	case wire.InvTypeTx:
		return s.pushTxMsg(sp, &iv.Hash, doneChan, wire.BaseEncoding)
	case wire.InvTypeWitnessBlock:
		return s.pushBlockMsg(sp, &iv.Hash, doneChan, wire.WitnessEncoding)
	case wire.InvTypeBlock:
		return s.pushBlockMsg(sp, &iv.Hash, doneChan, wire.BaseEncoding)
	case wire.InvTypeFilteredWitnessBlock:
		return s.pushMerkleBlockMsg(sp, &iv.Hash, doneChan, wire.WitnessEncoding)
	case wire.InvTypeFilteredBlock:
		return s.pushMerkleBlockMsg(sp, &iv.Hash, doneChan, wire.BaseEncoding)
	default:
		peerLog.Warnf("Unknown type in inventory request %d", iv.Type)
		if doneChan != nil {
			doneChan <- struct{}{}
		}
		return errors.New("unknown inventory type")
	}
}

// OnGetBlocks is invoked when a peer receives a getblocks message.
func (sp *serverPeer) OnGetBlocks(_ *peer.Peer, msg *wire.MsgGetBlocks) {
	chain := sp.server.chain
	hashList := chain.LocateBlocks(msg.BlockLocatorHashes, &msg.HashStop,
		wire.MaxBlocksPerMsg)

	invMsg := wire.NewMsgInv()
	for i := range hashList {
		iv := wire.NewInvVect(wire.InvTypeBlock, &hashList[i])
		invMsg.AddInvVect(iv)
	}

	if len(invMsg.InvList) > 0 {
		invListLen := len(invMsg.InvList)
		if invListLen == wire.MaxBlocksPerMsg {
			continueHash := invMsg.InvList[invListLen-1].Hash
			sp.continueHash = &continueHash
		}
		sp.QueueMessage(invMsg, nil)
	}
}

// OnGetHeaders is invoked when a peer receives a getheaders message.
func (sp *serverPeer) OnGetHeaders(_ *peer.Peer, msg *wire.MsgGetHeaders) {
	if !sp.server.syncManager.IsCurrent() {
		return
	}

	chain := sp.server.chain
	headers := chain.LocateHeaders(msg.BlockLocatorHashes, &msg.HashStop)

	blockHeaders := make([]*wire.BlockHeader, len(headers))
	for i := range headers {
		blockHeaders[i] = &headers[i]
	}
	sp.QueueMessage(&wire.MsgHeaders{Headers: blockHeaders}, nil)
}

// OnGetCFilters is invoked when a peer receives a getcfilters message.
func (sp *serverPeer) OnGetCFilters(_ *peer.Peer, msg *wire.MsgGetCFilters) {
	if !sp.server.syncManager.IsCurrent() {
		return
	}

	if msg.FilterType != wire.GCSFilterRegular {
		peerLog.Debug("Filter request for unknown filter: %v", msg.FilterType)
		return
	}

	hashes, err := sp.server.chain.HeightToHashRange(
		int32(msg.StartHeight), &msg.StopHash, wire.MaxGetCFiltersReqRange,
	)
	if err != nil {
		peerLog.Debugf("Invalid getcfilters request: %v", err)
		return
	}

	hashPtrs := make([]*chainhash.Hash, len(hashes))
	for i := range hashes {
		hashPtrs[i] = &hashes[i]
	}

	filters, err := sp.server.cfIndex.FiltersByBlockHashes(hashPtrs, msg.FilterType)
	if err != nil {
		peerLog.Errorf("Error retrieving cfilters: %v", err)
		return
	}

	for i, filterBytes := range filters {
		if len(filterBytes) == 0 {
			peerLog.Warnf("Could not obtain cfilter for %v", hashes[i])
			return
		}
		filterMsg := wire.NewMsgCFilter(msg.FilterType, &hashes[i], filterBytes)
		sp.QueueMessage(filterMsg, nil)
	}
}

// OnGetCFHeaders is invoked when a peer receives a getcfheader message.
func (sp *serverPeer) OnGetCFHeaders(_ *peer.Peer, msg *wire.MsgGetCFHeaders) {
	if !sp.server.syncManager.IsCurrent() {
		return
	}

	if msg.FilterType != wire.GCSFilterRegular {
		peerLog.Debug("Filter request for unknown headers for filter: %v", msg.FilterType)
		return
	}

	startHeight := int32(msg.StartHeight)
	maxResults := wire.MaxCFHeadersPerMsg
	if msg.StartHeight > 0 {
		startHeight--
		maxResults++
	}

	hashList, err := sp.server.chain.HeightToHashRange(
		startHeight, &msg.StopHash, maxResults,
	)
	if err != nil {
		peerLog.Debugf("Invalid getcfheaders request: %v", err)
	}

	if len(hashList) == 0 || (msg.StartHeight > 0 && len(hashList) == 1) {
		peerLog.Debug("No results for getcfheaders request")
		return
	}

	hashPtrs := make([]*chainhash.Hash, len(hashList))
	for i := range hashList {
		hashPtrs[i] = &hashList[i]
	}

	filterHashes, err := sp.server.cfIndex.FilterHashesByBlockHashes(hashPtrs, msg.FilterType)
	if err != nil {
		peerLog.Errorf("Error retrieving cfilter hashes: %v", err)
		return
	}

	headersMsg := wire.NewMsgCFHeaders()
	if msg.StartHeight > 0 {
		prevBlockHash := &hashList[0]
		headerBytes, err := sp.server.cfIndex.FilterHeaderByBlockHash(prevBlockHash, msg.FilterType)
		if err != nil {
			peerLog.Errorf("Error retrieving CF header: %v", err)
			return
		}
		if len(headerBytes) == 0 {
			peerLog.Warnf("Could not obtain CF header for %v", prevBlockHash)
			return
		}
		err = headersMsg.PrevFilterHeader.SetBytes(headerBytes)
		if err != nil {
			peerLog.Warnf("Committed filter header deserialize failed: %v", err)
			return
		}
		hashList = hashList[1:]
		filterHashes = filterHashes[1:]
	}

	for i, hashBytes := range filterHashes {
		if len(hashBytes) == 0 {
			peerLog.Warnf("Could not obtain CF hash for %v", hashList[i])
			return
		}
		filterHash, err := chainhash.NewHash(hashBytes)
		if err != nil {
			peerLog.Warnf("Committed filter hash deserialize failed: %v", err)
			return
		}
		headersMsg.AddCFHash(filterHash)
	}

	headersMsg.FilterType = msg.FilterType
	headersMsg.StopHash = msg.StopHash
	sp.QueueMessage(headersMsg, nil)
}

// OnGetCFCheckpt is invoked when a peer receives a getcfcheckpt message.
func (sp *serverPeer) OnGetCFCheckpt(_ *peer.Peer, msg *wire.MsgGetCFCheckpt) {
	if !sp.server.syncManager.IsCurrent() {
		return
	}

	if msg.FilterType != wire.GCSFilterRegular {
		peerLog.Debug("Filter request for unknown checkpoints for filter: %v", msg.FilterType)
		return
	}

	blockHashes, err := sp.server.chain.IntervalBlockHashes(
		&msg.StopHash, wire.CFCheckptInterval,
	)
	if err != nil {
		peerLog.Debugf("Invalid getcfcheckpt request: %v", err)
		return
	}

	checkptMsg := wire.NewMsgCFCheckpt(msg.FilterType, &msg.StopHash, len(blockHashes))

	sp.server.cfCheckptCachesMtx.RLock()
	checkptCache := sp.server.cfCheckptCaches[msg.FilterType]
	var updateCache bool
	if len(blockHashes) > len(checkptCache) {
		sp.server.cfCheckptCachesMtx.RUnlock()
		sp.server.cfCheckptCachesMtx.Lock()
		defer sp.server.cfCheckptCachesMtx.Unlock()
		checkptCache = sp.server.cfCheckptCaches[msg.FilterType]
		if len(blockHashes) > len(checkptCache) {
			updateCache = true
			additionalLength := len(blockHashes) - len(checkptCache)
			newEntries := make([]cfHeaderKV, additionalLength)
			peerLog.Infof("Growing size of checkpoint cache from %v to %v block hashes",
				len(checkptCache), len(blockHashes))
			checkptCache = append(sp.server.cfCheckptCaches[msg.FilterType], newEntries...)
		}
	} else {
		defer sp.server.cfCheckptCachesMtx.RUnlock()
	}

	var forkIdx int
	for forkIdx = len(blockHashes); forkIdx > 0; forkIdx-- {
		if checkptCache[forkIdx-1].blockHash == blockHashes[forkIdx-1] {
			break
		}
	}

	if updateCache || forkIdx < len(blockHashes) {
		for i := forkIdx; i < len(blockHashes); i++ {
			filter, err := sp.server.cfIndex.FilterByBlockHash(&blockHashes[i], msg.FilterType)
			if err != nil {
				peerLog.Warnf("Committed filter for %v not found: %v",
					blockHashes[i], err)
				return
			}
			headerBytes, err := sp.server.cfIndex.FilterHeaderByBlockHash(
				&blockHashes[i], msg.FilterType)
			if err != nil {
				peerLog.Warnf("Committed filter header for %v not found: %v",
					blockHashes[i], err)
				return
			}
			if len(headerBytes) == 0 {
				peerLog.Warnf("Could not obtain CF header for %v", blockHashes[i])
				return
			}
			filterHash, err := chainhash.NewHash(headerBytes)
			if err != nil {
				peerLog.Warnf("Error deserializing CF header: %v", err)
				return
			}
			checkptCache[i] = cfHeaderKV{
				blockHash:    blockHashes[i],
				filterHeader: *filterHash,
			}
			checkptMsg.AddCFHeader(filterHash)
			_ = filter
		}
		if updateCache {
			sp.server.cfCheckptCaches[msg.FilterType] = checkptCache
		}
	} else {
		for i := 0; i < len(blockHashes); i++ {
			checkptMsg.AddCFHeader(&checkptCache[i].filterHeader)
		}
	}

	sp.QueueMessage(checkptMsg, nil)
}

// OnAddrV2 is invoked when a peer receives an addrv2 message.
func (sp *serverPeer) OnAddrV2(_ *peer.Peer, msg *wire.MsgAddrV2) {
	if sp.addBanScore(0, 10, "addrv2") {
		return
	}

	for _, na := range msg.AddrList {
		// Skip addresses from the future.
		if na.Timestamp.After(time.Now().Add(time.Minute * 10)) {
			continue
		}

		// Add the address to the known addresses for the peer.
		sp.addKnownAddresses([]*wire.NetAddressV2{na})

		// Add the address to the address manager.
		sp.server.addrManager.AddAddress(na, na)
	}
}

// OnFeeFilter is invoked when a peer receives a feefilter message.
func (sp *serverPeer) OnFeeFilter(_ *peer.Peer, msg *wire.MsgFeeFilter) {
	atomic.StoreInt64(&sp.feeFilter, msg.MinFee)
}

// OnSendAddrV2 is invoked when a peer is ready to receive addrv2 messages.
func (sp *serverPeer) OnSendAddrV2(_ *peer.Peer, msg *wire.MsgSendAddrV2) {
	if sp.sentAddrs {
		peerLog.Debugf("Ignoring duplicate sendaddrv2 from %v", sp)
		return
	}
	sp.sentAddrs = true

	// Do not send addresses until the initial sync is complete.
	if !sp.server.syncManager.IsCurrent() {
		peerLog.Debugf("Ignoring sendaddrv2 from %v -- sync manager not current", sp)
		return
	}

	// Fetch addresses from the address manager and push them.
	addresses := sp.server.addrManager.AddressCache()
	sp.pushAddrMsg(addresses)
}

// pushTxMsg sends a transaction message for the provided transaction hash to
// the connected peer.
func (s *server) pushTxMsg(sp *serverPeer, hash *chainhash.Hash, doneChan chan<- struct{}, encoding wire.MessageEncoding) error {
	// Attempt to fetch the requested transaction from the pool.
	tx, err := s.txMemPool.FetchTransaction(hash)
	if err != nil {
		peerLog.Tracef("Unable to fetch tx %v from transaction pool: %v", hash, err)

		if doneChan != nil {
			doneChan <- struct{}{}
		}
		return err
	}

	// Create a new message to send the transaction.
	sp.QueueMessageWithEncoding(tx.MsgTx(), doneChan, encoding)
	return nil
}

// pushBlockMsg sends a block message for the provided block hash to the
// connected peer.
func (s *server) pushBlockMsg(sp *serverPeer, hash *chainhash.Hash, doneChan chan<- struct{}, encoding wire.MessageEncoding) error {
	blk, err := s.chain.BlockByHash(hash)
	if err != nil {
		peerLog.Tracef("Unable to fetch requested block %v: %v", hash, err)
		if doneChan != nil {
			doneChan <- struct{}{}
		}
		return err
	}

	// Queue the block message directly.
	sp.QueueMessageWithEncoding(blk.MsgBlock(), doneChan, encoding)
	return nil
}

// pushMerkleBlockMsg sends a merkleblock message for the provided block hash to
// the connected peer.
func (s *server) pushMerkleBlockMsg(sp *serverPeer, hash *chainhash.Hash, doneChan chan<- struct{}, encoding wire.MessageEncoding) error {
	// Do not send a response if the peer doesn't have a filter loaded.
	if !sp.filter.IsLoaded() {
		if doneChan != nil {
			doneChan <- struct{}{}
		}
		return nil
	}

	// Fetch the raw block bytes from the database.
	blk, err := s.chain.BlockByHash(hash)
	if err != nil {
		peerLog.Tracef("Unable to fetch requested block %v: %v", hash, err)
		if doneChan != nil {
			doneChan <- struct{}{}
		}
		return err
	}

	// Generate a merkle block by filtering the requested block according
	// to the filter for the peer.
	merkle, matchedTxIndices := bloom.NewMerkleBlock(blk, sp.filter)

	// Send the merkleblock.
	sp.QueueMessageWithEncoding(merkle, doneChan, encoding)

	// Finally, send any matched transactions.
	for _, txIndex := range matchedTxIndices {
		tx := blk.Transactions()[txIndex]
		sp.QueueMessageWithEncoding(tx.MsgTx(), nil, encoding)
	}

	return nil
}

// OnNotFound is invoked when a peer sends a notfound message.
func (sp *serverPeer) OnNotFound(p *peer.Peer, msg *wire.MsgNotFound) {
	// Ignored: sync manager handles notfound internally.
}

// announceBlock generates inventory vectors for the provided block and
// announces the new block to connected peers.
func (s *server) announceBlock(block *nogoutil.Block) {
	hash := block.Hash()
	iv := wire.NewInvVect(wire.InvTypeBlock, hash)
	s.RelayInventory(iv, block.MsgBlock())
}

// handleUpdatePeerHeights updates the heights of all peers who were known to
// announce a block that has recently been accepted to the main chain or
// recognized as an orphan.
func (s *server) handleUpdatePeerHeights(state *peerState, umsg updatePeerHeightsMsg) {
	state.forAllPeers(func(sp *serverPeer) {
		// The origin peer already has the updated height, so skip it.
		if sp.Peer == umsg.originPeer {
			return
		}

		// Only update peers known to have the announced block.
		if sp.LastBlock() != umsg.newHeight {
			return
		}

		// Update the peer to the latest height.
		sp.UpdateLastBlockHeight(umsg.newHeight)
	})

	// Update the heights map for sync peer candidacy purposes.
	if umsg.originPeer != nil {
		umsg.originPeer.UpdateLastBlockHeight(umsg.newHeight)
	}
}

// handleAddPeerMsg deals with adding new peers. It is invoked from the
// peerHandler goroutine.
func (s *server) handleAddPeerMsg(state *peerState, sp *serverPeer) {
	if sp == nil {
		return
	}

	// Ignore new peers if we're shutting down.
	if atomic.LoadInt32(&s.shutdown) != 0 {
		srvrLog.Infof("New peer %s ignored -- server is shutting down", sp)
		sp.Disconnect()
		return
	}

	// Disconnect banned peers.
	host, _, err := net.SplitHostPort(sp.Addr())
	if err != nil {
		srvrLog.Debugf("can't split hostport %v", err)
		sp.Disconnect()
		return
	}
	if banEnd, ok := state.banned[host]; ok {
		if time.Now().Before(banEnd) {
			srvrLog.Debugf("Peer %s is banned for another %v -- disconnecting",
				host, time.Until(banEnd))
			sp.Disconnect()
			return
		}

		srvrLog.Infof("Peer %s is no longer banned", host)
		delete(state.banned, host)
	}

	// Limit max number of total peers.
	if state.Count() >= cfg.MaxPeers {
		srvrLog.Infof("Max peers reached [%d] -- disconnecting peer %s",
			cfg.MaxPeers, sp)
		sp.Disconnect()
		return
	}

	// Add the new peer and start it.
	peers := state.inboundPeers
	if !sp.Inbound() {
		if sp.persistent {
			peers = state.persistentPeers
		} else {
			peers = state.outboundPeers
		}
	}
	peers[sp.ID()] = sp
	sp.peerAdded.Store(true)
}

// handleDonePeerMsg deals with peers that have signalled they are done. It is
// invoked from the peerHandler goroutine.
func (s *server) handleDonePeerMsg(state *peerState, sp *serverPeer) {
	var list map[int32]*serverPeer
	if sp.persistent {
		list = state.persistentPeers
	} else if sp.Inbound() {
		list = state.inboundPeers
	} else {
		list = state.outboundPeers
	}
	if _, ok := list[sp.ID()]; ok {
		delete(list, sp.ID())
		srvrLog.Debugf("Removed peer %s", sp)
		return
	}

	// Only notify the sync manager about peer disconnection if the peer
	// was actually registered (peerAdded was set during handleAddPeerMsg).
	if sp.peerAdded.Swap(false) {
		s.syncManager.DonePeer(sp.Peer)
	}

	// Remove the persistent address if requested.
	if sp.persistent && sp.connReq != nil {
		s.connManager.Remove(sp.connReq.ID())
	}

	// Close the connection for the peer.
	sp.Disconnect()

	// Update the address' last seen time if the peer acknowledges our
	// version and has sent us its version.
	if sp.VerAckReceived() && sp.VersionKnown() && sp.NA() != nil {
		s.addrManager.Connected(sp.NA())
	}

	// Peer cleanup is handled by the sync manager internally.
}

// handleBanPeerMsg deals with banning peers. It is invoked from the
// peerHandler goroutine.
func (s *server) handleBanPeerMsg(state *peerState, sp *serverPeer) {
	host, _, err := net.SplitHostPort(sp.Addr())
	if err != nil {
		srvrLog.Debugf("can't split ban peer %s %v", sp.Addr(), err)
		return
	}
	direction := directionString(sp.Inbound())
	srvrLog.Infof("Banned peer %s (%s) for %v", host, direction, cfg.BanDuration)
	state.banned[host] = time.Now().Add(cfg.BanDuration)
}

// handleRelayInvMsg deals with relaying inventory to peers that are not already
// known to have it. It is invoked from the peerHandler goroutine.
func (s *server) handleRelayInvMsg(state *peerState, msg relayMsg) {
	state.forAllPeers(func(sp *serverPeer) {
		if !sp.Connected() {
			return
		}

		// If the inventory is a block, relay to all peers.
		// Peer-level deduplication is handled by QueueInventory internally.

		// If the inventory is a tx and the peer has relaying disabled
		// or the tx doesn't meet the fee filter, skip it.
		if msg.invVect.Type == wire.InvTypeTx {
			if sp.relayTxDisabled() {
				return
			}

			txD, ok := msg.data.(*mempool.TxDesc)
			if !ok {
				peerLog.Warnf("Underlying data for tx inv relay is not a *mempool.TxDesc: %T",
					msg.data)
				return
			}

			feeFilter := atomic.LoadInt64(&sp.feeFilter)
			if feeFilter > 0 && txD.FeePerKB < feeFilter {
				return
			}

			if sp.filter.IsLoaded() {
				if !sp.filter.MatchTxAndUpdate(txD.Tx) {
					return
				}
			}
		}

		// Queue the inventory to be relayed with the next batch.
		sp.QueueInventory(msg.invVect)
	})
}

// handleBroadcastMsg deals with broadcasting messages to peers. It is invoked
// from the peerHandler goroutine.
func (s *server) handleBroadcastMsg(state *peerState, bmsg *broadcastMsg) {
	state.forAllPeers(func(sp *serverPeer) {
		if !sp.Connected() {
			return
		}

		for _, ep := range bmsg.excludePeers {
			if sp == ep {
				return
			}
		}

		sp.QueueMessage(bmsg.message, nil)
	})
}

type getConnCountMsg struct {
	reply chan int32
}

type getPeersMsg struct {
	reply chan []*serverPeer
}

type getOutboundGroup struct {
	key   string
	reply chan int
}

type getAddedNodesMsg struct {
	reply chan []*serverPeer
}

type disconnectNodeMsg struct {
	cmp   func(*serverPeer) bool
	reply chan error
}

type connectNodeMsg struct {
	addr      string
	permanent bool
	reply     chan error
}

type removeNodeMsg struct {
	cmp   func(*serverPeer) bool
	reply chan error
}

// handleQuery is the central handler for all queries and commands from other
// goroutines related to peer state.
func (s *server) handleQuery(state *peerState, querymsg interface{}) {
	switch msg := querymsg.(type) {
	case getConnCountMsg:
		nconnected := int32(0)
		state.forAllPeers(func(sp *serverPeer) {
			if sp.Connected() {
				nconnected++
			}
		})
		msg.reply <- nconnected

	case getPeersMsg:
		peers := make([]*serverPeer, 0, state.Count())
		state.forAllPeers(func(sp *serverPeer) {
			if !sp.Connected() {
				return
			}
			peers = append(peers, sp)
		})
		msg.reply <- peers

	case connectNodeMsg:
		if state.Count() >= cfg.MaxPeers {
			msg.reply <- errors.New("max peers reached")
			return
		}
		for _, peer := range state.persistentPeers {
			if peer.Addr() == msg.addr {
				if msg.permanent {
					msg.reply <- errors.New("peer already connected")
				} else {
					msg.reply <- errors.New("peer exists as a permanent peer")
				}
				return
			}
		}

		netAddr, err := addrStringToNetAddr(msg.addr)
		if err != nil {
			msg.reply <- err
			return
		}

		go s.connManager.Connect(&connmgr.ConnReq{
			Addr:      netAddr,
			Permanent: msg.permanent,
		})
		msg.reply <- nil

	case removeNodeMsg:
		found := disconnectPeer(state.persistentPeers, msg.cmp, "remove")
		if found {
			msg.reply <- nil
		} else {
			msg.reply <- errors.New("peer not found")
		}

	case disconnectNodeMsg:
		found := disconnectPeer(state.inboundPeers, msg.cmp, "disconnect")
		found = disconnectPeer(state.outboundPeers, msg.cmp, "disconnect") || found
		found = disconnectPeer(state.persistentPeers, msg.cmp, "disconnect") || found
		if found {
			msg.reply <- nil
		} else {
			msg.reply <- errors.New("peer not found")
		}

	case getOutboundGroup:
		count, ok := state.outboundGroups[msg.key]
		if ok {
			msg.reply <- count
		} else {
			msg.reply <- 0
		}

	case getAddedNodesMsg:
		peers := make([]*serverPeer, 0, len(state.persistentPeers))
		for _, sp := range state.persistentPeers {
			peers = append(peers, sp)
		}
		msg.reply <- peers
	}
}

// disconnectPeer attempts to disconnect the peer matching the passed criteria.
func disconnectPeer(peerMap map[int32]*serverPeer, compareFunc func(*serverPeer) bool, action string) bool {
	for addr, peer := range peerMap {
		if compareFunc(peer) {
			srvrLog.Debugf("%s peer %s (%s)", action, peer.Addr(), directionString(peer.Inbound()))
			delete(peerMap, addr)
			peer.Disconnect()
			return true
		}
	}
	return false
}

// peerLifecycleHandler manages the lifecycle events for peers.
func (s *server) peerLifecycleHandler(inboundPeers, outboundPeers []*serverPeer) {
	state := &peerState{
		inboundPeers:    make(map[int32]*serverPeer),
		outboundPeers:   make(map[int32]*serverPeer),
		persistentPeers: make(map[int32]*serverPeer),
		banned:          make(map[string]time.Time),
		outboundGroups:  make(map[string]int),
	}

	// Add peers whose connection attempt succeeded.
	for _, p := range inboundPeers {
		state.inboundPeers[p.ID()] = p
	}
	for _, p := range outboundPeers {
		if p.persistent {
			state.persistentPeers[p.ID()] = p
		} else {
			state.outboundPeers[p.ID()] = p
		}
	}

	s.connManager = &connmgr.ConnManager{}

	// Start outbound address discovery based on the chain parameters.
	go s.connManager.Start()

out:
	for {
		select {
		// Peer connected or disconnected.
		case event := <-s.peerLifecycle:
			switch event.action {
			case peerAdd:
				s.handleAddPeerMsg(state, event.sp)
			case peerDone:
				s.handleDonePeerMsg(state, event.sp)
			}

		// Block accepted in main chain or orphan, update peer height.
		case umsg := <-s.peerHeightsUpdate:
			s.handleUpdatePeerHeights(state, umsg)

		// Peer to ban.
		case p := <-s.banPeers:
			s.handleBanPeerMsg(state, p)

		// New inventory to potentially be relayed to other peers.
		case invMsg := <-s.relayInv:
			s.handleRelayInvMsg(state, invMsg)

		// Message to broadcast to all connected peers except those
		// which are excluded by the message.
		case bmsg := <-s.broadcast:
			s.handleBroadcastMsg(state, &bmsg)

		case qmsg := <-s.query:
			s.handleQuery(state, qmsg)

		case <-s.quit:
			// Disconnect all peers on server shutdown.
			state.forAllPeers(func(sp *serverPeer) {
				srvrLog.Tracef("Shutdown peer %s", sp)
				sp.Disconnect()
			})
			break out
		}
	}

	s.connManager.Stop()
	s.syncManager.Stop()
	s.addrManager.Stop()

	// Drain channels before exiting so nothing is left waiting around
	// to send.
cleanup:
	for {
		select {
		case <-s.peerLifecycle:
		case <-s.peerHeightsUpdate:
		case <-s.relayInv:
		case <-s.broadcast:
		case <-s.query:
		default:
			break cleanup
		}
	}
	s.wg.Done()
	srvrLog.Tracef("Peer handler done")
}

// BanPeer bans a peer that has already been connected to the server by ip.
func (s *server) BanPeer(sp *serverPeer) {
	s.banPeers <- sp
}

// RelayInventory relays the passed inventory vector to all connected peers
// that are not already known to have it.
func (s *server) RelayInventory(invVect *wire.InvVect, data interface{}) {
	s.relayInv <- relayMsg{invVect: invVect, data: data}
}

// BroadcastMessage sends msg to all peers currently connected to the server
// except those in the passed peers to exclude.
func (s *server) BroadcastMessage(msg wire.Message, exclPeers ...*serverPeer) {
	bmsg := broadcastMsg{message: msg, excludePeers: exclPeers}
	s.broadcast <- bmsg
}

// ConnectedCount returns the number of currently connected peers.
func (s *server) ConnectedCount() int32 {
	replyChan := make(chan int32)
	s.query <- getConnCountMsg{reply: replyChan}
	return <-replyChan
}

// AddBytesSent adds the passed number of bytes to the total bytes sent counter
// for the server. It is safe for concurrent access.
func (s *server) AddBytesSent(bytesSent uint64) {
	atomic.AddUint64(&s.bytesSent, bytesSent)
}

// AddBytesReceived adds the passed number of bytes to the total bytes received
// counter for the server. It is safe for concurrent access.
func (s *server) AddBytesReceived(bytesReceived uint64) {
	atomic.AddUint64(&s.bytesReceived, bytesReceived)
}

// NetTotals returns the sum of all bytes received and sent across the network
// for all peers. It is safe for concurrent access.
func (s *server) NetTotals() (uint64, uint64) {
	return atomic.LoadUint64(&s.bytesReceived),
		atomic.LoadUint64(&s.bytesSent)
}

// UpdatePeerHeights updates the heights of all peers who have announced the
// latest connected main chain block, or a recognized orphan.
func (s *server) UpdatePeerHeights(latestBlkHash *chainhash.Hash, latestHeight int32, updateSource *peer.Peer) {
	s.peerHeightsUpdate <- updatePeerHeightsMsg{
		newHash:    latestBlkHash,
		newHeight:  latestHeight,
		originPeer: updateSource,
	}
}

// AnnounceNewTransactions generates and relays inventory vectors for all of the
// passed transactions to all connected peers.
func (s *server) AnnounceNewTransactions(newTxs []*mempool.TxDesc) {
	for _, tx := range newTxs {
		iv := wire.NewInvVect(wire.InvTypeTx, tx.Tx.Hash())
		s.RelayInventory(iv, tx)
	}
}

// TransactionConfirmed marks the provided transaction as confirmed by the
// chain and removes it from the rebroadcast set.
func (s *server) TransactionConfirmed(tx *nogoutil.Tx) {
	iv := wire.NewInvVect(wire.InvTypeTx, tx.Hash())
	select {
	case s.modifyRebroadcastInv <- broadcastInventoryDel(iv):
	case <-s.quit:
	}
}

// AddRebroadcastInventory adds the provided inventory to the list of
// inventories to be rebroadcast at random intervals until they show up in a
// block.
func (s *server) AddRebroadcastInventory(iv *wire.InvVect, data interface{}) {
	select {
	case s.modifyRebroadcastInv <- broadcastInventoryAdd{invVect: iv, data: data}:
	case <-s.quit:
	}
}

// relayTransactions generates and relays inventory vectors for all of the
// passed transactions to all connected peers.
func (s *server) relayTransactions(txns []*mempool.TxDesc) {
	for _, txD := range txns {
		iv := wire.NewInvVect(wire.InvTypeTx, txD.Tx.Hash())
		s.RelayInventory(iv, txD)
	}
}

// rebroadcastHandler keeps track of user submitted inventories that we have
// sent out but have not yet made it into a block. We periodically rebroadcast
// them in case our peers restarted or otherwise lost track of them.
func (s *server) rebroadcastHandler() {
	timer := time.NewTimer(5 * time.Minute)
	pendingInvs := make(map[wire.InvVect]interface{})

out:
	for {
		select {
		case riv := <-s.modifyRebroadcastInv:
			switch msg := riv.(type) {
			case broadcastInventoryAdd:
				pendingInvs[*msg.invVect] = msg.data
			case broadcastInventoryDel:
				delete(pendingInvs, *msg)
			}

		case <-timer.C:
			// Re-broadcast any inventories still pending.
			for iv, data := range pendingInvs {
				select {
				case s.relayInv <- relayMsg{invVect: &iv, data: data}:
				case <-s.quit:
					break out
				}
			}

			// Reset the timer for the next broadcast interval.
			timer.Reset(time.Duration(cfg.RebroadcastInterval) * time.Second)

		case <-s.quit:
			break out
		}
	}

	timer.Stop()
	s.wg.Done()
}

// peerHandler is the main handler for the server that manages peer connections.
func (s *server) peerHandler() {
	// Start the address manager and sync manager.
	s.addrManager.Start()
	s.syncManager.Start()

	srvrLog.Tracef("Starting peer handler")

	// Start the connection manager.
	if !cfg.DisableListen {
		// Start outbound address discovery based on the chain parameters.
		go s.connManager.Start()
	}

	// The peer lifecycle handler needs the state maps initialized. We
	// create both empty slices for the initial call.
	s.peerLifecycleHandler(nil, nil)
}

// Start begins accepting connections from peers.
func (s *server) Start() {
	// Already started?
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	srvrLog.Trace("Starting server")

	// Start the peer handler which in turn starts address and sync
	// managers.
	s.wg.Add(1)
	go s.peerHandler()
}

// Stop gracefully shuts down the server by stopping and disconnecting all
// peers and the main listener.
func (s *server) Stop() error {
	// Make sure this only happens once.
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		srvrLog.Infof("Server is already in the process of shutting down")
		return nil
	}

	srvrLog.Warnf("Server shutting down")

	// Signal the remaining goroutines to quit.
	close(s.quit)

	return nil
}

// WaitForShutdown blocks until the main listener and peer handlers are stopped.
func (s *server) WaitForShutdown() {
	s.wg.Wait()
}

// ScheduleShutdown schedules a server shutdown after the specified duration.
func (s *server) ScheduleShutdown(duration time.Duration) {
	// Don't schedule shutdown more than once.
	if atomic.AddInt32(&s.shutdownSched, 1) != 1 {
		return
	}
	srvrLog.Warnf("Server shutdown in %v", duration)
	go func() {
		remaining := duration
		tickDuration := dynamicTickDuration(remaining)
		timer := time.NewTicker(tickDuration)
	out:
		for {
			select {
			case <-timer.C:
				remaining = remaining - tickDuration
				if remaining <= 0 {
					break out
				}
				tickDuration = dynamicTickDuration(remaining)
				timer.Reset(tickDuration)
				srvrLog.Warnf("Server shutdown in %v", remaining)
			case <-s.quit:
				return
			}
		}
		timer.Stop()
		srvrLog.Warnf("Server shutting down")
		s.Stop()
	}()
}

// dynamicTickDuration adjusts the ticker duration based on the remaining time.
func dynamicTickDuration(remaining time.Duration) time.Duration {
	minutes := remaining / time.Minute
	switch {
	case minutes > 0:
		return remaining - minutes*time.Minute + time.Second
	default:
		return time.Second
	}
}

// newPeerConfig returns the configuration for the given serverPeer.
func (s *server) newPeerConfig(sp *serverPeer) *peer.Config {
	return &peer.Config{
		Listeners: peer.MessageListeners{
			OnVersion:     sp.OnVersion,
			OnVerAck:      sp.OnVerAck,
			OnMemPool:     sp.OnMemPool,
			OnTx:          sp.OnTx,
			OnBlock:       sp.OnBlock,
			OnInv:         sp.OnInv,
			OnHeaders:     sp.OnHeaders,
			OnGetData:     sp.OnGetData,
			OnGetBlocks:   sp.OnGetBlocks,
			OnGetHeaders:  sp.OnGetHeaders,
			OnGetCFilters: sp.OnGetCFilters,
			OnGetCFHeaders: sp.OnGetCFHeaders,
			OnGetCFCheckpt: sp.OnGetCFCheckpt,
			OnAddrV2:      sp.OnAddrV2,
			OnFeeFilter:   sp.OnFeeFilter,
			OnSendAddrV2:  sp.OnSendAddrV2,
			OnNotFound:    sp.OnNotFound,
		},
		NewestBlock: func() (*chainhash.Hash, int32, error) {
			best := s.chain.BestSnapshot()
			return &best.Hash, best.Height, nil
		},
		HostToNetAddress: func(host string, port uint16, services wire.ServiceFlag) (*wire.NetAddressV2, error) {
			return s.addrManager.HostToNetAddress(host, port, services)
		},
		Proxy:               cfg.Proxy,
		UserAgentName:       userAgentName,
		UserAgentVersion:    userAgentVersion,
		ChainParams:         s.chainParams,
		Services:            s.services,
		DisableRelayTx:      cfg.BlocksOnly,
		ProtocolVersion:     peer.MaxProtocolVersion,
		TrickleInterval:     cfg.TrickleInterval,
		DisableStallHandler: cfg.DisableStallHandler,
	}
}

// outboundPeerConnected is invoked by the connection manager when a new
// outbound connection is established.
func (s *server) outboundPeerConnected(c *connmgr.ConnReq, conn net.Conn) {
	sp := newServerPeer(s, c.Permanent)
	p, err := peer.NewOutboundPeer(s.newPeerConfig(sp), c.Addr.String())
	if err != nil {
		srvrLog.Debugf("Cannot create outbound peer %s: %v", c.Addr, err)
		s.connManager.Disconnect(c.ID())
		return
	}
	sp.Peer = p
	sp.connReq = c
	sp.isWhitelisted = isWhitelisted(c.Addr.(*net.TCPAddr).IP)
	sp.AssociateConnection(conn)

	go s.peerDoneHandler(sp)
}

// inboundPeerConnected is invoked by the connection manager when a new inbound
// connection is established.
func (s *server) inboundPeerConnected(conn net.Conn) {
	sp := newServerPeer(s, false)
	sp.Peer = peer.NewInboundPeer(s.newPeerConfig(sp))
	sp.isWhitelisted = isWhitelisted(conn.RemoteAddr().(*net.TCPAddr).IP)
	sp.AssociateConnection(conn)

	go s.peerDoneHandler(sp)
}

// peerDoneHandler handles peer disconnection by sending a peerDone event to
// the peer lifecycle handler.
func (s *server) peerDoneHandler(sp *serverPeer) {
	sp.WaitForDisconnect()
	s.peerLifecycle <- peerLifecycleEvent{action: peerDone, sp: sp}

	select {
	case <-sp.verAckCh:
		// verAckCh was closed: OnVerAck was received during the
		// peer's lifetime. Do nothing.
	default:
		// verAckCh was never closed: the peer disconnected before
		// OnVerAck, so peerAdd was never sent. Skip peerDone as
		// well.
		return
	}

	s.peerLifecycle <- peerLifecycleEvent{action: peerDone, sp: sp}
}

// addrStringToNetAddr takes an address string in the form "host:port" and
// returns a net.Addr which maps to the original address with a nil port.
func addrStringToNetAddr(addr string) (net.Addr, error) {
	host, strPort, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	port, err := strconv.Atoi(strPort)
	if err != nil {
		return nil, err
	}

	// If the host is a hostname, return the address as-is.
	if host == "" {
		host = "127.0.0.1"
	}

	// If the host resolves to localhost, treat it as a local address.
	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no IPs for %s", host)
		}
		ip = ips[0]
	}

	return &net.TCPAddr{IP: ip, Port: port}, nil
}

// isWhitelisted returns whether the IP address is included in the whitelisted
// networks.
func isWhitelisted(ip net.IP) bool {
	if len(cfg.Whitelists) == 0 {
		return false
	}

	for _, cidr := range cfg.Whitelists {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

// shuffleAddrsV2 randomizes the order of the given NetAddressV2 slice using
// cryptographic randomness.
func shuffleAddrsV2(addrs []*wire.NetAddressV2) {
	n := len(addrs)
	if n <= 1 {
		return
	}

	var buf [8]byte
	for i := n - 1; i > 0; i-- {
		if _, err := rand.Read(buf[:]); err != nil {
			return
		}
		j := int(binary.LittleEndian.Uint64(buf[:]) % uint64(i+1))
		addrs[i], addrs[j] = addrs[j], addrs[i]
	}
}

// setupRPCListeners returns a slice of listeners that are configured for use
// with the RPC server.
func setupRPCListeners() ([]net.Listener, error) {
	listenFunc := net.Listen
	if !cfg.DisableTLS {
		if !fileExists(cfg.RPCKey) && !fileExists(cfg.RPCCert) {
			err := genCertPair(cfg.RPCCert, cfg.RPCKey)
			if err != nil {
				return nil, err
			}
		}
		keypair, err := tls.LoadX509KeyPair(cfg.RPCCert, cfg.RPCKey)
		if err != nil {
			return nil, err
		}

		tlsConfig := tls.Config{
			Certificates: []tls.Certificate{keypair},
			MinVersion:   tls.VersionTLS12,
		}

		listenFunc = func(net string, laddr string) (net.Listener, error) {
			return tls.Listen(net, laddr, &tlsConfig)
		}
	}

	netAddrs, err := parseListeners(cfg.RPCListeners)
	if err != nil {
		return nil, err
	}

	listeners := make([]net.Listener, 0, len(netAddrs))
	for _, addr := range netAddrs {
		listener, err := listenFunc(addr.Network(), addr.String())
		if err != nil {
			rpcsLog.Warnf("Can't listen on %s: %v", addr, err)
			continue
		}
		listeners = append(listeners, listener)
	}

	return listeners, nil
}

// newServer returns a new NogoCore server configured to listen on addr for the
// network type specified by chainParams. Use start to begin accepting
// connections from peers.
func newServer(listenAddrs, agentBlacklist, agentWhitelist []string,
	db database.DB, chainParams *chaincfg.Params,
	interrupt <-chan struct{}) (*server, error) {

	services := defaultServices
	if cfg.NoPeerBloomFilters {
		services &^= wire.SFNodeBloom
	}
	if cfg.NoCFilters {
		services &^= wire.SFNodeCF
	}
	if !cfg.V2Transport {
		services &^= wire.SFNodeP2PV2
	}

	amgr := addrmgr.New(cfg.DataDir, btcdLookup)

	var listeners []net.Listener
	var nat NAT
	if !cfg.DisableListen {
		var err error
		listeners, nat, err = initListeners(amgr, listenAddrs, services)
		if err != nil {
			return nil, err
		}
		if len(listeners) == 0 {
			return nil, errors.New("no valid listen address")
		}
	}

	if len(agentBlacklist) > 0 {
		srvrLog.Infof("User-agent blacklist %s", agentBlacklist)
	}
	if len(agentWhitelist) > 0 {
		srvrLog.Infof("User-agent whitelist %s", agentWhitelist)
	}

	s := server{
		chainParams:          chainParams,
		addrManager:          amgr,
		peerLifecycle:        make(chan peerLifecycleEvent, cfg.MaxPeers*2),
		banPeers:             make(chan *serverPeer, cfg.MaxPeers),
		query:                make(chan interface{}),
		relayInv:             make(chan relayMsg, cfg.MaxPeers),
		broadcast:            make(chan broadcastMsg, cfg.MaxPeers),
		quit:                 make(chan struct{}),
		modifyRebroadcastInv: make(chan interface{}),
		peerHeightsUpdate:    make(chan updatePeerHeightsMsg),
		nat:                  nat,
		db:                   db,
		timeSource:           blockchain.NewMedianTime(),
		services:             services,
		cfCheckptCaches:      make(map[wire.FilterType][]cfHeaderKV),
		agentBlacklist:       agentBlacklist,
		agentWhitelist:       agentWhitelist,
	}

	// Create the transaction and address indexes if needed.
	var indexes []indexers.Indexer
	if cfg.TxIndex || cfg.AddrIndex {
		if !cfg.TxIndex {
			indxLog.Infof("Transaction index enabled because it is required by the address index")
			cfg.TxIndex = true
		} else {
			indxLog.Info("Transaction index is enabled")
		}

		s.txIndex = indexers.NewTxIndex(db)
		indexes = append(indexes, s.txIndex)
	}
	if cfg.AddrIndex {
		indxLog.Info("Address index is enabled")
		s.addrIndex = indexers.NewAddrIndex(db, chainParams)
		indexes = append(indexes, s.addrIndex)
	}
	if !cfg.NoCFilters {
		indxLog.Info("Committed filter index is enabled")
		s.cfIndex = indexers.NewCfIndex(db, chainParams)
		indexes = append(indexes, s.cfIndex)
	}

	var indexManager blockchain.IndexManager
	if len(indexes) > 0 {
		indexManager = indexers.NewManager(db, indexes)
	}

	checkpoints := chainParams.Checkpoints

	// Create a new block chain instance with the appropriate configuration.
	var err error
	s.chain, err = blockchain.New(&blockchain.Config{
		DB:               s.db,
		Interrupt:        interrupt,
		ChainParams:      s.chainParams,
		Checkpoints:      checkpoints,
		TimeSource:       s.timeSource,
		IndexManager:     indexManager,
		UtxoCacheMaxSize: 100 * 1024 * 1024, // 100MB.
	})
	if err != nil {
		return nil, err
	}

	// Search for a FeeEstimator state in the database.
	db.Update(func(tx database.Tx) error {
		metadata := tx.Metadata()
		feeEstimationData := metadata.Get(mempool.EstimateFeeDatabaseKey)
		if feeEstimationData != nil {
			metadata.Delete(mempool.EstimateFeeDatabaseKey)
			var err error
			s.feeEstimator, err = mempool.RestoreFeeEstimator(feeEstimationData)
			if err != nil {
				peerLog.Errorf("Failed to restore fee estimator %v", err)
			}
		}
		return nil
	})

	if s.feeEstimator == nil || s.feeEstimator.LastKnownHeight() != s.chain.BestSnapshot().Height {
		s.feeEstimator = mempool.NewFeeEstimator(
			mempool.DefaultEstimateFeeMaxRollback,
			mempool.DefaultEstimateFeeMinRegisteredBlocks)
	}

	txC := mempool.Config{
		Policy: mempool.Policy{
			DisableRelayPriority: cfg.NoRelayPriority,
			AcceptNonStd:         cfg.RelayNonStd,
			FreeTxRelayLimit:     cfg.FreeTxRelayLimit,
			MaxOrphanTxs:         cfg.MaxOrphanTxs,
			MaxOrphanTxSize:      defaultMaxOrphanTxSize,
			MaxSigOpCostPerTx:    blockchain.MaxBlockSigOpsCost / 4,
			MinRelayTxFee:        nogoutil.Amount(cfg.minRelayTxFee),
			MaxTxVersion:         2,
			RejectReplacement:    cfg.RejectReplacement,
		},
		ChainParams:    chainParams,
		FetchUtxoView:  s.chain.FetchUtxoView,
		BestHeight:     func() int32 { return s.chain.BestSnapshot().Height },
		MedianTimePast: func() time.Time { return s.chain.BestSnapshot().MedianTime },
		CalcSequenceLock: func(tx *nogoutil.Tx, view *blockchain.UtxoViewpoint) (*blockchain.SequenceLock, error) {
			return s.chain.CalcSequenceLock(tx, view, true)
		},
		IsDeploymentActive: s.chain.IsDeploymentActive,
		AddrIndex:          s.addrIndex,
		FeeEstimator:       s.feeEstimator,
	}
	s.txMemPool = mempool.New(&txC)

	s.syncManager, err = netsync.New(&netsync.Config{
		PeerNotifier:       &s,
		Chain:              s.chain,
		TxMemPool:          s.txMemPool,
		ChainParams:        s.chainParams,
		DisableCheckpoints: cfg.DisableCheckpoints,
		MaxPeers:           cfg.MaxPeers,
		FeeEstimator:       s.feeEstimator,
	})
	if err != nil {
		return nil, err
	}

	// Create mining policy.
	policy := mining.Policy{
		BlockMinWeight:    cfg.BlockMinWeight,
		BlockMaxWeight:    cfg.BlockMaxWeight,
		BlockMinSize:      cfg.BlockMinSize,
		BlockMaxSize:      cfg.BlockMaxSize,
		BlockPrioritySize: cfg.BlockPrioritySize,
		TxMinFreeFee:      nogoutil.Amount(cfg.minRelayTxFee),
	}

	// Set up the block template generator for getblocktemplate RPC.
	_ = mining.NewBlkTmplGenerator(&policy, s.chainParams, s.txMemPool, s.chain,
		s.timeSource)

	// Only set up a NAT if we have a valid listen address and we're not
	// in regression test mode.
	if !cfg.DisableListen {
		// Configure the connection manager with the server's outbound
		// connection handler.
		cmgr, err := connmgr.New(&connmgr.Config{
			Listeners:      listeners,
			OnAccept:       s.inboundPeerConnected,
			RetryDuration:  connectionRetryInterval,
			TargetOutbound: uint32(cfg.TargetOutbound),
			Dial:           btcdDial,
			OnConnection:   s.outboundPeerConnected,
			GetNewAddress:  newAddressFunc(s.addrManager, s.services),
		})
		if err != nil {
			return nil, err
		}
		s.connManager = cmgr
	}

	// Set up the p2p downgrader.
	s.p2pDowngrader = peer.NewP2PDowngrader(100)

	// Start up persistent peers.
	permanentPeers := cfg.ConnectPeers
	if len(permanentPeers) == 0 {
		permanentPeers = cfg.AddPeers
	}
	for _, addr := range permanentPeers {
		netAddr, err := addrStringToNetAddr(addr)
		if err != nil {
			return nil, err
		}

		go s.connManager.Connect(&connmgr.ConnReq{
			Addr:      netAddr,
			Permanent: true,
		})
	}

	// Start the rebroadcast handler.
	s.wg.Add(1)
	go s.rebroadcastHandler()

	return &s, nil
}

// newAddressFunc returns a function closure that can be used to retrieve
// addresses from the address manager.
func newAddressFunc(addrMgr *addrmgr.AddrManager, services wire.ServiceFlag) func() (net.Addr, error) {
	return func() (net.Addr, error) {
		ka := addrMgr.GetAddress()
		if ka == nil {
			return nil, errors.New("no addresses found")
		}
		return &net.TCPAddr{
			IP:   net.ParseIP(strings.Split(ka.NetAddress().Addr.String(), ":")[0]),
			Port: int(ka.NetAddress().Port),
		}, nil
	}
}

// mergeCheckpoints merges the default checkpoints with additional user-specified
// checkpoints.
func mergeCheckpoints(defaultCP []chaincfg.Checkpoint, additional []chaincfg.Checkpoint) []chaincfg.Checkpoint {
	merged := make([]chaincfg.Checkpoint, 0, len(defaultCP)+len(additional))
	merged = append(merged, defaultCP...)
	for _, cp := range additional {
		found := false
		for _, existing := range defaultCP {
			if existing.Height == cp.Height {
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, cp)
		}
	}
	// Sort by height.
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Height < merged[j].Height
	})
	return merged
}

// sort is imported for mergeCheckpoints.
var _ = sort.Slice

// initListeners initializes the configured net listeners and adds any bound
// addresses to the address manager. Returns the listeners and a NAT interface,
// which is non-nil if UPnP is in use.
func initListeners(amgr *addrmgr.AddrManager, listenAddrs []string, services wire.ServiceFlag) ([]net.Listener, NAT, error) {
	// Listen for TCP connections.
	netAddrs, err := parseListeners(listenAddrs)
	if err != nil {
		return nil, nil, err
	}

	listeners := make([]net.Listener, 0, len(netAddrs))
	for _, addr := range netAddrs {
		listener, err := net.Listen(addr.Network(), addr.String())
		if err != nil {
			srvrLog.Warnf("Can't listen on %s: %v", addr, err)
			continue
		}
		listeners = append(listeners, listener)
	}

	var nat NAT
	if len(cfg.ExternalIPs) != 0 {
		defaultPort, err := strconv.ParseUint(cfg.DefaultPort, 10, 16)
		if err != nil {
			srvrLog.Errorf("Can not parse default port %s for external IP: %v",
				cfg.DefaultPort, err)
			return listeners, nat, nil
		}

		for _, sip := range cfg.ExternalIPs {
			eport := uint16(defaultPort)
			host, portstr, err := net.SplitHostPort(sip)
			if err != nil {
				host = sip
			} else {
				port, err := strconv.ParseUint(portstr, 10, 16)
				if err != nil {
					srvrLog.Warnf("Can not parse port from %s for external IP: %v", sip, err)
					continue
				}
				eport = uint16(port)
			}

			na, err := amgr.HostToNetAddress(host, eport, services)
			if err != nil {
				srvrLog.Warnf("Not adding %s as external IP: %v", sip, err)
				continue
			}

			err = amgr.AddLocalAddress(na, addrmgr.ManualPrio)
			if err != nil {
				srvrLog.Warnf("Skipping specified external IP: %v", err)
			}
		}
	} else {
		if cfg.Discover && !cfg.DisableUPnP {
			nat = discoverNAT()
		}
	}

	// Add bound addresses to address manager for local discovery.
	for _, listener := range listeners {
		addr := listener.Addr().String()
		err := addLocalAddress(amgr, addr, services)
		if err != nil {
			srvrLog.Warnf("Skipping bound address %s: %v", addr, err)
		}
	}

	return listeners, nat, nil
}

// addLocalAddress adds an address that this node is listening on to the
// address manager so that it may be relayed to peers.
func addLocalAddress(amgr *addrmgr.AddrManager, addr string, services wire.ServiceFlag) error {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return err
	}

	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		// If the bound address is unspecified, use the external IP.
		extIP, err := GetExternalIP()
		if err != nil {
			return err
		}
		host = extIP.String()
	}

	na, err := amgr.HostToNetAddress(host, uint16(port), services)
	if err != nil {
		return err
	}

	return amgr.AddLocalAddress(na, addrmgr.BoundPrio)
}

// discoverNAT returns a NAT interface if UPnP discovery is successful.
// UPnP discovery is best-effort; failure does not prevent node operation.
func discoverNAT() NAT {
	nat, err := Discover()
	if err != nil {
		srvrLog.Debugf("UPnP discovery failed: %v", err)
		return nil
	}
	addr, err := nat.GetExternalAddress()
	if err != nil {
		srvrLog.Debugf("UPnP external address lookup failed: %v", err)
	} else {
		srvrLog.Infof("UPnP NAT discovered, external address: %s", addr)
	}
	return nat
}

// GetExternalIP returns the external IP address via UPnP NAT,
// or falls back to localhost if no NAT is available.
func GetExternalIP() (net.IP, error) {
	nat := discoverNAT()
	if nat != nil {
		addr, err := nat.GetExternalAddress()
		if err == nil && !addr.IsLoopback() {
			return addr, nil
		}
	}
	return net.IPv4(127, 0, 0, 1), nil
}

// btcdLookup resolves a DNS address.
func btcdLookup(host string) ([]net.IP, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	return ips, nil
}

// btcdDial connects to the address on the named network.
func btcdDial(addr net.Addr) (net.Conn, error) {
	return net.Dial(addr.Network(), addr.String())
}

// parseListeners parses the list of listen addresses passed in.
func parseListeners(addrs []string) ([]net.Addr, error) {
	// Use the default listen addresses if none were provided.
	if len(addrs) == 0 {
		addrs = []string{
			net.JoinHostPort("", cfg.DefaultPort),
		}
	}

	listeners := make([]net.Addr, 0, len(addrs))
	for _, addr := range addrs {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if host == "" {
			host = "0.0.0.0"
		}

		// Try to resolve the address. If it fails, use it directly
		// as an IP.
		ip := net.ParseIP(host)
		if ip == nil {
			addrs, err := net.LookupHost(host)
			if err == nil && len(addrs) > 0 {
				ip = net.ParseIP(addrs[0])
			}
		}
		if ip == nil {
			return nil, fmt.Errorf("cannot resolve %s", host)
		}

		portNum, err := strconv.Atoi(port)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, &net.TCPAddr{
			IP:   ip,
			Port: portNum,
		})
	}

	return listeners, nil
}

// fileExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// genCertPair generates a key/cert pair to the paths provided.
func genCertPair(certFile, keyFile string) error {
	org := "nogocore autogenerated cert"
	validUntil := time.Now().Add(10 * 365 * 24 * time.Hour)

	cert, key, err := newTLSCertPair(org, validUntil, nil)
	if err != nil {
		return err
	}

	// Write cert and key files.
	if err = os.WriteFile(certFile, cert, 0644); err != nil {
		return err
	}
	if err = os.WriteFile(keyFile, key, 0600); err != nil {
		os.Remove(certFile)
		return err
	}

	return nil
}

// newTLSCertPair returns a new TLS certificate and key.
func newTLSCertPair(organization string, validUntil time.Time, extraHosts []string) (cert, key []byte, err error) {
	if runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		return nil, nil, errors.New("TLS certificate generation not available on this platform")
	}

	// Use crypto/tls to generate a self-signed certificate.
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{organization},
		},
		NotBefore:             time.Now(),
		NotAfter:              validUntil,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add localhost and hostname as DNS names.
	host, err := os.Hostname()
	if err == nil {
		template.DNSNames = append(template.DNSNames, host)
	}
	template.DNSNames = append(template.DNSNames, "localhost")
	for _, h := range extraHosts {
		template.DNSNames = append(template.DNSNames, h)
	}

	// Generate private key.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	var certBuf bytes.Buffer
	pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}

	var keyBuf bytes.Buffer
	pem.Encode(&keyBuf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	return certBuf.Bytes(), keyBuf.Bytes(), nil
}

// main is the entry point for the NogoCore full-node daemon.
func main() {
	// Perform any upgrades required by newer versions.
	if err := doUpgrades(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to perform upgrades: %v\n", err)
		os.Exit(1)
	}

	// Load configuration and start the node.
	cfg, _, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Set the active net params.
	activeNetParams = cfg.ActiveNetParams

	fmt.Printf("NogoCore Node %s\n", version())
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("Network: %s\n", activeNetParams.Name)
	fmt.Printf("Data Dir: %s\n", cfg.DataDir)

	// Setup interrupt handler with full signal support (SIGINT + SIGTERM on unix).
	interrupt := interruptListener()

	// Create the database.
	dbPath := filepath.Join(cfg.DataDir, "blocks")
	db, err := database.Create("ffldb", dbPath, activeNetParams.Net)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create the server.
	server, err := newServer(cfg.Listen, cfg.agentBlacklist(), cfg.agentWhitelist(),
		db, activeNetParams, interrupt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	// Start the server.
	server.Start()

	// Wait for shutdown signal.
	<-interrupt
	fmt.Println("\nShutting down NogoCore node...")
	server.Stop()
	server.WaitForShutdown()
	fmt.Println("Node shutdown complete.")
}
