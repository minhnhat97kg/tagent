package board

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
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

// RunWorkflow creates a ticket of workflow name and moves it to the workflow's
// first status. An empty title is the workflow name plus its inputs. actor is
// recorded on the events ("" = the caller). It does not register the repo: that
// is consent for its local schedules, so only the explicit CLI gives it.
func RunWorkflow(dir, name, title string, given map[string]string, actor string) (string, error) {
	dir = tasks.MainRoot(dir)
	scope := tasks.Scope(dir)
	w, err := workflow.Find(dir, name)
	if err != nil {
		return "", err
	}
	values, err := w.Values(given)
	if err != nil {
		return "", err
	}
	text := title
	if text == "" {
		text = w.Name
		keys := slices.Sorted(maps.Keys(values))
		for _, k := range keys {
			text += " " + k + "=" + values[k]
		}
	}
	t, err := tasks.Add(scope, text, nil, tasks.MutationOptions{Workflow: w.Name, Inputs: values, Actor: actor})
	if err != nil {
		return "", err
	}
	err = tasks.Move(scope, t.ID, w.Jobs[0].Status, "", tasks.MutationOptions{Actor: actor})
	return t.ID, err
}

// AgentAction is a send to agent in repo dir, dispatched the way a board
// trigger reaches a role: its standing pane, spawned when absent.
func AgentAction(dir, agent, prompt string) (Action, error) {
	dir = tasks.MainRoot(dir)
	a := Action{Dir: dir, Agent: agent, Prompt: prompt, LongLived: true}
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		return a, err
	}
	if r, ok := cfg.Roles[agent]; ok {
		a.Role, a.Model, a.Effort = r.Agent, r.Model, r.Effort
		if r.Tier != "" {
			route(&a, r, "")
		}
		if a.BlockReason != "" {
			return a, errors.New(a.BlockReason)
		}
	}
	return a, nil
}
