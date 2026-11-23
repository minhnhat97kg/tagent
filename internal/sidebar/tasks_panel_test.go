package sidebar

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestTasksPanelGroupsInBoardOrder(t *testing.T) {
	m := boardTestModel()
	m.boardMode = false
	view := ansi.Strip(strings.Join(m.taskLines(90, 15), "\n"))
	for _, want := range []string{"todo 2", "done 1", "TA-1 First card", "TA-3 Third card"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q: %s", want, view)
		}
	}
	if strings.Index(view, "todo 2") > strings.Index(view, "done 1") {
		t.Fatal("incorrect board order")
	}
}

func TestTasksPanelEnterOpensBoardDrawer(t *testing.T) {
	m := boardTestModel()
	m.boardMode, m.taskTitle = false, false
	m.boardDir, m.repoDir = t.TempDir(), ""
	next, _ := boardPress(m, "enter")
	if !next.boardMode || !next.boardDrawer {
		t.Fatal("enter did not open board detail")
	}
	detail := ansi.Strip(strings.Join(next.boardDetail(70, 25), "\n"))
	if !strings.Contains(detail, "Description") {
		t.Fatalf("missing shared detail: %s", detail)
	}
}
