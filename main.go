package main

import (
	"fmt"
	"math"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing"
	"github.com/lightningnetwork/lnd/routing/route"
)

func main() {
	path := "./sim_ln.json"

	graphInstance, err := parseGraph(true, path)
	if err != nil {
		fmt.Println("failed to parse graph:", err)
		return
	}

	channelRouterMap, err := createChannelRouteterMap(*graphInstance)
	if err != nil {
		fmt.Println("failed to create routers:", err)
		return
	}

	if len(graphInstance.simLnGraphInstance.SimNetwork) == 0 {
		fmt.Println("no channels defined")
		return
	}
	first := graphInstance.simLnGraphInstance.SimNetwork[0]
	last := graphInstance.simLnGraphInstance.SimNetwork[len(graphInstance.simLnGraphInstance.SimNetwork)-1]

	sourcePub := first.Node1.PubKey
	targetPub := last.Node2.PubKey

	router := channelRouterMap[sourcePub]
	if router == nil {
		fmt.Println("router for source not found")
		return
	}

	paymentAmt := lnwire.NewMSatFromSatoshis(40)
	restriction := &routing.RestrictParams{
		FeeLimit:          lnwire.NewMSatFromSatoshis(1000),
		ProbabilitySource: noProbabilitySource,
		CltvLimit:         math.MaxUint32,
	}

	targetVertex, err := route.NewVertexFromStr(targetPub)
	if err != nil {
		fmt.Println(err)
		return
	}
	sourceVertex, err := route.NewVertexFromStr(sourcePub)
	if err != nil {
		fmt.Println(err)
		return
	}

	req, err := routing.NewRouteRequest(
		sourceVertex, &targetVertex, paymentAmt, 0, restriction, nil, nil, nil, 18)
	if err != nil {
		fmt.Println(err)
		return
	}

	r, prob, err := router.FindRoute(req)
	fmt.Println("prob", prob)
	fmt.Println("route", r)
	fmt.Println("err", err)
}

func noProbabilitySource(route.Vertex, route.Vertex, lnwire.MilliSatoshi,
	btcutil.Amount) float64 {
	return 1
}
