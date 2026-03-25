package bus

import (
	"testing"
	"time"

	pb "github.com/safedoor/ostui/proto/protocol"
)

func TestNew(t *testing.T) {
	b := New()
	if b == nil {
		t.Fatal("New returned nil")
	}

	// Verify all channels are created.
	if b.StatsUpdate == nil {
		t.Fatal("StatsUpdate channel is nil")
	}
	if b.PromptReq == nil {
		t.Fatal("PromptReq channel is nil")
	}
	if b.AlertEvent == nil {
		t.Fatal("AlertEvent channel is nil")
	}
	if b.NodeEvent == nil {
		t.Fatal("NodeEvent channel is nil")
	}
	if b.NotifOut == nil {
		t.Fatal("NotifOut channel is nil")
	}
	if b.NotifReply == nil {
		t.Fatal("NotifReply channel is nil")
	}
	if b.Done == nil {
		t.Fatal("Done channel is nil")
	}
}

func TestNewBufferSizes(t *testing.T) {
	b := New()

	tests := []struct {
		name     string
		capacity int
		channel  interface{ Len() int }
	}{
		{"StatsUpdate", 16, nil},
		{"PromptReq", 1, nil},
		{"AlertEvent", 64, nil},
		{"NodeEvent", 8, nil},
		{"NotifOut", 64, nil},
		{"NotifReply", 64, nil},
		{"Done", 0, nil},
	}

	caps := []int{
		cap(b.StatsUpdate),
		cap(b.PromptReq),
		cap(b.AlertEvent),
		cap(b.NodeEvent),
		cap(b.NotifOut),
		cap(b.NotifReply),
		cap(b.Done),
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if caps[i] != tt.capacity {
				t.Errorf("cap(%s) = %d, want %d", tt.name, caps[i], tt.capacity)
			}
		})
	}
}

