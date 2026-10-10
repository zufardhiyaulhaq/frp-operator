package e2e

import "testing"

func TestPortsAreUnique(t *testing.T) {
	seen := map[int]bool{}
	for _, p := range allPorts() {
		if seen[p] {
			t.Fatalf("remote port %d is used by two cases; cases share one frps and run in parallel", p)
		}
		seen[p] = true
	}
}
