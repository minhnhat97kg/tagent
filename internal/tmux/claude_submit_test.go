package tmux

import "testing"

func TestClaudeEditorContains(t *testing.T) {
	rule := "──────────────────────────"
	status := "\n  Haiku 5h 70%\n  -- INSERT -- auto mode on\n"
	prompt := "Goal: make the YAML readable.\nRead the code and write .tasks/plans/26.md with the steps."
	cases := []struct {
		name, screen string
		want         bool
	}{
		{"whole prompt in box", "old output\n" + rule + "\n❯ " + prompt + "\n" + rule + status, true},
		{"box taller than pane, tail only", "  and write .tasks/plans/26.md with the steps.\n" + rule + status, true},
		{"paste placeholder", rule + "\n> [Pasted text #1 +40 lines]\n" + rule + status, true},
		{"submitted, empty box", prompt + "\n" + rule + "\n❯ \n" + rule + status, false},
		{"other text in box", rule + "\n❯ something else typed by hand\n" + rule + status, false},
		{"no rule", prompt, false},
	}
	for _, c := range cases {
		if got := claudeEditorContains(c.screen, prompt); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
