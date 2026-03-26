package bus

import (
	pb "github.com/safedoor/ostui/proto/protocol"
)

// PromptRequest is sent from the gRPC AskRule handler to the TUI.
// The gRPC goroutine blocks on ResponseCh until the user decides.
type PromptRequest struct {
	Connection *pb.Connection
	Peer       string
	NodeAddr   string
	IsLocal    bool
	ResponseCh chan *pb.Rule
}

// StatsUpdate carries per-ping statistics from a daemon node.
type StatsUpdate struct {
	Peer  string
	Stats *pb.Statistics
}

// NodeEventType describes what happened to a node.
type NodeEventType int

const (
	NodeAdded NodeEventType = iota
	NodeRemoved
	NodeUpdated
)

// NodeEvent signals a node lifecycle change.
type NodeEvent struct {
	Type   NodeEventType
	Addr   string
	Config *pb.ClientConfig
}

// AlertEvent carries an alert from a daemon.
type AlertEvent struct {
	Peer  string
	Alert *pb.Alert
}

// OutgoingNotification queues a notification for delivery to a specific daemon node.
type OutgoingNotification struct {
	NodeAddr     string
	Notification *pb.Notification
}

// NotifReply carries a daemon's reply to a previously sent notification.
type NotifReply struct {
	Peer  string
	Reply *pb.NotificationReply
}

// EventBus is the channel-based bridge between gRPC server goroutines and the TUI.
type EventBus struct {
	StatsUpdate  chan StatsUpdate        // Ping → dashboard
	PromptReq    chan PromptRequest      // AskRule → prompt modal
	AlertEvent   chan AlertEvent         // PostAlert → alert view
	NodeEvent    chan NodeEvent          // Subscribe/disconnect → node view
	NotifOut     chan OutgoingNotification // TUI → gRPC Notifications stream
	NotifReply   chan NotifReply         // gRPC Notifications → TUI (for tracking)
	Done         chan struct{}           // shutdown signal
}

// New creates an EventBus with appropriately sized channel buffers.
func New() *EventBus {
	return &EventBus{
		StatsUpdate: make(chan StatsUpdate, 16),
		PromptReq:   make(chan PromptRequest, 1),
		AlertEvent:  make(chan AlertEvent, 64),
		NodeEvent:   make(chan NodeEvent, 8),
		NotifOut:    make(chan OutgoingNotification, 64),
		NotifReply:  make(chan NotifReply, 64),
		Done:        make(chan struct{}),
	}
}
