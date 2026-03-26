package server

import (
	"testing"
	"time"

	pb "github.com/safedoor/ostui/proto/protocol"
)

func makeTestConfig() *pb.ClientConfig {
	return &pb.ClientConfig{
		Version: "1.2.3",
		Name:    "testhost",
		Config: `{
		"Server":{"Address":"unix:///tmp/osui.sock","LogFile":"/var/log/opensnitchd.log"},
		"DefaultAction":"deny","DefaultDuration":"once",
		"InterceptUnknown":false,"ProcMonitorMethod":"ebpf",
		"LogLevel":0,"LogUTC":true,"LogMicro":false,
		"Firewall":"iptables",
		"Stats":{"MaxEvents":150,"MaxStats":50}
	}`,
	}
}

func TestRegister(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	node := nr.Register("peer:1.2.3.4", cfg, "session1")
	if node == nil {
		t.Fatal("Register returned nil")
	}
	if node.Addr != "peer:1.2.3.4" {
		t.Fatalf("expected addr peer:1.2.3.4, got %s", node.Addr)
	}
	if !node.Online {
		t.Fatal("expected node to be online")
	}
	if node.Config.Name != "testhost" {
		t.Fatalf("expected name testhost, got %s", node.Config.Name)
	}
}

func TestGet(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	nr.Register("peer:1.2.3.4", cfg, "session1")

	node, ok := nr.Get("peer:1.2.3.4")
	if !ok || node == nil {
		t.Fatal("Get returned nil for registered node")
	}

	_, ok = nr.Get("peer:9.9.9.9")
	if ok {
		t.Fatal("Get returned true for non-existent node")
	}
}

func TestUnregister(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	nr.Register("peer:1.2.3.4", cfg, "session1")

	nr.Unregister("peer:1.2.3.4", "session1")
	node, ok := nr.Get("peer:1.2.3.4")
	if !ok {
		t.Fatal("node should still exist after Unregister")
	}
	if node.Online {
		t.Fatal("node should be offline after Unregister")
	}
}

func TestUnregisterWrongSession(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	nr.Register("peer:1.2.3.4", cfg, "session1")

	// Unregistering with a different session peer should be a no-op.
	nr.Unregister("peer:1.2.3.4", "session2")
	node, _ := nr.Get("peer:1.2.3.4")
	if !node.Online {
		t.Fatal("node should remain online when unregistered with wrong session")
	}
}

func TestUnregisterNonExistent(t *testing.T) {
	nr := newNodeRegistry()
	// Should not panic.
	nr.Unregister("peer:9.9.9.9", "session1")
}

func TestSendNotification(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	nr.Register("peer:1.2.3.4", cfg, "session1")

	notif := &pb.Notification{
		Id:   42,
		Type: pb.Action_ENABLE_FIREWALL,
		Data: "test",
	}

	replyCh := nr.SendNotification("peer:1.2.3.4", notif)
	if replyCh == nil {
		t.Fatal("SendNotification returned nil channel")
	}

	// Verify the notification is queued on the node.
	node, _ := nr.Get("peer:1.2.3.4")
	select {
	case queued := <-node.NotifQueue:
		if queued.Id != 42 {
			t.Fatalf("expected notification ID 42, got %d", queued.Id)
		}
		if queued.Type != pb.Action_ENABLE_FIREWALL {
			t.Fatalf("expected ENABLE_FIREWALL, got %v", queued.Type)
		}
	default:
		t.Fatal("expected notification in queue")
	}

	// Verify pending entry exists.
	nr.pendingMu.Lock()
	_, exists := nr.pending[42]
	nr.pendingMu.Unlock()
	if !exists {
		t.Fatal("notification not tracked in pending map")
	}
}

func TestSendNotificationNonExistent(t *testing.T) {
	nr := newNodeRegistry()
	notif := &pb.Notification{Id: 1, Type: pb.Action_ENABLE_FIREWALL}
	replyCh := nr.SendNotification("peer:9.9.9.9", notif)
	if replyCh != nil {
		t.Fatal("expected nil channel for non-existent node")
	}
}

