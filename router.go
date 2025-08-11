package main

import (
	"encoding/hex"

	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/routing"
)

func createChannelRouteterMap(graphInstance graphInstance) (map[string]*routing.ChannelRouter, error) {
	chain := newMockChain(101)
	channelRouterMap := make(map[string]*routing.ChannelRouter)
	seen := make(map[string]struct{})
	for _, node := range graphInstance.simLnGraphInstance.SimNetwork {
		if _, ok := seen[node.Node1.PubKey]; !ok {
			pub1Bytes, err := hex.DecodeString(node.Node1.PubKey)
			if err != nil {
				return nil, err
			}
			dbNode1 := &models.LightningNode{HaveNodeAnnouncement: true, AuthSigBytes: testSig.Serialize(), LastUpdate: testTime, Addresses: testAddrs, Alias: node.Node1.Alias, Features: testFeatures}
			copy(dbNode1.PubKeyBytes[:], pub1Bytes)
			router, err := routing.New(routing.Config{SelfNode: dbNode1.PubKeyBytes, RoutingGraph: graphInstance.graph, Chain: chain, GetLink: graphInstance.getLink})
			if err != nil {
				return nil, err
			}
			channelRouterMap[node.Node1.PubKey] = router
			seen[node.Node1.PubKey] = struct{}{}
		}
		if _, ok := seen[node.Node2.PubKey]; !ok {
			pub2Bytes, err := hex.DecodeString(node.Node2.PubKey)
			if err != nil {
				return nil, err
			}
			dbNode2 := &models.LightningNode{HaveNodeAnnouncement: true, AuthSigBytes: testSig.Serialize(), LastUpdate: testTime, Addresses: testAddrs, Alias: node.Node2.Alias, Features: testFeatures}
			copy(dbNode2.PubKeyBytes[:], pub2Bytes)
			router2, err := routing.New(routing.Config{SelfNode: dbNode2.PubKeyBytes, RoutingGraph: graphInstance.graph, Chain: chain, GetLink: graphInstance.getLink})
			if err != nil {
				return nil, err
			}
			channelRouterMap[node.Node2.PubKey] = router2
			seen[node.Node2.PubKey] = struct{}{}
		}
	}
	return channelRouterMap, nil
}
