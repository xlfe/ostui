package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/db"
	pb "github.com/safedoor/ostui/proto/protocol"
)

func TestRespondPersistsRuleToDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	m := newPromptModel("deny", "always", 30, d)

	responseCh := make(chan *pb.Rule, 1)
	m.Show(&bus.PromptRequest{
		Connection: sampleConnection(),
		Peer:       "unix:@",
		NodeAddr:   "unix:local",
		IsLocal:    true,
		ResponseCh: responseCh,
	})

	cmd := m.respond("allow")

	// Verify rule was sent to response channel.
	select {
	case rule := <-responseCh:
		if rule == nil {
			t.Fatal("expected non-nil rule on response channel")
		}
		if rule.Action != "allow" {
			t.Fatalf("expected action 'allow', got %q", rule.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for rule on response channel")
	}

	// Verify rule was persisted to DB.
	rules, err := d.GetRules("")
	if err != nil {
		t.Fatalf("GetRules failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule in DB, got %d", len(rules))
	}
	if rules[0].Action != "allow" {
		t.Fatalf("expected DB rule action 'allow', got %q", rules[0].Action)
	}
	if rules[0].Node != "unix:local" {
		t.Fatalf("expected DB rule node 'unix:local', got %q", rules[0].Node)
	}
	if rules[0].Enabled != "true" {
		t.Fatalf("expected DB rule enabled 'true', got %q", rules[0].Enabled)
	}
	if rules[0].OpOperand != "process.path" {
		t.Fatalf("expected DB rule operand 'process.path', got %q", rules[0].OpOperand)
	}
	if rules[0].OpData != "/usr/bin/curl" {
		t.Fatalf("expected DB rule opdata '/usr/bin/curl', got %q", rules[0].OpData)
	}

	// Verify cmd produces ruleCreatedMsg.
	if cmd == nil {
		t.Fatal("expected non-nil cmd from respond")
	}
	msg := cmd()
	if _, ok := msg.(ruleCreatedMsg); !ok {
		t.Fatalf("expected ruleCreatedMsg, got %T", msg)
	}

	// Verify prompt is deactivated.
	if m.active {
		t.Fatal("expected prompt to be deactivated after respond")
	}
}

func TestRespondDenyAction(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	m := newPromptModel("deny", "once", 30, d)

	responseCh := make(chan *pb.Rule, 1)
	m.Show(&bus.PromptRequest{
		Connection: sampleConnection(),
		NodeAddr:   "unix:local",
		ResponseCh: responseCh,
	})

	m.respond("deny")

	rule := <-responseCh
	if rule.Action != "deny" {
		t.Fatalf("expected action 'deny', got %q", rule.Action)
	}

	rules, err := d.GetRules("")
	if err != nil {
		t.Fatalf("GetRules failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Action != "deny" {
		t.Fatalf("expected DB rule action 'deny', got %q", rules[0].Action)
	}
}

func TestRespondRejectAction(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	m := newPromptModel("deny", "once", 30, d)

	responseCh := make(chan *pb.Rule, 1)
	m.Show(&bus.PromptRequest{
		Connection: sampleConnection(),
		NodeAddr:   "unix:local",
		ResponseCh: responseCh,
	})

	m.respond("reject")

	rule := <-responseCh
	if rule.Action != "reject" {
		t.Fatalf("expected action 'reject', got %q", rule.Action)
	}

	rules, err := d.GetRules("")
	if err != nil {
		t.Fatalf("GetRules failed: %v", err)
	}
	if rules[0].Action != "reject" {
		t.Fatalf("expected DB rule action 'reject', got %q", rules[0].Action)
	}
}

func TestRespondWithDifferentTargets(t *testing.T) {
	for i, mt := range matchTargets {
		t.Run(mt.label, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "test.db")
			d, err := db.Open(dbPath)
			if err != nil {
				t.Fatalf("failed to open DB: %v", err)
			}
			t.Cleanup(func() { d.Close() })

			m := newPromptModel("deny", "always", 30, d)

			responseCh := make(chan *pb.Rule, 1)
			m.Show(&bus.PromptRequest{
				Connection: sampleConnection(),
				NodeAddr:   "unix:local",
				ResponseCh: responseCh,
			})
			// Check only the target under test.
			m.targetChecked = [matchTargetCount]bool{}
			m.targetChecked[i] = true

			m.respond("allow")

			rules, err := d.GetRules("")
			if err != nil {
				t.Fatalf("GetRules failed: %v", err)
			}
			if len(rules) != 1 {
				t.Fatalf("expected 1 rule, got %d", len(rules))
			}
			if rules[0].OpOperand != mt.operand {
				t.Fatalf("expected operand %q, got %q", mt.operand, rules[0].OpOperand)
			}
		})
	}
}

