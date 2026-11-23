package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/roles"
)

const commentedBoard = `# project board
roles:
  # the planner picks the plan
  planner: {agent: pi, model: openai-codex/gpt-6.1-sol, effort: high}
  reviewer: {agent: claude, fresh_after: 3}
statuses:
  - {id: todo, next: [plan]}
  - {id: plan, role: planner, next: [done, blocked]}
  - {id: blocked}
  - {id: done}
`

func setup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	if err := os.MkdirAll(filepath.Join(dir, ".tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tasks", "board.yaml"), []byte(commentedBoard), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func board(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".tasks", "board.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestListMergesRepoBoardAndUsage(t *testing.T) {
	dir := setup(t)
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".claude", "agents", "reviewer.md"), []byte("---\ndescription: r\n---\nReview."), 0644)
	got := List(dir, "claude", []LivePane{{Role: "planner", Pane: "%1"}, {Role: "planner", Pane: "%2"}, {Role: "other", Pane: "%3"}})
	byName := map[string]Agent{}
	for _, a := range got {
		byName[a.Name] = a
	}
	p := byName["planner"]
	if p.Harness != "pi" || p.Model != "openai-codex/gpt-6.1-sol" || p.Effort != "high" || p.Source != "board" || p.Running != 2 || len(p.Panes) != 2 || p.Panes[1].Pane != "%2" {
		t.Fatalf("planner: %+v", p)
	}
	if len(p.UsedBy) != 1 || p.UsedBy[0] != "status plan" {
		t.Fatalf("usedBy: %v", p.UsedBy)
	}
	r := byName["reviewer"]
	if r.Source != "repo" || r.Harness != "claude" || !strings.Contains(r.Instructions, "Review.") {
		t.Fatalf("reviewer: %+v", r)
	}
}

