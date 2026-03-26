package tui

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/safedoor/ostui/internal/bus"
)

// notifyRuleRequest sends a desktop notification and terminal bell
// when a new connection prompt arrives.
func notifyRuleRequest(req *bus.PromptRequest) tea.Cmd {
	return func() tea.Msg {
		conn := req.Connection
		proc := extractProcessName(conn.ProcessPath)
		dest := conn.DstHost
		if dest == "" {
			dest = conn.DstIp
		}
		proto := strings.ToUpper(conn.Protocol)

		summary := fmt.Sprintf("ostui: %s → %s:%d", proc, dest, conn.DstPort)
		body := fmt.Sprintf(
			"Process: %s\nDestination: %s port %d (%s)\nPID: %d  UID: %d",
			conn.ProcessPath, dest, conn.DstPort, proto,
			conn.ProcessId, conn.UserId,
		)

		// Terminal bell — write directly to the TTY to bypass alt-screen capture.
		if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
			tty.Write([]byte("\a"))
			tty.Close()
		}

		// D-Bus desktop notification via notify-send.
		cmd := exec.Command("notify-send",
			"--urgency=critical",
			"--app-name=ostui",
			"--category=network",
			summary,
			body,
		)
		if err := cmd.Run(); err != nil {
			log.Printf("ERROR notify-send: %v", err)
		}

		return nil
	}
}
