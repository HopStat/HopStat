package domain

import "time"

type BGPNeighbor struct {
	ID         int64     `json:"id"`
	NodeID     int64     `json:"node_id"`
	LocalAS    uint32    `json:"local_as"`
	RemoteAS   uint32    `json:"remote_as"`
	PeeringIP  string    `json:"peering_ip"`
	NeighborIP string    `json:"neighbor_ip"`
	Multihop   bool      `json:"multihop"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type BGPSessionState string

const (
	BGPSessionIdle        BGPSessionState = "idle"
	BGPSessionConnect     BGPSessionState = "connect"
	BGPSessionActive      BGPSessionState = "active"
	BGPSessionOpenSent    BGPSessionState = "open_sent"
	BGPSessionOpenConfirm BGPSessionState = "open_confirm"
	BGPSessionEstablished BGPSessionState = "established"
)

type BGPSessionStatus struct {
	NeighborID       int64           `json:"neighbor_id"`
	NodeID           int64           `json:"node_id"`
	State            BGPSessionState `json:"state"`
	RemoteAS         uint32          `json:"remote_as"`
	NeighborIP       string          `json:"neighbor_ip"`
	PrefixesReceived int             `json:"prefixes_received"`
	Uptime           string          `json:"uptime"`
}

type BGPRouteEntry struct {
	Prefix     string `json:"prefix"`
	NextHop    string `json:"next_hop"`
	ASPath     string `json:"as_path"`
	Origin     string `json:"origin"`
	LocalPref  string `json:"local_pref"`
	MED        string `json:"med"`
	NeighborIP string `json:"neighbor_ip"`
	SourceASN  uint32 `json:"source_asn"`
	Best       bool   `json:"best"`
	Age        string `json:"age"`
}
