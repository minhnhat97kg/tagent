// Package workflow parses trusted, sequential ticket recipes.
package workflow

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"tagent/internal/apppath"
	"time"
)

// Error is a parse failure tied to a 1-based source line (0 when none).
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

var lineRE = regexp.MustCompile(`line (\d+)`)

// ErrorLine returns the source line of a Parse error, or 0 when it has none.
func ErrorLine(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Line
	}
	if err == nil {
		return 0
	}
	m := lineRE.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

type Input struct {
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
	Required    bool   `yaml:"required"`
}
type Job struct {
	ID        string   `yaml:"-"`
	Status    string   `yaml:"status"`
	Agent     string   `yaml:"agent"`
	Model     string   `yaml:"model"`
	Prompt    string   `yaml:"prompt"`
	Run       string   `yaml:"run"`
	Uses      string   `yaml:"uses"`
	Needs     names    `yaml:"needs"`
	Outputs   []string `yaml:"outputs"`
	OnFailure string   `yaml:"on-failure"`
	MaxLoops  int      `yaml:"max-loops"`
}
type names []string

// ScoutPrompt is the built-in prompt of `uses: scout`, the read-only first step.
const ScoutPrompt = "Scout ticket ${{ ticket.id }}: ${{ ticket.title }}. Read only, change nothing. Search first, read only the line ranges needed. Return one output `scout`, under 6000 characters: entry points (file:line), suspect code, related tests, open questions. No code dumps. Finish with the tagent MCP tool `task_move` (to = the forward status, outputs.scout); a shell may be unavailable."

func (s *names) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*s = []string{n.Value}
		return nil
	}
	var v []string
	err := n.Decode(&v)
	*s = v
	return err
}

type Workflow struct {
	Name, File  string
	Description string
	Global      bool
	Workplace   string // "" or "git-worktree" (default), "none": run in the main root, "folder": a plain directory of files, "browser": a Chrome profile and captures, "remote": the agent runs over ssh in board.yaml's remote dir, or "docker": the agent runs in a container of board.yaml's docker image
	Cleanup     string // "" or "remove" (default), or "keep": done keeps the worktree and its branch (or the folder, or the browser profile)
	Inputs      map[string]Input
	Jobs        []Job
	Err         error
}

