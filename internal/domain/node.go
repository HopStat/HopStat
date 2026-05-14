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
	CmdMTR        CommandType = "mtr"
	CmdBGPRoute   CommandType = "bgp_route"
	CmdASPath     CommandType = "as_path"
)

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
