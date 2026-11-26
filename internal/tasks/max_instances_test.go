package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoleMaxInstancesParse(t *testing.T) {
	cfg, err := ParseBoardConfig([]byte("roles:\n  builder: {agent: pi, max_instances: 2}\n  reviewer: {agent: pi}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Roles["builder"].MaxInstances != 2 || cfg.Roles["reviewer"].MaxInstances != 0 {
		t.Fatalf("roles: %+v", cfg.Roles)
	}
	if sum := cfg.Summary(); sum.Roles[0].MaxInstances != 2 {
		t.Fatalf("summary: %+v", sum.Roles)
	}
	for _, value := range []string{"-1", "1.5", "many"} {
		_, err := ParseBoardConfig([]byte("roles:\n  builder: {agent: pi, max_instances: " + value + "}\n"))
		if err == nil {
			t.Fatalf("max_instances %s accepted", value)
		}
	}
	_, err = ParseBoardConfig([]byte("roles:\n  slow: {agent: pi,\n    max_instances: -2}\n"))
	if err == nil || !strings.Contains(err.Error(), "max_instances must be 0 or more") {
		t.Fatalf("negative: %v", err)
	}
}

func TestSetRoleMaxInstancesKeepsComments(t *testing.T) {
	dir := t.TempDir()
	src := `# top comment
roles:
  # builder comment
  builder: {agent: pi} # line
  reviewer: {agent: pi, max_instances: 1}
default_workflow: ""
triggers: []
max_fires_per_hour: 20
`
	path := BoardConfigPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SetRoleMaxInstances(dir, "builder", 3); err != nil {
		t.Fatal(err)
	}
	if err := SetRoleMaxInstances(dir, "reviewer", 0); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	out := string(b)
	for _, want := range []string{"# top comment", "# builder comment", "# line", "max_instances: 3", "default_workflow"} {
		if !strings.Contains(out, want) {
			t.Fatalf("lost %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "reviewer: {agent: pi, max_instances") {
		t.Fatalf("reviewer limit not removed:\n%s", out)
	}
	cfg, err := LoadBoardConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Roles["builder"].MaxInstances != 3 || cfg.Roles["reviewer"].MaxInstances != 0 {
		t.Fatalf("roles: %+v", cfg.Roles)
	}
	if strings.Index(out, "builder") > strings.Index(out, "reviewer") {
		t.Fatalf("order changed:\n%s", out)
	}
	if err := SetRoleMaxInstances(dir, "ghost", 1); err == nil {
		t.Fatal("unknown role accepted")
	}
	if err := SetRoleMaxInstances(dir, "builder", -1); err == nil {
		t.Fatal("negative accepted")
	}
}
