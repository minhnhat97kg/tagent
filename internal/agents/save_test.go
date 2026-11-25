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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func byName(list []Agent) map[string]Agent {
	m := map[string]Agent{}
	for _, a := range list {
		m[a.Name] = a
	}
	return m
}

func TestListMergesRepoBoardAndUsage(t *testing.T) {
	dir := setup(t)
	writeFile(t, roles.LocalPath(dir, "researcher"), "Find the facts.")
	got := List(dir, "claude", []LivePane{{Role: "planner", Pane: "%1"}, {Role: "planner", Pane: "%2"}, {Role: "other", Pane: "%3"}})
	names := byName(got)
	p := names["planner"]
	if p.Harness != "pi" || p.Model != "openai-codex/gpt-6.1-sol" || p.Effort != "high" || p.Source != "board" || p.Running != 2 || len(p.Panes) != 2 || p.Panes[1].Pane != "%2" {
		t.Fatalf("planner: %+v", p)
	}
	// Flows live in workflows only; a deprecated board status no longer counts as usage.
	if len(p.UsedBy) != 0 {
		t.Fatalf("usedBy: %v", p.UsedBy)
	}
	r := names["reviewer"]
	if r.Source != "board" || r.Harness != "claude" {
		t.Fatalf("reviewer: %+v", r)
	}
	res := names["researcher"]
	if res.Source != "repo" || res.Harness != "claude" || res.Instructions != "Find the facts." || res.Path != roles.LocalPath(dir, "researcher") {
		t.Fatalf("researcher: %+v", res)
	}
}

// The process harness never filters the list: every instruction file and board
// role shows under any harness, and unresolved ones use the process harness.
func TestListIsNotHarnessFiltered(t *testing.T) {
	dir := setup(t)
	writeFile(t, roles.LocalPath(dir, "researcher"), "Find the facts.")
	writeFile(t, roles.LocalPath(dir, "scribe"), "Write it down.")
	for _, harness := range []string{"claude", "pi", "codex", "opencode"} {
		names := byName(List(dir, harness, nil))
		for _, want := range []string{"planner", "reviewer", "researcher", "scribe"} {
			if _, ok := names[want]; !ok {
				t.Fatalf("%s: missing %s in %v", harness, want, names)
			}
		}
		if names["researcher"].Harness != harness || names["scribe"].Harness != harness {
			t.Fatalf("%s: file-only agents must use the process harness: %+v", harness, names)
		}
	}
	// A board role overrides the process harness for its file.
	writeFile(t, tasksBoard(dir), "roles:\n  researcher: {agent: pi, model: m}\n")
	if a := byName(List(dir, "claude", nil))["researcher"]; a.Harness != "pi" || a.Model != "m" {
		t.Fatalf("board harness override: %+v", a)
	}
}

