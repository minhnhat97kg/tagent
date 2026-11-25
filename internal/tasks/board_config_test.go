package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoardConfigErrorLine(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		line int
		want string
	}{
		{"missing todo/done", "columns: [todo]\n", 1, "columns must include todo and done"},
		{"semantic status line", "statuses:\n  - {id: todo}\n  - {id: build, run: go test}\n  - {id: done}\n", 3, "requires pass and fail"},
		{"role line", "roles:\n  slow: {agent: pi, effort: ultra}\n", 2, "role slow"},
		{"trigger line", "triggers:\n  - on: nope\n    agent: planner\n", 2, "invalid trigger kind"},
		{"cycle line", "statuses:\n  - {id: todo, next: [ghost]}\n  - {id: done}\n", 2, "invalid next"},
		{"syntax", "columns: [todo, done]\nstatuses:\n  - {id: todo\n", 2, "yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseBoardConfig([]byte(tc.yaml))
			if err == nil {
				t.Fatal("accepted")
			}
			if line := BoardConfigErrorLine(err); line != tc.line {
				t.Fatalf("line = %d, want %d: %v", line, tc.line, err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%v does not mention %q", err, tc.want)
			}
		})
	}
	if BoardConfigErrorLine(nil) != 0 {
		t.Fatal("nil error has a line")
	}
}

func TestBoardConfigSummary(t *testing.T) {
	cfg, err := ParseBoardConfig([]byte(`roles:
  reviewer: {agent: claude, model: opus}
  builder: {agent: pi, effort: high, fresh_after: 3}
default_workflow: feature
triggers:
  - on: mention
    when: {to: '*', from: user}
    agent: '{{event.name}}'
`))
	if err != nil {
		t.Fatal(err)
	}
	sum := cfg.Summary()
	if sum.DefaultWorkflow != "feature" || len(sum.Roles) != 2 || len(sum.Triggers) != 1 || sum.MaxFiresPerHour != 20 {
		t.Fatalf("summary: %+v", sum)
	}
	if sum.Roles[0].Name != "builder" || sum.Roles[1].Name != "reviewer" {
		t.Fatalf("roles not sorted: %+v", sum.Roles)
	}
	if sum.Roles[0].Effort != "high" || sum.Roles[0].FreshAfter == nil || *sum.Roles[0].FreshAfter != 3 || sum.Roles[1].Model != "opus" {
		t.Fatalf("role details: %+v", sum.Roles)
	}
	if sum.Triggers[0].When != "to=* from=user" || sum.Triggers[0].Agent != "{{event.name}}" {
		t.Fatalf("trigger: %+v", sum.Triggers[0])
	}
}

