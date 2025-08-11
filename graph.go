package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/fn/v2"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/htlcswitch"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/tlv"

	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// shared test constants/vars for the package
var (
	testRBytes, _ = hex.DecodeString("8ce2bc69281ce27da07e6683571319d18e949ddfa2965fb6caa1bf0314f882d7")
	testSBytes, _ = hex.DecodeString("299105481d63e0f4bc2a88121167221b6700d72a0ead154c03be696a292d24ae")
	testRScalar   = new(btcec.ModNScalar)
	testSScalar   = new(btcec.ModNScalar)
	_             = testRScalar.SetByteSlice(testRBytes)
	_             = testSScalar.SetByteSlice(testSBytes)
	testSig       = ecdsa.NewSignature(testRScalar, testSScalar)

	testAuthProof = models.ChannelAuthProof{
		NodeSig1Bytes:    testSig.Serialize(),
		NodeSig2Bytes:    testSig.Serialize(),
		BitcoinSig1Bytes: testSig.Serialize(),
		BitcoinSig2Bytes: testSig.Serialize(),
	}
)

var (
	testTime     = time.Date(2018, time.January, 9, 14, 00, 00, 0, time.UTC)
	testAddr     = &net.TCPAddr{IP: (net.IP)([]byte{0xA, 0x0, 0x0, 0x1}), Port: 9000}
	testAddrs    = []net.Addr{testAddr}
	testFeatures = lnwire.NewFeatureVector(nil, lnwire.Features)
)

// graphInstance holds the in-memory graph and helpers for routing.
type graphInstance struct {
	graph              *graphdb.ChannelGraph
	graphBackend       kvdb.Backend
	simLnGraphInstance SimLnGraph
	links              map[lnwire.ShortChannelID]htlcswitch.ChannelLink
}

func parseGraph(useCache bool, path string) (*graphInstance, error) {
	graphJSON, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var g SimLnGraph
	if err := json.Unmarshal(graphJSON, &g); err != nil {
		return nil, err
	}
	aliasMap := make(map[string]route.Vertex)
	links := make(map[lnwire.ShortChannelID]htlcswitch.ChannelLink)

	graph, graphBackend, err := makeTestGraph(useCache)
	if err != nil {
		return nil, err
	}

	// Add vertices once per unique alias/pubkey
	for _, node := range g.SimNetwork {
		pubBytes, err := hex.DecodeString(node.Node1.PubKey)
		if err != nil {
			return nil, err
		}
		dbNode1 := &models.LightningNode{
			HaveNodeAnnouncement: true,
			AuthSigBytes:         testSig.Serialize(),
			LastUpdate:           testTime,
			Addresses:            testAddrs,
			Alias:                node.Node1.Alias,
			Features:             testFeatures,
		}
		copy(dbNode1.PubKeyBytes[:], pubBytes)
		if _, ok := aliasMap[dbNode1.Alias]; !ok {
			aliasMap[dbNode1.Alias] = dbNode1.PubKeyBytes
			if err := graph.AddLightningNode(dbNode1); err != nil {
				return nil, err
			}
		}

		dbNode2 := &models.LightningNode{
			HaveNodeAnnouncement: true,
			AuthSigBytes:         testSig.Serialize(),
			LastUpdate:           testTime,
			Addresses:            testAddrs,
			Alias:                node.Node2.Alias,
			Features:             testFeatures,
		}
		pubBytes2, err := hex.DecodeString(node.Node2.PubKey)
		if err != nil {
			return nil, err
		}
		copy(dbNode2.PubKeyBytes[:], pubBytes2)
		if _, ok := aliasMap[dbNode2.Alias]; !ok {
			aliasMap[dbNode2.Alias] = dbNode2.PubKeyBytes
			if err := graph.AddLightningNode(dbNode2); err != nil {
				return nil, err
			}
		}
	}

	for _, edges := range g.SimNetwork {
		node1Bytes, err := hex.DecodeString(edges.Node1.PubKey)
		if err != nil {
			return nil, err
		}
		node2Bytes, err := hex.DecodeString(edges.Node2.PubKey)
		if err != nil {
			return nil, err
		}

		chanID := edges.Scid
		fundingTXID := "33bd5d49a50e284221561b91e781f1fca0d60341c9f9dd785b5e379a6d88af3d"
		txidBytes, err := chainhash.NewHashFromStr(fundingTXID)
		if err != nil {
			return nil, err
		}

		fundingPoint := wire.OutPoint{
			Hash:  *txidBytes,
			Index: uint32(edges.Scid),
		}

		edgeInfo := models.ChannelEdgeInfo{
			ChannelID:    chanID,
			AuthProof:    &testAuthProof,
			ChannelPoint: fundingPoint,
			Capacity:     btcutil.Amount(edges.CapacityMsat / 1000),
		}
		copy(edgeInfo.NodeKey1Bytes[:], node1Bytes)
		copy(edgeInfo.NodeKey2Bytes[:], node2Bytes)
		copy(edgeInfo.BitcoinKey1Bytes[:], node1Bytes)
		copy(edgeInfo.BitcoinKey2Bytes[:], node2Bytes)

		shortID := lnwire.NewShortChanIDFromInt(edgeInfo.ChannelID)
		links[shortID] = &mockLink{
			bandwidth: lnwire.MilliSatoshi(edgeInfo.Capacity * 1000),
		}

		err = graph.AddChannelEdge(&edgeInfo)
		if err != nil && !errors.Is(err, graphdb.ErrEdgeAlreadyExist) {
			return nil, err
		}

		// edge info from Node2 -> Node1 using Node2 config
		edgePolicy := &models.ChannelEdgePolicy{
			SigBytes:                  testSig.Serialize(),
			MessageFlags:              1,
			ChannelFlags:              0,
			ChannelID:                 chanID,
			LastUpdate:                time.Unix(433453, 0),
			TimeLockDelta:             uint16(edges.Node2.CltvExpiryDelta),
			MinHTLC:                   lnwire.MilliSatoshi(edges.Node2.MinHTLCSizeMSat),
			MaxHTLC:                   lnwire.MilliSatoshi(edges.Node2.MaxHTLCSizeMSat),
			FeeBaseMSat:               lnwire.MilliSatoshi(edges.Node2.BaseFee),
			FeeProportionalMillionths: lnwire.MilliSatoshi(edges.Node2.FeeRateProp),
			ToNode:                    edgeInfo.NodeKey2Bytes,
			ExtraOpaqueData:           []byte{1, 0},
		}
		if err := graph.UpdateEdgePolicy(edgePolicy); err != nil {
			return nil, err
		}

		// edge info from Node1 -> Node2 using Node1 config
		edgePolic2 := &models.ChannelEdgePolicy{
			SigBytes:                  testSig.Serialize(),
			MessageFlags:              1,
			ChannelFlags:              1,
			ChannelID:                 chanID,
			LastUpdate:                time.Unix(433453, 0),
			TimeLockDelta:             uint16(edges.Node1.CltvExpiryDelta),
			MinHTLC:                   lnwire.MilliSatoshi(edges.Node1.MinHTLCSizeMSat),
			MaxHTLC:                   lnwire.MilliSatoshi(edges.Node1.MaxHTLCSizeMSat),
			FeeBaseMSat:               lnwire.MilliSatoshi(edges.Node1.BaseFee),
			FeeProportionalMillionths: lnwire.MilliSatoshi(edges.Node1.FeeRateProp),
			ToNode:                    edgeInfo.NodeKey1Bytes,
			ExtraOpaqueData:           []byte{1, 0},
		}
		if err := graph.UpdateEdgePolicy(edgePolic2); err != nil {
			return nil, err
		}
	}

	return &graphInstance{
		graph:              graph,
		graphBackend:       graphBackend,
		simLnGraphInstance: g,
		links:              links,
	}, nil
}

