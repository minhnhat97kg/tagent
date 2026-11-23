package board

import (
	"fmt"
	"slices"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

// Start uses a workflow or advances through the board's configured columns.
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
	status := ""
	if name == "board" {
		i := slices.Index(cfg.Columns, t.Status)
		if i < 0 || i+1 >= len(cfg.Columns) {
			return fmt.Errorf("no next board status after %s", t.Status)
		}
		status = cfg.Columns[i+1]
	} else {
		w, e := workflow.Find(tasks.MainRoot(dir), name)
		if e != nil {
			return e
		}
		if len(w.Jobs) == 0 {
			return fmt.Errorf("workflow has no jobs")
		}
		if err = tasks.SetWorkflow(scope, id, name, nil); err != nil {
			return err
		}
		status = w.Jobs[0].Status
	}
	if err = Register(dir); err != nil {
		return err
	}
	_, err = tasks.Update(scope, id, status, nil, tasks.MutationOptions{Actor: "user"})
	return err
}
