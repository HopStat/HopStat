package domain

import "testing"

func TestNodeCanExecute(t *testing.T) {
	node := &Node{
		EnabledCmds: []CommandType{CmdPing, CmdTraceroute},
	}

	if !node.CanExecute(CmdPing) {
		t.Error("should be able to execute ping")
	}
	if !node.CanExecute(CmdTraceroute) {
		t.Error("should be able to execute traceroute")
	}
	if node.CanExecute(CmdBGPRoute) {
		t.Error("should not be able to execute bgp_route")
	}
	if node.CanExecute(CmdASPath) {
		t.Error("should not be able to execute as_path")
	}
}

func TestNodeCanExecuteEmpty(t *testing.T) {
	node := &Node{EnabledCmds: []CommandType{}}
	if node.CanExecute(CmdPing) {
		t.Error("empty cmds should not allow any command")
	}
}

func TestNodeTypeConstants(t *testing.T) {
	if NodeTypeStandalone != "standalone" {
		t.Error("NodeTypeStandalone mismatch")
	}
	if NodeTypeLGNode != "lg_node" {
		t.Error("NodeTypeLGNode mismatch")
	}
}

func TestQueryStatusConstants(t *testing.T) {
	if StatusPending != "pending" {
		t.Error("StatusPending mismatch")
	}
	if StatusRunning != "running" {
		t.Error("StatusRunning mismatch")
	}
	if StatusDone != "done" {
		t.Error("StatusDone mismatch")
	}
	if StatusError != "error" {
		t.Error("StatusError mismatch")
	}
}

func TestSeverityConstants(t *testing.T) {
	if SeverityReject != "reject" {
		t.Error("SeverityReject mismatch")
	}
	if SeverityWarning != "warning" {
		t.Error("SeverityWarning mismatch")
	}
	if SeverityInfo != "info" {
		t.Error("SeverityInfo mismatch")
	}
	if SeveritySuccess != "success" {
		t.Error("SeveritySuccess mismatch")
	}
}

func TestSentinelErrors(t *testing.T) {
	errs := map[string]error{
		"ErrNodeNotFound":    ErrNodeNotFound,
		"ErrCommandDisabled": ErrCommandDisabled,
		"ErrInvalidTarget":   ErrInvalidTarget,
		"ErrTimeout":         ErrTimeout,
		"ErrCircuitOpen":     ErrCircuitOpen,
		"ErrQueryPoolFull":   ErrQueryPoolFull,
		"ErrUnauthorized":    ErrUnauthorized,
	}
	for name, err := range errs {
		if err == nil {
			t.Errorf("%s is nil", name)
		}
	}
}
