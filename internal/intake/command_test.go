package intake

import (
	"strings"
	"testing"
)

func TestCommandSafety(t *testing.T) {
	for _, s := range []string{"tagent reload", "tagent task show abc", "tagent task comment abc \"two words\"", "tagent workflow run research --input goal=topic", "tagent ctx ls", "tagent ctx --global add \"a note\""} {
		if _, e := CommandArgs(s); e != nil {
			t.Fatal(s, e)
		}
	}
	for _, s := range []string{"sh reload", "tagent task rm abc", "tagent install", "tagent merge abc", "tagent discard abc", "tagent config set x y", "tagent ctx clear", "tagent ctx --global rm abc", "tagent workflow new x", "tagent reload\nlist", "tagent task show \"unterminated"} {
		if _, e := CommandArgs(s); e == nil {
			t.Fatal(s)
		}
	}
	for _, c := range ";|&`$()><" {
		if _, e := CommandArgs("tagent task comment id \"a" + string(c) + "b\""); e == nil {
			t.Fatal(c)
		}
	}
	args, e := CommandArgs("tagent task comment abc \"two words\"")
	if e != nil || args[4] != "two words" {
		t.Fatal(args, e)
	}
}
func TestResponses(t *testing.T) {
	r, e := ParseResponse("CMD: tagent reload")
	if e != nil || r.Command != "tagent reload" {
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

func TestLegacyCommandAlias(t *testing.T) {
	for _, command := range []string{"tagent reload", "tmux-agent reload"} {
		if _, err := CommandArgs(command); err != nil {
			t.Fatal(err)
		}
		if _, err := CommandArgs(command + "; evil"); err == nil {
			t.Fatal("accepted shell syntax")
		}
	}
}
