package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"tagent/internal/workflow"
	"testing"
)

func TestWorkflowFormRoundTrip(t *testing.T) {
	source := `# retained comment
name: Example
inputs:
  goal: {required: true, default: hello}
jobs:
  build:
    agent: implementer
    prompt: |
      Build ${{ inputs.goal }}
      Keep this line.
    outputs: [artifact]
    max-loops: 3
  test:
    needs: [build]
    run: |
      echo "a=b"
      go test ./...
    on-failure: build
`
	original, err := workflow.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := patchWorkflow(source, &workflowPatch{Fields: map[string]string{"description": "Friendly"}})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := workflow.Parse([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original.Jobs, parsed.Jobs) || !reflect.DeepEqual(original.Inputs, parsed.Inputs) || !strings.Contains(out, "# retained comment") {
		t.Fatal(out)
	}
	unchanged, _, err := patchWorkflow(source, nil)
	if err != nil || unchanged != source {
		t.Fatal("no-op changed source")
	}
	steps := []string{"test"}
	if _, _, err = patchWorkflow(source, &workflowPatch{Steps: &steps}); err == nil {
		t.Fatal("accepted dangling failure target")
	}
	steps = []string{"build", "build"}
	if _, _, err = patchWorkflow(source, &workflowPatch{Steps: &steps}); err == nil {
		t.Fatal("accepted duplicate")
	}
	steps = []string{"build", "extra", "test"}
	if _, p, err := patchWorkflow(source, &workflowPatch{Steps: &steps}); err != nil || len(p.Jobs) != 3 {
		t.Fatalf("add: %v", err)
	}
	if _, _, err = patchWorkflow(source, &workflowPatch{Jobs: map[string]map[string]string{"build": {"run": "echo conflict"}}}); err == nil {
		t.Fatal("accepted run/agent")
	}
}
func TestWorkflowRunInputs(t *testing.T) {
	old := workflowCommand
	defer func() { workflowCommand = old }()
	root := t.TempDir()
	s := &server{dir: root}
	dir := workflow.LocalDir(root)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "recipe.yaml")
	calls := 0
	var args []string
	workflowCommand = func(cwd string, a []string) error {
		calls++
		args = a
		if cwd != root {
			t.Errorf("cwd %s", cwd)
		}
		return nil
	}
	call := func(body any, want int) {
		t.Helper()
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		s.workflows(w, httptest.NewRequest("POST", "/api/workflows", strings.NewReader(string(b))))
		if w.Code != want {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
	}
	if err := os.WriteFile(file, []byte("inputs:\n  goal: {required: true}\njobs: {test: {run: echo ok}}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	call(map[string]any{"Name": "recipe", "Op": "run", "Inputs": map[string]string{}}, 400)
	if calls != 0 {
		t.Fatal("ran with missing input")
	}
	value := "a=b ; $(echo no)\n spaced"
	call(map[string]any{"Name": "recipe", "Op": "run", "Goal": "ignored", "Inputs": map[string]string{"z": "last", "goal": value}}, 200)
	want := []string{"workflow", "run", file, "--input", "goal=" + value, "--input", "z=last"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("%q", args)
	}
	call(map[string]any{"Name": "recipe", "Op": "run", "Goal": "legacy"}, 200)
	call(map[string]any{"Name": "recipe", "Op": "run", "Goal": ""}, 400)
	call(map[string]any{"Name": "recipe", "Op": "run", "Inputs": map[string]any{"goal": 42}}, 400)
	if err := os.WriteFile(file, []byte("jobs: {test: {run: echo ok}}"), 0644); err != nil {
		t.Fatal(err)
	}
	call(map[string]any{"Name": "recipe", "Op": "run", "Inputs": map[string]string{}}, 200)
}
