package intake

import (
	"strings"
	"testing"
)

func TestCommandSafety(t *testing.T) {
	for _, s := range []string{"tmux-agent reload", "tmux-agent task show abc", "tmux-agent task comment abc \"two words\"", "tmux-agent workflow run research --input goal=topic", "tmux-agent ctx ls", "tmux-agent ctx --global add \"a note\""} {
		if _, e := CommandArgs(s); e != nil {
			t.Fatal(s, e)
		}
	}
	for _, s := range []string{"sh reload", "tmux-agent task rm abc", "tmux-agent install", "tmux-agent merge abc", "tmux-agent discard abc", "tmux-agent config set x y", "tmux-agent ctx clear", "tmux-agent ctx --global rm abc", "tmux-agent workflow new x", "tmux-agent reload\nlist", "tmux-agent task show \"unterminated"} {
		if _, e := CommandArgs(s); e == nil {
			t.Fatal(s)
		}
	}
	for _, c := range ";|&`$()><" {
		if _, e := CommandArgs("tmux-agent task comment id \"a" + string(c) + "b\""); e == nil {
			t.Fatal(c)
		}
	}
	args, e := CommandArgs("tmux-agent task comment abc \"two words\"")
	if e != nil || args[4] != "two words" {
		t.Fatal(args, e)
	}
}
func TestResponses(t *testing.T) {
	r, e := ParseResponse("CMD: tmux-agent reload")
	if e != nil || r.Command != "tmux-agent reload" {
		t.Fatal(r, e)
	}
	r, e = ParseResponse("ESCALATE: Needs architectural judgement")
	if e != nil || r.Escalate == "" {
		t.Fatal(r, e)
	}
	p := Prompt("!reload", nil, Context{Workflows: []string{"research: Research a topic"}, Tickets: []string{"abc todo Ticket"}}, false)
	for _, s := range []string{"! operations", "ESCALATE", "task move", "abc todo Ticket", "research: Research a topic"} {
		if !strings.Contains(p, s) {
			t.Fatal(s)
		}
	}
}
