package tasks

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"tagent/internal/sharedctx"
	"tagent/internal/workflow"
	"tagent/internal/workplace"
)

// TicketWorkplace is where a ticket's workflow keeps its work, chosen at dispatch.
// Lookup errors are returned: a missing recipe never falls back to a worktree.
func TicketWorkplace(scope string, t Task) (workplace.Workplace, error) {
	dir, ok := scopeRoots.Load(scope)
	if !ok {
		return nil, fmt.Errorf("unknown repository scope")
	}
	base, err := configForScope(scope)
	if err != nil {
		return nil, err
	}
	w, err := FindWorkflow(dir.(string), base, t.Workflow)
	if err != nil {
		return nil, err
	}
	switch w.Workplace {
	case "none":
		return workplace.None{}, nil
	case "folder":
		return workplace.Folder{Base: realpath(filepath.Join(sharedctx.Dir(scope), "folder")), Keep: w.Cleanup == "keep"}, nil
	case "browser":
		return workplace.Browser{Base: realpath(filepath.Join(sharedctx.Dir(scope), "browser")), Keep: w.Cleanup == "keep"}, nil
	case "remote":
		if base.Remote == (workplace.Remote{}) {
			return nil, fmt.Errorf("workplace remote needs remote: {host, dir} in board.yaml")
		}
		return base.Remote, nil
	case "docker":
		if base.Docker.Image == "" {
			return nil, fmt.Errorf("workplace docker needs docker: {image} in board.yaml")
		}
		d := base.Docker
		d.Base, d.Keep = realpath(filepath.Join(sharedctx.Dir(scope), "docker")), w.Cleanup == "keep"
		return d, nil
	}
	return workplace.Git{Keep: w.Cleanup == "keep"}, nil
}

// ConfigForTask is the single per-ticket configuration boundary.
func ConfigForTask(dir string, base BoardConfig, t Task) (BoardConfig, error) {
	if t.Workflow == "" || (base.Legacy && t.Workflow == "board") {
		// The implicit board workflow is the legacy status table itself, so
		// old next/pass/fail semantics survive exactly.
		return base, nil
	}
	w, e := FindWorkflow(dir, base, t.Workflow)
	if e != nil {
		return base, e
	}
	overrides := base.Statuses
	base.Statuses = []Status{{ID: "todo", Next: []string{w.Jobs[0].Status}}}
	base.Columns = []string{"todo"}
	target := func(id string) string {
		for _, j := range w.Jobs {
			if j.ID == id {
				return j.Status
			}
		}
		return id
	}
	for i, j := range w.Jobs {
		next := "done"
		if i+1 < len(w.Jobs) {
			next = w.Jobs[i+1].Status
		}
		fail := "blocked"
		if j.OnFailure != "" {
			fail = target(j.OnFailure)
		}
		s := Status{Needs: []string(j.Needs), Failure: fail, ID: j.Status, Role: j.Agent, Model: j.Model, Prompt: j.Prompt, Job: j.ID, Uses: j.Uses, Outputs: j.Outputs, Run: j.Run, MaxLoops: j.MaxLoops, Next: []string{next}}
		for _, x := range []string{"blocked", fail} {
			found := false
			for _, n := range s.Next {
				found = found || n == x
			}
			if !found {
				s.Next = append(s.Next, x)
			}
		}
		if j.Run != "" {
			s.Pass = next
			s.Fail = fail
		}
		base.Statuses = append(base.Statuses, s)
		base.Columns = append(base.Columns, s.ID)
	}
	base.Statuses = append(base.Statuses, Status{ID: "blocked"}, Status{ID: "done", Next: []string{}})
	base.Columns = append(base.Columns, "blocked", "done")
	base.addDeclared()
	for i := range base.Statuses {
		for _, s := range overrides {
			if s.ID == base.Statuses[i].ID {
				base.Statuses[i].FreshAfter = s.FreshAfter
			}
		}
	}
	return base, nil
}
func configForTaskScope(scope string, t Task) (BoardConfig, error) {
	base, e := configForScope(scope)
	if e != nil {
		return base, e
	}
	if t.Workflow == "" {
		return base, nil
	}
	dir, ok := scopeRoots.Load(scope)
	if !ok {
		return base, fmt.Errorf("unknown repository scope")
	}
	return ConfigForTask(dir.(string), base, t)
}

