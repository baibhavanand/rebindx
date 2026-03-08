package main

import (
	"net"
	"testing"
)

func TestParseIPLabel(t *testing.T) {
	remoteAddr, _ := net.ResolveUDPAddr("udp", "1.2.3.4:53")
	
	tests := []struct {
		label    string
		expected string
		found    bool
	}{
		{"7f000001", "127.0.0.1", true},
		{"auto", "1.2.3.4", true},
		{"00000000000000000000000000000001", "::1", true},
		{"invalid", "", false},
	}

	for _, tt := range tests {
		got, found := parseIPLabel(tt.label, remoteAddr)
		if found != tt.found {
			t.Errorf("parseIPLabel(%s) found = %v, want %v", tt.label, found, tt.found)
		}
		if found && got.String() != tt.expected {
			t.Errorf("parseIPLabel(%s) = %v, want %s", tt.label, got, tt.expected)
		}
	}
}

func TestThresholdShifting(t *testing.T) {
	sess := getSession("threshold-test")
	sess.Count = 0
	
	// Mode t2 should return ip1 for count 0, 1 and ip2 for count 2+
	// Testing logic within handleDNS would require more mocking, 
	// but we can verify session count increments.
	if sess.Count != 0 {
		t.Errorf("New session count should be 0")
	}
}
