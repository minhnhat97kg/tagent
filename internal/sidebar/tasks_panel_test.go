package sidebar

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestTasksPanelEnterOpensBoardDrawer(t *testing.T) {
	m := boardTestModel()
	m.maximized = false
	m.boardDir, m.repoDir = t.TempDir(), ""
	next, _ := boardPress(m, "enter")
	if !next.boardDrawer {
		t.Fatal("enter did not open board detail")
	}
	detail := ansi.Strip(strings.Join(next.boardDetail(70, 25), "\n"))
	if !strings.Contains(detail, "Description") {
		t.Fatalf("missing shared detail: %s", detail)
	}
}
