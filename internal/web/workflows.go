package web

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

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
		for _, f := range workflow.List(root) {
			b, err := os.ReadFile(f.File)
			message := ""
			if f.Err != nil {
				message = f.Err.Error()
			} else if err != nil {
				message = err.Error()
			}
			out = append(out, map[string]any{"name": f.Name, "key": strings.TrimSuffix(filepath.Base(f.File), filepath.Ext(f.File)), "global": f.Global, "yaml": string(b), "description": f.Description, "jobs": f.Jobs, "error": message})
		}
		writeJSON(w, out)
		return
	}
	var o struct {
		Name, YAML, Op, Goal string
		Global               bool
	}
	if err := readBody(r, &o); err != nil {
		fail(w, err)
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
		if strings.TrimSpace(o.Goal) == "" {
			fail(w, fmt.Errorf("goal is required"))
			return
		}
		var bin string
		bin, err = os.Executable()
		if err == nil {
			cmd := exec.Command(bin, "workflow", "run", file, "--input", "goal="+o.Goal)
			cmd.Dir = root
			var output []byte
			output, err = cmd.CombinedOutput()
			if err != nil {
				err = fmt.Errorf("%s: %w", output, err)
			}
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
