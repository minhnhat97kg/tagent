// Package routing picks the harness and model for a role that declares a tier,
// from the account-wide ~/.config/tagent/routing.yaml and the provider quota left.
package routing

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"tagent/internal/apppath"
	"tagent/internal/quota"

	"gopkg.in/yaml.v3"
)

// Config maps each tier to its candidates, tried in order as "harness:model".
type Config struct {
	Limits        map[string]float64  `yaml:"limits,omitempty" json:"limits,omitempty"` // percent used that counts as exhausted, per provider
	WhenExhausted string              `yaml:"when_exhausted,omitempty" json:"when_exhausted,omitempty"`
	Tiers         map[string][]string `yaml:"tiers,omitempty" json:"tiers,omitempty"`
}

// Candidate is one harness:model of a tier with what the quota says about it.
// State is available, exhausted or unknown; unknown counts as available.
type Candidate struct {
	Harness, Model string
	State, Why     string
	Reset          time.Time // when the exhausted windows free up; zero if unknown
}

// Choice is the candidate a dispatch runs. Reason names the candidates skipped for quota.
type Choice struct {
	Harness, Model, Reason string
}

const defaultLimit = 90

var harnesses = []string{"claude", "pi", "codex"}

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
	return c, c.validate()
}

func (c Config) validate() error {
	for p, v := range c.Limits {
		if p != "claude" && p != "codex" {
			return fmt.Errorf("limits: unknown provider %q (claude or codex)", p)
		}
		if v < 1 || v > 100 {
			return fmt.Errorf("limits %s: %v must be 1 to 100", p, v)
		}
	}
	if c.WhenExhausted != "" && c.WhenExhausted != "block" && c.WhenExhausted != "first" {
		return fmt.Errorf("when_exhausted %q: use block or first", c.WhenExhausted)
	}
	for tier, list := range c.Tiers {
		if len(list) == 0 {
			return fmt.Errorf("tier %s: no candidates", tier)
		}
		for _, s := range list {
			if _, _, err := parse(s); err != nil {
				return fmt.Errorf("tier %s: %w", tier, err)
			}
		}
	}
	return nil
}

func parse(s string) (harness, model string, err error) {
	harness, model, ok := strings.Cut(s, ":")
	if !ok || model == "" || !slices.Contains(harnesses, harness) {
		return "", "", fmt.Errorf("candidate %q: want claude:MODEL, pi:MODEL or codex:MODEL", s)
	}
	return harness, model, nil
}

// provider names the quota a candidate draws on; "" means unknown.
func provider(harness, model string) string {
	switch {
	case harness == "claude":
		return "claude"
	case harness == "codex" || (harness == "pi" && strings.HasPrefix(model, "openai-codex/")):
		return "codex"
	}
	return ""
}

func (c Config) limit(provider string) float64 {
	if v, ok := c.Limits[provider]; ok {
		return v
	}
	return defaultLimit
}

// Candidates returns the tier's candidates judged against snap, in config order.
// A window whose reset has passed is ignored: the snapshot predates it.
func (c Config) Candidates(tier string, snap quota.Snapshot) ([]Candidate, error) {
	list, ok := c.Tiers[tier]
	if !ok {
		return nil, fmt.Errorf("unknown tier %q", tier)
	}
	var out []Candidate
	now := time.Now()
	for _, s := range list {
		h, m, err := parse(s)
		if err != nil {
			return nil, err
		}
		cd := Candidate{Harness: h, Model: m, State: "unknown"}
		name := provider(h, m)
		if name != "" {
			p, label := snap.Claude, "Claude"
			if name == "codex" {
				p, label = snap.Codex, "Codex"
			}
			var parts []string
			counted := false
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
				counted = true
				if *w.win.Used >= c.limit(name) {
					parts = append(parts, fmt.Sprintf("%s %s %.0f%%", label, w.name, *w.win.Used))
					if at.After(cd.Reset) {
						cd.Reset = at
					}
				}
			}
			switch {
			case len(parts) > 0:
				cd.State, cd.Why = "exhausted", strings.Join(parts, ", ")
			case counted:
				cd.State = "available"
			}
		}
		out = append(out, cd)
	}
	return out, nil
}

// First is the tier's first candidate: what a dispatch runs when no quota is known.
func (c Config) First(tier string) (Choice, error) { return c.Select(tier, quota.Snapshot{}) }

// Select picks the first candidate of tier that still has quota. When all are
// exhausted, when_exhausted decides: first runs the first candidate, block (the
// default) returns an error with the windows and the earliest reset.
func (c Config) Select(tier string, snap quota.Snapshot) (Choice, error) {
	cands, err := c.Candidates(tier, snap)
	if err != nil {
		return Choice{}, err
	}
	if len(cands) == 0 {
		return Choice{}, fmt.Errorf("tier %s has no candidates", tier)
	}
	var skipped []string
	for _, cd := range cands {
		if cd.State != "exhausted" {
			return Choice{Harness: cd.Harness, Model: cd.Model, Reason: strings.Join(skipped, "; ")}, nil
		}
		skipped = append(skipped, cd.Why)
	}
	if c.WhenExhausted == "first" {
		return Choice{Harness: cands[0].Harness, Model: cands[0].Model, Reason: strings.Join(skipped, "; ")}, nil
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
