package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
	"time"
)

// workflowCommand is a seam for endpoint tests; production still invokes the CLI.
var workflowCommand = func(root string, args []string) error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", output, err)
	}
	return nil
}

var workflowName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// workflowPath rejects both lexical traversal and symlinks out of the project.
func workflowPath(root, name string) (string, error) {
	if !workflowName.MatchString(name) {
		return "", fmt.Errorf("invalid workflow name")
	}
	dir := workflow.LocalDir(root)
	ext := ".yaml"
	if _, err := os.Lstat(filepath.Join(dir, name+".yml")); err == nil {
		ext = ".yml"
	}
	for _, p := range []string{filepath.Join(root, ".tasks"), dir, filepath.Join(dir, name+ext)} {
		st, err := os.Lstat(p)
		if err == nil && st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("workflow symlinks are not allowed")
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return filepath.Join(dir, name+ext), nil
}

func (s *server) workflows(w http.ResponseWriter, r *http.Request) {
	root := tasks.MainRoot(s.projectDir(r))
	if r.Method == "GET" {
		out := []map[string]any{}
		tickets := tasks.List(tasks.Scope(root))
		config, _ := tasks.LoadBoardConfig(root)
		for _, f := range workflow.List(root) {
			b, err := os.ReadFile(f.File)
			message := ""
			if f.Err != nil {
				message = f.Err.Error()
			} else if err != nil {
				message = err.Error()
			}
			total, running := 0, 0
			var last time.Time
			for _, t := range tickets {
				if t.Workflow != f.Name {
					continue
				}
				total++
				if t.Status != "todo" && config.Lane(t.Status) != "done" {
					running++
				}
				if t.Created.After(last) {
					last = t.Created
				}
			}
			out = append(out, map[string]any{"name": f.Name, "key": strings.TrimSuffix(filepath.Base(f.File), filepath.Ext(f.File)), "global": f.Global, "yaml": string(b), "description": f.Description, "jobs": f.Jobs, "inputs": f.Inputs, "total": total, "running": running, "lastUsed": last, "error": message, "errorLine": workflow.ErrorLine(f.Err)})
		}
		writeJSON(w, out)
		return
	}
	var o struct {
		Name, YAML, Op, Goal string
		Global               bool
		Inputs               map[string]string
		Patch                *workflowPatch
	}
	if err := readBody(r, &o); err != nil {
		fail(w, err)
		return
	}
	if o.Op == "validate" || o.Op == "patch" {
		source, preview, err := patchWorkflow(o.YAML, o.Patch)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "line": workflow.ErrorLine(err)})
			return
		}
		writeJSON(w, map[string]any{"yaml": source, "preview": preview})
		return
	}
	if o.Global && o.Op != "run" {
		fail(w, fmt.Errorf("global workflows are read-only"))
		return
	}
	file, err := workflowPath(root, o.Name)
	if err != nil {
		fail(w, err)
		return
	}
	if o.Global && o.Op == "run" {
		found := false
		for _, f := range workflow.List(root) {
			if f.Global && strings.TrimSuffix(filepath.Base(f.File), filepath.Ext(f.File)) == o.Name {
				file = f.File
				found = true
				break
			}
		}
		if !found {
			fail(w, fmt.Errorf("global workflow not found"))
			return
		}
	}
	if o.Op == "run" {
		if o.Inputs == nil && strings.TrimSpace(o.Goal) == "" {
			fail(w, fmt.Errorf("goal is required"))
			return
		}
		given := o.Inputs
		if given == nil {
			given = map[string]string{"goal": o.Goal}
		}
		recipe := workflow.Load(file)
		err = recipe.Err
		if err == nil {
			_, err = recipe.Values(given)
		}
		if err == nil {
			args := []string{"workflow", "run", file}
			keys := make([]string, 0, len(given))
			for key := range given {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				args = append(args, "--input", key+"="+given[key])
			}
			err = workflowCommand(root, args)
		}
	} else if r.Method == "DELETE" {
		err = os.Remove(file)
	} else {
		create := o.Op == "new"
		if create {
			o.YAML = workflow.Template(o.Name)
		}
		if _, err = workflow.Parse([]byte(o.YAML)); err == nil {
			err = os.MkdirAll(filepath.Dir(file), 0755)
			if err == nil {
				flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
				if create {
					flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
				}
				var f *os.File
				f, err = os.OpenFile(file, flags, 0644)
				if err == nil {
					_, err = f.WriteString(o.YAML)
					closeErr := f.Close()
					if err == nil {
						err = closeErr
					}
				}
			}
		}
	}
	if err != nil {
		fail(w, err)
	}
}
