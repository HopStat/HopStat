package domain

type QuickQuery struct {
	ID        int64  `json:"id"`
	Command   string `json:"command"`
	Name      string `json:"name"`
	Target    string `json:"target"`
	NodeID    *int64 `json:"node_id,omitempty"`
	SortOrder int    `json:"sort_order"`
	Active    bool   `json:"active"`
}
