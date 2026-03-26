package tui

import (
	"testing"

	"github.com/safedoor/ostui/internal/bus"
	pb "github.com/safedoor/ostui/proto/protocol"
)

func TestNotifyRuleRequestReturnsCmd(t *testing.T) {
	req := &bus.PromptRequest{
		Connection: &pb.Connection{
			ProcessPath: "/usr/bin/curl",
			DstHost:     "example.com",
			DstIp:       "93.184.216.34",
			DstPort:     443,
			Protocol:    "tcp",
			ProcessId:   1234,
			UserId:      1000,
		},
		NodeAddr: "unix:local",
	}

	cmd := notifyRuleRequest(req)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from notifyRuleRequest")
	}
}

func TestNotifyRuleRequestFallsBackToIP(t *testing.T) {
	req := &bus.PromptRequest{
		Connection: &pb.Connection{
			ProcessPath: "/usr/bin/curl",
			DstHost:     "", // empty host
			DstIp:       "10.0.0.1",
			DstPort:     80,
			Protocol:    "tcp",
		},
		NodeAddr: "unix:local",
	}

	// Should not panic when DstHost is empty (falls back to DstIp).
	cmd := notifyRuleRequest(req)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
}
