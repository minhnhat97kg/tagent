package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/library"
)

func codexFixture(t *testing.T) (o spawnOpts, base string) {
	t.Helper()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cwd, base := t.TempDir(), t.TempDir()
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(base, "config.toml"), "model = \"m\"\n[mcp_servers.old]\ncommand = \"old\"\n")
	write(filepath.Join(base, "auth.json"), "{}")
	write(filepath.Join(base, "skills", "mine", "SKILL.md"), "mine")
	write(filepath.Join(cwd, ".tasks", "library", "skills", "demo", "SKILL.md"), "---\nname: demo\n---\n")
	write(filepath.Join(cwd, ".tasks", "library", "mcp.yml"), "servers:\n  - name: same\n    command: srv\n    env: {TOKEN: \"${TOKEN}\"}\n  - name: renamed\n    command: srv\n    env: {KEY: \"${TOKEN}\"}\n")
	write(filepath.Join(cwd, ".tasks", "board.yaml"), `roles:
  both: {agent: codex, skills: [demo], mcp: [tagent]}
  noskills: {agent: codex, skills: []}
  nomcp: {agent: codex, mcp: []}
  same: {agent: codex, mcp: [same]}
  renamed: {agent: codex, mcp: [renamed]}
  plain: {agent: codex}
`)
	return spawnOpts{Agent: "codex", ConfigDir: cwd}, base
}

func codexConfig(t *testing.T, o spawnOpts, role, base string) (home, config string) {
	t.Helper()
	home, err := codexRoleHome(o, role, base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return home, string(b)
}

func TestCodexHomeGenerated(t *testing.T) {
	o, base := codexFixture(t)
	before, _ := os.ReadFile(filepath.Join(base, "config.toml"))
	home, config := codexConfig(t, o, "both", base)
	if !strings.Contains(config, `model = "m"`) || !strings.Contains(config, "mcp_servers.tagent") || strings.Contains(config, "command = \"old\"") {
		t.Errorf("config:\n%s", config)
	}
	if _, err := os.Stat(filepath.Join(home, "skills", "demo", "SKILL.md")); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(filepath.Join(home, "skills", "mine")); err == nil {
		t.Error("an unselected skill was linked")
	}
	if target, _ := os.Readlink(filepath.Join(home, "auth.json")); target != filepath.Join(base, "auth.json") {
		t.Errorf("auth link %q", target)
	}
	if again, _ := codexConfig(t, o, "both", base); again == home {
		t.Error("two launches share a home")
	}
	if after, _ := os.ReadFile(filepath.Join(base, "config.toml")); string(after) != string(before) {
		t.Error("the user's config changed")
	}
	if args, err := roleArgs(o, "both"); err != nil || len(args) != 0 {
		t.Errorf("codex flags %q %v", args, err)
	}
}

// An unset field keeps what the user has; an empty list means none.
func TestCodexHomeDefaultsAndNone(t *testing.T) {
	o, base := codexFixture(t)
	home, config := codexConfig(t, o, "noskills", base)
	if entries, _ := os.ReadDir(filepath.Join(home, "skills")); len(entries) != 0 || !strings.Contains(config, "[mcp_servers.old]") {
		t.Errorf("skills none: %d skills\n%s", len(entries), config)
	}
	home, config = codexConfig(t, o, "nomcp", base)
	if _, err := os.Stat(filepath.Join(home, "skills", "mine")); err != nil || strings.Contains(config, "mcp_servers") {
		t.Errorf("mcp none: %v\n%s", err, config)
	}
	if home, err := codexRoleHome(o, "plain", base); err != nil || home != "" {
		t.Errorf("a role without a selection got a home: %q %v", home, err)
	}
}

func TestCodexHomeNoAuth(t *testing.T) {
	o, base := codexFixture(t)
	os.Remove(filepath.Join(base, "auth.json"))
	home, _ := codexConfig(t, o, "both", base)
	if _, err := os.Lstat(filepath.Join(home, "auth.json")); err == nil {
		t.Error("linked a login that does not exist")
	}
}

func TestCodexHomeKeepsEnvRefs(t *testing.T) {
	o, base := codexFixture(t)
	t.Setenv("TOKEN", "s3cret")
	if _, err := codexRoleHome(o, "same", base); err == nil {
		t.Fatal("an unapproved server launched")
	}
	lib, err := library.Load(o.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"same", "renamed"} {
		if err := library.Approve(filepath.Join(dir, "tagent"), lib.Servers[name]); err != nil {
			t.Fatal(err)
		}
	}
	if _, config := codexConfig(t, o, "same", base); !strings.Contains(config, `env_vars = ["TOKEN"]`) || strings.Contains(config, "s3cret") {
		t.Errorf("config:\n%s", config)
	}
	if _, err := codexRoleHome(o, "renamed", base); err == nil {
		t.Error("a renamed variable must be reported, not written")
	}
}
