package domain

import (
	"context"
	"testing"
)

func TestWithOnLineGetOnLine(t *testing.T) {
	ctx := context.Background()
	if GetOnLine(ctx) != nil {
		t.Fatal("expected nil callback on bare context")
	}

	var lines []string
	ctx = WithOnLine(ctx, func(line string) {
		lines = append(lines, line)
	})

	fn := GetOnLine(ctx)
	if fn == nil {
		t.Fatal("expected callback on enriched context")
	}
	fn("hello")
	if len(lines) != 1 || lines[0] != "hello" {
		t.Fatalf("callback lines = %v, want [hello]", lines)
	}

	ctx = context.WithValue(ctx, onLineCtxKey, "not a func")
	if GetOnLine(ctx) != nil {
		t.Fatal("expected nil when context value has wrong type")
	}
}
