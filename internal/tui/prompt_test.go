package tui

import (
	"fmt"
	"strings"
	"testing"

	pb "github.com/safedoor/ostui/proto/protocol"
)

func sampleConnection() *pb.Connection {
	return &pb.Connection{
		Protocol:    "tcp",
		SrcIp:       "127.0.0.1",
		SrcPort:     12345,
		DstIp:       "93.184.216.34",
		DstHost:     "example.com",
		DstPort:     443,
		UserId:      1000,
		ProcessId:   9876,
		ProcessPath: "/usr/bin/curl",
		ProcessCwd:  "/home/user",
		ProcessArgs: []string{"/usr/bin/curl", "https://example.com"},
	}
}

func TestGenerateRuleName(t *testing.T) {
	conn := sampleConnection()

	name := generateRuleName("allow", conn)
	if name == "" {
		t.Fatal("generateRuleName returned empty string")
	}
	if !strings.HasPrefix(name, "allow-curl-443-") {
		t.Fatalf("expected name to start with 'allow-curl-443-', got %q", name)
	}
}

func TestGenerateRuleNameActions(t *testing.T) {
	conn := sampleConnection()

	actions := []string{"allow", "deny", "reject"}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			name := generateRuleName(action, conn)
			if !strings.HasPrefix(name, action+"-") {
				t.Fatalf("expected name to start with %q-, got %q", action, name)
			}
		})
	}
}

func TestGenerateRuleNameExtractsBasename(t *testing.T) {
	conn := &pb.Connection{
		ProcessPath: "/very/long/path/to/myapp",
		DstPort:     8080,
	}
	name := generateRuleName("deny", conn)
	if !strings.Contains(name, "myapp") {
		t.Fatalf("expected name to contain 'myapp', got %q", name)
	}
	if strings.Contains(name, "/very/long") {
		t.Fatalf("expected name to NOT contain full path, got %q", name)
	}
}

func TestGenerateRuleNameUnique(t *testing.T) {
	conn := sampleConnection()
	names := make(map[string]bool)
	for i := 0; i < 100; i++ {
		name := generateRuleName("allow", conn)
		if names[name] {
			t.Fatalf("duplicate rule name generated: %s", name)
		}
		names[name] = true
	}
}

func TestMatchTargetsCount(t *testing.T) {
	if len(matchTargets) != 7 {
		t.Fatalf("expected 7 match targets, got %d", len(matchTargets))
	}
}

func TestMatchTargetsAllEntries(t *testing.T) {
	conn := sampleConnection()

	expected := []struct {
		label   string
		opType  string
		operand string
		data    string
	}{
		{"from this executable", "simple", "process.path", "/usr/bin/curl"},
		{"from this command line", "simple", "process.command", "/usr/bin/curl https://example.com"},
		{"to this port", "simple", "dest.port", "443"},
		{"to this IP", "simple", "dest.ip", "93.184.216.34"},
		{"to this host", "simple", "dest.host", "example.com"},
		{"from this user", "simple", "user.id", "1000"},
		{"from this PID", "simple", "process.id", "9876"},
	}

	for i, exp := range expected {
		t.Run(exp.label, func(t *testing.T) {
			mt := matchTargets[i]
			if mt.label != exp.label {
				t.Errorf("label: got %q, want %q", mt.label, exp.label)
			}
			if mt.opType != exp.opType {
				t.Errorf("opType: got %q, want %q", mt.opType, exp.opType)
			}
			if mt.operand != exp.operand {
				t.Errorf("operand: got %q, want %q", mt.operand, exp.operand)
			}
			data := mt.dataFn(conn)
			if data != exp.data {
				t.Errorf("data: got %q, want %q", data, exp.data)
			}
		})
	}
}

func TestMatchTargetProcessPath(t *testing.T) {
	conn := &pb.Connection{ProcessPath: "/opt/app/bin/server"}
	mt := matchTargets[0]
	if mt.dataFn(conn) != "/opt/app/bin/server" {
		t.Fatalf("expected /opt/app/bin/server, got %s", mt.dataFn(conn))
	}
}

func TestMatchTargetProcessCommand(t *testing.T) {
	conn := &pb.Connection{ProcessArgs: []string{"/bin/cmd", "--flag", "value"}}
	mt := matchTargets[1]
	got := mt.dataFn(conn)
	if got != "/bin/cmd --flag value" {
		t.Fatalf("expected '/bin/cmd --flag value', got %q", got)
	}
}