func TestHandleReply(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	nr.Register("peer:1.2.3.4", cfg, "session1")

	notif := &pb.Notification{
		Id:   100,
		Type: pb.Action_ENABLE_FIREWALL,
		Data: "test",
	}
	replyCh := nr.SendNotification("peer:1.2.3.4", notif)

	// Simulate a reply.
	reply := &pb.NotificationReply{
		Id:   100,
		Code: pb.NotificationReplyCode_OK,
		Data: "test",
	}
	nr.HandleReply(reply)

	// The reply should arrive on the channel.
	select {
	case r := <-replyCh:
		if r.Code != pb.NotificationReplyCode_OK {
			t.Fatalf("expected OK reply, got %v", r.Code)
		}
		if r.Data != "test" {
			t.Fatalf("expected data 'test', got %s", r.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reply")
	}

	// Pending entry should be removed after reply.
	nr.pendingMu.Lock()
	_, exists := nr.pending[100]
	nr.pendingMu.Unlock()
	if exists {
		t.Fatal("notification should be removed from pending after reply")
	}
}

func TestHandleReplyUnknownID(t *testing.T) {
	nr := newNodeRegistry()
	// Should not panic.
	reply := &pb.NotificationReply{Id: 999, Code: pb.NotificationReplyCode_OK}
	nr.HandleReply(reply)
}

func TestBroadcast(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	nr.Register("peer:1.2.3.4", cfg, "session1")
	nr.Register("peer:5.6.7.8", cfg, "session2")

	notif := &pb.Notification{
		Id:   200,
		Type: pb.Action_ENABLE_INTERCEPTION,
		Data: "broadcast_test",
	}
	nr.Broadcast(notif)

	// Both nodes should have the notification.
	for _, addr := range []string{"peer:1.2.3.4", "peer:5.6.7.8"} {
		node, _ := nr.Get(addr)
		select {
		case queued := <-node.NotifQueue:
			if queued.Data != "broadcast_test" {
				t.Fatalf("expected broadcast_test, got %s for %s", queued.Data, addr)
			}
		default:
			t.Fatalf("no notification received for %s", addr)
		}
	}
}

func TestBroadcastSkipsOffline(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	nr.Register("peer:1.2.3.4", cfg, "session1")
	nr.Register("peer:5.6.7.8", cfg, "session2")

	// Take one node offline.
	nr.Unregister("peer:5.6.7.8", "session2")

	notif := &pb.Notification{Id: 201, Type: pb.Action_ENABLE_INTERCEPTION}
	nr.Broadcast(notif)

	// Online node should have the notification.
	node1, _ := nr.Get("peer:1.2.3.4")
	select {
	case <-node1.NotifQueue:
		// good
	default:
		t.Fatal("online node should receive broadcast")
	}

	// Offline node should NOT have the notification.
	node2, _ := nr.Get("peer:5.6.7.8")
	select {
	case <-node2.NotifQueue:
		t.Fatal("offline node should not receive broadcast")
	default:
		// good
	}
}

func TestStopNotifications(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	nr.Register("peer:1.2.3.4", cfg, "session1")

	nr.StopNotifications("peer:1.2.3.4")

	node, _ := nr.Get("peer:1.2.3.4")
	select {
	case notif := <-node.NotifQueue:
		if notif.Type != -1 {
			t.Fatalf("expected sentinel notification (Type -1), got %v", notif.Type)
		}
	default:
		t.Fatal("expected sentinel notification in queue")
	}
}

func TestStopNotificationsNonExistent(t *testing.T) {
	nr := newNodeRegistry()
	// Should not panic.
	nr.StopNotifications("peer:9.9.9.9")
}

func TestRegisterDuplicatePeer(t *testing.T) {
	nr := newNodeRegistry()
	cfg1 := makeTestConfig()
	cfg1.Name = "host1"
	nr.Register("peer:1.2.3.4", cfg1, "session1")

	cfg2 := makeTestConfig()
	cfg2.Name = "host2"
	nr.Register("peer:1.2.3.4", cfg2, "session2")

	all := nr.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 node after duplicate register, got %d", len(all))
	}

	node, _ := nr.Get("peer:1.2.3.4")
	if node.Config.Name != "host2" {
		t.Fatalf("expected updated config name host2, got %s", node.Config.Name)
	}
	if node.SessionPeer != "session2" {
		t.Fatalf("expected updated session session2, got %s", node.SessionPeer)
	}
}

func TestAll(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()

	if len(nr.All()) != 0 {
		t.Fatal("expected empty All for new registry")
	}

	nr.Register("peer:1.2.3.4", cfg, "s1")
	nr.Register("peer:5.6.7.8", cfg, "s2")
	nr.Register("peer:10.0.0.1", cfg, "s3")

	all := nr.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(all))
	}

	// Verify all addresses are present.
	addrs := map[string]bool{}
	for _, n := range all {
		addrs[n.Addr] = true
	}
	for _, want := range []string{"peer:1.2.3.4", "peer:5.6.7.8", "peer:10.0.0.1"} {
		if !addrs[want] {
			t.Fatalf("missing node %s in All()", want)
		}
	}
}

func TestParsePeerFormats(t *testing.T) {
	tests := []struct {
		name      string
		peer      string
		wantProto string
		wantAddr  string
	}{
		{"empty peer", "", "unix", "local"},
		{"unix socket", "unix:/tmp/osui.sock", "unix", "local"},
		{"unix at sign", "unix:@", "unix", "local"},
		{"tcp ipv4", "tcp:1.2.3.4:50051", "tcp", "1.2.3.4:50051"},
		{"no colon", "something", "unix", "something"},
		{"ipv4 format", "ipv4:192.168.1.1", "ipv4", "192.168.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto, addr := parsePeer(tt.peer)
			if proto != tt.wantProto {
				t.Errorf("parsePeer(%q) proto = %q, want %q", tt.peer, proto, tt.wantProto)
			}
			if addr != tt.wantAddr {
				t.Errorf("parsePeer(%q) addr = %q, want %q", tt.peer, addr, tt.wantAddr)
			}
		})
	}
}

