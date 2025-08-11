package main

// SimLnGraph is the struct which corresponds to the JSON format used to encode
// graphs.
type SimLnGraph struct {
	SimNetwork []SimLnNode `json:"sim_network"`
}

type SimLnNode struct {
	Scid         uint64 `json:"scid"`
	CapacityMsat uint64 `json:"capacity_msat"`
	Node1        Node   `json:"node_1"`
	Node2        Node   `json:"node_2"`
}

type Node struct {
	PubKey          string `json:"pubkey"`
	Alias           string `json:"alias"`
	MaxHTLC         int64  `json:"max_htlc_count"`
	MaxInFlightMsat int64  `json:"max_in_flight_msat"`
	MinHTLCSizeMSat int64  `json:"min_htlc_size_msat"`
	MaxHTLCSizeMSat int64  `json:"max_htlc_size_msat"`
	CltvExpiryDelta int64  `json:"cltv_expiry_delta"`
	BaseFee         int64  `json:"base_fee"`
	FeeRateProp     int64  `json:"fee_rate_prop"`
}
