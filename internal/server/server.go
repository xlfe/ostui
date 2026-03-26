package server

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"time"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/config"
	"github.com/safedoor/ostui/internal/db"
	pb "github.com/safedoor/ostui/proto/protocol"
)

type Server struct {
	cfg      *config.Config
	bus      *bus.EventBus
	db       *db.DB
	grpc     *grpc.Server
	svc      *service
	listener net.Listener
}

func New(cfg *config.Config, eventBus *bus.EventBus, database *db.DB) (*Server, error) {
	s := &Server{
		cfg: cfg,
		bus: eventBus,
		db:  database,
	}

	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(cfg.MaxMsgLength),
		grpc.MaxSendMsgSize(cfg.MaxMsgLength),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    10 * time.Second,
			Timeout: 30 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             3 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	s.grpc = grpc.NewServer(opts...)
	s.svc = newService(cfg, eventBus, database)
	pb.RegisterUIServer(s.grpc, s.svc)

	return s, nil
}

// RouteNotifications forwards TUI-originated notifications to the correct node queues.
func (s *Server) RouteNotifications() {
	for {
		select {
		case <-s.bus.Done:
			return
		case out := <-s.bus.NotifOut:
			if out.NodeAddr == "" {
				// Broadcast to all nodes.
				s.svc.nodes.Broadcast(out.Notification)
			} else {
				node, ok := s.svc.nodes.Get(out.NodeAddr)
				if !ok {
					log.Fatalf("FATAL: RouteNotifications: node %s not found, notification type=%s cannot be delivered", out.NodeAddr, out.Notification.Type)
				}
				select {
				case node.NotifQueue <- out.Notification:
				default:
					log.Fatalf("FATAL: RouteNotifications: node %s NotifQueue full, notification type=%s dropped", out.NodeAddr, out.Notification.Type)
				}
			}
		}
	}
}

func (s *Server) Start() error {
	proto, addr := s.cfg.SocketProto()

	if proto == "unix" {
		// Remove stale socket file if it exists.
		os.Remove(addr)
	}

	var err error
	s.listener, err = net.Listen(proto, addr)
	if err != nil {
		return fmt.Errorf("listen %s://%s: %w", proto, addr, err)
	}

	log.Printf("gRPC server listening on %s://%s", proto, addr)
	return s.grpc.Serve(s.listener)
}

func (s *Server) Stop() {
	log.Println("gRPC server stopping")
	s.grpc.GracefulStop()
	if s.listener != nil {
		s.listener.Close()
	}
	// Cleanup unix socket.
	proto, addr := s.cfg.SocketProto()
	if proto == "unix" {
		os.Remove(addr)
	}
}

// parsePeer extracts protocol and address from a gRPC peer string.
// peerFromCtx returns "network:address", e.g. "unix:@" or "tcp:1.2.3.4:50051".
func parsePeer(peer string) (proto, addr string) {
	if peer == "" {
		return "unix", "local"
	}
	parts := strings.SplitN(peer, ":", 2)
	if len(parts) < 2 {
		return "unix", peer
	}
	proto = parts[0]
	addr = parts[1]
	// Normalize unix socket peers to "local" since they all share the same socket.
	if proto == "unix" {
		addr = "local"
	}
	return proto, addr
}

// isLocalPeer checks if the peer is a local connection.
func isLocalPeer(proto string) bool {
	return proto == "unix"
}
