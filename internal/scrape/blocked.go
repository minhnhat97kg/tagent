package scrape

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"tagent/internal/board"
	"tagent/internal/notify"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

// A ticket that waits on a human (status blocked) for longer than
// @agent_blocked_hours (default 24) gets one notification per blocked spell.
// A spell is the blocked event's seq: moving the ticket out and back in is a new
// spell and alerts again. The alerted spells live in the scope's board dir, so a
// restart does not repeat them.
// ponytail: no lock across tmux servers: two scrapers on one repo may both alert once.

const (
	blockedOption       = "@agent_blocked_hours"
	defaultBlockedHours = 24
	blockedAlerted      = "blocked-alerted"
)

// sendBlocked is a seam so tests can capture the alert.
var sendBlocked = notify.Send

func blockedAfter() time.Duration {
	if v, ok := tmux.GetOption(blockedOption); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			return time.Duration(n) * time.Hour
		}
	}
	return defaultBlockedHours * time.Hour
}

// checkBlocked alerts once per blocked spell of every registered repo's
// ticket that has been blocked longer than the limit.
func checkBlocked(now time.Time) {
	limit := blockedAfter()
	for _, dir := range board.Registered() {
		scope := tasks.Scope(dir)
		var blocked []tasks.Task
		isBlocked := map[string]bool{}
		for _, t := range tasks.List(scope) {
			if t.Status == "blocked" {
				blocked = append(blocked, t)
				isBlocked[t.ID] = true
			}
		}
		path := filepath.Join(tasks.BoardDir(scope), blockedAlerted)
		alerted := map[string]string{} // ticket -> seq of the spell already alerted
		if b, err := os.ReadFile(path); err == nil {
			json.Unmarshal(b, &alerted) // corrupt: treated as empty
		}
		changed := false
		for id := range alerted { // a ticket that left blocked has no spell to remember
			if !isBlocked[id] {
				delete(alerted, id)
				changed = true
			}
		}
		if len(blocked) == 0 && !changed {
			continue
		}
		events, err := tasks.ReadEvents(scope, 0)
		if err != nil {
			continue
		}
		since := map[string]tasks.Event{} // ticket -> its latest move to blocked
		for _, e := range events {
			if e.Kind == "status" && e.To == "blocked" {
				since[e.Task] = e
			}
		}
		var alerts []string
		for _, t := range blocked {
			ev, ok := since[t.ID]
			if !ok || now.Sub(ev.Time) < limit {
				continue
			}
			spell := strconv.FormatInt(ev.Seq, 10)
			if alerted[t.ID] == spell {
				continue
			}
			alerted[t.ID] = spell
			changed = true
			alerts = append(alerts, fmt.Sprintf("#%s %s — blocked for %dh, needs an answer", t.ID, t.Title, int(now.Sub(ev.Time).Hours())))
		}
		if !changed {
			continue
		}
		b, _ := json.Marshal(alerted)
		if tasks.AtomicWrite(path, b) != nil { // not recorded: no alert, so it cannot repeat every minute
			continue
		}
		for _, a := range alerts {
			sendBlocked("tagent: blocked ticket in "+filepath.Base(dir), a)
		}
	}
}
