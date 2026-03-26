package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/config"
	"github.com/safedoor/ostui/internal/db"
	pb "github.com/safedoor/ostui/proto/protocol"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func testConfig() *config.Config {
	return &config.Config{
		DefaultAction:   "deny",
		DefaultDuration: "once",
		DefaultTimeout:  5,
	}
}

func TestDefaultRule(t *testing.T) {
	d := openTestDB(t)
	eb := bus.New()
	cfg := testConfig()
	svc := newService(cfg, eb, d)

	conn := &pb.Connection{
		ProcessPath: "/usr/bin/curl",
		DstHost:     "example.com",
		DstPort:     443,
	}
	rule := svc.defaultRule(conn)

	if rule.Action != "deny" {
		t.Fatalf("expected default action 'deny', got %q", rule.Action)
	}
	if rule.Duration != "once" {
		t.Fatalf("expected duration 'once', got %q", rule.Duration)
	}
	if rule.Operator == nil {
		t.Fatal("expected operator, got nil")
	}
	if rule.Operator.Operand != "process.path" {
		t.Fatalf("expected operand 'process.path', got %q", rule.Operator.Operand)
	}
	if rule.Operator.Data != "/usr/bin/curl" {
		t.Fatalf("expected data '/usr/bin/curl', got %q", rule.Operator.Data)
	}
	if rule.Name == "" {
		t.Fatal("expected non-empty rule name")
	}
	if !rule.Enabled {
		t.Fatal("expected rule to be enabled")
	}
}

func TestRuleToRow(t *testing.T) {
	rule := &pb.Rule{
		Name:        "test-rule",
		Enabled:     true,
		Precedence:  false,
		Nolog:       true,
		Action:      "allow",
		Duration:    "always",
		Description: "a test rule",
		Created:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
		Operator: &pb.Operator{
			Type:      "simple",
			Operand:   "process.path",
			Data:      "/usr/bin/app",
			Sensitive: true,
		},
	}

	row := ruleToRow(rule)

	if row.Name != "test-rule" {
		t.Fatalf("expected name 'test-rule', got %q", row.Name)
	}
	if row.Enabled != "true" {
		t.Fatalf("expected enabled 'true', got %q", row.Enabled)
	}
	if row.Precedence != "false" {
		t.Fatalf("expected precedence 'false', got %q", row.Precedence)
	}
	if row.Nolog != "true" {
		t.Fatalf("expected nolog 'true', got %q", row.Nolog)
	}
	if row.Action != "allow" {
		t.Fatalf("expected action 'allow', got %q", row.Action)
	}
	if row.Duration != "always" {
		t.Fatalf("expected duration 'always', got %q", row.Duration)
	}
	if row.OpType != "simple" {
		t.Fatalf("expected opType 'simple', got %q", row.OpType)
	}
	if row.OpOperand != "process.path" {
		t.Fatalf("expected operand 'process.path', got %q", row.OpOperand)
	}
	if row.OpData != "/usr/bin/app" {
		t.Fatalf("expected opData '/usr/bin/app', got %q", row.OpData)
	}
	if row.OpSensitive != "true" {
		t.Fatalf("expected opSensitive 'true', got %q", row.OpSensitive)
	}
	if row.Description != "a test rule" {
		t.Fatalf("expected description 'a test rule', got %q", row.Description)
	}
	if row.Created == "" {
		t.Fatal("expected non-empty created timestamp")
	}
}

func TestRuleToRowNilOperator(t *testing.T) {
	rule := &pb.Rule{
		Name:    "no-op-rule",
		Enabled: false,
		Action:  "deny",
	}
	row := ruleToRow(rule)

	if row.OpType != "" {
		t.Fatalf("expected empty opType, got %q", row.OpType)
	}
	if row.OpOperand != "" {
		t.Fatalf("expected empty operand, got %q", row.OpOperand)
	}
	if row.Enabled != "false" {
		t.Fatalf("expected enabled 'false', got %q", row.Enabled)
	}
}

func TestRuleToRowZeroCreated(t *testing.T) {
	rule := &pb.Rule{
		Name:    "zero-ts",
		Enabled: true,
		Action:  "allow",
		Created: 0,
	}
	row := ruleToRow(rule)

	if row.Created != "" {
		t.Fatalf("expected empty created for zero timestamp, got %q", row.Created)
	}
}

