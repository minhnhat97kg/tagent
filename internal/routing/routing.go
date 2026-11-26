// Package routing picks the harness and model for a role that declares a tier,
// from the account-wide ~/.config/tagent/routing.yaml and the provider quota left.
package routing

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tagent/internal/apppath"
	"tagent/internal/harnessusage"
	"tagent/internal/quota"
	"tagent/internal/roles"
	"tagent/internal/tasks"

	"gopkg.in/yaml.v3"
)

// Config maps each tier to its candidates, tried in order as "harness:model" or
// "harness:model@N": N is the percent at which that candidate is skipped (default: its provider's limit).
type Config struct {
	Limits        map[string]float64  `yaml:"limits,omitempty" json:"limits,omitempty"` // percent used that counts as exhausted, per provider
	WhenExhausted string              `yaml:"when_exhausted,omitempty" json:"when_exhausted,omitempty"`
	Tiers         map[string][]string `yaml:"tiers,omitempty" json:"tiers,omitempty"`
}

// Candidate is one harness:model of a tier with what the quota says about it.
// State is available, exhausted or unknown; unknown counts as available.
type Candidate struct {
	Ref            string // as written in the config, with @N
	Harness, Model string
	Until          float64  // own threshold; 0 when the candidate has none
	Threshold      float64  // percent that counts as exhausted; 0 when the provider is unknown
	Used           *float64 // highest counted window; nil when none is known
	State, Why     string
	Reset          time.Time // when the exhausted windows free up; zero if unknown
}

// Choice is the candidate a dispatch runs. Reason names the candidates skipped for quota.
type Choice struct {
	Ref, Harness, Model, Reason string
}

const defaultLimit = 90

var tierName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Reader is the usage readings of a configured harness that has usage commands;
// ok is false when it has none. A nil Reader reads nothing: every such quota is unknown.
type Reader func(harness string) ([]harnessusage.Reading, bool)

// Path is the routing config file.
func Path() string { return filepath.Join(apppath.ConfigDir(), "routing.yaml") }

// Load reads the routing config; a missing file is an empty config.
func Load() (Config, error) {
	var c Config
	b, err := os.ReadFile(Path())
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(&c); err != nil && err != io.EOF {
		return c, fmt.Errorf("%s: %w", Path(), err)
	}
	return c, c.Validate()
}

// Save validates c and writes it to Path. ponytail: last write wins; YAML comments are not preserved.
func Save(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(Path()), 0700); err != nil {
		return err
	}
	return tasks.AtomicWrite(Path(), b)
}

// Validate reports why c cannot be saved or dispatched.
func (c Config) Validate() error {
	for p, v := range c.Limits {
		if p != "claude" && p != "codex" && !hasUsage(p) {
			return fmt.Errorf("limits: unknown provider %q (claude, codex, or a harness with usage commands)", p)
		}
		if v < 1 || v > 100 {
			return fmt.Errorf("limits %s: %v must be 1 to 100", p, v)
		}
	}
	if c.WhenExhausted != "" && c.WhenExhausted != "block" && c.WhenExhausted != "first" {
		return fmt.Errorf("when_exhausted %q: use block or first", c.WhenExhausted)
	}
	for tier, list := range c.Tiers {
		if !tierName.MatchString(tier) {
			return fmt.Errorf("tier %q: use letters, digits, - and _, starting with a letter or digit", tier)
		}
		if len(list) == 0 {
			return fmt.Errorf("tier %s: no candidates", tier)
		}
		for _, s := range list {
			if _, _, _, err := parse(s); err != nil {
				return fmt.Errorf("tier %s: %w", tier, err)
			}
		}
	}
	return nil
}

// parse splits harness:model[@N]. A trailing @N is a threshold only if it is a short
// number; otherwise the @ belongs to the model id (e.g. vertex ids). until is 0 when absent.
func parse(s string) (harness, model string, until float64, err error) {
	harness, model, ok := strings.Cut(s, ":")
	if !ok || !roles.KnownHarness(harness) {
		return "", "", 0, fmt.Errorf("candidate %q: want HARNESS:MODEL[@1-100], HARNESS one of %s", s, strings.Join(roles.HarnessNames(), ", "))
	}
	if i := strings.LastIndex(model, "@"); i >= 0 && len(model)-i <= 8 {
		if v, perr := strconv.ParseFloat(model[i+1:], 64); perr == nil {
			if !(v >= 1 && v <= 100) {
				return "", "", 0, fmt.Errorf("candidate %q: threshold must be 1 to 100", s)
			}
			until, model = v, model[:i]
		}
	}
	if model == "" {
		return "", "", 0, fmt.Errorf("candidate %q: want HARNESS:MODEL[@1-100]", s)
	}
	return harness, model, until, nil
}

// win is one counted usage window: a percentage and when it resets (zero if unknown).
type win struct {
	name  string
	used  float64
	reset time.Time
}