func Parse(b []byte) (Workflow, error) {
	var raw struct {
		Name        string           `yaml:"name"`
		Description string           `yaml:"description"`
		Workplace   yaml.Node        `yaml:"workplace"`
		Cleanup     yaml.Node        `yaml:"cleanup"`
		Inputs      map[string]Input `yaml:"inputs"`
		Jobs        yaml.Node        `yaml:"jobs"`
	}
	d := yaml.NewDecoder(strings.NewReader(string(b)))
	d.KnownFields(true)
	if err := d.Decode(&raw); err != nil {
		return Workflow{}, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return Workflow{}, fmt.Errorf("expected one YAML document")
	}
	w := Workflow{Name: raw.Name, Description: raw.Description, Inputs: raw.Inputs}
	if raw.Workplace.Kind != 0 {
		switch raw.Workplace.Value {
		case "git-worktree", "none", "folder", "browser", "remote", "docker":
			w.Workplace = raw.Workplace.Value
		default:
			return w, errAt(raw.Workplace.Line, "unknown workplace %q (git-worktree, none, folder, browser, remote or docker)", raw.Workplace.Value)
		}
	}
	if raw.Cleanup.Kind != 0 {
		switch raw.Cleanup.Value {
		case "remove", "keep":
			w.Cleanup = raw.Cleanup.Value
		default:
			return w, errAt(raw.Cleanup.Line, "unknown cleanup %q (remove or keep)", raw.Cleanup.Value)
		}
	}
	if raw.Jobs.Kind != yaml.MappingNode || len(raw.Jobs.Content) == 0 {
		return w, fmt.Errorf("no jobs")
	}
	ids := map[string]Job{}
	lines := map[string]int{}
	statuses := map[string]string{"todo": "", "done": "", "blocked": ""}
	for i := 0; i < len(raw.Jobs.Content); i += 2 {
		var j Job
		key, n := raw.Jobs.Content[i], raw.Jobs.Content[i+1]
		j.ID = key.Value
		data, e := yaml.Marshal(n)
		if e != nil {
			return w, e
		}
		dec := yaml.NewDecoder(strings.NewReader(string(data)))
		dec.KnownFields(true)
		if e = dec.Decode(&j); e != nil {
			// The fragment restarts at line 1, so shift the reported line onto the file.
			line := n.Line + max(ErrorLine(e), 1) - 1
			msg := strings.Replace(e.Error(), lineRE.FindString(e.Error()), fmt.Sprintf("line %d", line), 1)
			return w, &Error{line, msg}
		}
		if _, ok := ids[j.ID]; ok {
			return w, errAt(key.Line, "duplicate job %s", j.ID)
		}
		if j.Uses != "" {
			if j.Uses != "scout" || j.Run != "" || j.Agent != "" || j.Prompt != "" || j.Model != "" {
				return w, errAt(key.Line, "job %s: uses scout takes no run, agent, prompt or model", j.ID)
			}
			j.Agent, j.Prompt = "ba", ScoutPrompt
			if len(j.Outputs) == 0 {
				j.Outputs = []string{"scout"}
			}
			if j.Status == "" {
				j.Status = "scouting"
			}
		}
		if j.Status == "" {
			j.Status = j.ID
		}
		if _, ok := statuses[j.Status]; ok || j.Status == "closed" {
			return w, errAt(key.Line, "duplicate/reserved status %s", j.Status)
		}
		if (j.Run == "") == (j.Agent == "") || (j.Run != "" && (j.Prompt != "" || j.Model != "")) || j.MaxLoops < 0 {
			return w, errAt(key.Line, "job %s requires run or agent (+ optional prompt)", j.ID)
		}
		if j.Run != "" && (w.Workplace == "remote" || w.Workplace == "docker") {
			return w, errAt(key.Line, "job %s: run jobs cannot run in a %s workplace", j.ID, w.Workplace)
		}
		ids[j.ID] = j
		lines[j.ID] = key.Line
		statuses[j.Status] = j.ID
	}
	next := map[string]string{}
	root := ""
	for id, j := range ids {
		if len(j.Needs) > 1 {
			return w, errAt(lines[id], "parallel jobs not supported yet")
		}
		if len(j.Needs) == 0 {
			if root != "" {
				return w, errAt(lines[id], "parallel jobs not supported yet")
			}
			root = id
		} else {
			p := j.Needs[0]
			if _, ok := ids[p]; !ok {
				return w, errAt(lines[id], "unknown needs %s", p)
			}
			if next[p] != "" {
				return w, errAt(lines[id], "parallel jobs not supported yet")
			}
			next[p] = id
		}
	}
	seen := map[string]bool{}
	for id := root; id != "" && !seen[id]; id = next[id] {
		seen[id] = true
		w.Jobs = append(w.Jobs, ids[id])
	}
	if len(seen) != len(ids) {
		for i := 0; i < len(raw.Jobs.Content); i += 2 {
			if id := raw.Jobs.Content[i].Value; !seen[id] {
				return w, errAt(raw.Jobs.Content[i].Line, "jobs need each other in a cycle")
			}
		}
		return w, fmt.Errorf("jobs need each other in a cycle")
	}
	for _, j := range w.Jobs {
		if j.OnFailure != "" {
			a, job := ids[j.OnFailure]
			owner, status := statuses[j.OnFailure]
			if !job && !status {
				return w, errAt(lines[j.ID], "unknown on-failure %s", j.OnFailure)
			}
			if job && status && owner != a.ID {
				return w, errAt(lines[j.ID], "ambiguous on-failure %s", j.OnFailure)
			}
		}
	}
	return w, nil
}
func errAt(line int, format string, a ...any) error {
	return &Error{line, fmt.Sprintf(format, a...)}
}
func LocalDir(root string) string { return filepath.Join(root, ".tasks", "workflows") }
func GlobalDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		h, _ := os.UserHomeDir()
		base = filepath.Join(h, ".config")
	}
	return filepath.Join(apppath.Dir(base), "workflows")
}