func TestStatsUpdate(t *testing.T) {
	b := New()

	stats := &pb.Statistics{
		DaemonVersion: "v1.0.0",
		Rules:         10,
		Uptime:        3600,
		Connections:   100,
		Dropped:       5,
	}

	update := StatsUpdate{
		Peer:  "peer:1.2.3.4",
		Stats: stats,
	}

	// Send.
	select {
	case b.StatsUpdate <- update:
	case <-time.After(time.Second):
		t.Fatal("timed out sending StatsUpdate")
	}

	// Receive.
	select {
	case received := <-b.StatsUpdate:
		if received.Peer != "peer:1.2.3.4" {
			t.Fatalf("expected peer peer:1.2.3.4, got %s", received.Peer)
		}
		if received.Stats.DaemonVersion != "v1.0.0" {
			t.Fatalf("expected version v1.0.0, got %s", received.Stats.DaemonVersion)
		}
		if received.Stats.Connections != 100 {
			t.Fatalf("expected 100 connections, got %d", received.Stats.Connections)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out receiving StatsUpdate")
	}
}

func TestPromptReq(t *testing.T) {
	b := New()

	conn := &pb.Connection{
		Protocol:    "tcp",
		DstIp:       "8.8.8.8",
		DstHost:     "dns.google",
		DstPort:     53,
		ProcessPath: "/usr/bin/dig",
	}

	responseCh := make(chan *pb.Rule, 1)
	req := PromptRequest{
		Connection: conn,
		Peer:       "peer:1.2.3.4",
		IsLocal:    true,
		ResponseCh: responseCh,
	}

	// Send.
	select {
	case b.PromptReq <- req:
	case <-time.After(time.Second):
		t.Fatal("timed out sending PromptReq")
	}

	// Receive.
	select {
	case received := <-b.PromptReq:
		if received.Peer != "peer:1.2.3.4" {
			t.Fatalf("expected peer peer:1.2.3.4, got %s", received.Peer)
		}
		if !received.IsLocal {
			t.Fatal("expected IsLocal true")
		}
		if received.Connection.DstHost != "dns.google" {
			t.Fatalf("expected dst host dns.google, got %s", received.Connection.DstHost)
		}
		if received.ResponseCh == nil {
			t.Fatal("ResponseCh should not be nil")
		}

		// Respond.
		rule := &pb.Rule{Name: "test-rule", Action: "allow"}
		received.ResponseCh <- rule
	case <-time.After(time.Second):
		t.Fatal("timed out receiving PromptReq")
	}

	// Verify response.
	select {
	case rule := <-responseCh:
		if rule.Name != "test-rule" {
			t.Fatalf("expected rule name test-rule, got %s", rule.Name)
		}
		if rule.Action != "allow" {
			t.Fatalf("expected action allow, got %s", rule.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out receiving response")
	}
}

func TestPromptReqBufferSize(t *testing.T) {
	b := New()

	// PromptReq has buffer size 1, so one send should not block.
	b.PromptReq <- PromptRequest{Peer: "test"}

	// Second send should block (buffer full).
	select {
	case b.PromptReq <- PromptRequest{Peer: "test2"}:
		t.Fatal("expected second send to block on buffer-1 channel")
	default:
		// expected
	}
}

func TestDone(t *testing.T) {
	b := New()

	// Done channel should be unbuffered.
	if cap(b.Done) != 0 {
		t.Fatalf("expected Done capacity 0, got %d", cap(b.Done))
	}

	// Closing Done should unblock receivers.
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(b.Done)
	}()

	select {
	case <-b.Done:
		// success
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Done signal")
	}

	// Multiple receives after close should work.
	select {
	case <-b.Done:
	default:
		t.Fatal("Done channel should remain closed")
	}
}

func TestAlertEvent(t *testing.T) {
	b := New()

	alert := AlertEvent{
		Peer: "peer:1.2.3.4",
		Alert: &pb.Alert{
			Id: 1,
		},
	}

	select {
	case b.AlertEvent <- alert:
	case <-time.After(time.Second):
		t.Fatal("timed out sending AlertEvent")
	}

	select {
	case received := <-b.AlertEvent:
		if received.Peer != "peer:1.2.3.4" {
			t.Fatalf("expected peer peer:1.2.3.4, got %s", received.Peer)
		}
		if received.Alert.Id != 1 {
			t.Fatalf("expected alert ID 1, got %d", received.Alert.Id)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out receiving AlertEvent")
	}
}

func TestNodeEvent(t *testing.T) {
	b := New()

	tests := []struct {
		name string
		typ  NodeEventType
	}{
		{"NodeAdded", NodeAdded},
		{"NodeRemoved", NodeRemoved},
		{"NodeUpdated", NodeUpdated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := NodeEvent{
				Type: tt.typ,
				Addr: "peer:1.2.3.4",
				Config: &pb.ClientConfig{
					Name:    "testhost",
					Version: "v1.0",
				},
			}
			b.NodeEvent <- evt

			received := <-b.NodeEvent
			if received.Type != tt.typ {
				t.Errorf("expected type %v, got %v", tt.typ, received.Type)
			}
			if received.Addr != "peer:1.2.3.4" {
				t.Errorf("expected addr peer:1.2.3.4, got %s", received.Addr)
			}
		})
	}
}

func TestNotifOut(t *testing.T) {
	b := New()

	notif := OutgoingNotification{
		NodeAddr: "peer:1.2.3.4",
		Notification: &pb.Notification{
			Id:   42,
			Type: pb.Action_ENABLE_FIREWALL,
			Data: "test",
		},
	}

	b.NotifOut <- notif

	received := <-b.NotifOut
	if received.NodeAddr != "peer:1.2.3.4" {
		t.Fatalf("expected node addr peer:1.2.3.4, got %s", received.NodeAddr)
	}
	if received.Notification.Id != 42 {
		t.Fatalf("expected notification ID 42, got %d", received.Notification.Id)
	}
}

func TestNotifReply(t *testing.T) {
	b := New()

	reply := NotifReply{
		Peer: "peer:1.2.3.4",
		Reply: &pb.NotificationReply{
			Id:   42,
			Code: pb.NotificationReplyCode_OK,
			Data: "success",
		},
	}

	b.NotifReply <- reply

	received := <-b.NotifReply
	if received.Peer != "peer:1.2.3.4" {
		t.Fatalf("expected peer peer:1.2.3.4, got %s", received.Peer)
	}
	if received.Reply.Code != pb.NotificationReplyCode_OK {
		t.Fatalf("expected OK, got %v", received.Reply.Code)
	}
}

func TestNodeEventTypeConstants(t *testing.T) {
	if NodeAdded != 0 {
		t.Fatalf("expected NodeAdded=0, got %d", NodeAdded)
	}
	if NodeRemoved != 1 {
		t.Fatalf("expected NodeRemoved=1, got %d", NodeRemoved)
	}
	if NodeUpdated != 2 {
		t.Fatalf("expected NodeUpdated=2, got %d", NodeUpdated)
	}
}
