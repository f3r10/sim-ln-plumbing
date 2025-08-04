package main

import (
	// "bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwallet"

	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing"
	"github.com/lightningnetwork/lnd/routing/route"
)

// SimLnGraph is the struct which corresponds to the JSON format used to encode
// graphs.
type SimLnGraph struct {
	SimNetwork []SimLnNode `json:"sim_network"`
}

type SimLnNode struct {
	Scid  uint64   `json:"scid"`
	CapacityMsat  uint64 `json:"capacity_msat"`
	Node1 Node `json:"node_1"`
	Node2 Node `json:"node_2"`
}

type Node struct {
	PubKey  string `json:"pubkey"`
	Alias string `json:alias`
	MaxHTLC int64  `json:"max_htlc_count"`
	MaxInFlightMsat int64  `json:"max_in_flight_msat"`
	MinHTLCSizeMSat      int64  `json:"min_htlc_size_msat"`
	MaxHTLCSizeMSat      int64  `json:"max_htlc_size_msat"`
	CltvExpiryDelta      int64  `json:"cltv_expiry_delta"`
	BaseFee      int64  `json:"base_fee"`
	FeeRateProp      int64  `json:"fee_rate_prop"`
}


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
	uniquePaymentID uint64 = 1 // to be used atomically

	testAddr = &net.TCPAddr{IP: (net.IP)([]byte{0xA, 0x0, 0x0, 0x1}),
		Port: 9000}
	testAddrs = []net.Addr{testAddr}

	testFeatures = lnwire.NewFeatureVector(nil, lnwire.Features)

	testTime = time.Date(2018, time.January, 9, 14, 00, 00, 0, time.UTC)

	priv1, _    = btcec.NewPrivateKey()
	bitcoinKey1 = priv1.PubKey()

	priv2, _    = btcec.NewPrivateKey()
	bitcoinKey2 = priv2.PubKey()
)

