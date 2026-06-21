package driver

import (
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestNewDriverUnknownType(t *testing.T) {
	node := &domain.Node{Type: "unknown"}
	if _, err := NewDriver(node, nil); err == nil {
		t.Fatal("expected error for unknown node type")
	}
}
