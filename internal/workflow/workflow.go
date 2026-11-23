// Package workflow parses trusted, sequential ticket recipes.
package workflow

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

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
	Needs     names    `yaml:"needs"`
	Outputs   []string `yaml:"outputs"`
	OnFailure string   `yaml:"on-failure"`
	MaxLoops  int      `yaml:"max-loops"`
}
type names []string

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
	Inputs      map[string]Input
	Jobs        []Job
	Err         error
}

func Parse(b []byte) (Workflow, error) {
	var raw struct {
		Name        string           `yaml:"name"`
		Description string           `yaml:"description"`
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
	if raw.Jobs.Kind != yaml.MappingNode || len(raw.Jobs.Content) == 0 {
		return w, fmt.Errorf("no jobs")
	}
	ids := map[string]Job{}
	statuses := map[string]string{"todo": "", "done": "", "blocked": ""}
	for i := 0; i < len(raw.Jobs.Content); i += 2 {
		var j Job
		n := raw.Jobs.Content[i+1]
		data, e := yaml.Marshal(n)
		if e != nil {
			return w, e
		}
		dec := yaml.NewDecoder(strings.NewReader(string(data)))
		dec.KnownFields(true)
		if e = dec.Decode(&j); e != nil {
			return w, e
		}
		j.ID = raw.Jobs.Content[i].Value
		if _, ok := ids[j.ID]; ok {
			return w, fmt.Errorf("duplicate job %s", j.ID)
		}
		if j.Status == "" {
			j.Status = j.ID
		}
		if _, ok := statuses[j.Status]; ok {
			return w, fmt.Errorf("duplicate/reserved status %s", j.Status)
		}
		if (j.Run == "") == (j.Agent == "") || (j.Run != "" && (j.Prompt != "" || j.Model != "")) || j.MaxLoops < 0 {
			return w, fmt.Errorf("job %s requires run or agent (+ optional prompt)", j.ID)
		}
		ids[j.ID] = j
		statuses[j.Status] = j.ID
	}
	next := map[string]string{}
	root := ""
	for id, j := range ids {
		if len(j.Needs) > 1 {
			return w, fmt.Errorf("parallel jobs not supported yet")
		}
		if len(j.Needs) == 0 {
			if root != "" {
				return w, fmt.Errorf("parallel jobs not supported yet")
			}
			root = id
		} else {
			p := j.Needs[0]
			if _, ok := ids[p]; !ok {
				return w, fmt.Errorf("unknown needs %s", p)
			}
			if next[p] != "" {
				return w, fmt.Errorf("parallel jobs not supported yet")
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
		return w, fmt.Errorf("jobs need each other in a cycle")
	}
	for _, j := range w.Jobs {
		if j.OnFailure != "" {
			a, job := ids[j.OnFailure]
			owner, status := statuses[j.OnFailure]
			if !job && !status {
				return w, fmt.Errorf("unknown on-failure %s", j.OnFailure)
			}
			if job && status && owner != a.ID {
				return w, fmt.Errorf("ambiguous on-failure %s", j.OnFailure)
			}
		}
	}
	return w, nil
}
func LocalDir(root string) string { return filepath.Join(root, ".tasks", "workflows") }
func GlobalDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		h, _ := os.UserHomeDir()
		base = filepath.Join(h, ".config")
	}
	return filepath.Join(base, "tmux-agent", "workflows")
}
func Load(file string) Workflow {
	b, e := os.ReadFile(file)
	var w Workflow
	if e == nil {
		w, e = Parse(b)
	}
	w.File = file
	w.Err = e
	if w.Name == "" {
		w.Name = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	}
	return w
}
func List(root string) []Workflow {
	var out []Workflow
	for i, dir := range []string{LocalDir(root), GlobalDir()} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.y*ml"))
		sort.Strings(files)
		for _, f := range files {
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
