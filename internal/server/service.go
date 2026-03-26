package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc/peer"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/config"
	"github.com/safedoor/ostui/internal/db"
	pb "github.com/safedoor/ostui/proto/protocol"
)

type service struct {
	pb.UnimplementedUIServer

	cfg   *config.Config
	bus   *bus.EventBus
	db    *db.DB
	nodes *NodeRegistry

	lastStatsMu sync.Mutex
	lastStats   map[string]map[int64]struct{}
}

func newService(cfg *config.Config, eventBus *bus.EventBus, database *db.DB) *service {
	return &service{
		cfg:       cfg,
		bus:       eventBus,
		db:        database,
		nodes:     newNodeRegistry(),
		lastStats: make(map[string]map[int64]struct{}),
	}
}

func (s *service) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingReply, error) {
	peerAddr := peerFromCtx(ctx)
	proto, addr := parsePeer(peerAddr)
	nodeAddr := proto + ":" + addr

	if req.Stats != nil {
		s.processEvents(nodeAddr, req.Stats)
		s.updateStats(req.Stats)

		if err := s.db.UpsertNode(
			nodeAddr, "", req.Stats.DaemonVersion,
			fmt.Sprintf("%d", req.Stats.Uptime),
			fmt.Sprintf("%d", req.Stats.Rules),
			fmt.Sprintf("%d", req.Stats.Connections),
			fmt.Sprintf("%d", req.Stats.Dropped),
			req.Stats.DaemonVersion, "online",
		); err != nil {
			log.Printf("ERROR db.UpsertNode(%s): %v", nodeAddr, err)
		}

		select {
		case s.bus.StatsUpdate <- bus.StatsUpdate{Peer: nodeAddr, Stats: req.Stats}:
		default:
			log.Fatalf("FATAL: StatsUpdate channel full, update for %s cannot be delivered", nodeAddr)
		}
	}

	return &pb.PingReply{Id: req.Id}, nil
}

func (s *service) AskRule(ctx context.Context, conn *pb.Connection) (*pb.Rule, error) {
	peerAddr := peerFromCtx(ctx)
	proto, addr := parsePeer(peerAddr)
	nodeAddr := proto + ":" + addr

	log.Printf("AskRule from %s: %s -> %s:%d (%s)",
		nodeAddr, extractProcName(conn.ProcessPath), conn.DstHost, conn.DstPort, conn.Protocol)

	responseCh := make(chan *pb.Rule, 1)

	prompt := bus.PromptRequest{
		Connection: conn,
		Peer:       peerAddr,
		NodeAddr:   nodeAddr,
		IsLocal:    isLocalPeer(proto),
		ResponseCh: responseCh,
	}

	select {
	case s.bus.PromptReq <- prompt:
		log.Printf("AskRule prompt sent to TUI")
	case <-ctx.Done():
		log.Printf("AskRule context done before prompt sent, applying default")
		return s.defaultRule(conn), nil
	}

	timeout := time.Duration(s.cfg.DefaultTimeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case rule := <-responseCh:
		if rule != nil {
			log.Printf("AskRule response: %s (action=%s duration=%s)", rule.Name, rule.Action, rule.Duration)
			return rule, nil
		}
		log.Printf("AskRule got nil response, applying default")
		return s.defaultRule(conn), nil
	case <-timer.C:
		log.Printf("AskRule timeout (%v), applying default action=%s", timeout, s.cfg.DefaultAction)
		return s.defaultRule(conn), nil
	case <-ctx.Done():
		log.Printf("AskRule context cancelled, applying default")
		return s.defaultRule(conn), nil
	}
}

func (s *service) Subscribe(ctx context.Context, clientConfig *pb.ClientConfig) (*pb.ClientConfig, error) {
	peerAddr := peerFromCtx(ctx)
	proto, addr := parsePeer(peerAddr)
	nodeAddr := proto + ":" + addr

	log.Printf("Subscribe from %s (name=%s version=%s rules=%d)",
		nodeAddr, clientConfig.Name, clientConfig.Version, len(clientConfig.Rules))

	s.nodes.Register(nodeAddr, clientConfig, peerAddr)

	if len(clientConfig.Rules) > 0 {
		rows := make([]db.RuleRow, len(clientConfig.Rules))
		for i, r := range clientConfig.Rules {
			rows[i] = ruleToRow(r)
		}
		if err := s.db.BulkInsertRules(nodeAddr, rows); err != nil {
			log.Printf("ERROR db.BulkInsertRules(%s, %d rules): %v", nodeAddr, len(rows), err)
		}
	}

	if err := s.db.UpsertNode(nodeAddr, clientConfig.Name, clientConfig.Version,
		"0", fmt.Sprintf("%d", len(clientConfig.Rules)),
		"0", "0", clientConfig.Version, "online"); err != nil {
		log.Printf("ERROR db.UpsertNode(%s): %v", nodeAddr, err)
	}

	select {
	case s.bus.NodeEvent <- bus.NodeEvent{Type: bus.NodeAdded, Addr: nodeAddr, Config: clientConfig}:
	default:
		log.Fatalf("FATAL: NodeEvent channel full, NodeAdded for %s cannot be delivered", nodeAddr)
	}

	s.overwriteConfig(clientConfig)

	return clientConfig, nil
}

