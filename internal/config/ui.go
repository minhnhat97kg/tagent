package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/sidebar"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

// Settings tabs: general is the shared Editor's list; board edits board.yaml.
const (
	tabGeneral = iota
	tabBoard
	tabSkills
	tabMCP
	tabCount
)

var tabNames = [tabCount]string{"General", "Board", "Skills", "MCP"}

// RunUI is the settings screen (`tagent config` with no arguments, or
// `tagent config board` for the Board tab; `,` in the sidebar opens it in a
// popup). It wraps the shared Editor.
func RunUI(tab int) int {
	opts := tmux.GetAllGlobalOptions()
	m := uiModel{theme: sidebar.ThemeFromOptions(opts), ed: NewEditor(), tab: tab}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return 1
	}
	return 0
}

type uiModel struct {
	theme         sidebar.Theme
	width, height int
	ed            *Editor
	tab           int
	board         boardDraft
	libraryIndex  int
}

func (m uiModel) Init() tea.Cmd { return nil }

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ed.Resize(msg.Width)
	case tea.KeyMsg:
		if !m.ed.Editing() && msg.String() == "tab" {
			m.tab = (m.tab + 1) % tabCount
			return m, nil
		}
		if m.tab == tabSkills || m.tab == tabMCP {
			if msg.String() == "q" || msg.String() == "esc" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			if msg.String() == "j" {
				m.libraryIndex++
				return m, nil
			}
			if msg.String() == "k" {
				if m.libraryIndex > 0 {
					m.libraryIndex--
				}
				return m, nil
			}
			if msg.String() == "e" {
				return m.editLibrary()
			}
			if msg.String() == "a" && m.tab == tabMCP {
				return m.approveLibraryServer()
			}
			return m, nil
		}
		if m.tab == tabBoard {
			switch msg.String() {
			case "e":
				return m.editBoard()
			case "q", "esc", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}
		quit, cmd := m.ed.HandleKey(msg)
		if quit {
			return m, tea.Quit
		}
		return m, cmd
	case boardEditedMsg:
		return m.finishBoard(msg), nil
	case libraryEditedMsg:
		if msg.err != nil {
			m.ed.SetFlash("editor: "+msg.err.Error(), true)
		}
		return m, nil
	}
	return m, nil
}

func (m uiModel) View() string {
	if m.width == 0 {
		return ""
	}
	fg := func(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	dim, accent := fg(m.theme.TextDim), fg(m.theme.Accent).Bold(true)
	title := lipgloss.NewStyle().Background(m.theme.Accent).Foreground(lipgloss.Color("0")).Bold(true).Render(" \uf013 tagent settings ")
	var tabs []string
	for i, n := range tabNames {
		if i == m.tab {
			tabs = append(tabs, accent.Render("["+n+"]"))
		} else {
			tabs = append(tabs, dim.Render(" "+n+" "))
		}
	}
	lines := []string{title + "  " + strings.Join(tabs, " ") + dim.Render("  "+ConfigFile()), ""}

	body, selLine := m.boardBody(dim, accent), 0
	if m.tab == tabGeneral {
		body, selLine = m.ed.Body(m.theme, 32, true)
	} else if m.tab == tabSkills || m.tab == tabMCP {
		body = m.libraryBody(dim, accent)
	}
	footerH := 4
	room := m.height - len(lines) - footerH
	if room < 3 {
		room = 3
	}
	start := 0
	if selLine >= room {
		start = selLine - room + 1
	}
	end := min(len(body), start+room)
	lines = append(lines, body[start:end]...)
	for len(lines) < m.height-footerH {
		lines = append(lines, "")
	}

	help := ""
	if m.tab == tabGeneral {
		s := m.ed.Selected()
		help = s.Key
		if s.Help != "" {
			if help != "" {
				help += " — "
			}
			help += s.Help
		}
		if s.Default != "" {
			help += " · default " + s.Default
		}
		if s.Reload {
			help += " · rebinds the key"
		}
	}
	flash, flashBad := m.ed.Flash()
	flashLine := ""
	if flash != "" {
		c := m.theme.StatusRunning
		if flashBad {
			c = m.theme.StatusError
		}
		flashLine = fg(c).Render(flash)
	}
	keys := "tab switch tab · j/k move · enter/space toggle · ←/→ choose · e edit · r reset · a add workspace · x remove workspace · q close"
	switch {
	case m.ed.Editing():
		keys = "enter save · esc cancel"
	case m.tab == tabBoard:
		keys = "tab switch tab · e edit board.yaml in $EDITOR · q close"
	case m.tab == tabSkills:
		keys = "tab switch · j/k select · e edit selected library skill · q close"
	case m.tab == tabMCP:
		keys = "tab switch · j/k select · e edit mcp.yml · a approve selected server · q close"
	}
	lines = append(lines, "", dim.Render(ansi.Truncate(help, m.width, "…")), flashLine, accent.Render(ansi.Truncate(keys, m.width, "…")))
	return strings.Join(lines, "\n")
}

// boardBody is the Board tab: where board.yaml is and what `e` does with it.
func (m uiModel) boardBody(dim, accent lipgloss.Style) []string {
	dir := tasks.MainRoot(workDir())
	lines := []string{accent.Render("board.yaml"), "  " + tasks.BoardConfigPath(dir), ""}
	lines = append(lines, dim.Render("Roles, triggers and default_workflow. `e` opens the file in $EDITOR."),
		dim.Render("It is saved only when it parses; a rejected edit stays as a draft for the next `e`."))
	if _, err := os.Stat(draftPath(dir)); err == nil {
		lines = append(lines, "", dim.Render("a rejected draft is waiting: "+draftPath(dir)))
	}
	return lines
}

// ReloadKeys re-runs the plugin so key bindings pick up new keys, after
// unbinding the old one.
func ReloadKeys(oldKey string) error {
	if oldKey != "" {
		tmux.RunTmux("unbind", oldKey)
	}
	bin, ok := tmux.GetOption("@agent_sidebar_bin")
	if !ok {
		return fmt.Errorf("plugin not loaded")
	}
	entry := filepath.Join(filepath.Dir(filepath.Dir(bin)), "assets", "tagent.tmux")
	if _, err := os.Stat(entry); err != nil {
		return err
	}
	_, err := tmux.RunTmuxCapture("run-shell", entry)
	return err
}

func cycle(choices []string, cur string, step int) string {
	for i, c := range choices {
		if c == cur {
			return choices[(i+step+len(choices))%len(choices)]
		}
	}
	return choices[0]
}

// Display is a value as shown to the user (an em dash when empty).
func Display(v string) string {
	if v == "" {
		return "—"
	}
	return v
}