func (g *graphInstance) getLink(chanID lnwire.ShortChannelID) (htlcswitch.ChannelLink, error) {
	link, ok := g.links[chanID]
	if !ok {
		return nil, fmt.Errorf("link not found in mock: %v", chanID)
	}
	return link, nil
}

type mockChain struct {
	lnwallet.BlockChainIO

	blocks           map[chainhash.Hash]*wire.MsgBlock
	blockIndex       map[uint32]chainhash.Hash
	blockHeightIndex map[chainhash.Hash]uint32

	utxos map[wire.OutPoint]wire.TxOut

	bestHeight int32

	sync.RWMutex
}

func newMockChain(currentHeight uint32) *mockChain {
	return &mockChain{
		bestHeight:       int32(currentHeight),
		blocks:           make(map[chainhash.Hash]*wire.MsgBlock),
		utxos:            make(map[wire.OutPoint]wire.TxOut),
		blockIndex:       make(map[uint32]chainhash.Hash),
		blockHeightIndex: make(map[chainhash.Hash]uint32),
	}
}

func (m *mockChain) GetBestBlock() (*chainhash.Hash, int32, error) {
	m.RLock()
	defer m.RUnlock()

	blockHash := m.blockIndex[uint32(m.bestHeight)]
	return &blockHash, m.bestHeight, nil
}

// makeTestGraph creates a new instance of a channeldb.ChannelGraph for testing purposes.
func makeTestGraph(useCache bool) (*graphdb.ChannelGraph, kvdb.Backend, error) {
	backend, _, err := kvdb.GetTestBackend("/tmp/sim-16", "cgr")
	if err != nil {
		return nil, nil, err
	}
	graph, err := graphdb.NewChannelGraph(
		&graphdb.Config{KVDB: backend},
		graphdb.WithUseGraphCache(useCache),
	)
	if err != nil {
		return nil, nil, err
	}
	graph.Start()
	return graph, backend, nil
}

type mockLink struct {
	htlcswitch.ChannelLink
	bandwidth         lnwire.MilliSatoshi
	mayAddOutgoingErr error
	ineligible        bool
}

func (m *mockLink) Bandwidth() lnwire.MilliSatoshi { return m.bandwidth }
func (m *mockLink) AuxBandwidth(lnwire.MilliSatoshi, lnwire.ShortChannelID, fn.Option[tlv.Blob], htlcswitch.AuxTrafficShaper) fn.Result[htlcswitch.OptionalBandwidth] {
	return fn.Ok(htlcswitch.OptionalBandwidth{})
}
func (m *mockLink) EligibleToForward() bool                        { return !m.ineligible }
func (m *mockLink) MayAddOutgoingHtlc(_ lnwire.MilliSatoshi) error { return m.mayAddOutgoingErr }
func (m *mockLink) FundingCustomBlob() fn.Option[tlv.Blob]         { return fn.None[tlv.Blob]() }
func (m *mockLink) CommitmentCustomBlob() fn.Option[tlv.Blob]      { return fn.None[tlv.Blob]() }
