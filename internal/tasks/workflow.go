package tasks

import (
	"fmt"
	"tagent/internal/workflow"
)

// ConfigForTask is the single per-ticket configuration boundary.
func ConfigForTask(dir string, base BoardConfig, t Task) (BoardConfig, error) {
	if t.Workflow == "" {
		return base, nil
	}
	w, e := workflow.Find(MainRoot(dir), t.Workflow)
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
		s := Status{Needs: []string(j.Needs), Failure: fail, ID: j.Status, Role: j.Agent, Model: j.Model, Prompt: j.Prompt, Job: j.ID, Outputs: j.Outputs, Run: j.Run, MaxLoops: j.MaxLoops, Next: []string{next}}
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