func (s *service) Notifications(stream pb.UI_NotificationsServer) error {
	peerAddr := peerFromCtx(stream.Context())
	proto, addr := parsePeer(peerAddr)
	nodeAddr := proto + ":" + addr

	node, ok := s.nodes.Get(nodeAddr)
	if !ok {
		log.Printf("ERROR Notifications: unknown node %s", nodeAddr)
		return fmt.Errorf("unknown node: %s", nodeAddr)
	}

	log.Printf("Notifications stream opened for %s", nodeAddr)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			reply, err := stream.Recv()
			if err != nil {
				log.Printf("Notifications recv from %s ended: %v", nodeAddr, err)
				return
			}
			s.nodes.HandleReply(reply)
			select {
			case s.bus.NotifReply <- bus.NotifReply{Peer: nodeAddr, Reply: reply}:
			default:
				log.Fatalf("FATAL: NotifReply channel full, reply id=%d from %s cannot be delivered", reply.Id, nodeAddr)
			}
		}
	}()

	for {
		select {
		case notif := <-node.NotifQueue:
			if notif == nil || notif.Type == -1 {
				log.Printf("Notifications stream closing for %s (sentinel)", nodeAddr)
				return nil
			}
			log.Printf("Sending notification type=%s to %s", notif.Type, nodeAddr)
			if err := stream.Send(notif); err != nil {
				log.Printf("ERROR notification send to %s: %v", nodeAddr, err)
				return err
			}
		case <-done:
			s.handleDisconnect(nodeAddr, peerAddr)
			return nil
		case <-stream.Context().Done():
			s.handleDisconnect(nodeAddr, peerAddr)
			return stream.Context().Err()
		}
	}
}

func (s *service) PostAlert(ctx context.Context, alert *pb.Alert) (*pb.MsgResponse, error) {
	peerAddr := peerFromCtx(ctx)
	proto, addr := parsePeer(peerAddr)
	nodeAddr := proto + ":" + addr

	body := ""
	switch d := alert.Data.(type) {
	case *pb.Alert_Text:
		body = d.Text
	case *pb.Alert_Proc:
		body = fmt.Sprintf("process: %s (pid %d)", d.Proc.Path, d.Proc.Pid)
	case *pb.Alert_Conn:
		body = fmt.Sprintf("connection: %s:%d -> %s:%d", d.Conn.SrcIp, d.Conn.SrcPort, d.Conn.DstIp, d.Conn.DstPort)
	case *pb.Alert_Rule:
		body = fmt.Sprintf("rule: %s", d.Rule.Name)
	case *pb.Alert_Fwrule:
		body = fmt.Sprintf("fw rule: %s", d.Fwrule.Description)
	}

	log.Printf("PostAlert from %s: type=%s priority=%s what=%s body=%q",
		nodeAddr, alert.Type, alert.Priority, alert.What, body)

	if err := s.db.InsertAlert(
		nodeAddr,
		alert.Type.String(),
		alert.Action.String(),
		alert.Priority.String(),
		alert.What.String(),
		body,
	); err != nil {
		log.Printf("ERROR db.InsertAlert(%s): %v", nodeAddr, err)
	}

	select {
	case s.bus.AlertEvent <- bus.AlertEvent{Peer: nodeAddr, Alert: alert}:
	default:
		log.Fatalf("FATAL: AlertEvent channel full, alert from %s cannot be delivered: %s", nodeAddr, body)
	}

	return &pb.MsgResponse{Id: alert.Id}, nil
}

// --- helpers ---

