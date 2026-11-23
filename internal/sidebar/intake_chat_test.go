package sidebar

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"tagent/internal/intake"
)

func TestIntakeChatLeftRight(t *testing.T) {
	m := Model{icons: DefaultIcons(), theme: DefaultTheme(), width: 60, height: 24}
	m.intakeState = &intakeState{Idea: "làm UI đẹp hơn, có nhiều icon hoặc màu", Turns: []intake.Turn{{Question: "Which screens?", Answer: "board and detail"}},
		Draft: intake.Draft{Title: "UI color & icon refresh", Goal: "Prettier board.", Acceptance: []string{"icons colored"}, Workflow: "feature"}}
	view := ansi.Strip(m.intakeView())
	if testing.Verbose() {
		fmt.Println(view)
	}
	lines := strings.Split(view, "\n")
	find := func(s string) string {
		for _, l := range lines {
			if strings.Contains(l, s) {
				return l
			}
		}
		t.Fatalf("missing %q:\n%s", s, view)
		return ""
	}
	if l := find("board and detail"); !strings.HasSuffix(strings.TrimRight(l, " │"), "board and detail") || strings.Index(l, "board") < 20 {
		t.Fatalf("your answer is not on the right: %q", l)
	}
	if l := find("Which screens?"); strings.Index(l, "Which") > 4 {
		t.Fatalf("bonsai is not on the left: %q", l)
	}
	find("UI color & icon refresh")
	find("Workflow: feature")
	find("Enter create")
	if len(lines) != 24 { // fills the pane
		t.Fatalf("height %d", len(lines))
	}
}
