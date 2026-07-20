package diagnostics

import "testing"

func TestRequireLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:6062", "[::1]:6062", "localhost:6062"} {
		if err := requireLoopback(addr); err != nil {
			t.Fatalf("requireLoopback(%q): %v", addr, err)
		}
	}
}

func TestRequireLoopbackRejectsPublicAndWildcardAddresses(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:6062", ":6062", "192.0.2.1:6062"} {
		if err := requireLoopback(addr); err == nil {
			t.Fatalf("requireLoopback(%q) unexpectedly succeeded", addr)
		}
	}
}