func main() {
	path := "./sim_ln.json"
	graphJSON, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// First unmarshal the JSON graph into an instance of the SimLnGraph
	// struct. Using the struct tags created above in the struct, the JSON
	// will be properly parsed into the struct above.
	var g SimLnGraph
	if err := json.Unmarshal(graphJSON, &g); err != nil {
		return
	}
	aliasMap := make(map[string]route.Vertex)
	var source *models.LightningNode

	graph, _, err := makeTestGraph(true)

	//add Vertexes
	for _, node := range g.SimNetwork{
		pubBytes, err := hex.DecodeString(node.Node1.PubKey)
		if err != nil {
			return
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
		// We require all aliases within the graph to be unique for our
		// tests.
		if _, ok := aliasMap[dbNode1.Alias]; ok {
			return
		}

		// If the alias is unique, then add the node to the
		// alias map for easy lookup.
		aliasMap[dbNode1.Alias] = dbNode1.PubKeyBytes

		//TODO check which is the source in sim_ln
		source = dbNode1

		if err := graph.AddLightningNode(dbNode1); err != nil {
			return
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
			return
		}
		copy(dbNode2.PubKeyBytes[:], pubBytes2)
		// We require all aliases within the graph to be unique for our
		// tests.
		if _, ok := aliasMap[dbNode2.Alias]; ok {
			return
		}

		// If the alias is unique, then add the node to the
		// alias map for easy lookup.
		aliasMap[dbNode2.Alias] = dbNode2.PubKeyBytes
		fmt.Println("pubBytes! {}", aliasMap)
		if err := graph.AddLightningNode(dbNode2); err != nil {
			return
		}

		
	}

	// if source != nil {
	// 	// Set the selected source node
	// 	if err := graph.SetSourceNode(source); err != nil {
	// 		return
	// 	}
	// }

	for _, edges := range g.SimNetwork{
		node1Bytes, err := hex.DecodeString(edges.Node1.PubKey)
		fmt.Println("node1Bytes ", node1Bytes)
		if err != nil {
			return
		}

		node2Bytes, err := hex.DecodeString(edges.Node2.PubKey)
		fmt.Println("node2Bytes ", node2Bytes)
		if err != nil {
			fmt.Println("return 1")
			return
		}

		//TODO why is necessary this check
		// if bytes.Compare(node1Bytes, node2Bytes) == 1 {
		// 	fmt.Println("return 2")
		// 	return 
		// }

		//TODO what is a channel point, how to get it??
		fundingTXID := "33bd5d49a50e284221561b91e781f1fca0d60341c9f9dd785b5e379a6d88af3d" 
		txidBytes, err := chainhash.NewHashFromStr(fundingTXID)
		if err != nil {
			fmt.Println("return 3")
			return
		}

		fundingPoint := wire.OutPoint {
			Hash: *txidBytes,
			Index: 0,
		}

		edgeInfo := models.ChannelEdgeInfo{
			ChannelID: edges.Scid,
			AuthProof: &testAuthProof,
			ChannelPoint: fundingPoint,
			// Features: lnwire.EmptyFeatureVector(),
			Capacity: btcutil.Amount(edges.CapacityMsat),
		}

		copy(edgeInfo.NodeKey1Bytes[:], node1Bytes)
		copy(edgeInfo.NodeKey2Bytes[:], node2Bytes)
		copy(edgeInfo.BitcoinKey1Bytes[:], node1Bytes)
		copy(edgeInfo.BitcoinKey2Bytes[:], node2Bytes)

		err = graph.AddChannelEdge(&edgeInfo)
		if err != nil && !errors.Is(err, graphdb.ErrEdgeAlreadyExist) {
			return
		}


		//TODO check ChannelFlags
		channelFlags := lnwire.ChanUpdateChanFlags(1)
		isUpdate1 := channelFlags&lnwire.ChanUpdateDirection == 1
		targetNode := edgeInfo.NodeKey1Bytes
		if isUpdate1 {
			fmt.Println("should be targeNode node2")
			targetNode = edgeInfo.NodeKey2Bytes
		}

		edgePolicy := &models.ChannelEdgePolicy{
			SigBytes:                  testSig.Serialize(),
			//TODO check messages flags
			MessageFlags:              lnwire.ChanUpdateMsgFlags(1),
			ChannelFlags:              channelFlags,
			ChannelID:                 edges.Scid,
			LastUpdate:                testTime,
			TimeLockDelta:             uint16(edges.Node1.CltvExpiryDelta),
			MinHTLC:                   lnwire.MilliSatoshi(edges.Node1.MinHTLCSizeMSat),
			MaxHTLC:                   lnwire.MilliSatoshi(edges.Node1.MaxHTLCSizeMSat),
			FeeBaseMSat:               lnwire.MilliSatoshi(edges.Node1.BaseFee),
			FeeProportionalMillionths: lnwire.MilliSatoshi(edges.Node1.FeeRateProp),
			ToNode:                    targetNode,
		}
		if err := graph.UpdateEdgePolicy(edgePolicy); err != nil {
			return
		}

	}

	fmt.Println("this is the graph ", graph)
	chain := newMockChain(101)
	router, err := routing.New(routing.Config{
		SelfNode: source.PubKeyBytes,
		RoutingGraph: graph,
		Chain: chain,


	})

	paymentAmt := lnwire.NewMSatFromSatoshis(100)
	restriction := &routing.RestrictParams{
		FeeLimit: lnwire.NewMSatFromSatoshis(10),
		ProbabilitySource: noProbabilitySource,
		CltvLimit: math.MaxUint32,
	}
	target := aliasMap["alias1"]
	fmt.Println(target)
	req, err := routing.NewRouteRequest(
		source.PubKeyBytes, &target, paymentAmt, 0, restriction, nil, nil, nil, 18)
		
	route, a, err := router.FindRoute(req)
	fmt.Println("A ", a)
	fmt.Println(err)
	fmt.Println("this is the route ", route)
}

func noProbabilitySource(route.Vertex, route.Vertex, lnwire.MilliSatoshi,
	btcutil.Amount) float64 {
	return 1
}

type mockChain struct {
	lnwallet.BlockChainIO

	blocks map[chainhash.Hash]*wire.MsgBlock
	blockIndex map[uint32]chainhash.Hash
	blockHeightIndex map[chainhash.Hash]uint32

	utxos map[wire.OutPoint]wire.TxOut

	bestHeight int32

	sync.RWMutex
}

func newMockChain(currentHeight uint32) *mockChain {
	return &mockChain{
		bestHeight: int32(currentHeight),
		blocks: make(map[chainhash.Hash]*wire.MsgBlock),
		utxos: make(map[wire.OutPoint]wire.TxOut),
		blockIndex: make(map[uint32]chainhash.Hash),
		blockHeightIndex: make(map[chainhash.Hash]uint32),
	}
}

func (m *mockChain) GetBestBlock() (*chainhash.Hash, int32, error) {
	m.RLock()
	defer m.RUnlock()

	blockHash := m.blockIndex[uint32(m.bestHeight)]

	return &blockHash, m.bestHeight, nil
}

// makeTestGraph creates a new instance of a channeldb.ChannelGraph for testing
// purposes.
func makeTestGraph(useCache bool) (*graphdb.ChannelGraph,
	kvdb.Backend, error) {

	// Create channelgraph for the first time.
	backend, _, err := kvdb.GetTestBackend("/tmp/asd", "cgr")
	if err != nil {
		return nil, nil, err
	}

	// t.Cleanup(backendCleanup)

	graph, err := graphdb.NewChannelGraph(
		&graphdb.Config{KVDB: backend},
		graphdb.WithUseGraphCache(useCache),
	)
	if err != nil {
		return nil, nil, err
	}

	graph.Start()
	// require.NoError(t, graph.Start())
	// t.Cleanup(func() {
	// 	require.NoError(t, graph.Stop())
	// })

	return graph, backend, nil
}
