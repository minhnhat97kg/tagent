package board

import (
	"fmt"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

// Migration is what board migrate did (or would do) to one ticket.
type Migration struct {
	ID       string
	From     string
	Workflow string
	Job      string
	To       string
	Skipped  string // reason the ticket was left untouched
}

// Migrate moves tickets that still use deprecated board-flow statuses onto the
// board's default_workflow, matching the old status to a job id (plan -> plan)
// or a job status. It is explicit and idempotent; unmatched tickets stay put.
func Migrate(dir string, dryRun bool) ([]Migration, error) {
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		return nil, err
	}
	name := cfg.DefaultWorkflow
	if name == "" {
		return nil, fmt.Errorf("no default_workflow in .tasks/board.yaml")
	}
	w, err := tasks.FindWorkflow(dir, cfg, name)
	if err != nil {
		return nil, err
	}
	scope := tasks.Scope(dir)
	var out []Migration
	for _, t := range tasks.List(scope) {
		if (t.Workflow != "" && t.Workflow != "board") || t.Status == "todo" || t.Status == "blocked" || t.Status == "done" || t.Status == "closed" {
			continue
		}
		m := Migration{ID: t.ID, From: t.Status, Workflow: name}
		job := matchJob(w, t.Status)
		if job == nil {
			m.Skipped = "no job matches this status"
			out = append(out, m)
			continue
		}
		m.Job, m.To = job.ID, job.Status
		if dryRun {
			out = append(out, m)
			continue
		}
		if err := tasks.AttachWorkflow(scope, t.ID, name); err != nil {
			m.Skipped = err.Error()
			out = append(out, m)
			continue
		}
		if err := tasks.Move(scope, t.ID, job.Status, "migrated to workflow "+name, tasks.MutationOptions{Actor: "user"}); err != nil {
			m.Skipped = err.Error()
		}
		out = append(out, m)
	}
	return out, nil
}

func matchJob(w workflow.Workflow, status string) *workflow.Job {
	for i := range w.Jobs {
		if w.Jobs[i].ID == status {
			return &w.Jobs[i]
		}
	}
	for i := range w.Jobs {
		if w.Jobs[i].Status == status {
			return &w.Jobs[i]
		}
	}
	return nil
}
