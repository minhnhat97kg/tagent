package board

import (
	"fmt"
	"strings"
	"tagent/internal/tasks"
)

// loopOwner reports which looped step a hand-back counts against: the step that
// hands the ticket back when it is looped (review -> build), else the step it is
// handed back into when that one is looped (test -> build).
func loopOwner(cfg tasks.BoardConfig, ev tasks.Event) (tasks.Status, bool) {
	if ev.Kind != "status" || ev.From == ev.To || ev.To == "blocked" {
		return tasks.Status{}, false
	}
	from, to := statusByID(cfg, ev.From), statusByID(cfg, ev.To)
	if to.ID == "" || failureOf(cfg, from) != to.ID {
		return tasks.Status{}, false
	}
	if from.MaxLoops > 0 {
		return from, true
	}
	if to.MaxLoops > 0 {
		return to, true
	}
	return tasks.Status{}, false
}

// failureOf is the status a step hands back to on a failure. Workflow steps carry
// it; a legacy board status has none, so it falls back to the earlier status in its
// next list, as legacyWorkflow does.
func failureOf(cfg tasks.BoardConfig, s tasks.Status) string {
	if s.Failure != "" {
		return s.Failure
	}
	if s.Fail != "" {
		return s.Fail
	}
	at := -1
	for i, x := range cfg.Statuses {
		if x.ID == s.ID {
			at = i
		}
	}
	for _, n := range s.Next {
		for i, x := range cfg.Statuses {
			if x.ID == n && i < at && n != "blocked" && n != "todo" && n != "done" {
				return n
			}
		}
	}
	return ""
}

func statusByID(cfg tasks.BoardConfig, id string) tasks.Status {
	for _, s := range cfg.Statuses {
		if s.ID == id {
			return s
		}
	}
	return tasks.Status{}
}

// loopCapBlock counts the hand-backs owned by ev's looped step since it last left
// blocked, and returns a block reason when this one is past the cap.
func loopCapBlock(dir string, cfg tasks.BoardConfig, ev tasks.Event, comment tasks.TaskComment) (string, error) {
	owner, ok := loopOwner(cfg, ev)
	if !ok {
		return "", nil
	}
	history, err := tasks.ReadEvents(tasks.Scope(dir), 0)
	if err != nil {
		return "", err
	}
	count := 0
	for _, e := range history {
		if e.Seq > ev.Seq || e.Task != ev.Task || e.Kind != "status" || e.From == e.To {
			continue
		}
		if e.From == "blocked" {
			count = 0
		}
		if o, ok := loopOwner(cfg, e); ok && o.ID == owner.ID {
			count++
		}
	}
	if count <= owner.MaxLoops {
		return "", nil
	}
	return fmt.Sprintf("%s rejected %d/%d; last findings: %s", owner.ID, count, owner.MaxLoops, excerpt(comment.Body)), nil
}

func excerpt(body string) string {
	line := strings.TrimSpace(body)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if r := []rune(line); len(r) > 200 {
		line = string(r[:200]) + "…"
	}
	return line
}
