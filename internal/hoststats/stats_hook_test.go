package hoststats

import "testing"

func TestCPUCoresFallback(t *testing.T) {
	old := numCPUFunc
	numCPUFunc = func() int { return 0 }
	t.Cleanup(func() { numCPUFunc = old })
	if got := cpuCores(); got != 1 {
		t.Fatalf("cpuCores() = %d", got)
	}
}
