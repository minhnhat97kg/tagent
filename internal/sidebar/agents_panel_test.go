package sidebar

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tasks"
)

func TestLocalDefinitionsIncludeBoardRoles(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Skip("git unavailable")
	}
	os.MkdirAll(filepath.Join(dir, ".tasks"), 0o755)
	os.WriteFile(filepath.Join(dir, ".tasks", "board.yaml"), []byte("roles:\n  researcher: {agent: pi, model: m}\n  planner: {agent: pi}\n"), 0o644)
	defs := tasks.LocalDefinitions(dir, "claude")
	if len(defs) != 2 || defs[0].Name != "planner" || defs[1].Name != "researcher" || !strings.HasSuffix(defs[1].Path, "board.yaml") {
		t.Fatalf("%+v", defs)
	}
}

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
