package metrics

import (
	"cmp"
	"fmt"
	"sort"
	"strings"
	"time"

	"tagent/internal/tasks"
	"tagent/internal/usage"
)

// JobUsage is one (role, session) row: re-entries of a job into the same
// session are one row, counted once.
type JobUsage struct {
	Role    string        `json:"role"`
	Harness string        `json:"harness"`
	Model   string        `json:"model"`
	Effort  string        `json:"effort"`
	Pane    string        `json:"pane"`
	Loops   int           `json:"loops"`
	Elapsed time.Duration `json:"elapsed"`
	Count   usage.Count   `json:"count"`
	Has     bool          `json:"has"` // false: the harness reports nothing, show n/a
	Live    bool          `json:"live"`
	Cost    *Money        `json:"cost,omitempty"` // nil: no usage or unknown model
}

type ModelUsage struct {
	Harness string      `json:"harness"`
	Model   string      `json:"model"`
	Count   usage.Count `json:"count"`
	Has     bool        `json:"has"`
}

type TicketUsage struct {
	Jobs     []JobUsage    `json:"jobs"`
	Models   []ModelUsage  `json:"models"`
	Total    usage.Count   `json:"total"`
	Partial  bool          `json:"partial"` // some job has no usage: total is a lower bound
	Elapsed  time.Duration `json:"elapsed"`
	Loops    int           `json:"loops"`
	Cost     *Money        `json:"cost,omitempty"`     // API-equivalent total of the priced jobs
	Unpriced bool          `json:"unpriced,omitempty"` // some job has usage but no known model price
}

func addCount(a *usage.Count, b usage.Count) {
	a.Input += b.Input
	a.Output += b.Output
	a.CacheRead += b.CacheRead
	a.CacheWrite += b.CacheWrite
	a.CacheWrite1h += b.CacheWrite1h
	a.Long.Add(b.Long)
	a.Total += b.Total
}

// resolve finds the stored session of a dispatch: pane name first (pane ids are
// reused after a tmux restart), then pane id. Among several sessions with that
// name it takes the newest one that started before the dispatch.
func resolve(s State, e tasks.RunEntry) (best Session, found bool) {
	better := func(c, b Session) bool {
		cn, bn := e.PaneName != "" && c.Agent == e.PaneName, e.PaneName != "" && b.Agent == e.PaneName
		ce, be := !c.FirstSeen.After(e.Start.Add(time.Minute)), !b.FirstSeen.After(e.Start.Add(time.Minute))
		if cn != bn {
			return cn
		}
		if ce != be {
			return ce
		}
		return c.FirstSeen.After(b.FirstSeen) == ce
	}
	for _, c := range s.Sessions {
		// pane id only when the name cannot decide: ids are reused after a tmux restart
		byID := e.Pane != "" && c.Pane == e.Pane && (e.PaneName == "" || c.Agent == "")
		if (e.PaneName != "" && c.Agent == e.PaneName) || byID {
			if !found || better(c, best) {
				best, found = c, true
			}
		}
	}
	return
}

// withRates fills in the billing detail of a record saved before usage kept it, from
// the transcript, when that still holds exactly the same tokens.
// ponytail: such a record is re-read (once per process) on every view of its ticket.
func withRates(s Session) usage.Count {
	c := s.Usage
	if s.Harness == "claude" && c.CacheWrite1h == 0 && c.Long == (usage.Split{}) {
		if now, ok := usage.Read(s.Harness, s.ID); ok && now.Total == c.Total {
			return now
		}
	}
	return c
}