// Legacy harness directories are still read and shown as local, but the app
// never writes (or deletes) them.
func TestLegacyDirsAreReadOnly(t *testing.T) {
	dir := setup(t)
	legacy := filepath.Join(roles.DirFor(dir, "claude"), "old.md")
	writeFile(t, legacy, "---\nname: old\ndescription: Old role\n---\nOld instructions.")
	a, ok := Find(dir, "pi", "old")
	if !ok || a.Source != "legacy" || a.Harness != "pi" || a.Instructions != "---\nname: old\ndescription: Old role\n---\nOld instructions." {
		t.Fatalf("legacy: %+v", a)
	}
	if err := Save(dir, "pi", Agent{Name: "old", Harness: "pi", Instructions: "new"}); err == nil {
		t.Fatal("legacy file was written")
	}
	if err := Delete(dir, "pi", "old"); err == nil {
		t.Fatal("legacy file was deleted")
	}
	if b, _ := os.ReadFile(legacy); !strings.Contains(string(b), "Old instructions.") {
		t.Fatalf("legacy file changed: %s", b)
	}
	// Copy to repo creates the app-owned file and leaves the legacy one alone.
	if err := Save(dir, "pi", Agent{Name: "old", Harness: "pi", Source: "repo", Instructions: "new"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(roles.LocalPath(dir, "old")); string(b) != "new\n" {
		t.Fatalf("copy to repo: %q", b)
	}
	if b, _ := os.ReadFile(legacy); !strings.Contains(string(b), "Old instructions.") {
		t.Fatal("copy changed the legacy file")
	}
	if a, _ := Find(dir, "pi", "old"); a.Source != "repo" || a.Instructions != "new" {
		t.Fatalf("copy did not shadow legacy: %+v", a)
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
		{Name: "g", Harness: "claude", Source: "global"},
		{Name: "g", Harness: "claude", Source: "legacy"},
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
	path := roles.LocalPath(dir, "writer")
	b, err := os.ReadFile(path)
	if err != nil || string(b) != Template("writer")+"\n" || strings.HasPrefix(string(b), "---") || !strings.Contains(string(b), "# writer") {
		t.Fatalf("file: %v %q", err, b)
	}
	if !strings.Contains(board(t, dir), "writer:") {
		t.Fatal("created agent did not record its harness in board.yaml")
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
		t.Fatalf("save by name did not update the app file: %q", b)
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
	if exists(path) || strings.Contains(board(t, dir), "writer:") {
		t.Fatal("agent not deleted")
	}
}

// Pi roles keep plain markdown instructions like every other harness.
func TestSavePiInstructions(t *testing.T) {
	dir := setup(t)
	if err := Save(dir, "claude", Agent{Name: "implementer", Harness: "pi", Source: "board", Instructions: "Implement it."}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(roles.LocalPath(dir, "implementer")); string(b) != "Implement it.\n" {
		t.Fatalf("pi instructions: %q", b)
	}
	a, _ := Find(dir, "claude", "implementer")
	if a.Harness != "pi" || a.Source != "repo" || a.Instructions != "Implement it." {
		t.Fatalf("implementer: %+v", a)
	}
}

func TestEnsureFileCreatesAppFile(t *testing.T) {
	dir := setup(t)
	a, err := EnsureFile(dir, "claude", Agent{Name: "newbie", Harness: "pi", Source: "board"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Path != roles.LocalPath(dir, "newbie") || a.Source != "repo" || !strings.Contains(a.Instructions, "Describe how this agent works.") {
		t.Fatalf("newbie: %+v", a)
	}
	if b, _ := os.ReadFile(a.Path); strings.HasPrefix(string(b), "---") {
		t.Fatalf("template carries frontmatter: %q", b)
	}
	// An existing app-owned file is kept as is.
	again, err := EnsureFile(dir, "claude", a)
	if err != nil || again.Path != a.Path || again.Instructions != a.Instructions {
		t.Fatalf("existing file: %+v %v", again, err)
	}
	// A legacy file is copied into the repo, which is where edits go.
	legacy := filepath.Join(roles.DirFor(dir, "claude"), "old.md")
	writeFile(t, legacy, "old")
	if _, err := EnsureFile(dir, "claude", Agent{Name: "old", Harness: "claude", Source: "legacy", Path: legacy}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(roles.LocalPath(dir, "old")); string(b) != Template("old")+"\n" {
		t.Fatalf("copy to repo: %q", b)
	}
}

// The request's path is never trusted: a save writes only to the app's file for the name.
func TestSaveIgnoresRequestPath(t *testing.T) {
	dir := setup(t)
	victim := filepath.Join(t.TempDir(), "victim.md")
	writeFile(t, victim, "keep")
	if err := Save(dir, "claude", Agent{Name: "writer", Harness: "claude", Path: victim, Source: "repo", Instructions: "x"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("write escaped to %s: %q", victim, b)
	}
	if !exists(roles.LocalPath(dir, "writer")) {
		t.Fatal("new file not created under .tasks/agents")
	}
}

func TestSaveGlobalInPlaceAndCopy(t *testing.T) {
	dir := setup(t)
	before := board(t, dir)
	writeFile(t, filepath.Join(roles.GlobalDir(), "gl.md"), "global\n")
	if err := Save(dir, "claude", Agent{Name: "gl", Harness: "claude", Instructions: "global edited", Source: "global"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(roles.GlobalDir(), "gl.md")); string(b) != "global edited\n" {
		t.Fatalf("global edit: %q", b)
	}
	if exists(roles.LocalPath(dir, "gl")) || board(t, dir) != before {
		t.Fatal("global edit leaked into the repo")
	}
	if err := Save(dir, "claude", Agent{Name: "gl", Harness: "claude", Instructions: "copy", Source: "repo"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(roles.GlobalDir(), "gl.md")); string(b) != "global edited\n" {
		t.Fatal("copy to repo changed the global file")
	}
	if b, _ := os.ReadFile(roles.LocalPath(dir, "gl")); string(b) != "copy\n" {
		t.Fatalf("copy to repo: %q", b)
	}
}

func TestSaveRefusesTraversalName(t *testing.T) {
	dir := setup(t)
	if err := Save(dir, "claude", Agent{Name: "../x", Harness: "claude", Instructions: "x"}); err == nil {
		t.Fatal("traversal name accepted")
	}
}

// A legacy file without a role entry shows the process harness, not its directory.
func TestListLegacyUsesProcessHarnessWithoutRole(t *testing.T) {
	dir := setup(t)
	writeFile(t, tasksBoard(dir), "roles:\n  scribe: {model: m}\n")
	writeFile(t, filepath.Join(roles.DirFor(dir, "claude"), "scribe.md"), "---\ndescription: s\n---\nWrite.")
	a, ok := Find(dir, "pi", "scribe")
	if !ok || a.Harness != "pi" || a.Model != "m" || a.Source != "legacy" {
		t.Fatalf("scribe: %+v", a)
	}
	if a := byName(List(dir, "claude", nil))["scribe"]; a.Harness != "claude" {
		t.Fatalf("scribe under claude: %+v", a)
	}
}

func TestSaveWithoutInstructionsKeepsFile(t *testing.T) {
	dir := setup(t)
	path := roles.LocalPath(dir, "writer")
	writeFile(t, path, "# writer\n\nHand written.\n")
	before := readFile(t, path)
	// A model-only save (web model/effort) must not create or drop the file.
	if err := Save(dir, "claude", Agent{Name: "writer", Harness: "claude", Model: "m2"}); err != nil {
		t.Fatal(err)
	}
	if after := readFile(t, path); after != before {
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

func TestTierRoleRoundTrip(t *testing.T) {
	dir := setup(t)
	if err := Save(dir, "pi", Agent{Name: "planner", Tier: "fast", Effort: "low", Source: "board"}); err != nil {
		t.Fatal(err)
	}
	a, _ := Find(dir, "pi", "planner")
	if a.Tier != "fast" || a.Harness != "" || a.Model != "" || a.Effort != "low" {
		t.Fatalf("tier role %+v", a)
	}
	if b := board(t, dir); !strings.Contains(b, "# the planner picks the plan") || strings.Contains(b, "agent: pi") {
		t.Fatalf("board after tier:\n%s", b)
	}
	if err := Save(dir, "pi", Agent{Name: "planner", Tier: "fast", Model: "x", Source: "board"}); err == nil {
		t.Fatal("tier with model should be refused")
	}
	if err := Save(dir, "pi", Agent{Name: "planner", Harness: "claude", Model: "opus", Source: "board"}); err != nil {
		t.Fatal(err)
	}
	a, _ = Find(dir, "pi", "planner")
	if a.Tier != "" || a.Harness != "claude" || a.Model != "opus" {
		t.Fatalf("fixed role %+v", a)
	}
	if b := board(t, dir); strings.Contains(b, "tier:") || !strings.Contains(b, "# the planner picks the plan") {
		t.Fatalf("board after fixed:\n%s", b)
	}
}
