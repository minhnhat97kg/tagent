package sidebar

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestAgentsPanelScrollsToCursor(t *testing.T) {
	m := Model{icons: DefaultIcons(), theme: DefaultTheme(), dashboard: true, agentFocus: 1}
	m.rows = []Row{{IsRepoAgent: true, Header: "Global agents"}}
	for i := 0; i < 8; i++ {
		m.rows = append(m.rows, Row{IsRepoAgent: true, RepoAgentName: fmt.Sprintf("agent%d", i)})
	}
	m.definitionCursor = 7
	lines, _ := m.roleLines(36, 5)
	view := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(view, "agent7") || strings.Contains(view, "agent0") {
		t.Fatalf("selected definition not scrolled into view:\n%s", view)
	}
}
