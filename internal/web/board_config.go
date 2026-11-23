package web

import (
	"encoding/json"
	"net/http"
	"os"

	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

// boardWorkflowOption names a local or global workflow for the default picker.
type boardWorkflowOption struct {
	Name   string `json:"name"`
	Global bool   `json:"global"`
}

// boardConfigJSON is the raw YAML plus the same summary the TUI shows.
type boardConfigJSON struct {
	Dir       string                `json:"dir"`
	Path      string                `json:"path"`
	YAML      string                `json:"yaml"`
	Default   string                `json:"default"`
	Exists    bool                  `json:"exists"`
	Error     string                `json:"error,omitempty"`
	Line      int                   `json:"line,omitempty"`
	Summary   *tasks.BoardSummary   `json:"summary,omitempty"`
	Workflows []boardWorkflowOption `json:"workflows"`
}

func workflowOptions(dir string) []boardWorkflowOption {
	out := []boardWorkflowOption{}
	for _, w := range workflow.List(tasks.MainRoot(dir)) {
		out = append(out, boardWorkflowOption{Name: w.Name, Global: w.Global})
	}
	return out
}

// boardConfig reads and writes .tasks/board.yaml for the web config page.
// GET returns the raw file (comments intact) and its summary; PUT validates
// before saving atomically, or only validates when the body asks for it.
func (s *server) boardConfig(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	path := tasks.BoardConfigPath(dir)
	switch r.Method {
	case http.MethodGet:
		out := boardConfigJSON{Dir: dir, Path: path, Default: tasks.BoardExample, Workflows: workflowOptions(dir)}
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			out.Exists, out.YAML = true, string(b)
			if cfg, e := tasks.ParseBoardConfigFor(dir, b); e != nil {
				out.Error, out.Line = e.Error(), tasks.BoardConfigErrorLine(e)
			} else {
				sum := cfg.Summary()
				out.Summary = &sum
			}
		case os.IsNotExist(err):
			// No local file: show the effective (global or default) summary, but an
			// empty editor so the first save creates the project's own file.
			if cfg, e := tasks.LoadBoardConfig(dir); e != nil {
				out.Error, out.Line = e.Error(), tasks.BoardConfigErrorLine(e)
			} else {
				sum := cfg.Summary()
				out.Summary = &sum
			}
		default:
			fail(w, err)
			return
		}
		writeJSON(w, out)
	case http.MethodPut:
		var o struct {
			YAML     string `json:"yaml"`
			Validate bool   `json:"validate"`
		}
		if err := readBody(r, &o); err != nil {
			fail(w, err)
			return
		}
		cfg, err := tasks.ParseBoardConfigFor(dir, []byte(o.YAML))
		if err != nil {
			boardConfigFail(w, err)
			return
		}
		sum := cfg.Summary()
		if o.Validate {
			writeJSON(w, map[string]any{"summary": sum})
			return
		}
		if err := tasks.SaveBoardConfig(dir, []byte(o.YAML)); err != nil {
			boardConfigFail(w, err)
			return
		}
		writeJSON(w, boardConfigJSON{Dir: dir, Path: path, YAML: o.YAML, Default: tasks.BoardExample, Exists: true, Summary: &sum, Workflows: workflowOptions(dir)})
	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// boardDefaultWorkflow changes only default_workflow in board.yaml, keeping
// comments and key order. The UI calls it from both the select and the TUI.
func (s *server) boardDefaultWorkflow(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	var o struct {
		Name string `json:"name"`
	}
	if err := readBody(r, &o); err != nil {
		fail(w, err)
		return
	}
	if err := tasks.SetDefaultWorkflow(dir, o.Name); err != nil {
		boardConfigFail(w, err)
		return
	}
	path := tasks.BoardConfigPath(dir)
	b, err := os.ReadFile(path)
	if err != nil {
		fail(w, err)
		return
	}
	cfg, err := tasks.ParseBoardConfigFor(dir, b)
	if err != nil {
		boardConfigFail(w, err)
		return
	}
	sum := cfg.Summary()
	writeJSON(w, boardConfigJSON{Dir: dir, Path: path, YAML: string(b), Default: tasks.BoardExample, Exists: true, Summary: &sum, Workflows: workflowOptions(dir)})
}

// boardConfigFail carries the line the editor should mark, like workflow validation.
func boardConfigFail(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "line": tasks.BoardConfigErrorLine(err)})
}
