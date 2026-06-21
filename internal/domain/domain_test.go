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
}

func TestNodeCanExecuteEmpty(t *testing.T) {
	node := &Node{EnabledCmds: []CommandType{}}
	if node.CanExecute(CmdPing) {
		t.Error("empty cmds should not allow any command")
	}
}

func TestNormalizeEnabledCmds(t *testing.T) {
	got := NormalizeEnabledCmds([]CommandType{
		CmdPing, "as_path", CmdTraceroute, "mtr", CmdPing, CmdBGPRoute,
	})
	want := []CommandType{CmdPing, CmdTraceroute, CmdBGPRoute}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestNormalizeEnabledCmdStrings(t *testing.T) {
	got := NormalizeEnabledCmdStrings([]string{"as_path", "ping", "unknown", "traceroute"})
	if len(got) != 2 || got[0] != CmdPing || got[1] != CmdTraceroute {
		t.Fatalf("unexpected result: %v", got)
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
	if NormalizeSeverity(SeverityAlert) != SeverityAlert {
		t.Error("SeverityAlert mismatch")
	}
	if NormalizeSeverity("reject") != SeverityAlert {
		t.Error("legacy reject should normalize to alert")
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

func TestValidNodeCommands(t *testing.T) {
	cmds := ValidNodeCommands()
	want := []CommandType{CmdPing, CmdTraceroute, CmdBGPRoute}
	if len(cmds) != len(want) {
		t.Fatalf("got %v, want %v", cmds, want)
	}
	for i := range want {
		if cmds[i] != want[i] {
			t.Fatalf("got %v, want %v", cmds, want)
		}
	}
}

func TestDefaultEnabledCmds(t *testing.T) {
	if got := DefaultEnabledCmds(); len(got) != 3 {
		t.Fatalf("DefaultEnabledCmds() = %v", got)
	}
}

func TestIsSupportedNodeCommand(t *testing.T) {
	if !IsSupportedNodeCommand("ping") {
		t.Fatal("ping should be supported")
	}
	if IsSupportedNodeCommand("mtr") {
		t.Fatal("mtr should not be supported")
	}
}

func TestSentinelErrors(t *testing.T) {
	errs := map[string]error{
		"ErrNodeNotFound":    ErrNodeNotFound,
		"ErrCommandDisabled": ErrCommandDisabled,
		"ErrInvalidTarget":   ErrInvalidTarget,
		"ErrDNSNotFound":     ErrDNSNotFound,
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
