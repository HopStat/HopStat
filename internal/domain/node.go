package domain

import "time"

type NodeType string

const (
	NodeTypeStandalone NodeType = "standalone"
	NodeTypeLGNode     NodeType = "lg_node"
)

type CommandType string

const (
	CmdPing       CommandType = "ping"
	CmdTraceroute CommandType = "traceroute"
	CmdBGPRoute   CommandType = "bgp_route"
)

func ValidNodeCommands() []CommandType {
	return []CommandType{CmdPing, CmdTraceroute, CmdBGPRoute}
}

func IsSupportedNodeCommand(cmd string) bool {
	switch CommandType(cmd) {
	case CmdPing, CmdTraceroute, CmdBGPRoute:
		return true
	default:
		return false
	}
}

// NormalizeEnabledCmds drops deprecated or unknown commands and deduplicates.
func NormalizeEnabledCmds(cmds []CommandType) []CommandType {
	valid := map[CommandType]bool{
		CmdPing: true, CmdTraceroute: true, CmdBGPRoute: true,
	}
	seen := map[CommandType]bool{}
	out := make([]CommandType, 0, len(cmds))
	for _, c := range cmds {
		if !valid[c] || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

func NormalizeEnabledCmdStrings(cmds []string) []CommandType {
	parsed := make([]CommandType, 0, len(cmds))
	for _, cmd := range cmds {
		if IsSupportedNodeCommand(cmd) {
			parsed = append(parsed, CommandType(cmd))
		}
	}
	return NormalizeEnabledCmds(parsed)
}

func DefaultEnabledCmds() []CommandType {
	return ValidNodeCommands()
}

type StandaloneBGPConfig struct {
	RouterID      string `json:"router_id"`
	LocalAS       uint32 `json:"local_as"`
	PeerAS        uint32 `json:"peer_as"`
	PeerAddr      string `json:"peer_addr"`
	PeerPort      uint16 `json:"peer_port"`
	AuthPwd       string `json:"auth_pwd,omitempty"`
	PassiveMode   bool   `json:"passive_mode"`
	ToolsSourceIP string `json:"tools_source_ip"`
}

type Node struct {
	ID           int64              `json:"id"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Type         NodeType           `json:"type"`
	City         string             `json:"city"`
	Country      string             `json:"country"`
	Lat          *float64           `json:"lat,omitempty"`
	Lon          *float64           `json:"lon,omitempty"`
	CredentialID *int64             `json:"credential_id,omitempty"`
	Active       bool               `json:"active"`
	IsDefault    bool               `json:"is_default"`
	EnabledCmds  []CommandType      `json:"enabled_cmds"`
	BGPConfig    *StandaloneBGPConfig `json:"bgp_config,omitempty"`
	AgentURL     string             `json:"agent_url"`
	AgentToken   string             `json:"agent_token,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

func (n *Node) CanExecute(cmd CommandType) bool {
	for _, c := range n.EnabledCmds {
		if c == cmd {
			return true
		}
	}
	return false
}
