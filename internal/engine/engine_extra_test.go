package engine

import (
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestSanitizeErrorMsgNonEmpty(t *testing.T) {
	if msg := SanitizeErrorMsg(domain.ErrRateLimited); msg == "" {
		t.Fatal("expected non-empty sanitized message")
	}
	if msg := SanitizeErrorMsg(domain.ErrInvalidTarget); msg == "" {
		t.Fatal("expected non-empty sanitized message for invalid target")
	}
}

func TestClassifyErrorInvalidNodeConfig(t *testing.T) {
	if got := ClassifyError(domain.ErrInvalidNodeConfig); got != "INTERNAL_ERROR" {
		t.Errorf("got %q, want INTERNAL_ERROR", got)
	}
}

func TestSanitizeErrorMsgEarlyStop(t *testing.T) {
	if msg := SanitizeErrorMsg(domain.ErrEarlyStop); msg == "command timed out" {
		t.Fatalf("early stop should not be reported as command timeout")
	}
}
