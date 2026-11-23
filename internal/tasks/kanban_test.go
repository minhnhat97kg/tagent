package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func testBoard(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	return dir, Scope(dir)
}
func TestKanbanMutationEvents(t *testing.T) {
	dir, scope := testBoard(t)
	if err := SaveBoardConfig(dir, []byte("columns: [todo, verify, done]\n")); err != nil {
		t.Fatal(err)
	}
	task, err := Add(scope, "Title\nDescription", []string{"planner"})
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Title" {
		t.Fatal(task)
	}
	if _, err = Update(scope, task.ID, "review", nil); err == nil {
		t.Fatal("unconfigured status accepted")
	}
	if _, err = Update(scope, task.ID, "verify", []string{"implementer"}); err != nil {
		t.Fatal(err)
	}
	before, _ := ReadEvents(scope, 0)
	Update(scope, task.ID, "verify", []string{"implementer"})
	after, _ := ReadEvents(scope, 0)
	if len(before) != len(after) {
		t.Fatal("no-op emitted events")
	}
	if err = SetDue(scope, task.ID, "2026-02-30"); err == nil {
		t.Fatal("bad date")
	}
	if err = SetDue(scope, task.ID, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	if err = SetDue(scope, task.ID, ""); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "diff.patch")
	os.WriteFile(src, []byte("a diff"), 0600)
	if err = Comment(scope, task.ID, "user", "Hello @planner, @planner and @dev.one", []string{src}); err != nil {
		t.Fatal(err)
	}
	task, _ = Get(scope, task.ID)
	a := task.Comments[0].Attachments[0]
	f, err := OpenAttachment(scope, task.ID, a.File)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err = OpenAttachment(scope, task.ID, "../diff.patch"); err == nil {
		t.Fatal("traversal accepted")
	}
	if err = EditDetails(scope, task.ID, "Independent", "new body"); err != nil {
		t.Fatal(err)
	}
	Edit(scope, task.ID, "another body")
	SetNote(scope, task.ID, "done")
	done, err := Update(scope, task.ID, "done", nil)
	if err != nil || done.CompletedAt.IsZero() {
		t.Fatal(done, err)
	}
	reopened, err := Update(scope, task.ID, "todo", nil)
	if err != nil || !reopened.CompletedAt.IsZero() || reopened.Title != "Independent" {
		t.Fatal(reopened, err)
	}
	ev, err := ReadEvents(scope, 0)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for i, e := range ev {
		if e.Seq != int64(i+1) {
			t.Fatal(e)
		}
		kinds = append(kinds, e.Kind)
	}
	want := []string{"create", "assign", "assign", "assign", "status", "due", "due", "comment", "mention", "status", "status"}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("events %v", kinds)
	}
	if !reflect.DeepEqual(ev[8].Names, []string{"planner", "dev.one"}) {
		t.Fatal(ev[8])
	}
	// Simulate a torn append; the next mutation trims it before appending.
	log, _ := os.OpenFile(filepath.Join(BoardDir(scope), "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	log.WriteString(`{"seq":`)
	log.Close()
	if err = SetDue(scope, task.ID, "2026-12-02"); err != nil {
		t.Fatal(err)
	}
	ev, err = ReadEvents(scope, 0)
	if err != nil || len(ev) != 12 {
		t.Fatal(len(ev), err)
	}
}
func TestOldTaskAndMentions(t *testing.T) {
	_, scope := testBoard(t)
	os.MkdirAll(BoardDir(scope), 0700)
	old := []byte(`{"id":"abc","text":"Old title\nbody","status":"todo","note":"kept"}`)
	os.WriteFile(file(scope, "abc"), old, 0600)
	task, err := Get(scope, "abc")
	if err != nil || task.Title != "Old title" || task.Comments == nil || task.Note != "kept" {
		t.Fatal(task, err)
	}
	got, _ := os.ReadFile(file(scope, "abc"))
	if string(got) != string(old) {
		t.Fatal("read rewrote file")
	}
	if got := Mentions("mail a@b.com @one @two_2 @one (@dev-x) @@skip"); !reflect.DeepEqual(got, []string{"one", "two_2", "dev-x"}) {
		t.Fatal(got)
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(file(scope, "abc"), link)
	if err = Comment(scope, "abc", "u", "x", []string{link}); err == nil {
		t.Fatal("symlink accepted")
	}
}
func TestConcurrentEventSequence(t *testing.T) {
	_, scope := testBoard(t)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := Add(scope, fmt.Sprint(i), nil); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	ev, err := ReadEvents(scope, 0)
	if err != nil || len(ev) != 12 {
		t.Fatal(len(ev), err)
	}
	for i, e := range ev {
		if e.Seq != int64(i+1) {
			b, _ := json.Marshal(ev)
			t.Fatal(string(b))
		}
	}
}
func TestBoardConfigValidation(t *testing.T) {
	for _, s := range []string{"columns: [todo]", "columns: [todo, done, todo]", "max_fires_per_hour: -1", "oops: yes", "triggers: [{on: xyz, agent: a}]", "triggers: [{on: 'create|assign', agent: a}]", "triggers: [{on: assign, agent: a, workflow: b}]", "triggers: [{on: mention, agent: '{{bad}}'}]", "triggers: [{on: assign, when: {oops: x}, agent: a}]", "---\ncolumns: [todo,done]\n---\nfoo: bar"} {
		if _, err := ParseBoardConfig([]byte(s)); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	dir, _ := testBoard(t)
	global := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "tagent")
	os.MkdirAll(global, 0700)
	os.WriteFile(filepath.Join(global, "board.yaml"), []byte("columns: [todo, global, done]\nroles: {planner: {agent: pi, effort: high}}"), 0600)
	c, err := LoadBoardConfig(dir)
	if err != nil || c.Columns[1] != "global" || c.Roles["planner"].Effort != "high" {
		t.Fatal(c, err)
	}
	SaveBoardConfig(dir, []byte("triggers: []\nroles: {planner: {agent: pi, effort: low}}"))
	c, err = LoadBoardConfig(dir)
	if err != nil || len(c.Triggers) != 0 || c.Columns[1] != "doing" || c.Roles["planner"].Effort != "low" {
		t.Fatal(c, err)
	}
}