// Old board.yaml files with statuses load as the implicit "board" workflow with
// the same status ids, and their next/pass/fail semantics keep working.
func TestLegacyStatusesLoadAsBoardWorkflow(t *testing.T) {
	cfg, err := ParseBoardConfig([]byte(`roles:
  builder: {agent: pi}
statuses:
  - {id: todo, next: [plan, build]}
  - {id: plan, role: builder, next: [build, blocked]}
  - {id: build, run: go test ./..., pass: review, fail: plan}
  - {id: review, role: builder, next: [done, build, blocked], max_loops: 3}
  - {id: blocked}
  - {id: done}
`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Legacy || cfg.DefaultWorkflow != "board" {
		t.Fatalf("legacy flags: %+v", cfg)
	}
	if len(cfg.Warnings) == 0 || !strings.Contains(cfg.Warnings[0], "deprecated") {
		t.Fatalf("missing deprecation warning: %v", cfg.Warnings)
	}
	w, err := FindWorkflow(t.TempDir(), cfg, "board")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, j := range w.Jobs {
		ids = append(ids, j.ID)
	}
	if strings.Join(ids, ",") != "plan,build,review" {
		t.Fatalf("job ids: %v", ids)
	}
	if w.Jobs[0].Status != "plan" || w.Jobs[1].Status != "build" {
		t.Fatalf("status ids changed: %+v", w.Jobs)
	}
	// A ticket on the implicit board workflow keeps the legacy status table,
	// including its arbitrary next edges.
	ticket := Task{Workflow: "board", Status: "review"}
	flow, err := ConfigForTask(t.TempDir(), cfg, ticket)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range flow.Statuses {
		if s.ID == "review" && !strings.Contains(strings.Join(s.Next, ","), "build") {
			t.Fatalf("legacy next lost: %+v", s)
		}
	}
}

func TestColumnsOnlyBoardStaysInert(t *testing.T) {
	cfg, err := ParseBoardConfig([]byte("columns: [todo, doing, review, blocked, done]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Legacy || cfg.DefaultWorkflow != "" {
		t.Fatalf("columns-only board got a flow: %+v", cfg)
	}
	if len(cfg.Warnings) == 0 {
		t.Fatal("columns key not warned about")
	}
}

func TestDefaultWorkflowMustExist(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".tasks", "workflows"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseBoardConfigFor(dir, []byte("default_workflow: missing\n")); err == nil || !strings.Contains(err.Error(), "default_workflow") {
		t.Fatalf("missing workflow accepted: %v", err)
	}
	path := filepath.Join(dir, ".tasks", "workflows", "feature.yml")
	if err := os.WriteFile(path, []byte("name: feature\njobs:\n  build: {agent: implementer}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseBoardConfigFor(dir, []byte("default_workflow: feature\n"))
	if err != nil || cfg.DefaultWorkflow != "feature" {
		t.Fatalf("valid default rejected: %+v %v", cfg, err)
	}
}

func TestSetDefaultWorkflow(t *testing.T) {
	dir := t.TempDir()
	wf := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(wf, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"feature", "review"} {
		if err := os.WriteFile(filepath.Join(wf, name+".yml"), []byte("name: "+name+"\njobs:\n  build: {agent: implementer}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := BoardConfigPath(dir)
	source := "# keep this comment\nroles:\n  planner: {agent: pi} # inline comment\nmax_fires_per_hour: 9\n"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	read := func() string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	if err := SetDefaultWorkflow(dir, "feature"); err != nil {
		t.Fatal(err)
	}
	got := read()
	for _, want := range []string{"# keep this comment", "# inline comment", "max_fires_per_hour: 9", "default_workflow: feature"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "default_workflow:") != 1 {
		t.Fatalf("duplicate key:\n%s", got)
	}

	if err := SetDefaultWorkflow(dir, "review"); err != nil {
		t.Fatal(err)
	}
	if got := read(); !strings.Contains(got, "default_workflow: review") || strings.Contains(got, "default_workflow: feature") {
		t.Fatalf("replace:\n%s", got)
	}

	before := read()
	err := SetDefaultWorkflow(dir, "ghost")
	if err == nil || !strings.Contains(err.Error(), "default_workflow") || BoardConfigErrorLine(err) == 0 {
		t.Fatalf("unknown workflow: %v (line %d)", err, BoardConfigErrorLine(err))
	}
	if read() != before {
		t.Fatal("rejected name changed the file")
	}

	if err := SetDefaultWorkflow(dir, ""); err != nil {
		t.Fatal(err)
	}
	if got := read(); strings.Contains(got, "default_workflow") {
		t.Fatalf("remove:\n%s", got)
	}

	// A missing file is created from the example template.
	other := t.TempDir()
	owf := filepath.Join(other, ".tasks", "workflows")
	if err := os.MkdirAll(owf, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(owf, "feature.yml"), []byte("name: feature\njobs:\n  build: {agent: implementer}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SetDefaultWorkflow(other, "feature"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(BoardConfigPath(other))
	if err != nil || !strings.Contains(string(b), "default_workflow: feature") {
		t.Fatalf("created:\n%s\n%v", b, err)
	}
}

func TestRemoteBoardConfigValidated(t *testing.T) {
	for _, src := range []string{
		"remote: {host: \"-oProxyCommand=x\", dir: /srv/p}\n",
		"remote: {host: devbox, dir: relative/p}\n",
	} {
		if _, err := ParseBoardConfigFor(t.TempDir(), []byte(src)); err == nil || BoardConfigErrorLine(err) != 1 {
			t.Fatalf("%q accepted or wrong line: %v", src, err)
		}
	}
	cfg, err := ParseBoardConfigFor(t.TempDir(), []byte("remote: {host: devbox, dir: /srv/p}\n"))
	if err != nil || cfg.Remote.Host != "devbox" || cfg.Remote.Dir != "/srv/p" {
		t.Fatalf("valid remote rejected: %+v %v", cfg.Remote, err)
	}
}
