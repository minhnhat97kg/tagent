// Package agents is the one read/write model for agent roles: repo and global
// instruction files plus the harness/model/effort in .tasks/board.yaml.
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
	Source       string     `json:"source"` // repo, global or board
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

// List merges repo files, global files and board.yaml roles; repo wins over global,
// and board values (harness, model, effort) win over the files' own harness.
func List(dir, harness string, live []LivePane) []Agent {
	harness = roles.Harness(harness)
	cfg, _ := tasks.LoadBoardConfig(dir)
	var out []Agent
	seen := map[string]bool{}
	add := func(d roles.Definition, source string) {
		if seen[d.Name] {
			return
		}
		seen[d.Name] = true
		h := d.Harness
		if h == "global" {
			h = harness
		}
		out = append(out, Agent{Name: d.Name, Harness: h, Source: source, Path: d.Path, Instructions: d.Instructions})
	}
	for _, d := range roles.ListIn(dir, harness) {
		add(d, "repo")
	}
	for _, d := range roles.ListGlobal(harness) {
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
			out[i].Harness = harness // a board-only role with no agent: runs on the process harness
		}
	}
	wfs := workflow.List(tasks.MainRoot(dir))
	for i := range out {
		a := &out[i]
		for _, s := range cfg.Statuses {
			if s.Role == a.Name {
				a.UsedBy = append(a.UsedBy, "status "+s.ID)
			}
		}
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
