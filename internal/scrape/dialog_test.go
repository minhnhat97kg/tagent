package scrape

import (
	"strings"
	"testing"
)

func TestParseDialogClaudeMenu(t *testing.T) {
	screen := `
 │ gofmt -l . ; go vet ./...
 This command requires approval
 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for: gofmt -l .
   3. No
 Esc to cancel · Tab to amend
`
	d, ok := ParseDialog(screen)
	if !ok || d.Question != "Do you want to proceed?" || len(d.Options) != 3 {
		t.Fatalf("dialog = %+v, %v", d, ok)
	}
	if d.Options[0] != (Option{"1", "Yes"}) || d.Options[2] != (Option{"3", "No"}) {
		t.Errorf("options = %+v", d.Options)
	}
}

func TestParseDialogYesNoAndNone(t *testing.T) {
	d, ok := ParseDialog("Apply this change? [y/n]\n")
	if !ok || len(d.Options) != 2 || d.Options[0].Key != "y" {
		t.Errorf("y/n = %+v %v", d, ok)
	}
	if _, ok := ParseDialog("all tests pass\n> \n"); ok {
		t.Error("no prompt, no dialog")
	}
	if got := AnswerKeys("y"); len(got) != 2 || got[1] != "Enter" {
		t.Errorf("y needs Enter: %v", got)
	}
	if got := AnswerKeys("2"); len(got) != 1 || got[0] != "2" {
		t.Errorf("digit picks directly: %v", got)
	}
}

// A prompt near the top of a tall, mostly empty pane must still be found.
func TestParseDialogTallPane(t *testing.T) {
	screen := " Do you want to proceed?\n ❯ 1. Yes\n   2. No\n" + strings.Repeat("\n", 60)
	if d, ok := ParseDialog(screen); !ok || len(d.Options) != 2 {
		t.Fatalf("dialog = %+v %v", d, ok)
	}
}

func TestParseDialogArrowMenu(t *testing.T) {
	screen := `
 Accessing workspace:
 /private/tmp
 Quick safety check: Is this a project you created or one you
 trust? (Like your own code, a well-known open source project,
 or work from your team). If not, take a moment to review what's
 in this folder first.
 Claude Code'll be able to read, edit, and execute files here.
 Security guide
 ❯ No, exit
   Yes, I trust this folder
 Enter to confirm · Esc to cancel
`
	d, ok := ParseDialog(screen)
	if !ok || !d.Arrow || d.Selected != 0 || len(d.Options) != 2 || d.Options[1] != (Option{"2", "Yes, I trust this folder"}) {
		t.Fatalf("dialog = %+v, %v", d, ok)
	}
	if got := strings.Join(arrowKeys(d, "2"), " "); got != "Down Enter" {
		t.Errorf("key 2 = %q, want Down Enter", got)
	}
	if got := strings.Join(arrowKeys(d, "1"), " "); got != "Enter" {
		t.Errorf("key 1 = %q, want Enter", got)
	}
	if arrowKeys(d, "9") != nil {
		t.Error("out-of-range key must send nothing")
	}
}

func TestParseDialogCodexNeedsEnter(t *testing.T) {
	screen := `
  Would you like to run the following command?
  $ ./scripts/review.sh
› 1. Yes, proceed (y)
  2. Yes, and don't ask again (p)
  3. No, and tell Codex what to do differently (esc)
  Press enter to confirm or esc to cancel
`
	d, ok := ParseDialog(screen)
	if !ok || len(d.Options) != 3 || !d.Confirm {
		t.Fatalf("dialog = %+v, %v", d, ok)
	}
	claude, _ := ParseDialog(" Do you want to proceed?\n ❯ 1. Yes\n   2. No\n Esc to cancel · Tab to amend\n")
	if claude.Confirm {
		t.Error("Claude's digit answers at once, no Enter")
	}
}
