package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func boardConfigCall(t *testing.T, s *server, method, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, "/api/board/config", strings.NewReader(body))
	s.boardConfig(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: %d %s", method, body, w.Code, w.Body.String())
	}
	return w
}

func TestBoardConfigAPI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no user-level fallback in tests
	s := &server{dir: dir}
	path := filepath.Join(dir, ".tasks", "board.yaml")

	// No local file: the editor is empty but the effective defaults are shown.
	w := boardConfigCall(t, s, "GET", "", http.StatusOK)
	var got boardConfigJSON
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Exists || got.YAML != "" || got.Summary == nil || got.Default == "" {
		t.Fatalf("missing config: %+v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("GET created the file")
	}

	// Invalid YAML is refused with the line, and nothing is written.
	w = boardConfigCall(t, s, "PUT", `{"yaml":"columns: [todo]\n"}`, http.StatusBadRequest)
	if g := w.Body.String(); !strings.Contains(g, `"line":1`) || !strings.Contains(g, "todo and done") {
		t.Fatalf("error line: %s", g)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid config was saved")
	}

	// Validate-only previews the summary and never saves.
	source := "# keep this comment\ncolumns: [todo, doing, done]\nmax_fires_per_hour: 4\ntriggers:\n  - on: assign\n    when: {to: builder}\n    agent: builder\n"
	body, _ := json.Marshal(map[string]any{"yaml": source, "validate": true})
	boardConfigCall(t, s, "PUT", string(body), http.StatusOK)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("validate-only saved the file")
	}

	// A valid save keeps comments byte for byte.
	body, _ = json.Marshal(map[string]any{"yaml": source})
	boardConfigCall(t, s, "PUT", string(body), http.StatusOK)
	saved, err := os.ReadFile(path)
	if err != nil || string(saved) != source {
		t.Fatalf("saved:\n%s\n%v", saved, err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}

	// GET returns the raw YAML and the same summary the TUI builds.
	w = boardConfigCall(t, s, "GET", "", http.StatusOK)
	got = boardConfigJSON{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Exists || got.YAML != source || got.Summary == nil || got.Summary.MaxFiresPerHour != 4 || len(got.Summary.Triggers) != 1 {
		t.Fatalf("read back: %+v", got)
	}
	if got.Summary.Triggers[0].When != "to=builder" {
		t.Fatalf("trigger: %+v", got.Summary.Triggers[0])
	}

	// A rejected save leaves the previous file untouched (atomic refusal).
	body, _ = json.Marshal(map[string]any{"yaml": "statuses:\n  - {id: todo}\n  - {id: build, run: go test}\n  - {id: blocked}\n  - {id: done}\n"})
	w = boardConfigCall(t, s, "PUT", string(body), http.StatusBadRequest)
	if g := w.Body.String(); !strings.Contains(g, `"line":3`) {
		t.Fatalf("semantic line: %s", g)
	}
	if now, _ := os.ReadFile(path); string(now) != source {
		t.Fatalf("invalid save changed the file:\n%s", now)
	}

	// default_workflow is resolved against the repository's workflows.
	wfDir := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(wfDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "feature.yml"), []byte("name: feature\njobs:\n  build: {agent: implementer}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]any{"yaml": "default_workflow: feature\n"})
	w = boardConfigCall(t, s, "PUT", string(body), http.StatusOK)
	var saved2 boardConfigJSON
	if err := json.Unmarshal(w.Body.Bytes(), &saved2); err != nil {
		t.Fatal(err)
	}
	if saved2.Summary == nil || saved2.Summary.DefaultWorkflow != "feature" {
		t.Fatalf("default workflow summary: %+v", saved2)
	}
	body, _ = json.Marshal(map[string]any{"yaml": "default_workflow: ghost\n"})
	w = boardConfigCall(t, s, "PUT", string(body), http.StatusBadRequest)
	if g := w.Body.String(); !strings.Contains(g, `"line":1`) || !strings.Contains(g, "not found") {
		t.Fatalf("unknown workflow: %s", g)
	}
}

func TestBoardConfigAPIReadsBrokenFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(dir, ".tasks", "board.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("columns: [todo, done]\nstatuses:\n  - {id: todo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w := boardConfigCall(t, &server{dir: dir}, "GET", "", http.StatusOK)
	var got boardConfigJSON
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Exists || got.Error == "" || got.Line != 2 || !strings.Contains(got.YAML, "statuses:") {
		t.Fatalf("broken file: %+v", got)
	}
}

func TestBoardDefaultWorkflowEndpoint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := &server{dir: dir}
	path := filepath.Join(dir, ".tasks", "board.yaml")
	wf := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(wf, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"feature", "review"} {
		if err := os.WriteFile(filepath.Join(wf, name+".yml"), []byte("name: "+name+"\njobs:\n  build: {agent: implementer}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	source := "# keep me\ndefault_workflow: feature\nmax_fires_per_hour: 4\n"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}

	call := func(body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/board/config/default-workflow", strings.NewReader(body))
		s.boardDefaultWorkflow(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
		return w
	}

	w := call(`{"name":"review"}`, http.StatusOK)
	var got boardConfigJSON
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Summary == nil || got.Summary.DefaultWorkflow != "review" || len(got.Workflows) != 2 {
		t.Fatalf("response: %+v", got)
	}
	b, _ := os.ReadFile(path)
	for _, want := range []string{"# keep me", "default_workflow: review", "max_fires_per_hour: 4"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("missing %q:\n%s", want, b)
		}
	}

	w = call(`{"name":"ghost"}`, http.StatusBadRequest)
	if g := w.Body.String(); !strings.Contains(g, `"line":2`) || !strings.Contains(g, "not found") {
		t.Fatalf("unknown workflow: %s", g)
	}
	if now, _ := os.ReadFile(path); !strings.Contains(string(now), "default_workflow: review") {
		t.Fatal("rejected name changed the file")
	}

	call(`{"name":""}`, http.StatusOK)
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "default_workflow") || !strings.Contains(string(b), "# keep me") {
		t.Fatalf("remove: %s", b)
	}
}

func TestBoardRoleLimitEndpoint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := &server{dir: dir}
	path := filepath.Join(dir, ".tasks", "board.yaml")
	source := "# keep me\nroles:\n  # builder\n  builder: {agent: pi}\n  reviewer: {agent: pi}\nmax_fires_per_hour: 4\n"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	call := func(body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		s.boardRoleLimit(w, httptest.NewRequest("PUT", "/api/board/config/role-limit", strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
		return w
	}
	w := call(`{"role":"builder","max":2}`, http.StatusOK)
	var got boardConfigJSON
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Summary == nil || got.Summary.Roles[0].MaxInstances != 2 {
		t.Fatalf("response: %+v", got.Summary)
	}
	b, _ := os.ReadFile(path)
	for _, want := range []string{"# keep me", "# builder", "max_instances: 2", "max_fires_per_hour: 4"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("missing %q:\n%s", want, b)
		}
	}
	for _, body := range []string{`{"role":"builder","max":-1}`, `{"role":"builder","max":1.5}`, `{"role":"builder","max":"x"}`} {
		call(body, http.StatusBadRequest)
	}
	if now, _ := os.ReadFile(path); string(now) != string(b) {
		t.Fatal("rejected value changed the file")
	}
	call(`{"role":"ghost","max":1}`, http.StatusBadRequest)
	call(`{"role":"builder","max":0}`, http.StatusOK)
	if now, _ := os.ReadFile(path); strings.Contains(string(now), "max_instances") {
		t.Fatalf("0 keeps the key:\n%s", now)
	}
}
