// Package agents is the one read/write model for agent roles. tmux-agent owns
// the metadata (harness/model/effort in .tasks/board.yaml) and the instructions
// (.tasks/agents/<name>.md); instruction files are never filtered by harness.
package agents

import (
	"maps"
	"slices"
	"strings"

	"tagent/internal/roles"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
)

type Agent struct {
	Name         string     `json:"name"`
	Harness      string     `json:"harness"`
	Model        string     `json:"model"`
	Effort       string     `json:"effort"`
	Source       string     `json:"source"` // repo, legacy, global or board
	Path         string     `json:"path"`
	Instructions string     `json:"instructions"`
	Running      int        `json:"running"`
	Panes        []LivePane `json:"panes"`
	UsedBy       []string   `json:"usedBy"`
}

// LivePane is an agent pane and the role it runs (board target, else pane name).
type LivePane struct {
	Role   string `json:"-"`
	Pane   string `json:"pane"`
	Status string `json:"status"`
}

// LivePanes lists every agent pane with the role it runs.
func LivePanes() []LivePane {
	var out []LivePane
	for _, s := range tmux.QuerySessions() {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				role, _, _ := strings.Cut(p.BoardTarget, "#")
				if role == "" {
					role = p.Name
				}
				if role != "" {
					out = append(out, LivePane{Role: role, Pane: p.PaneID, Status: string(p.Status)})
				}
			}
		}
	}
	return out
}

// List merges app-owned instruction files, legacy harness files, global files and
// board.yaml roles. The process harness never filters the list: it only fills in
// the harness of an agent that no board.yaml role configures. Repo wins over
// global, and board values (harness, model, effort) win over everything.
func List(dir, harness string, live []LivePane) []Agent {
	process := roles.Harness(harness)
	cfg, _ := tasks.LoadBoardConfig(dir)
	var out []Agent
	seen := map[string]bool{}
	add := func(d roles.Definition, source string) {
		if seen[d.Name] {
			return
		}
		seen[d.Name] = true
		// Files never imply a harness: only a board.yaml role does, else the process.
		out = append(out, Agent{Name: d.Name, Harness: process, Source: source, Path: d.Path, Instructions: d.Instructions})
	}
	for _, d := range roles.ListLocal(dir) {
		source := "repo"
		if d.Native {
			source = "legacy"
		}
		add(d, source)
	}
	for _, d := range roles.ListGlobal() {
		add(d, "global")
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Roles)) {
		r := cfg.Roles[name]
		i := slices.IndexFunc(out, func(a Agent) bool { return a.Name == name })
		if i < 0 {
			out = append(out, Agent{Name: name, Source: "board"})
			i = len(out) - 1
		}
		if r.Agent != "" {
			out[i].Harness = r.Agent
		}
		out[i].Model, out[i].Effort = r.Model, r.Effort
		if out[i].Harness == "" {
			out[i].Harness = process // a board-only role with no agent: runs on the process harness
		}
	}
	wfs := workflow.List(tasks.MainRoot(dir))
	for i := range out {
		a := &out[i]
		for _, w := range wfs {
			for _, j := range w.Jobs {
				if j.Agent == a.Name {
					a.UsedBy = append(a.UsedBy, "workflow "+w.Name+" · "+j.Status)
				}
			}
		}
		for _, p := range live {
			if p.Role == a.Name {
				a.Panes = append(a.Panes, p)
			}
		}
		a.Running = len(a.Panes)
	}
	slices.SortFunc(out, func(a, b Agent) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Find returns the agent called name, as List sees it.
func Find(dir, harness, name string) (Agent, bool) {
	for _, a := range List(dir, harness, nil) {
		if a.Name == name {
			return a, true
		}
	}
	return Agent{}, false
}