func TestSaveKeepsCommentsAndFreshAfter(t *testing.T) {
	dir := setup(t)
	if err := Save(dir, "claude", Agent{Name: "planner", Harness: "pi", Model: "new-model", Effort: "low"}); err != nil {
		t.Fatal(err)
	}
	b := board(t, dir)
	for _, want := range []string{"# project board", "# the planner picks the plan", "model: new-model", "effort: low"} {
		if !strings.Contains(b, want) {
			t.Fatalf("missing %q in:\n%s", want, b)
		}
	}
	if err := Save(dir, "claude", Agent{Name: "reviewer", Harness: "claude", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board(t, dir), "fresh_after: 3") {
		t.Fatal("fresh_after lost")
	}
}

func TestSaveRejectsBadInputAndLeavesBoardAlone(t *testing.T) {
	dir := setup(t)
	before := board(t, dir)
	bad := []Agent{
		{Name: "../x", Harness: "claude"},
		{Name: "x", Harness: "opencode"},
		{Name: "x", Harness: "pi", Effort: "ultra"},
		{Name: "x", Harness: "pi", Instructions: "hi"},
		{Name: "g", Harness: "claude", Source: "global"},
	}
	for _, a := range bad {
		if err := Save(dir, "claude", a); err == nil {
			t.Fatalf("accepted %+v", a)
		}
	}
	if board(t, dir) != before {
		t.Fatal("board changed on rejected save")
	}
}

func TestSaveInstructionsAndDelete(t *testing.T) {
	dir := setup(t)
	if err := Save(dir, "claude", Agent{Name: "writer", Harness: "claude", Instructions: Template("writer")}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".claude", "agents", "writer.md")
	if b, err := os.ReadFile(path); err != nil || !strings.Contains(string(b), "name: writer") {
		t.Fatalf("file: %v %q", err, b)
	}
	w, _ := Find(dir, "claude", "writer")
	w.Instructions = "changed"
	if err := Save(dir, "claude", w); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "changed\n" {
		t.Fatalf("not updated: %q", b)
	}
	if err := Save(dir, "claude", Agent{Name: "writer", Harness: "claude", Instructions: "x"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "x\n" {
		t.Fatalf("save by name did not update the repo file: %q", b)
	}
	if err := Delete(dir, "claude", "planner"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(board(t, dir), "planner:") {
		t.Fatal("role not deleted")
	}
	if err := Delete(dir, "claude", "writer"); err != nil {
		t.Fatal(err)
	}
	if exists(path) {
		t.Fatal("file not deleted")
	}
}

func TestCodexEditsOnlyDeveloperInstructions(t *testing.T) {
	dir := setup(t)
	path := filepath.Join(dir, ".codex", "agents", "scout.toml")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("# scout keeps its own notes\nname = \"scout\"\ndescription = \"finds things\"\ndeveloper_instructions = \"old\"\nmodel = \"m\"\n"), 0644)
	a, ok := Find(dir, "codex", "scout")
	if !ok || a.Harness != "codex" || a.Instructions != "old" {
		t.Fatalf("scout: %+v", a)
	}
	a.Instructions = "new"
	if err := Save(dir, "codex", a); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	for _, want := range []string{`# scout keeps its own notes`, `description = "finds things"`, `model = "m"`, `developer_instructions = "new"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("missing %q in %s", want, b)
		}
	}
}

// The request's path is never trusted: a save writes only to the repo's own file for the name.
func TestSaveIgnoresRequestPath(t *testing.T) {
	dir := setup(t)
	victim := filepath.Join(t.TempDir(), "victim.md")
	os.WriteFile(victim, []byte("keep"), 0644)
	if err := Save(dir, "claude", Agent{Name: "writer", Harness: "claude", Path: victim, Source: "repo", Instructions: "x"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("write escaped to %s: %q", victim, b)
	}
	if !exists(filepath.Join(dir, ".claude", "agents", "writer.md")) {
		t.Fatal("new file not created under the harness directory")
	}
}

func TestSaveRefusesGlobalWrite(t *testing.T) {
	dir := setup(t)
	os.MkdirAll(filepath.Join(roles.GlobalDir(), "claude"), 0755)
	os.WriteFile(filepath.Join(roles.GlobalDir(), "claude", "gl.md"), []byte("---\nname: gl\n---\nglobal\n"), 0644)
	if err := Save(dir, "claude", Agent{Name: "gl", Harness: "claude", Instructions: "x", Source: "global"}); err == nil {
		t.Fatal("global agent written in place")
	}
	if err := Save(dir, "claude", Agent{Name: "gl", Harness: "claude", Instructions: "---\nname: gl\n---\ncopy\n", Source: "repo"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(roles.GlobalDir(), "claude", "gl.md")); !strings.Contains(string(b), "global") {
		t.Fatal("copy to repo changed the global file")
	}
}

func TestSaveRefusesTraversalName(t *testing.T) {
	dir := setup(t)
	if err := Save(dir, "claude", Agent{Name: "../x", Harness: "claude", Instructions: "x"}); err == nil {
		t.Fatal("traversal name accepted")
	}
}

func TestListKeepsFileHarnessWhenBoardOmitsAgent(t *testing.T) {
	dir := setup(t)
	os.WriteFile(tasksBoard(dir), []byte("roles:\n  scribe: {model: m}\n"), 0644)
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".claude", "agents", "scribe.md"), []byte("---\ndescription: s\n---\nWrite."), 0644)
	a, ok := Find(dir, "claude", "scribe")
	if !ok || a.Harness != "claude" || a.Model != "m" {
		t.Fatalf("scribe: %+v", a)
	}
}

func TestSaveWithoutInstructionsKeepsFile(t *testing.T) {
	dir := setup(t)
	if err := Save(dir, "claude", Agent{Name: "writer", Harness: "claude", Instructions: Template("writer")}); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, filepath.Join(dir, ".claude", "agents", "writer.md"))
	// A model-only save (web model/effort) must not drop the frontmatter or the body.
	if err := Save(dir, "claude", Agent{Name: "writer", Harness: "claude", Model: "m2"}); err != nil {
		t.Fatal(err)
	}
	if after := readFile(t, filepath.Join(dir, ".claude", "agents", "writer.md")); after != before {
		t.Fatalf("instruction file rewritten:\n%s", after)
	}
	if !strings.Contains(board(t, dir), "model: m2") {
		t.Fatal("model not saved")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func tasksBoard(dir string) string { return filepath.Join(dir, ".tasks", "board.yaml") }
