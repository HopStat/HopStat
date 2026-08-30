package domain

import (
	"time"
)

type BGPPeerType string

const (
	BGPPeerInternal BGPPeerType = "internal"
	BGPPeerExternal BGPPeerType = "external"
)

func PeerTypeFor(localAS, remoteAS uint32) BGPPeerType {
	if localAS > 0 && remoteAS == localAS {
		return BGPPeerInternal
	}
	return BGPPeerExternal
}

func (p BGPPeerType) IsInternal() bool {
	return p == BGPPeerInternal
}

type BGPNeighbor struct {
	ID             int64       `json:"id"`
	NodeID         int64       `json:"node_id"`
	LocalAS        uint32      `json:"local_as"`
	RemoteAS       uint32      `json:"remote_as"`
	PeeringIP      string      `json:"peering_ip"`
	NeighborIP     string      `json:"neighbor_ip"`
	IPv6PeeringIP  string      `json:"ipv6_peering_ip"`
	IPv6NeighborIP string      `json:"ipv6_neighbor_ip"`
	Multihop       bool        `json:"multihop"`
	PeerType       BGPPeerType `json:"peer_type"`
	DefaultRouteAS uint32      `json:"default_route_as,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
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

// BGPPathDetail is one path exactly as it reached us, for the admin lookup tool. It keeps
// the fields the normal result drops — the ADD-PATH identifier and the full attribute set —
// so a disagreement with the router can be diagnosed from what was actually advertised.
type BGPPathDetail struct {
	Prefix      string   `json:"prefix"`
	NeighborIP  string   `json:"neighbor_ip"`
	NodeName    string   `json:"node_name,omitempty"`
	Identifier  uint32   `json:"identifier"`
	SourceASN   uint32   `json:"source_asn"`
	Best        bool     `json:"best"`
	Age         string   `json:"age"`
	NextHop     string   `json:"next_hop"`
	ASPath      string   `json:"as_path"`
	Origin      string   `json:"origin"`
	LocalPref   string   `json:"local_pref"`
	MED         string   `json:"med"`
	Communities []string `json:"communities,omitempty"`
	Attributes  []string `json:"attributes"`
}

type BGPRouteEntry struct {
	Prefix       string   `json:"prefix"`
	NextHop      string   `json:"next_hop"`
	ASPath       string   `json:"as_path"`
	Origin       string   `json:"origin"`
	LocalPref    string   `json:"local_pref"`
	MED          string   `json:"med"`
	NeighborIP   string   `json:"neighbor_ip"`
	SourceASN    uint32   `json:"source_asn"`
	Best         bool     `json:"best"`
	Age          string   `json:"age"`
	AgeSeconds   int64    `json:"age_seconds,omitempty"` // for best-path tie-breaking
	OriginatorID string   `json:"originator_id,omitempty"`
	ClusterList  []string `json:"cluster_list,omitempty"`
	Communities  []string `json:"communities,omitempty"`
}
