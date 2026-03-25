package config

import (
	"strings"
	"testing"
)

func TestSocketProtoUnixLong(t *testing.T) {
	c := &Config{Socket: "unix:///tmp/osui.sock"}
	proto, addr := c.SocketProto()
	if proto != "unix" {
		t.Fatalf("expected proto unix, got %s", proto)
	}
	if addr != "/tmp/osui.sock" {
		t.Fatalf("expected addr /tmp/osui.sock, got %s", addr)
	}
}

func TestSocketProtoUnixShort(t *testing.T) {
	c := &Config{Socket: "unix:/var/run/osui.sock"}
	proto, addr := c.SocketProto()
	if proto != "unix" {
		t.Fatalf("expected proto unix, got %s", proto)
	}
	if addr != "/var/run/osui.sock" {
		t.Fatalf("expected addr /var/run/osui.sock, got %s", addr)
	}
}

func TestSocketProtoTCP(t *testing.T) {
	c := &Config{Socket: "0.0.0.0:50051"}
	proto, addr := c.SocketProto()
	if proto != "tcp" {
		t.Fatalf("expected proto tcp, got %s", proto)
	}
	if addr != "0.0.0.0:50051" {
		t.Fatalf("expected addr 0.0.0.0:50051, got %s", addr)
	}
}

func TestSocketProtoTCPLocalhost(t *testing.T) {
	c := &Config{Socket: "127.0.0.1:50051"}
	proto, addr := c.SocketProto()
	if proto != "tcp" {
		t.Fatalf("expected proto tcp, got %s", proto)
	}
	if addr != "127.0.0.1:50051" {
		t.Fatalf("expected addr 127.0.0.1:50051, got %s", addr)
	}
}

func TestSocketProtoFormats(t *testing.T) {
	tests := []struct {
		name      string
		socket    string
		wantProto string
		wantAddr  string
	}{
		{"unix triple slash", "unix:///tmp/osui.sock", "unix", "/tmp/osui.sock"},
		{"unix single colon", "unix:/tmp/osui.sock", "unix", "/tmp/osui.sock"},
		{"unix var run", "unix:///var/run/opensnitchd.sock", "unix", "/var/run/opensnitchd.sock"},
		{"tcp all interfaces", "0.0.0.0:50051", "tcp", "0.0.0.0:50051"},
		{"tcp localhost", "127.0.0.1:50051", "tcp", "127.0.0.1:50051"},
		{"tcp ipv4", "192.168.1.1:50051", "tcp", "192.168.1.1:50051"},
		{"short string", "x", "tcp", "x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Socket: tt.socket}
			proto, addr := c.SocketProto()
			if proto != tt.wantProto {
				t.Errorf("proto = %q, want %q", proto, tt.wantProto)
			}
			if addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tt.wantAddr)
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	c := &Config{
		Socket:          "unix:///tmp/osui.sock",
		DefaultAction:   "deny",
		DefaultDuration: "until restart",
		DefaultTimeout:  30,
		MaxMsgLength:    4194304,
	}

	if c.DefaultAction != "deny" {
		t.Fatalf("expected default action deny, got %s", c.DefaultAction)
	}
	if c.DefaultDuration != "until restart" {
		t.Fatalf("expected default duration 'until restart', got %s", c.DefaultDuration)
	}
	if c.DefaultTimeout != 30 {
		t.Fatalf("expected default timeout 30, got %d", c.DefaultTimeout)
	}
	if c.MaxMsgLength != 4194304 {
		t.Fatalf("expected max msg length 4194304, got %d", c.MaxMsgLength)
	}
}

func TestDefaultsActions(t *testing.T) {
	actions := []string{"allow", "deny", "reject"}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			c := &Config{DefaultAction: action}
			if c.DefaultAction != action {
				t.Errorf("expected action %s, got %s", action, c.DefaultAction)
			}
		})
	}
}

func TestDefaultsDurations(t *testing.T) {
	durations := []string{"once", "30s", "5m", "15m", "30m", "1h", "12h", "until restart", "always"}
	for _, dur := range durations {
		t.Run(dur, func(t *testing.T) {
			c := &Config{DefaultDuration: dur}
			if c.DefaultDuration != dur {
				t.Errorf("expected duration %s, got %s", dur, c.DefaultDuration)
			}
		})
	}
}

func TestString(t *testing.T) {
	c := &Config{
		Socket:          "unix:///tmp/osui.sock",
		DBFile:          "/tmp/test.db",
		DefaultAction:   "deny",
		DefaultDuration: "until restart",
		DefaultTimeout:  30,
	}

	s := c.String()
	if s == "" {
		t.Fatal("String() returned empty")
	}

	expected := []string{"unix:///tmp/osui.sock", "/tmp/test.db", "deny", "until restart", "30"}
	for _, sub := range expected {
		if !strings.Contains(s, sub) {
			t.Errorf("String() = %q, expected to contain %q", s, sub)
		}
	}
}

func TestStringFormat(t *testing.T) {
	c := &Config{
		Socket:          "0.0.0.0:50051",
		DBFile:          "/data/ostui.db",
		DefaultAction:   "allow",
		DefaultDuration: "always",
		DefaultTimeout:  60,
	}

	s := c.String()

	want := "socket=0.0.0.0:50051 db=/data/ostui.db action=allow duration=always timeout=60"
	if s != want {
		t.Errorf("String() = %q, want %q", s, want)
	}
}

func TestConfigFields(t *testing.T) {
	c := &Config{
		Socket:          "unix:///tmp/test.sock",
		DBFile:          "/tmp/test.db",
		LogFile:         "/var/log/test.log",
		LogLevel:        "debug",
		DefaultAction:   "reject",
		DefaultDuration: "5m",
		DefaultTimeout:  45,
		MaxMsgLength:    8388608,
	}

	if c.Socket != "unix:///tmp/test.sock" {
		t.Error("Socket field")
	}
	if c.DBFile != "/tmp/test.db" {
		t.Error("DBFile field")
	}
	if c.LogFile != "/var/log/test.log" {
		t.Error("LogFile field")
	}
	if c.LogLevel != "debug" {
		t.Error("LogLevel field")
	}
	if c.DefaultAction != "reject" {
		t.Error("DefaultAction field")
	}
	if c.DefaultDuration != "5m" {
		t.Error("DefaultDuration field")
	}
	if c.DefaultTimeout != 45 {
		t.Error("DefaultTimeout field")
	}
	if c.MaxMsgLength != 8388608 {
		t.Error("MaxMsgLength field")
	}
}

func TestSocketProtoEdgeCases(t *testing.T) {
	// Very short socket strings should fall through to TCP.
	c := &Config{Socket: "ab"}
	proto, _ := c.SocketProto()
	if proto != "tcp" {
		t.Errorf("expected tcp for short socket, got %s", proto)
	}

	// Empty socket.
	c = &Config{Socket: ""}
	proto, addr := c.SocketProto()
	if proto != "tcp" {
		t.Errorf("expected tcp for empty socket, got %s", proto)
	}
	if addr != "" {
		t.Errorf("expected empty addr for empty socket, got %s", addr)
	}
}

func TestTimeoutBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		timeout int
	}{
		{"zero", 0},
		{"one", 1},
		{"typical", 30},
		{"large", 999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{DefaultTimeout: tt.timeout}
			if c.DefaultTimeout != tt.timeout {
				t.Errorf("expected timeout %d, got %d", tt.timeout, c.DefaultTimeout)
			}
		})
	}
}