func TestIsLocalPeer(t *testing.T) {
	tests := []struct {
		proto string
		want  bool
	}{
		{"unix", true},
		{"tcp", false},
		{"ipv4", false},
		{"ipv6", false},
	}
	for _, tt := range tests {
		t.Run(tt.proto, func(t *testing.T) {
			got := isLocalPeer(tt.proto)
			if got != tt.want {
				t.Errorf("isLocalPeer(%q) = %v, want %v", tt.proto, got, tt.want)
			}
		})
	}
}

func TestRegisterOnlineLastSeen(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	before := time.Now()
	node := nr.Register("peer:1.2.3.4", cfg, "s1")
	after := time.Now()

	if !node.Online {
		t.Fatal("newly registered node should be online")
	}
	if node.LastSeen.Before(before) || node.LastSeen.After(after) {
		t.Fatalf("LastSeen %v not within [%v, %v]", node.LastSeen, before, after)
	}
}

func TestRegisterUpdatesLastSeen(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	node := nr.Register("peer:1.2.3.4", cfg, "s1")
	first := node.LastSeen

	time.Sleep(time.Millisecond)
	nr.Register("peer:1.2.3.4", cfg, "s2")
	if !node.LastSeen.After(first) {
		t.Fatal("LastSeen should be updated on re-register")
	}
}

func TestNodeQueueBufferSize(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	node := nr.Register("peer:1.2.3.4", cfg, "s1")

	if cap(node.NotifQueue) != 64 {
		t.Fatalf("expected NotifQueue capacity 64, got %d", cap(node.NotifQueue))
	}
}

func TestSendNotificationQueueCapacity(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	nr.Register("peer:1.2.3.4", cfg, "s1")

	// Verify we can send up to the queue capacity without issues.
	node, _ := nr.Get("peer:1.2.3.4")
	for i := 0; i < cap(node.NotifQueue); i++ {
		notif := &pb.Notification{Id: uint64(i), Type: pb.Action_ENABLE_FIREWALL}
		replyCh := nr.SendNotification("peer:1.2.3.4", notif)
		if replyCh == nil {
			t.Fatalf("SendNotification returned nil on send %d", i)
		}
	}
	// Queue should now be full (64 items).
	if len(node.NotifQueue) != cap(node.NotifQueue) {
		t.Fatalf("expected queue to be full (%d), got %d", cap(node.NotifQueue), len(node.NotifQueue))
	}
}

func TestMultipleHandleReplies(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()
	nr.Register("peer:1.2.3.4", cfg, "s1")

	var channels []chan *pb.NotificationReply
	for i := uint64(1); i <= 5; i++ {
		notif := &pb.Notification{Id: i, Type: pb.Action_ENABLE_FIREWALL}
		ch := nr.SendNotification("peer:1.2.3.4", notif)
		channels = append(channels, ch)
	}

	// Reply to all in reverse order.
	for i := uint64(5); i >= 1; i-- {
		nr.HandleReply(&pb.NotificationReply{Id: i, Code: pb.NotificationReplyCode_OK, Data: "ok"})
	}

	for i, ch := range channels {
		select {
		case r := <-ch:
			if r.Code != pb.NotificationReplyCode_OK {
				t.Fatalf("reply %d: expected OK, got %v", i+1, r.Code)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for reply %d", i+1)
		}
	}
}

func TestBroadcastEmptyRegistry(t *testing.T) {
	nr := newNodeRegistry()
	// Should not panic.
	nr.Broadcast(&pb.Notification{Id: 1, Type: pb.Action_ENABLE_FIREWALL})
}

func TestGetNonExistentNode(t *testing.T) {
	nr := newNodeRegistry()
	node, ok := nr.Get("peer:99.99.99.99")
	if ok || node != nil {
		t.Fatal("expected nil/false for non-existent node")
	}
}

func TestRegisterMultipleNodes(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()

	addrs := []string{
		"peer:1.2.3.4", "peer:5.6.7.8", "peer:10.0.0.1",
		"unix:local", "tcp:192.168.1.1:50051",
	}

	for _, addr := range addrs {
		nr.Register(addr, cfg, "session-"+addr)
	}

	all := nr.All()
	if len(all) != len(addrs) {
		t.Fatalf("expected %d nodes, got %d", len(addrs), len(all))
	}
}

func TestUnregisterThenReRegister(t *testing.T) {
	nr := newNodeRegistry()
	cfg := makeTestConfig()

	nr.Register("peer:1.2.3.4", cfg, "s1")
	nr.Unregister("peer:1.2.3.4", "s1")

	node, _ := nr.Get("peer:1.2.3.4")
	if node.Online {
		t.Fatal("should be offline after unregister")
	}

	// Re-register should bring it back online.
	nr.Register("peer:1.2.3.4", cfg, "s2")
	node, _ = nr.Get("peer:1.2.3.4")
	if !node.Online {
		t.Fatal("should be online after re-register")
	}
	if node.SessionPeer != "s2" {
		t.Fatalf("expected session s2, got %s", node.SessionPeer)
	}
}