func TestMatchTargetProcessCommandEmpty(t *testing.T) {
	conn := &pb.Connection{}
	mt := matchTargets[1]
	got := mt.dataFn(conn)
	if got != "" {
		t.Fatalf("expected empty string for nil args, got %q", got)
	}
}

func TestMatchTargetDestPort(t *testing.T) {
	conn := &pb.Connection{DstPort: 8080}
	mt := matchTargets[2]
	got := mt.dataFn(conn)
	if got != "8080" {
		t.Fatalf("expected '8080', got %q", got)
	}
}

func TestMatchTargetDestIP(t *testing.T) {
	conn := &pb.Connection{DstIp: "10.0.0.1"}
	mt := matchTargets[3]
	if mt.dataFn(conn) != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %s", mt.dataFn(conn))
	}
}

func TestMatchTargetDestHost(t *testing.T) {
	conn := &pb.Connection{DstHost: "api.example.com"}
	mt := matchTargets[4]
	if mt.dataFn(conn) != "api.example.com" {
		t.Fatalf("expected api.example.com, got %s", mt.dataFn(conn))
	}
}

func TestMatchTargetUserID(t *testing.T) {
	conn := &pb.Connection{UserId: 0}
	mt := matchTargets[5]
	if mt.dataFn(conn) != "0" {
		t.Fatalf("expected '0', got %s", mt.dataFn(conn))
	}
	conn.UserId = 65534
	if mt.dataFn(conn) != "65534" {
		t.Fatalf("expected '65534', got %s", mt.dataFn(conn))
	}
}

func TestMatchTargetProcessID(t *testing.T) {
	conn := &pb.Connection{ProcessId: 42}
	mt := matchTargets[6]
	if mt.dataFn(conn) != "42" {
		t.Fatalf("expected '42', got %s", mt.dataFn(conn))
	}
}

func TestDurations(t *testing.T) {
	expected := []string{"once", "30s", "5m", "15m", "30m", "1h", "12h", "until restart", "always"}
	if len(durations) != len(expected) {
		t.Fatalf("expected %d durations, got %d", len(expected), len(durations))
	}
	for i, want := range expected {
		if durations[i] != want {
			t.Errorf("durations[%d] = %q, want %q", i, durations[i], want)
		}
	}
}

func TestNewPromptModel(t *testing.T) {
	m := newPromptModel("deny", "until restart", 30, nil)
	if m.defaultAction != "deny" {
		t.Fatalf("expected default action deny, got %s", m.defaultAction)
	}
	if m.defaultDuration != "until restart" {
		t.Fatalf("expected default duration 'until restart', got %s", m.defaultDuration)
	}
	if m.defaultTimeout != 30 {
		t.Fatalf("expected default timeout 30, got %d", m.defaultTimeout)
	}
	// "until restart" is at index 7.
	if m.selectedDuration != 7 {
		t.Fatalf("expected selectedDuration 7, got %d", m.selectedDuration)
	}
}

func TestNewPromptModelDurationIndex(t *testing.T) {
	tests := []struct {
		dur      string
		wantIdx  int
	}{
		{"once", 0},
		{"30s", 1},
		{"5m", 2},
		{"15m", 3},
		{"30m", 4},
		{"1h", 5},
		{"12h", 6},
		{"until restart", 7},
		{"always", 8},
		{"unknown", 0}, // defaults to 0 if not found
	}
	for _, tt := range tests {
		t.Run(tt.dur, func(t *testing.T) {
			m := newPromptModel("deny", tt.dur, 30, nil)
			if m.selectedDuration != tt.wantIdx {
				t.Errorf("selectedDuration for %q = %d, want %d", tt.dur, m.selectedDuration, tt.wantIdx)
			}
		})
	}
}

func TestMatchTargetDataFnFormats(t *testing.T) {
	conn := &pb.Connection{
		DstPort:   443,
		UserId:    1000,
		ProcessId: 9876,
	}

	// dest.port should format as string.
	portData := matchTargets[2].dataFn(conn)
	if portData != fmt.Sprintf("%d", 443) {
		t.Errorf("dest.port format: got %q, want '443'", portData)
	}

	// user.id should format as string.
	userIDData := matchTargets[5].dataFn(conn)
	if userIDData != fmt.Sprintf("%d", 1000) {
		t.Errorf("user.id format: got %q, want '1000'", userIDData)
	}

	// process.id should format as string.
	pidData := matchTargets[6].dataFn(conn)
	if pidData != fmt.Sprintf("%d", 9876) {
		t.Errorf("process.id format: got %q, want '9876'", pidData)
	}
}
