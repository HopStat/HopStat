package domain

type AggregateInfo struct {
	ASN     uint32 `json:"asn"`
	Address string `json:"address"`
}

type BGPRoute struct {
	Prefix      string         `json:"prefix"`
	NextHop     string         `json:"next_hop"`
	ASPath      []uint32       `json:"as_path"`
	LocalPref   uint32         `json:"local_pref"`
	MED         uint32         `json:"med"`
	Origin      string         `json:"origin"`
	Communities []string       `json:"communities"`
	Aggregate   *AggregateInfo `json:"aggregate,omitempty"`
	Status      string         `json:"status,omitempty"`
	Protocol    string         `json:"protocol,omitempty"`
	Age         string         `json:"age,omitempty"`
	Rejected    bool           `json:"rejected"`
}

type BGPResult struct {
	Routes []BGPRoute `json:"routes"`
	Raw    string     `json:"raw,omitempty"`
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

type MTRHop struct {
	Number int     `json:"number"`
	Host   string  `json:"host"`
	Loss   float64 `json:"loss"`
	Sent   int     `json:"sent"`
	Recv   int     `json:"recv"`
	Last   float64 `json:"last"`
	Avg    float64 `json:"avg"`
	Best   float64 `json:"best"`
	Worst  float64 `json:"worst"`
	StDev  float64 `json:"st_dev"`
	ASInfo *ASInfo `json:"as_info,omitempty"`
}

type MTRResult struct {
	Hops []MTRHop `json:"hops"`
	Raw  string   `json:"raw,omitempty"`
}