func (s *service) processEvents(nodeAddr string, stats *pb.Statistics) {
	if len(stats.Events) == 0 {
		return
	}

	s.lastStatsMu.Lock()
	seen, exists := s.lastStats[nodeAddr]
	if !exists {
		seen = make(map[int64]struct{})
		s.lastStats[nodeAddr] = seen
	}
	s.lastStatsMu.Unlock()

	for _, event := range stats.Events {
		if _, dup := seen[event.Unixnano]; dup {
			continue
		}
		seen[event.Unixnano] = struct{}{}

		conn := event.Connection
		rule := event.Rule
		if conn == nil || rule == nil {
			continue
		}

		if err := s.db.InsertConnection(
			event.Time, nodeAddr, rule.Action, conn.Protocol,
			conn.SrcIp, fmt.Sprintf("%d", conn.SrcPort),
			conn.DstIp, conn.DstHost, fmt.Sprintf("%d", conn.DstPort),
			fmt.Sprintf("%d", conn.UserId), fmt.Sprintf("%d", conn.ProcessId),
			conn.ProcessPath, db.FormatArgs(conn.ProcessArgs), conn.ProcessCwd,
			rule.Name,
		); err != nil {
			log.Printf("ERROR db.InsertConnection: %v", err)
		}
	}

	newSeen := make(map[int64]struct{}, len(stats.Events))
	for _, e := range stats.Events {
		newSeen[e.Unixnano] = struct{}{}
	}
	s.lastStatsMu.Lock()
	s.lastStats[nodeAddr] = newSeen
	s.lastStatsMu.Unlock()
}

func (s *service) updateStats(stats *pb.Statistics) {
	for _, pair := range []struct {
		table string
		data  map[string]uint64
	}{
		{"hosts", stats.ByHost},
		{"procs", stats.ByExecutable},
		{"addrs", stats.ByAddress},
		{"ports", stats.ByPort},
		{"users", stats.ByUid},
	} {
		if err := s.db.UpsertStats(pair.table, pair.data); err != nil {
			log.Printf("ERROR db.UpsertStats(%s): %v", pair.table, err)
		}
	}
}

func (s *service) handleDisconnect(nodeAddr, sessionPeer string) {
	log.Printf("Node disconnected: %s", nodeAddr)
	s.nodes.Unregister(nodeAddr, sessionPeer)
	if err := s.db.SetNodeStatus(nodeAddr, "offline"); err != nil {
		log.Printf("ERROR db.SetNodeStatus(%s, offline): %v", nodeAddr, err)
	}

	select {
	case s.bus.NodeEvent <- bus.NodeEvent{Type: bus.NodeRemoved, Addr: nodeAddr}:
	default:
		log.Fatalf("FATAL: NodeEvent channel full, NodeRemoved for %s cannot be delivered", nodeAddr)
	}
}

func (s *service) defaultRule(conn *pb.Connection) *pb.Rule {
	return &pb.Rule{
		Created:  time.Now().Unix(),
		Name:     fmt.Sprintf("ostui-default-%d", time.Now().UnixNano()),
		Enabled:  true,
		Action:   s.cfg.DefaultAction,
		Duration: "once",
		Operator: &pb.Operator{
			Type:    "simple",
			Operand: "process.path",
			Data:    conn.ProcessPath,
		},
	}
}

func (s *service) overwriteConfig(config *pb.ClientConfig) {
	if config.Config == "" {
		return
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(config.Config), &parsed); err != nil {
		log.Printf("WARN overwriteConfig: failed to parse daemon config JSON: %v", err)
		return
	}
	parsed["DefaultAction"] = s.cfg.DefaultAction
	parsed["DefaultDuration"] = s.cfg.DefaultDuration
	b, err := json.Marshal(parsed)
	if err != nil {
		log.Printf("WARN overwriteConfig: failed to marshal config: %v", err)
		return
	}
	config.Config = string(b)
}

func ruleToRow(r *pb.Rule) db.RuleRow {
	opType, opOperand, opData, opSensitive := "", "", "", "false"
	if r.Operator != nil {
		opType = r.Operator.Type
		opOperand = r.Operator.Operand
		opData = r.Operator.Data
		if r.Operator.Sensitive {
			opSensitive = "true"
		}
	}
	enabled := "false"
	if r.Enabled {
		enabled = "true"
	}
	precedence := "false"
	if r.Precedence {
		precedence = "true"
	}
	nolog := "false"
	if r.Nolog {
		nolog = "true"
	}
	created := ""
	if r.Created > 0 {
		created = time.Unix(r.Created, 0).Format(time.RFC3339)
	}
	return db.RuleRow{
		Name: r.Name, Enabled: enabled, Precedence: precedence,
		Action: r.Action, Duration: r.Duration, OpType: opType,
		OpSensitive: opSensitive, OpOperand: opOperand, OpData: opData,
		Description: r.Description, Nolog: nolog, Created: created,
	}
}

func peerFromCtx(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return ""
	}
	return p.Addr.Network() + ":" + p.Addr.String()
}

// extractProcName is a simple basename for logging (avoid import cycle with tui).
func extractProcName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
