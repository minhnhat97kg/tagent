package board

import (
	"errors"
	"fmt"
	"tagent/internal/tasks"
)

// Start begins a ticket's workflow. An empty name uses the ticket's workflow,
// else the board's default_workflow.
func Start(dir, id, name string) error {
	scope := tasks.Scope(dir)
	t, err := tasks.Get(scope, id)
	if err != nil {
		return err
	}
	if t.Status == "closed" {
		return fmt.Errorf("ticket #%s is closed: reopen it with task set %s todo", id, id)
	}
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		return err
	}
	if name == "" {
		name = t.Workflow
	}
	if name == "" {
		name = cfg.DefaultWorkflow
	}
	if name == "" {
		return fmt.Errorf("no default workflow: set default_workflow in .tasks/board.yaml or pick a workflow")
	}
	w, err := tasks.FindWorkflow(dir, cfg, name)
	if err != nil {
		return err
	}
	if len(w.Jobs) == 0 {
		return fmt.Errorf("workflow has no jobs")
	}
	if t.Workflow != name {
		if err = tasks.SetWorkflow(scope, id, name); err != nil {
			return err
		}
	}
	if err = Register(dir); err != nil {
		return err
	}
	_, err = tasks.Update(scope, id, w.Jobs[0].Status, nil, tasks.MutationOptions{Actor: "user"})
	return err
}

// autoStart starts the todo dependents with auto_start set, once their last
// dependency is done. Tickets without a workflow are left for a human.
func autoStart(dir, done string) error {
	scope := tasks.Scope(dir)
	var errs []error
	for _, d := range tasks.Dependents(scope, done) {
		if !d.AutoStart || d.Status != "todo" || d.Workflow == "" || len(tasks.OpenNeeds(scope, d)) > 0 {
			continue
		}
		if err := Start(dir, d.ID, ""); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := tasks.Comment(scope, d.ID, "board", "auto-started: #"+done+" done", nil, tasks.MutationOptions{Actor: "board"}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
