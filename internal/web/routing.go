package web

import (
	"net/http"
	"sort"
	"time"

	"tagent/internal/quota"
	"tagent/internal/routing"
	"tagent/internal/tasks"
)

type routingWindow struct {
	UsedPercent *float64 `json:"usedPercent"`
	ResetsAt    *string  `json:"resetsAt"`
}

type routingProvider struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Five  routingWindow `json:"five"`
	Week  routingWindow `json:"week"`
	Stale bool          `json:"stale"`
}

type routingCandidate struct {
	Ref   string `json:"ref"`
	State string `json:"state"`
	Why   string `json:"why"`
}

type routingTier struct {
	Name       string             `json:"name"`
	UsedBy     []string           `json:"usedBy"`
	Candidates []routingCandidate `json:"candidates"`
	Selected   string             `json:"selected"`
	Reason     string             `json:"reason"`
}

type routingView struct {
	Path          string             `json:"path"`
	Error         string             `json:"error,omitempty"`
	Limits        map[string]float64 `json:"limits"`
	WhenExhausted string             `json:"when_exhausted"`
	Providers     []routingProvider  `json:"providers"`
	Tiers         []routingTier      `json:"tiers"`
}

func routingWin(w quota.Window) routingWindow {
	out := routingWindow{UsedPercent: w.Used}
	if at := w.ResetAt(); !at.IsZero() {
		s := at.Format(time.RFC3339)
		out.ResetsAt = &s
	}
	return out
}

// routingUsers maps each tier to the roles of dir that name it.
func routingUsers(dir string) map[string][]string {
	out := map[string][]string{}
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		return out
	}
	for role, r := range cfg.Roles {
		if r.Tier != "" {
			out[r.Tier] = append(out[r.Tier], role)
		}
	}
	for _, roles := range out {
		sort.Strings(roles)
	}
	return out
}

// routingState is the page's state: quota and selection come from internal/routing only.
func (s *server) routingState(dir string) routingView {
	snap := s.quota()
	v := routingView{Path: routing.Path(), Providers: []routingProvider{
		{ID: "claude", Name: "Claude", Five: routingWin(snap.Claude.Five), Week: routingWin(snap.Claude.Week), Stale: snap.Claude.Stale},
		{ID: "codex", Name: "Codex", Five: routingWin(snap.Codex.Five), Week: routingWin(snap.Codex.Week), Stale: snap.Codex.Stale},
	}, Tiers: []routingTier{}}
	cfg, err := routing.Load()
	if err != nil {
		v.Error, cfg = err.Error(), routing.Config{}
	}
	v.Limits = map[string]float64{"claude": cfg.Limit("claude"), "codex": cfg.Limit("codex")}
	v.WhenExhausted = cfg.WhenExhausted
	if v.WhenExhausted == "" {
		v.WhenExhausted = "block"
	}
	if err != nil {
		return v
	}
	users := routingUsers(dir)
	names := make([]string, 0, len(cfg.Tiers))
	for name := range cfg.Tiers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := routingTier{Name: name, UsedBy: users[name], Candidates: []routingCandidate{}}
		if t.UsedBy == nil {
			t.UsedBy = []string{}
		}
		cands, _ := cfg.Candidates(name, snap)
		for _, c := range cands {
			t.Candidates = append(t.Candidates, routingCandidate{Ref: c.Harness + ":" + c.Model, State: c.State, Why: c.Why})
		}
		if choice, err := cfg.Select(name, snap); err != nil {
			t.Reason = err.Error()
		} else {
			t.Selected, t.Reason = choice.Harness+":"+choice.Model, choice.Reason
		}
		v.Tiers = append(v.Tiers, t)
	}
	return v
}

// routing reads (GET) or replaces (PUT) the account's routing.yaml. PUT never
// overwrites a file that does not load, and never writes a body without tiers.
func (s *server) routingAPI(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	if r.Method == http.MethodPut {
		if _, err := routing.Load(); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		var c routing.Config
		if err := readBody(r, &c); err != nil {
			fail(w, err)
			return
		}
		if c.Tiers == nil {
			http.Error(w, "tiers missing", http.StatusBadRequest)
			return
		}
		if err := c.Validate(); err != nil {
			fail(w, err)
			return
		}
		if err := routing.Save(c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	writeJSON(w, s.routingState(dir))
}
