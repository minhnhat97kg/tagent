package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/roles"
)

func TestRolesAPI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	os.MkdirAll(filepath.Join(dir, ".tasks"), 0755)
	os.WriteFile(filepath.Join(dir, ".tasks", "board.yaml"), []byte("# keep me\nroles:\n  planner: {agent: pi, model: m1}\n"), 0644)
	s := &server{dir: dir}
	call := func(method, body string, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		s.roles(w, httptest.NewRequest(method, "/api/roles", strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, body, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if out := call("GET", ``, 200); !strings.Contains(out, `"name":"planner"`) || !strings.Contains(out, `"source":"board"`) {
		t.Fatalf("list: %s", out)
	} else if !strings.Contains(out, `"panes":`) || !strings.Contains(out, `"usedBy":`) {
		t.Fatalf("list must expose live panes and usage: %s", out)
	}
	call("POST", `{"name":"planner","harness":"pi","model":"m2","effort":"high"}`, 200)
	call("POST", `{"name":"planner","harness":"pi","effort":"ultra"}`, 400)
	call("POST", `{"name":"","harness":"pi"}`, 400)
	call("POST", `{"name":"x","harness":"opencode"}`, 400)
	call("POST", `{"name":"../x","harness":"claude","instructions":"hi"}`, 400)
	call("POST", `{"name":"writer","harness":"claude","instructions":"---\nname: writer\n---\nWrite."}`, 200)
	call("DELETE", `{"name":"nobody"}`, 400)
	call("DELETE", `{"name":"writer"}`, 200)
	b, _ := os.ReadFile(filepath.Join(dir, ".tasks", "board.yaml"))
	if !strings.Contains(string(b), "# keep me") || !strings.Contains(string(b), "model: m2") || strings.Contains(string(b), "writer") {
		t.Fatalf("board:\n%s", b)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "agents", "writer.md")); !os.IsNotExist(err) {
		t.Fatal("repo file not deleted")
	}
	os.MkdirAll(filepath.Join(roles.GlobalDir(), "claude"), 0755)
	os.WriteFile(filepath.Join(roles.GlobalDir(), "claude", "gl.md"), []byte("---\nname: gl\n---\nglobal\n"), 0644)
	call("DELETE", `{"name":"gl"}`, 400)
	call("POST", `{"name":"gl","harness":"claude","instructions":"x","source":"global"}`, 400)
	if _, err := os.Stat(filepath.Join(roles.GlobalDir(), "claude", "gl.md")); err != nil {
		t.Fatal("global file removed")
	}
}