func TestExtractProcName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/usr/bin/curl", "curl"},
		{"/opt/app/server", "server"},
		{"binary", "binary"},
		{"", ""},
		{"/", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractProcName(tt.path)
			if got != tt.want {
				t.Errorf("extractProcName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestParsePeerFromService(t *testing.T) {
	tests := []struct {
		peer      string
		wantProto string
		wantAddr  string
	}{
		{"unix:@", "unix", "local"},
		{"unix:/tmp/sock", "unix", "local"},
		{"tcp:1.2.3.4:50051", "tcp", "1.2.3.4:50051"},
		{"", "unix", "local"},
		{"something", "unix", "something"},
	}
	for _, tt := range tests {
		t.Run(tt.peer, func(t *testing.T) {
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

func TestProcessEventsDeduplication(t *testing.T) {
	d := openTestDB(t)
	eb := bus.New()
	cfg := testConfig()
	svc := newService(cfg, eb, d)

	conn := &pb.Connection{
		ProcessPath: "/usr/bin/curl",
		DstIp:       "1.2.3.4",
		DstHost:     "example.com",
		DstPort:     443,
		Protocol:    "tcp",
		SrcIp:       "127.0.0.1",
		SrcPort:     12345,
		UserId:      1000,
		ProcessId:   9876,
		ProcessCwd:  "/home/user",
	}
	rule := &pb.Rule{Name: "test-rule", Action: "allow"}

	stats := &pb.Statistics{
		Events: []*pb.Event{
			{
				Time:       time.Now().Format(time.RFC3339),
				Unixnano:  12345,
				Connection: conn,
				Rule:       rule,
			},
		},
	}

	// Process events twice — second time should be deduplicated.
	svc.processEvents("unix:local", stats)
	svc.processEvents("unix:local", stats)

	conns, err := d.GetConnections(100)
	if err != nil {
		t.Fatalf("GetConnections failed: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection (deduplication), got %d", len(conns))
	}
}

func TestProcessEventsNewBatchReplacesOld(t *testing.T) {
	d := openTestDB(t)
	eb := bus.New()
	cfg := testConfig()
	svc := newService(cfg, eb, d)

	conn := &pb.Connection{
		ProcessPath: "/usr/bin/curl",
		DstIp:       "1.2.3.4",
		DstPort:     443,
		Protocol:    "tcp",
		SrcIp:       "127.0.0.1",
		SrcPort:     12345,
		UserId:      1000,
		ProcessId:   9876,
		ProcessCwd:  "/home",
	}
	rule := &pb.Rule{Name: "rule1", Action: "allow"}

	stats1 := &pb.Statistics{
		Events: []*pb.Event{
			{Time: time.Now().Format(time.RFC3339), Unixnano: 100, Connection: conn, Rule: rule},
		},
	}
	svc.processEvents("unix:local", stats1)

	// New batch with different nanotime.
	conn2 := &pb.Connection{
		ProcessPath: "/usr/bin/wget",
		DstIp:       "5.6.7.8",
		DstPort:     80,
		Protocol:    "tcp",
		SrcIp:       "127.0.0.1",
		SrcPort:     12346,
		UserId:      1000,
		ProcessId:   9877,
		ProcessCwd:  "/home",
	}
	stats2 := &pb.Statistics{
		Events: []*pb.Event{
			{Time: time.Now().Format(time.RFC3339), Unixnano: 200, Connection: conn2, Rule: rule},
		},
	}
	svc.processEvents("unix:local", stats2)

	conns, err := d.GetConnections(100)
	if err != nil {
		t.Fatalf("GetConnections failed: %v", err)
	}
	if len(conns) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(conns))
	}
}

func TestUpdateStats(t *testing.T) {
	d := openTestDB(t)
	eb := bus.New()
	cfg := testConfig()
	svc := newService(cfg, eb, d)

	stats := &pb.Statistics{
		ByHost:       map[string]uint64{"example.com": 10, "google.com": 5},
		ByExecutable: map[string]uint64{"/usr/bin/curl": 7},
		ByAddress:    map[string]uint64{"1.2.3.4": 3},
		ByPort:       map[string]uint64{"443": 15},
		ByUid:        map[string]uint64{"1000": 20},
	}

	svc.updateStats(stats)

	// Verify hosts.
	rows, err := d.GetTopStats("hosts", 10)
	if err != nil {
		t.Fatalf("GetTopStats(hosts) failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 host rows, got %d", len(rows))
	}

	// Verify procs.
	rows, err = d.GetTopStats("procs", 10)
	if err != nil {
		t.Fatalf("GetTopStats(procs) failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 proc row, got %d", len(rows))
	}
	if rows[0].What != "/usr/bin/curl" {
		t.Fatalf("expected proc '/usr/bin/curl', got %q", rows[0].What)
	}
}

func TestOverwriteConfig(t *testing.T) {
	d := openTestDB(t)
	eb := bus.New()
	cfg := &config.Config{
		DefaultAction:   "allow",
		DefaultDuration: "always",
	}
	svc := newService(cfg, eb, d)

	clientCfg := &pb.ClientConfig{
		Config: `{"DefaultAction":"deny","DefaultDuration":"once","Other":"value"}`,
	}
	svc.overwriteConfig(clientCfg)

	// Should overwrite DefaultAction and DefaultDuration with our config values.
	if clientCfg.Config == "" {
		t.Fatal("expected non-empty config")
	}
	// The config should now contain our overwritten values.
	// Since it's JSON, just check the string contains the expected values.
	if !containsStr(clientCfg.Config, `"DefaultAction":"allow"`) {
		t.Fatalf("expected DefaultAction to be overwritten to 'allow', got: %s", clientCfg.Config)
	}
	if !containsStr(clientCfg.Config, `"DefaultDuration":"always"`) {
		t.Fatalf("expected DefaultDuration to be overwritten to 'always', got: %s", clientCfg.Config)
	}
}

func TestOverwriteConfigEmptyString(t *testing.T) {
	d := openTestDB(t)
	eb := bus.New()
	cfg := testConfig()
	svc := newService(cfg, eb, d)

	clientCfg := &pb.ClientConfig{Config: ""}
	svc.overwriteConfig(clientCfg)
	// Should not panic or modify anything.
	if clientCfg.Config != "" {
		t.Fatalf("expected empty config to remain empty, got %q", clientCfg.Config)
	}
}

func TestOverwriteConfigInvalidJSON(t *testing.T) {
	d := openTestDB(t)
	eb := bus.New()
	cfg := testConfig()
	svc := newService(cfg, eb, d)

	clientCfg := &pb.ClientConfig{Config: "not json"}
	svc.overwriteConfig(clientCfg)
	// Should not panic; config should remain unchanged.
	if clientCfg.Config != "not json" {
		t.Fatalf("expected config to remain unchanged on invalid JSON, got %q", clientCfg.Config)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
