package intake

import (
	"strings"
	"testing"
)

const ticket = "TICKET\ntitle: Build intake\ngoal: Clarify tickets\nacceptance:\n- Tests pass\nout of scope:\n- Commands\nworkflow: unknown"

func TestProtocol(t *testing.T) {
	p := Prompt("idea", []Turn{{"scope?", "small"}}, Context{Workflows: []string{"build"}}, true)
	for _, s := range []string{"build", "no more questions", "small", "at most 3", "ONE"} {
		if !strings.Contains(p, s) {
			t.Fatal(s)
		}
	}
	if !strings.Contains(p, "only ! requests may propose CMD") {
		t.Fatal("work prompt must restrict commands")
	}
	for _, s := range []string{ticket, "```yaml\n" + ticket + "\n```", strings.ReplaceAll(ticket, "title:", "TITLE:")} {
		q, d, e := Parse(s)
		if e != nil || q != "" {
			t.Fatal(e)
		}
		d.Normalize([]string{"build"})
		if d.Workflow != "" {
			t.Fatal(d)
		}
		want := "Build intake\n\nGoal\nClarify tickets\n\nAcceptance\n- Tests pass\n\nOut of scope\n- Commands"
		if d.Text() != want {
			t.Fatal(d.Text())
		}
	}
	q, _, e := Parse(" Q: Scope? ")
	if e != nil || q != "Scope?" {
		t.Fatal(q, e)
	}
	for _, s := range []string{"", "Q:", "garbage", "TICKET\ntitle: nope"} { // mixed TICKET+Q is accepted (TestParseMixedTicketAndQuestion)
		if _, _, e := Parse(s); e == nil {
			t.Fatal(s)
		}
	}
}

func TestParseOneLineTicket(t *testing.T) {
	_, d, err := Parse("TICKET title: UI color & icon refresh goal: Make the board prettier with icons and colors. acceptance: - status icons colored - theme respected out of scope: - new layouts workflow: feature")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "UI color & icon refresh" || d.Goal != "Make the board prettier with icons and colors." || len(d.Acceptance) != 2 || len(d.OutOfScope) != 1 || d.Workflow != "feature" {
		t.Fatalf("%+v", d)
	}
	if _, d, err = Parse("TICKET\ntitle: x\ngoal: y"); err != nil || d.Title != "x" || d.Workflow != "" {
		t.Fatalf("minimal ticket: %+v %v", d, err)
	}
}

func TestParseMixedTicketAndQuestion(t *testing.T) {
	reply := "TICKET\ntitle: Enhance UI with more icons/colors\ngoal: Improve visual appeal\nout of scope: Functional changes\nworkflow: review\nQ: What specific style should the icons follow?\n"
	q, d, err := Parse(reply)
	if err != nil || q != "" || d.Title != "Enhance UI with more icons/colors" || len(d.OutOfScope) != 1 || d.Workflow != "review" {
		t.Fatalf("%q %+v %v", q, d, err)
	}
	if q, _, err = Parse("Sure!\nQ: Which screens?\nthanks"); err != nil || q != "Which screens?" {
		t.Fatalf("question in prose: %q %v", q, err)
	}
}