// TicketUsageOf attributes stored session usage to one ticket through its
// dispatch events (role, harness, model, pane name), so closed panes still
// count. ponytail: a session is whole-pane usage, so a pane reused across
// tickets over-counts; pane names are role-ticket which mostly prevents it.
func TicketUsageOf(s State, t tasks.Task, cfg tasks.BoardConfig, events []tasks.Event, now time.Time) TicketUsage {
	var entries []tasks.RunEntry
	for _, e := range tasks.BuildRuns(t, cfg, events, now) {
		if e.Role != "" || e.Pane != "" {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 { // not a workflow ticket: raw dispatches still attribute
		for _, ev := range events {
			if ev.Task == t.ID && ev.Kind == "dispatch" && (ev.Role != "" || ev.Pane != "") {
				entries = append(entries, tasks.RunEntry{Role: ev.Role, Harness: ev.Harness, Model: ev.Model, Effort: ev.Effort, Pane: ev.Pane, PaneName: ev.PaneName, Start: ev.Time, Loop: 1})
			}
		}
	}
	var u TicketUsage
	rows := map[string]int{}
	seen := map[string]bool{}
	for _, e := range entries {
		sess, ok := resolve(s, e)
		key := e.Role + "|" + e.Harness + "|" + e.Model
		if ok {
			key = sess.Key
			seen[key] = true
		}
		i, have := rows[key]
		if !have {
			j := JobUsage{Role: e.Role, Harness: e.Harness, Model: e.Model, Effort: e.Effort, Pane: e.Pane}
			if ok {
				j.Harness, j.Has, j.Live = sess.Harness, sess.HasUsage, sess.Status != "closed"
				j.Count = withRates(sess)
			}
			u.Jobs = append(u.Jobs, j)
			i = len(u.Jobs) - 1
			rows[key] = i
		}
		u.Jobs[i].Loops++
		u.Jobs[i].Elapsed += e.Duration
	}
	// Manual `task start ID pane` links keep working: runs no dispatch resolved to.
	for _, r := range RunsOf(s, t.ID) {
		if seen[r.SessionKey] {
			continue
		}
		seen[r.SessionKey] = true
		n, ok := r.Tokens()
		j := JobUsage{Role: r.Agent, Loops: 1, Elapsed: r.Elapsed(now), Has: ok, Live: r.Ended.IsZero()}
		j.Count.Total = n
		if sess, found := s.Sessions[r.SessionKey]; found {
			j.Harness = sess.Harness
		}
		u.Jobs = append(u.Jobs, j)
	}
	models := map[string]int{}
	for _, j := range u.Jobs {
		u.Loops += j.Loops
		u.Elapsed += j.Elapsed
		k := j.Harness + "/" + j.Model
		i, ok := models[k]
		if !ok {
			u.Models = append(u.Models, ModelUsage{Harness: j.Harness, Model: j.Model})
			i = len(u.Models) - 1
			models[k] = i
		}
		if j.Has {
			addCount(&u.Total, j.Count)
			addCount(&u.Models[i].Count, j.Count)
			u.Models[i].Has = true
		} else {
			u.Partial = true
		}
	}
	for i := range u.Jobs {
		j := &u.Jobs[i]
		if !j.Has {
			continue
		}
		p, ok := priceOf(cfg, j.Model)
		if !ok {
			u.Unpriced = true
			continue
		}
		c := costOf(j.Count, p)
		j.Cost = &c
		if u.Cost == nil {
			u.Cost = &Money{}
		}
		u.Cost.Low += c.Low
		u.Cost.High += c.High
	}
	sort.SliceStable(u.Models, func(a, b int) bool { return u.Models[a].Count.Total > u.Models[b].Count.Total })
	return u
}

// Lines is the text block shared by the CLI and the TUI.
func (u TicketUsage) Lines(compact func(int64) string) []string {
	if len(u.Jobs) == 0 {
		return nil
	}
	tok := func(c usage.Count, has bool) string {
		if !has {
			return "n/a"
		}
		s := compact(c.Total)
		if c.CacheRead+c.CacheWrite > 0 {
			s += " (cache " + compact(c.CacheRead) + "/" + compact(c.CacheWrite) + ")"
		}
		return s
	}
	head := "Usage: " + compact(u.Total.Total) + " tokens"
	if u.Partial {
		head += " (partial)"
	}
	if u.Cost != nil {
		head += " · " + usd(*u.Cost) + " API-equivalent"
	}
	if u.Unpriced {
		head += " (unknown model: tokens only)"
	}
	lines := []string{head + fmt.Sprintf(" · %d loops · %s", u.Loops, u.Elapsed.Round(time.Second))}
	for _, j := range u.Jobs {
		parts := []string{j.Role, strings.Trim(j.Harness+"/"+j.Model, "/"), j.Effort, tok(j.Count, j.Has)}
		if j.Cost != nil {
			parts = append(parts, "≈"+usd(*j.Cost))
		}
		if j.Loops > 1 {
			parts = append(parts, fmt.Sprintf("×%d", j.Loops))
		}
		parts = append(parts, j.Elapsed.Round(time.Second).String())
		lines = append(lines, "  "+strings.Join(compactStrings(parts), "  "))
	}
	if len(u.Models) > 1 {
		for _, m := range u.Models {
			lines = append(lines, "  "+cmp.Or(strings.Trim(m.Harness+"/"+m.Model, "/"), "?")+"  "+tok(m.Count, m.Has))
		}
	}
	return lines
}

func compactStrings(in []string) []string {
	out := in[:0:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
