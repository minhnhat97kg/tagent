package scrape

import (
	"regexp"
	"strings"
	"time"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

// A board agent that goes idle without moving its ticket leaves the workflow stuck: only
// `task move` advances it. nudgeStuck reminds such a pane once per ticket status.
const unmovedAfter = time.Minute

var (
	ticketSuffix = regexp.MustCompile(`-(\d+)$`)
	idleSince    = map[string]time.Time{} // pane -> when it was first seen idle
	nudged       = map[string]bool{}      // pane|status|updated -> reminder already sent
	lastStuck    time.Time
)

func nudgeStuck() {
	if time.Since(lastStuck) < 10*time.Second {
		return
	}
	lastStuck = time.Now()
	out, ok := tmux.RunTmux("list-panes", "-a", "-F", "#{pane_id}|#{@pane_name}|#{@pane_role}|#{@pane_status}|#{pane_current_path}")
	if !ok {
		return
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.SplitN(line, "|", 5)
		if len(f) < 5 || f[3] != string(tmux.StatusIdle) {
			continue
		}
		id, name, role, path := f[0], f[1], f[2], f[4]
		m := ticketSuffix.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		if role == "" {
			role = strings.TrimSuffix(name, m[0]) // board panes are named <role>-<ticket>
		}
		seen[id] = true
		if _, ok := idleSince[id]; !ok {
			idleSince[id] = time.Now()
		}
		if time.Since(idleSince[id]) < unmovedAfter {
			continue
		}
		t, ok := ownedTicket(tasks.MainRoot(path), m[1], role)
		if !ok {
			continue
		}
		key := id + "|" + t.Status + "|" + t.Updated.String()
		if nudged[key] || t.Updated.After(idleSince[id]) {
			continue
		}
		nudged[key] = true
		tmux.TypeLine(id, "You stopped without moving ticket "+m[1]+" (still "+t.Status+"). Run `tagent task move "+m[1]+" NEXT \"verdict/handoff\"` now, or `task move "+m[1]+" blocked \"one question\"`. Idle does not mean done.")
	}
	for id := range idleSince {
		if !seen[id] {
			delete(idleSince, id)
		}
	}
}

// ownedTicket returns the ticket when its current status belongs to role's job,
// i.e. the agent that stopped is the one expected to move it next.
func ownedTicket(dir, id, role string) (tasks.Task, bool) {
	t, err := tasks.Get(tasks.Scope(dir), id)
	if err != nil || t.Workflow == "" || t.Status == "todo" || t.Status == "blocked" || t.Status == "done" {
		return t, false
	}
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		return t, false
	}
	w, err := tasks.FindWorkflow(dir, cfg, t.Workflow)
	if err != nil {
		return t, false
	}
	for _, j := range w.Jobs {
		if j.Status == t.Status && j.Agent == role {
			return t, true
		}
	}
	return t, false
}
