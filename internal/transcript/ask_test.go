package transcript

import "testing"

const askScreen = ` ☐ Pick one
Pick one
❯ 1. Red
     Choose Red
  2. Green
     Choose Green
↓ 3. Blue
     Choose Blue
──────────────────────────────
  5. Chat about this
 ↑ 1 more line
Enter to select · ↑/↓ to navigate · Esc to cancel
`

func TestParseAsk(t *testing.T) {
	a := ParseAsk("old output\n" + askScreen)
	if a == nil || a.Question != "Pick one" || a.Header != "Pick one" || !a.Other || a.Multi {
		t.Fatalf("got %+v", a)
	}
	if len(a.Options) != 3 || a.Options[1].N != 2 || a.Options[1].Label != "Green" || a.Options[1].Description != "Choose Green" {
		t.Fatalf("options %+v", a.Options)
	}
}

func TestParseAskIgnoresOtherScreens(t *testing.T) {
	perm := "Do you want to proceed?\n❯ 1. Yes\n  2. No\nEsc to cancel · Tab to amend\n"
	for name, s := range map[string]string{"permission": perm, "answered": "● Answered\n❯ \n──────\n", "empty": ""} {
		if ParseAsk(s) != nil {
			t.Errorf("%s matched", name)
		}
	}
}

func TestParseAskClippedHeader(t *testing.T) {
	a := ParseAsk("Which one?\n❯ 1. A\n  2. B\nEnter to select · Esc to cancel\n")
	if a == nil || a.Question != "Which one?" || len(a.Options) != 2 {
		t.Fatalf("got %+v", a)
	}
}

func TestParseAskClipped(t *testing.T) {
	a := ParseAsk(" ☐ Pick\nPick\n❯ 1. A\n  2. B\n ↑ 2 more lines\nEnter to select · Esc to cancel\n")
	if a == nil || !a.Clipped {
		t.Fatalf("clipped screen not flagged: %+v", a)
	}
	if b := ParseAsk(askScreen); b == nil || !b.Clipped {
		t.Fatalf("sample screen has a 'more line' marker: %+v", b)
	}
	if c := ParseAsk("Which?\n❯ 1. A\n  2. B\nEnter to select · Esc to cancel\n"); c == nil || c.Clipped {
		t.Fatalf("complete screen flagged: %+v", c)
	}
}
