package board

import (
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