// source names the quota a candidate draws on and its windows; "" means unknown.
// A configured harness with usage commands is its own provider. Otherwise it keeps
// the quota of its kind unless its entry sets env, which means another account.
func source(h, m string, specs []roles.HarnessSpec, snap quota.Snapshot, read Reader, now time.Time) (string, []win) {
	spec, configured := find(specs, h)
	if configured && len(spec.Usage) > 0 {
		var rs []harnessusage.Reading
		if read != nil {
			rs, _ = read(h)
		}
		var ws []win
		for _, r := range rs {
			ws = append(ws, win{cmp.Or(r.Label, "usage"), r.Used, time.Time{}})
		}
		return h, ws
	}
	if configured && len(spec.Env) > 0 {
		return "", nil
	}
	name := ""
	switch kind := roles.Harness(h); {
	case kind == "claude":
		name = "claude"
	case kind == "codex" || (kind == "pi" && strings.HasPrefix(m, "openai-codex/")):
		name = "codex"
	}
	if name == "" {
		return "", nil
	}
	p := snap.Claude
	if name == "codex" {
		p = snap.Codex
	}
	var ws []win
	for _, w := range []struct {
		name string
		win  quota.Window
	}{{"5h", p.Five}, {"7d", p.Week}} {
		if w.win.Used == nil {
			continue
		}
		at := w.win.ResetAt()
		if !at.IsZero() && at.Before(now) {
			continue
		}
		ws = append(ws, win{w.name, *w.win.Used, at})
	}
	return name, ws
}

// hasUsage reports whether name is a configured harness with usage commands, the only harness quota that has its own limit key.
func hasUsage(name string) bool {
	specs, _ := roles.LoadHarnesses()
	spec, ok := find(specs, name)
	return ok && len(spec.Usage) > 0
}

// find returns the harnesses.yml entry called name.
func find(specs []roles.HarnessSpec, name string) (roles.HarnessSpec, bool) {
	for _, h := range specs {
		if h.Name == name {
			return h, true
		}
	}
	return roles.HarnessSpec{}, false
}

// Limit is the percent used that counts as exhausted for provider when a candidate has no @N.
func (c Config) Limit(provider string) float64 {
	if v, ok := c.Limits[provider]; ok {
		return v
	}
	return defaultLimit
}

// Candidates returns the tier's candidates judged against snap and read, in config order.
// A window whose reset has passed is ignored: the snapshot predates it.
func (c Config) Candidates(tier string, snap quota.Snapshot, read Reader) ([]Candidate, error) {
	list, ok := c.Tiers[tier]
	if !ok {
		return nil, fmt.Errorf("unknown tier %q", tier)
	}
	specs, _ := roles.LoadHarnesses()
	var out []Candidate
	now := time.Now()
	for _, s := range list {
		h, m, until, err := parse(s)
		if err != nil {
			return nil, err
		}
		cd := Candidate{Ref: s, Harness: h, Model: m, Until: until, State: "unknown"}
		name, ws := source(h, m, specs, snap, read, now)
		if name != "" {
			cd.Threshold = until
			if cd.Threshold == 0 {
				cd.Threshold = c.Limit(name)
			}
			var parts []string
			for _, w := range ws {
				if cd.Used == nil || w.used > *cd.Used {
					used := w.used
					cd.Used = &used
				}
				if w.used >= cd.Threshold {
					parts = append(parts, fmt.Sprintf("%s %.0f%% >= %g", w.name, w.used, cd.Threshold))
					if w.reset.After(cd.Reset) {
						cd.Reset = w.reset
					}
				}
			}
			switch {
			case len(parts) > 0:
				cd.State, cd.Why = "exhausted", strings.Join(parts, ", ")
			case len(ws) > 0:
				cd.State = "available"
			}
		}
		out = append(out, cd)
	}
	return out, nil
}

// First is the tier's first candidate: what a dispatch runs when no quota is known.
func (c Config) First(tier string) (Choice, error) { return c.Select(tier, quota.Snapshot{}, nil) }

// Select picks the first candidate of tier that still has quota. When all are
// exhausted, when_exhausted decides: first runs the first candidate, block (the
// default) returns an error with the windows and the earliest reset.
func (c Config) Select(tier string, snap quota.Snapshot, read Reader) (Choice, error) {
	cands, err := c.Candidates(tier, snap, read)
	if err != nil {
		return Choice{}, err
	}
	if len(cands) == 0 {
		return Choice{}, fmt.Errorf("tier %s has no candidates", tier)
	}
	var skipped []string
	for _, cd := range cands {
		if cd.State != "exhausted" {
			return Choice{Ref: cd.Ref, Harness: cd.Harness, Model: cd.Model, Reason: strings.Join(skipped, "; ")}, nil
		}
		skipped = append(skipped, cd.Harness+":"+cd.Model+" "+cd.Why)
	}
	if c.WhenExhausted == "first" {
		skipped = append(skipped, "all exhausted, running first")
		return Choice{Ref: cands[0].Ref, Harness: cands[0].Harness, Model: cands[0].Model, Reason: strings.Join(skipped, "; ")}, nil
	}
	var earliest time.Time
	for _, cd := range cands {
		if !cd.Reset.IsZero() && (earliest.IsZero() || cd.Reset.Before(earliest)) {
			earliest = cd.Reset
		}
	}
	msg := fmt.Sprintf("tier %s exhausted: %s", tier, strings.Join(skipped, "; "))
	if !earliest.IsZero() {
		msg += ", earliest reset " + earliest.Local().Format("Jan 2 15:04")
	}
	return Choice{}, errors.New(msg)
}