func TestRespondWithDifferentDurations(t *testing.T) {
	for i, dur := range durations {
		t.Run(dur, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "test.db")
			d, err := db.Open(dbPath)
			if err != nil {
				t.Fatalf("failed to open DB: %v", err)
			}
			t.Cleanup(func() { d.Close() })

			m := newPromptModel("deny", dur, 30, d)

			responseCh := make(chan *pb.Rule, 1)
			m.Show(&bus.PromptRequest{
				Connection: sampleConnection(),
				NodeAddr:   "unix:local",
				ResponseCh: responseCh,
			})
			m.selectedDuration = i

			m.respond("allow")

			rules, err := d.GetRules("")
			if err != nil {
				t.Fatalf("GetRules failed: %v", err)
			}
			if rules[0].Duration != dur {
				t.Fatalf("expected duration %q, got %q", dur, rules[0].Duration)
			}
		})
	}
}

func TestRespondNilRequestIsNoop(t *testing.T) {
	m := newPromptModel("deny", "once", 30, nil)
	m.active = true
	m.request = nil

	cmd := m.respond("allow")

	if cmd != nil {
		t.Fatal("expected nil cmd for nil request")
	}
	if m.active {
		t.Fatal("expected prompt to be deactivated")
	}
}

func TestRespondNilResponseChIsNoop(t *testing.T) {
	m := newPromptModel("deny", "once", 30, nil)
	m.active = true
	m.request = &bus.PromptRequest{
		Connection: sampleConnection(),
		ResponseCh: nil,
	}

	cmd := m.respond("allow")

	if cmd != nil {
		t.Fatal("expected nil cmd for nil ResponseCh")
	}
}

