package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Socket          string
	DBFile          string
	LogFile         string
	LogLevel        string
	DefaultAction   string
	DefaultDuration string
	DefaultTimeout  int
	MaxMsgLength    int
	GroupWindow      int // seconds for recent connection grouping
}

func Load() *Config {
	c := &Config{}

	flag.StringVar(&c.Socket, "socket", "unix:///tmp/osui.sock", "gRPC socket address")
	flag.StringVar(&c.DBFile, "db-file", "", "SQLite database file (empty for in-memory)")
	flag.StringVar(&c.LogFile, "log-file", "", "Log file path (empty for stderr)")
	flag.StringVar(&c.LogLevel, "log-level", "info", "Log level: debug, info, warn, error")
	flag.StringVar(&c.DefaultAction, "default-action", "deny", "Default action: allow, deny, reject")
	flag.StringVar(&c.DefaultDuration, "default-duration", "until restart", "Default rule duration")
	flag.IntVar(&c.DefaultTimeout, "default-timeout", 90, "Prompt timeout in seconds")
	flag.IntVar(&c.MaxMsgLength, "max-msg-length", 4194304, "gRPC max message length in bytes")
	flag.IntVar(&c.GroupWindow, "group-window", 60, "Seconds to group recent connections by process+destination")
	flag.Parse()

	if c.DBFile == "" {
		dataDir, err := os.UserConfigDir()
		if err != nil {
			dataDir = filepath.Join(os.Getenv("HOME"), ".config")
		}
		dir := filepath.Join(dataDir, "ostui")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to create config dir %s: %v\n", dir, err)
		}
		c.DBFile = filepath.Join(dir, "ostui.db")
	}

	return c
}

func (c *Config) SocketProto() (proto, addr string) {
	s := c.Socket
	// Parse "unix:///tmp/osui.sock" or "0.0.0.0:50051"
	if len(s) > 7 && s[:7] == "unix://" {
		return "unix", s[7:]
	}
	if len(s) > 5 && s[:5] == "unix:" {
		return "unix", s[5:]
	}
	return "tcp", s
}

func (c *Config) String() string {
	return fmt.Sprintf("socket=%s db=%s action=%s duration=%s timeout=%d",
		c.Socket, c.DBFile, c.DefaultAction, c.DefaultDuration, c.DefaultTimeout)
}
