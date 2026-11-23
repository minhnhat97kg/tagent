package sidebar

import (
	tea "github.com/charmbracelet/bubbletea"
	"os/exec"
	"strings"
	"tmux-agent/internal/tmux"
)

// popup runs a shell command in a tmux popup over pane; with no pane
// (standalone, outside tmux) it takes over the terminal instead.
func popup(pane, cmd string, done func() tea.Msg) tea.Cmd {
	if pane == "" {
		return tea.ExecProcess(exec.Command("sh", "-c", cmd), func(error) tea.Msg { return done() })
	}
	return func() tea.Msg {
		args := append([]string{"display-popup", "-E", "-w", "90%", "-h", "90%", "-t", pane}, popupStyle()...)
		tmux.RunTmux(append(args, cmd)...)
		return done()
	}
}

// popupStyle paints the popup to match the theme (it inherits the terminal
// background otherwise, which hides a dark theme's light text).
func popupStyle() []string {
	bg, fg := "colour255", "colour235"
	if t, _ := tmux.GetOption(tmux.AgentTheme); strings.EqualFold(strings.TrimSpace(t), "dark") {
		bg, fg = "colour234", "colour255"
	}
	return []string{"-s", "bg=" + bg + ",fg=" + fg}
}

func gitDo(root, ok string, args ...string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		text := ok
		if err != nil {
			text = "! " + strings.TrimSpace(string(out))
		} else if ok != "" {
			text = "✓ " + ok
		}
		return actionResult(text, nil)
	}
}
