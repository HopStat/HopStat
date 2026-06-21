package domain

import "context"

type NodeRepository interface {
	GetAll(ctx context.Context) ([]*Node, error)
	GetActive(ctx context.Context) ([]*Node, error)
	GetByID(ctx context.Context, id int64) (*Node, error)
	Create(ctx context.Context, node *Node) (*Node, error)
	Update(ctx context.Context, node *Node) (*Node, error)
	SetDefault(ctx context.Context, id int64) error
	Delete(ctx context.Context, id int64) error
	UpdateEnabledCmds(ctx context.Context, id int64, cmds []CommandType) error
}

type AuditRepository interface {
	Log(ctx context.Context, entry *AuditEntry) error
	List(ctx context.Context, filter AuditFilter) ([]*AuditEntry, int, error)
	CountToday(ctx context.Context) (int, error)
	Cleanup(ctx context.Context, olderThan string) (int64, error)
}

type UserRepository interface {
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	UpdateCredentials(ctx context.Context, id int64, email, passwordHash string) (*User, error)
	UpdateLastLogin(ctx context.Context, id int64) error
}

type CommunityRuleRepository interface {
	GetAll(ctx context.Context) ([]*CommunityRule, error)
	GetActive(ctx context.Context) ([]*CommunityRule, error)
	GetActiveRulesForNode(ctx context.Context, nodeID int64) ([]*CommunityRule, error)
	Create(ctx context.Context, rule *CommunityRule) (*CommunityRule, error)
	Update(ctx context.Context, rule *CommunityRule) (*CommunityRule, error)
	Delete(ctx context.Context, id int64) error
	Toggle(ctx context.Context, id int64) error
}

type BGPNeighborRepository interface {
	GetAll(ctx context.Context) ([]*BGPNeighbor, error)
	GetByNodeID(ctx context.Context, nodeID int64) ([]*BGPNeighbor, error)
	GetByID(ctx context.Context, id int64) (*BGPNeighbor, error)
	Create(ctx context.Context, n *BGPNeighbor) (*BGPNeighbor, error)
	Update(ctx context.Context, n *BGPNeighbor) (*BGPNeighbor, error)
	Delete(ctx context.Context, id int64) error
}
