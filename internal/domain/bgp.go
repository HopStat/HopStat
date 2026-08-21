package domain

type AggregateInfo struct {
	ASN     uint32 `json:"asn"`
	Address string `json:"address"`
}

type BGPRoute struct {
	Prefix       string           `json:"prefix"`
	NextHop      string           `json:"next_hop"`
	ASPath       []uint32         `json:"as_path"`
	LocalPref    uint32           `json:"local_pref"`
	MED          uint32           `json:"med"`
	Origin       string           `json:"origin"`
	Communities  []string         `json:"communities"`
	MatchedRules []*CommunityRule `json:"matched_rules,omitempty"`
	Status       string           `json:"status,omitempty"`
	Protocol     string           `json:"protocol,omitempty"`
	Age          string           `json:"age,omitempty"`
	ViaDefaultRoute bool          `json:"via_default_route,omitempty"`
	Best         bool             `json:"best,omitempty"`
	NodeName     string           `json:"node_name,omitempty"`
	Aggregate    *AggregateInfo   `json:"aggregate,omitempty"`
	Rejected     bool             `json:"rejected"`
}

// NodeASPath is one path a node holds for a target, used to draw the multi-node
// network map. A node contributes its selected route plus any backup paths it also
// carries, so the map can show what traffic would fall back to. NoRoute keeps the
// node on the map even when it sees nothing — "this node has no path" is part of
// the comparison.
type NodeASPath struct {
	NodeID          int64    `json:"node_id"`
	NodeName        string   `json:"node_name"`
	Prefix          string   `json:"prefix,omitempty"`
	ASPath          []uint32 `json:"as_path,omitempty"`
	Best            bool     `json:"best,omitempty"`
	ViaDefaultRoute bool     `json:"via_default_route,omitempty"`
	NoRoute         bool     `json:"no_route,omitempty"`
}

type BGPResult struct {
	Routes   []BGPRoute `json:"routes"`
	Raw      string     `json:"raw,omitempty"`
	TargetAS *ASInfo    `json:"target_as,omitempty"`
}

type ASPathEntry struct {
	Prefix string   `json:"prefix"`
	ASPath []uint32 `json:"as_path"`
}

type ASPathResult struct {
	ASN      uint32        `json:"asn"`
	Prefixes []ASPathEntry `json:"prefixes"`
	Raw      string        `json:"raw,omitempty"`
}

type ASInfo struct {
	ASN         uint32 `json:"asn"`
	OrgName     string `json:"org_name"`
	ShortName   string `json:"short_name"`
	CountryCode string `json:"country_code"`
	FlagEmoji   string `json:"flag_emoji"`
}

type Hop struct {
	Number int     `json:"number"`
	IP     string  `json:"ip"`
	Host   string  `json:"host"`
	RTT    []float64 `json:"rtt"`
	ASInfo *ASInfo `json:"as_info,omitempty"`
}

type PingResult struct {
	PacketsSent int     `json:"packets_sent"`
	PacketsRecv int     `json:"packets_recv"`
	PacketLoss  float64 `json:"packet_loss"`
	MinRTT      float64 `json:"min_rtt"`
	AvgRTT      float64 `json:"avg_rtt"`
	MaxRTT      float64 `json:"max_rtt"`
	Raw         string  `json:"raw,omitempty"`
}

type TracerouteResult struct {
	Hops []Hop  `json:"hops"`
	Raw  string `json:"raw,omitempty"`
}