func TestRespondNilDBSkipsPersistence(t *testing.T) {
	m := newPromptModel("deny", "always", 30, nil)

	responseCh := make(chan *pb.Rule, 1)
	m.Show(&bus.PromptRequest{
		Connection: sampleConnection(),
		NodeAddr:   "unix:local",
		ResponseCh: responseCh,
	})

	// Should not panic even with nil DB.
	cmd := m.respond("allow")

	// Rule should still be sent to response channel.
	select {
	case rule := <-responseCh:
		if rule.Action != "allow" {
			t.Fatalf("expected 'allow', got %q", rule.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for rule")
	}

	// cmd should still be non-nil (ruleCreatedMsg).
	if cmd == nil {
		t.Fatal("expected non-nil cmd even with nil DB")
	}
}

func TestRespondUsesNodeAddrFromRequest(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	m := newPromptModel("deny", "always", 30, d)

	responseCh := make(chan *pb.Rule, 1)
	m.Show(&bus.PromptRequest{
		Connection: sampleConnection(),
		NodeAddr:   "tcp:10.0.0.1:50051",
		ResponseCh: responseCh,
	})

	m.respond("allow")
	<-responseCh

	rules, err := d.GetRules("")
	if err != nil {
		t.Fatalf("GetRules failed: %v", err)
	}
	if rules[0].Node != "tcp:10.0.0.1:50051" {
		t.Fatalf("expected node 'tcp:10.0.0.1:50051', got %q", rules[0].Node)
	}
}

func TestRespondDefaultsNodeToUnixLocal(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	m := newPromptModel("deny", "always", 30, d)

	responseCh := make(chan *pb.Rule, 1)
	m.Show(&bus.PromptRequest{
		Connection: sampleConnection(),
		NodeAddr:   "",
		ResponseCh: responseCh,
	})

	m.respond("allow")
	<-responseCh

	rules, err := d.GetRules("")
	if err != nil {
		t.Fatalf("GetRules failed: %v", err)
	}
	if rules[0].Node != "unix:local" {
		t.Fatalf("expected node 'unix:local', got %q", rules[0].Node)
	}
}

func TestRespondCompoundRuleProcessAndHost(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	m := newPromptModel("deny", "always", 30, d)

	responseCh := make(chan *pb.Rule, 1)
	m.Show(&bus.PromptRequest{
		Connection: sampleConnection(),
		NodeAddr:   "unix:local",
		ResponseCh: responseCh,
	})
	// Check both "from this executable" (0) and "to this host" (4).
	m.targetChecked = [matchTargetCount]bool{}
	m.targetChecked[0] = true // process.path
	m.targetChecked[4] = true // dest.host

	m.respond("allow")

	// Verify the protobuf rule sent to daemon has list operator.
	rule := <-responseCh
	if rule.Operator.Type != "list" {
		t.Fatalf("expected list operator, got %q", rule.Operator.Type)
	}
	if rule.Operator.Operand != "list" {
		t.Fatalf("expected operand 'list', got %q", rule.Operator.Operand)
	}
	if len(rule.Operator.List) != 2 {
		t.Fatalf("expected 2 sub-operators, got %d", len(rule.Operator.List))
	}
	if rule.Operator.List[0].Operand != "process.path" {
		t.Fatalf("expected first sub-operator process.path, got %q", rule.Operator.List[0].Operand)
	}
	if rule.Operator.List[0].Data != "/usr/bin/curl" {
		t.Fatalf("expected first sub-operator data '/usr/bin/curl', got %q", rule.Operator.List[0].Data)
	}
	if rule.Operator.List[1].Operand != "dest.host" {
		t.Fatalf("expected second sub-operator dest.host, got %q", rule.Operator.List[1].Operand)
	}
	if rule.Operator.List[1].Data != "example.com" {
		t.Fatalf("expected second sub-operator data 'example.com', got %q", rule.Operator.List[1].Data)
	}

	// Verify DB has list type with JSON data.
	rules, err := d.GetRules("")
	if err != nil {
		t.Fatalf("GetRules failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].OpType != "list" {
		t.Fatalf("expected DB opType 'list', got %q", rules[0].OpType)
	}
	if rules[0].OpOperand != "list" {
		t.Fatalf("expected DB opOperand 'list', got %q", rules[0].OpOperand)
	}
	// OpData should be valid JSON array.
	if !strings.HasPrefix(rules[0].OpData, "[") {
		t.Fatalf("expected DB opData to be JSON array, got %q", rules[0].OpData)
	}
}

func TestRespondCompoundRuleThreeConditions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	m := newPromptModel("deny", "always", 30, d)

	responseCh := make(chan *pb.Rule, 1)
	m.Show(&bus.PromptRequest{
		Connection: sampleConnection(),
		NodeAddr:   "unix:local",
		ResponseCh: responseCh,
	})
	// Check executable + host + port.
	m.targetChecked = [matchTargetCount]bool{}
	m.targetChecked[0] = true // process.path
	m.targetChecked[2] = true // dest.port
	m.targetChecked[4] = true // dest.host

	m.respond("deny")

	rule := <-responseCh
	if len(rule.Operator.List) != 3 {
		t.Fatalf("expected 3 sub-operators, got %d", len(rule.Operator.List))
	}
	if rule.Action != "deny" {
		t.Fatalf("expected action 'deny', got %q", rule.Action)
	}
}

func TestRespondSingleCheckStaysSimple(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	m := newPromptModel("deny", "always", 30, d)

	responseCh := make(chan *pb.Rule, 1)
	m.Show(&bus.PromptRequest{
		Connection: sampleConnection(),
		NodeAddr:   "unix:local",
		ResponseCh: responseCh,
	})
	// Only executable checked — should produce simple rule, not list.
	m.targetChecked = [matchTargetCount]bool{}
	m.targetChecked[0] = true

	m.respond("allow")

	rule := <-responseCh
	if rule.Operator.Type != "simple" {
		t.Fatalf("single target should produce simple operator, got %q", rule.Operator.Type)
	}
	if rule.Operator.Operand != "process.path" {
		t.Fatalf("expected operand 'process.path', got %q", rule.Operator.Operand)
	}
	if len(rule.Operator.List) != 0 {
		t.Fatalf("simple rule should have no sub-operators, got %d", len(rule.Operator.List))
	}
}
