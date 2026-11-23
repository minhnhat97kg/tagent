package web

import (
	"encoding/json"
	"fmt"
	"strings"
	"tagent/internal/tmux"
)

type terminalSize struct {
	Cols, Rows int
	Action     string
}

func parseTerminalResize(data []byte) (terminalSize, error) {
	var m struct {
		Type string `json:"type"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return terminalSize{}, err
	}
	if m.Type == "release" {
		return terminalSize{Action: "release"}, nil
	}
	if (m.Type != "resize" && m.Type != "handoff") || m.Cols <= 0 || m.Rows <= 0 {
		return terminalSize{}, fmt.Errorf("invalid terminal resize")
	}
	return terminalSize{max(40, min(300, m.Cols)), max(10, min(120, m.Rows)), m.Type}, nil
}

// Unknown/malformed client state is deliberately treated as an attached terminal.
func controlClientsOnly(clients string, ok bool) bool {
	if !ok {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(clients), "\n") {
		if line != "" && strings.TrimSpace(line) != "1" {
			return false
		}
	}
	return true
}

func sessionResizable(session string) bool {
	clients, ok := tmux.RunTmux("list-clients", "-t", session, "-F", "#{client_control_mode}")
	return controlClientsOnly(clients, ok)
}
