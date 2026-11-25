package workflow

import "testing"

func TestScoutJobExpands(t *testing.T) {
	w, e := Parse([]byte("jobs:\n  scout: {uses: scout}\n  plan: {needs: scout, agent: planner}\n"))
	if e != nil || len(w.Jobs) != 2 {
		t.Fatal(w, e)
	}
	s := w.Jobs[0]
	if s.Status != "scouting" || s.Agent != "ba" || len(s.Outputs) != 1 || s.Outputs[0] != "scout" || s.Prompt != ScoutPrompt {
		t.Fatalf("scout job %+v", s)
	}
}

func TestScoutJobRejectsOtherFields(t *testing.T) {
	for _, src := range []string{
		"jobs:\n  scout: {uses: scout, run: go test ./...}\n",
		"jobs:\n  scout: {uses: scout, agent: planner}\n",
		"jobs:\n  scout: {uses: review}\n",
	} {
		if _, e := Parse([]byte(src)); e == nil {
			t.Fatal(src)
		}
	}
}