// FindWorkflow resolves a workflow by name, with the implicit "board" workflow
// of a deprecated statuses-based board.yaml as a fallback.
func FindWorkflow(dir string, base BoardConfig, name string) (workflow.Workflow, error) {
	w, err := workflow.Find(MainRoot(dir), name)
	if err == nil {
		return w, nil
	}
	if base.Legacy && name == "board" {
		if lw := legacyWorkflow(base); len(lw.Jobs) > 0 {
			return lw, nil
		}
	}
	return workflow.Workflow{}, err
}

// legacyWorkflow converts old board.yaml statuses into a linear workflow with
// the same status ids, so running tickets keep their exact status names.
func legacyWorkflow(c BoardConfig) workflow.Workflow {
	w := workflow.Workflow{Name: "board", Description: "implicit workflow from board.yaml statuses (deprecated)"}
	index := map[string]int{}
	for i, s := range c.Statuses {
		index[s.ID] = i
	}
	prev := ""
	for _, s := range c.Statuses {
		if s.ID == "todo" || s.ID == "blocked" || s.ID == "done" {
			continue
		}
		j := workflow.Job{ID: s.ID, Status: s.ID, Agent: s.Role, Model: s.Model, Prompt: s.Prompt, Run: s.Run, MaxLoops: s.MaxLoops}
		if prev != "" {
			j.Needs = []string{prev}
		}
		if s.Run != "" {
			j.OnFailure = s.Fail
		} else {
			for _, n := range s.Next {
				if at, ok := index[n]; ok && at < index[s.ID] && n != "blocked" && n != "todo" && n != "done" {
					j.OnFailure = n
					break
				}
			}
		}
		w.Jobs = append(w.Jobs, j)
		prev = s.ID
	}
	return w
}

// jobStatus maps a workflow job id onto the status that job lands in.
func jobStatus(cfg BoardConfig, to string) string {
	if to == "" || slices.Contains(cfg.Columns, to) {
		return to
	}
	for _, s := range cfg.Statuses {
		if s.Job != "" && s.Job == to {
			return s.ID
		}
	}
	return to
}

// attachDefault gives a ticket without a workflow the board's default workflow
// when it leaves todo. The initial entry may name any job id or job status; the
// caller skips the next-status check for that first move.
func attachDefault(scope string, t *Task, cfg BoardConfig, to string) (BoardConfig, string, bool, error) {
	if t.Workflow != "" || t.Status != "todo" || to == "todo" || to == "blocked" || to == "done" || cfg.DefaultWorkflow == "" {
		return cfg, to, false, nil
	}
	dir, ok := scopeRoots.Load(scope)
	if !ok {
		return cfg, to, false, fmt.Errorf("unknown repository scope")
	}
	w, err := FindWorkflow(dir.(string), cfg, cfg.DefaultWorkflow)
	if err != nil {
		return cfg, to, false, err
	}
	if len(w.Jobs) == 0 {
		return cfg, to, false, fmt.Errorf("workflow %q has no jobs", cfg.DefaultWorkflow)
	}
	values := map[string]string{}
	var missing []string
	for k, v := range w.Inputs {
		if v.Required && v.Default == "" && values[k] == "" {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 1 {
		return cfg, to, false, fmt.Errorf("missing inputs: %s; use workflow run %s --input name=value", strings.Join(missing, ", "), cfg.DefaultWorkflow)
	}
	if len(missing) == 1 {
		values[missing[0]] = t.Text
	}
	inputs, err := w.Values(values)
	if err != nil {
		return cfg, to, false, err
	}
	t.Workflow, t.Inputs = cfg.DefaultWorkflow, inputs
	cfg, err = ConfigForTask(dir.(string), cfg, *t)
	if err != nil {
		return cfg, to, false, err
	}
	if to == "" {
		return cfg, w.Jobs[0].Status, true, nil
	}
	return cfg, jobStatus(cfg, to), true, nil
}
