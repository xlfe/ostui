package server

import (
	"sync"
	"time"

	pb "github.com/safedoor/ostui/proto/protocol"
)

// NodeState holds per-node runtime state.
type NodeState struct {
	Addr         string
	Config       *pb.ClientConfig
	NotifQueue   chan *pb.Notification // outgoing notifications to this daemon
	LastSeen     time.Time
	Online       bool
	SessionPeer  string // gRPC context.peer() for this session

	mu sync.Mutex
}

// NodeRegistry manages connected daemon nodes.
type NodeRegistry struct {
	mu    sync.RWMutex
	nodes map[string]*NodeState

	// Track sent notification IDs → response channels.
	pendingMu sync.Mutex
	pending   map[uint64]chan *pb.NotificationReply
}

func newNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes:   make(map[string]*NodeState),
		pending: make(map[uint64]chan *pb.NotificationReply),
	}
}

// Register adds or updates a node.
func (nr *NodeRegistry) Register(addr string, config *pb.ClientConfig, sessionPeer string) *NodeState {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	if node, ok := nr.nodes[addr]; ok {
		node.mu.Lock()
		node.Config = config
		node.LastSeen = time.Now()
		node.Online = true
		node.SessionPeer = sessionPeer
		node.mu.Unlock()
		return node
	}

	node := &NodeState{
		Addr:        addr,
		Config:      config,
		NotifQueue:  make(chan *pb.Notification, 64),
		LastSeen:    time.Now(),
		Online:      true,
		SessionPeer: sessionPeer,
	}
	nr.nodes[addr] = node
	return node
}

// Unregister marks a node as offline.
func (nr *NodeRegistry) Unregister(addr, sessionPeer string) {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	node, ok := nr.nodes[addr]
	if !ok {
		return
	}
	node.mu.Lock()
	defer node.mu.Unlock()

	// Only unregister if this is the current session (handle reconnections).
	if node.SessionPeer != sessionPeer {
		return
	}
	node.Online = false
}

// Get returns a node by address.
func (nr *NodeRegistry) Get(addr string) (*NodeState, bool) {
	nr.mu.RLock()
	defer nr.mu.RUnlock()
	n, ok := nr.nodes[addr]
	return n, ok
}

// All returns all nodes.
func (nr *NodeRegistry) All() []*NodeState {
	nr.mu.RLock()
	defer nr.mu.RUnlock()
	result := make([]*NodeState, 0, len(nr.nodes))
	for _, n := range nr.nodes {
		result = append(result, n)
	}
	return result
}

// SendNotification queues a notification for a specific node and returns a channel
// that will receive the reply.
func (nr *NodeRegistry) SendNotification(addr string, notif *pb.Notification) chan *pb.NotificationReply {
	nr.mu.RLock()
	node, ok := nr.nodes[addr]
	nr.mu.RUnlock()
	if !ok {
		return nil
	}

	replyCh := make(chan *pb.NotificationReply, 1)
	nr.pendingMu.Lock()
	nr.pending[notif.Id] = replyCh
	nr.pendingMu.Unlock()

	select {
	case node.NotifQueue <- notif:
	default:
		// Queue full, drop oldest.
		select {
		case <-node.NotifQueue:
		default:
		}
		node.NotifQueue <- notif
	}

	return replyCh
}

// Broadcast sends a notification to all online nodes.
func (nr *NodeRegistry) Broadcast(notif *pb.Notification) {
	nr.mu.RLock()
	defer nr.mu.RUnlock()
	for _, node := range nr.nodes {
		if node.Online {
			select {
			case node.NotifQueue <- notif:
			default:
			}
		}
	}
}

// HandleReply matches a reply to a pending notification.
func (nr *NodeRegistry) HandleReply(reply *pb.NotificationReply) {
	nr.pendingMu.Lock()
	ch, ok := nr.pending[reply.Id]
	if ok {
		delete(nr.pending, reply.Id)
	}
	nr.pendingMu.Unlock()

	if ok && ch != nil {
		select {
		case ch <- reply:
		default:
		}
	}
}

// StopNotifications sends a sentinel to close the notification stream for a node.
func (nr *NodeRegistry) StopNotifications(addr string) {
	nr.mu.RLock()
	node, ok := nr.nodes[addr]
	nr.mu.RUnlock()
	if !ok {
		return
	}

	sentinel := &pb.Notification{Type: -1}
	select {
	case node.NotifQueue <- sentinel:
	default:
	}
}
