package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetWorkflow(t *testing.T) {
	dir, scope := testBoard(t)
	path := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, inputs string }{{"single", "  goal: {required: true}\n"}, {"multi", "  a: {required: true}\n  b: {required: true}\n"}} {
		if err := os.WriteFile(filepath.Join(path, tc.name+".yml"), []byte("name: "+tc.name+"\ninputs:\n"+tc.inputs+"jobs:\n  build: {run: echo ok}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ticket, err := Add(scope, "full ticket\nmore text", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = SetWorkflow(scope, ticket.ID, "multi", nil); err == nil || !strings.Contains(err.Error(), "a, b; use workflow run multi") {
		t.Fatal(err)
	}
	got, _ := Get(scope, ticket.ID)
	if got.Workflow != "" {
		t.Fatal("error changed ticket")
	}
	if err = SetWorkflow(scope, ticket.ID, "single", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = Get(scope, ticket.ID)
	if got.Inputs["goal"] != ticket.Text {
		t.Fatal(got)
	}
	if _, err = Update(scope, ticket.ID, "build", nil); err != nil {
		t.Fatal(err)
	}
	if err = SetWorkflow(scope, ticket.ID, "single", nil); err == nil {
		t.Fatal("accepted non-todo")
	}
}