// loadCache keeps the parsed result per file. The sidebar draws every card on
// every frame, and each card resolves its workflow, so re-parsing the YAML each
// time dominated the CPU. A file is re-parsed only when its mtime or size changes.
// ponytail: stat-keyed cache, no inotify; a same-size rewrite within one mtime tick is missed.
var loadCache sync.Map // file -> cachedWorkflow

type cachedWorkflow struct {
	mod  time.Time
	size int64
	w    Workflow
}

func Load(file string) Workflow {
	st, statErr := os.Stat(file)
	if statErr == nil {
		if v, ok := loadCache.Load(file); ok {
			if c := v.(cachedWorkflow); c.mod.Equal(st.ModTime()) && c.size == st.Size() {
				return c.w
			}
		}
	}
	b, readErr := os.ReadFile(file)
	var w Workflow
	e := readErr
	if e == nil {
		w, e = Parse(b)
	}
	w.File = file
	w.Err = e
	if w.Name == "" {
		w.Name = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	}
	// Parse errors are a pure function of the bytes, so they are cached too.
	if statErr == nil && readErr == nil {
		loadCache.Store(file, cachedWorkflow{mod: st.ModTime(), size: st.Size(), w: w})
	}
	return w
}

// dirCache keeps each workflow dir's file list, keyed by the dir's mtime (adding,
// removing or renaming a file changes it). Load still checks each file itself.
// ponytail: a file added within the same mtime tick as the last glob is missed until the next change.
var dirCache sync.Map // dir -> cachedDir

type cachedDir struct {
	mod   time.Time
	files []string
}

func globWorkflows(dir string) []string {
	st, err := os.Stat(dir)
	if err == nil {
		if v, ok := dirCache.Load(dir); ok {
			if c := v.(cachedDir); c.mod.Equal(st.ModTime()) {
				return c.files
			}
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	sort.Strings(files)
	if err == nil {
		dirCache.Store(dir, cachedDir{mod: st.ModTime(), files: files})
	}
	return files
}

func List(root string) []Workflow {
	var out []Workflow
	for i, dir := range []string{LocalDir(root), GlobalDir()} {
		for _, f := range globWorkflows(dir) {
			w := Load(f)
			w.Global = i == 1
			out = append(out, w)
		}
	}
	return out
}
func Find(root, name string) (Workflow, error) {
	if filepath.IsAbs(name) {
		w := Load(name)
		return w, w.Err
	}
	for _, w := range List(root) {
		if w.Name == name || strings.TrimSuffix(filepath.Base(w.File), filepath.Ext(w.File)) == name {
			return w, w.Err
		}
	}
	return Workflow{}, fmt.Errorf("workflow %q not found", name)
}

var expr = regexp.MustCompile(`\$\{\{\s*([^{}]+?)\s*\}\}`)

func Expand(s string, inputs map[string]string, outputs map[string]map[string]string, id, title string) string {
	return expr.ReplaceAllStringFunc(s, func(m string) string {
		k := strings.TrimSpace(expr.FindStringSubmatch(m)[1])
		p := strings.Split(k, ".")
		switch {
		case k == "ticket.id":
			return id
		case k == "ticket.title":
			return title
		case len(p) == 2 && p[0] == "inputs":
			return inputs[p[1]]
		case len(p) == 4 && p[0] == "jobs" && p[2] == "outputs":
			return outputs[p[1]][p[3]]
		}
		return ""
	})
}
func (w Workflow) Values(given map[string]string) (map[string]string, error) {
	v := map[string]string{}
	for k, i := range w.Inputs {
		v[k] = i.Default
	}
	for k, x := range given {
		v[k] = x
	}
	for k, i := range w.Inputs {
		if i.Required && v[k] == "" {
			return nil, fmt.Errorf("required input %s", k)
		}
	}
	return v, nil
}
func Template(name string) string {
	return "name: " + name + "\ndescription: Describe when to use this workflow\ninputs:\n  goal: {required: true}\njobs:\n  build:\n    agent: implementer\n    prompt: \"Implement ${{ inputs.goal }}\"\n  test:\n    needs: build\n    run: go test ./...\n"
}
