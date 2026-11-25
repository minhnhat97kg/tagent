package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScoutOutputIsCapped(t *testing.T) {
	dir, scope := testBoard(t)
	path := filepath.Join(dir, ".tasks", "workflows")
	if e := os.MkdirAll(path, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(path, "scouted.yml"), []byte("name: scouted\ninputs:\n  goal: {required: true}\njobs:\n  scout: {uses: scout}\n  plan: {needs: scout, agent: planner}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	ticket, e := Add(scope, "scout cap", nil, MutationOptions{Workflow: "scouted", Inputs: map[string]string{"goal": "x"}})
	if e != nil {
		t.Fatal(e)
	}
	if e = Move(scope, ticket.ID, "scouting", ""); e != nil {
		t.Fatal(e)
	}
	big := map[string]string{"scout": strings.Repeat("x", scoutOutputMax+1)}
	if e = Move(scope, ticket.ID, "plan", "", MutationOptions{Outputs: big}); e == nil || !strings.Contains(e.Error(), "shorten and resubmit") {
		t.Fatalf("oversized scout output accepted: %v", e)
	}
	ok := map[string]string{"scout": strings.Repeat("x", scoutOutputMax)}
	if e = Move(scope, ticket.ID, "plan", "", MutationOptions{Outputs: ok}); e != nil {
		t.Fatal(e)
	}
}
