package metrics

import (
	"cmp"
	"fmt"

	"tagent/internal/tasks"
	"tagent/internal/usage"
)

// prices are Anthropic API list prices, USD per MTok (Pricing page, checked
// 2026-11-26). board.yaml `pricing:` adds or overrides models without code.
var prices = map[string]tasks.ModelPrice{
	"claude-opus-5-5":   {Input: 4, Output: 20, CacheRead: 0.20, CacheWrite: 5, CacheWrite1h: 8},
	"claude-sonnet-5-5": {Input: 2, Output: 10, CacheRead: 0.10, CacheWrite: 2.50, CacheWrite1h: 4},
	"claude-haiku-5-5": {Input: 0.10, Output: 0.50, CacheRead: 0.01, CacheWrite: 0.125, CacheWrite1h: 0.20,
		Over100K: &tasks.ModelPrice{Input: 0.50, Output: 2.50, CacheRead: 0.05, CacheWrite: 0.625, CacheWrite1h: 1}},
}

// Money is an API-equivalent estimate in USD, never a bill: subscription
// logins are priced at API rates too. Low < High only when the input/output
// split is unknown, then the uncached tokens are priced as all input (Low) and
// all output (High).
type Money struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// claudeAliases maps the short names a board role may use to the current
// generation, which is what "sonnet" and "haiku" mean without a version.
// ponytail: an alias follows the current generation; pin the full model id in
// the role if Claude Code resolves the alias to an older model.
var claudeAliases = map[string]string{
	"opus":   "claude-opus-5-5",
	"sonnet": "claude-sonnet-5-5",
	"haiku":  "claude-haiku-5-5",
}

// priceOf returns the board.yaml price for model, else the built-in one.
// ok=false for an unknown model: the caller shows tokens only, no guess.
func priceOf(cfg tasks.BoardConfig, model string) (tasks.ModelPrice, bool) {
	if model == "" {
		return tasks.ModelPrice{}, false
	}
	if p, ok := cfg.Pricing[model]; ok {
		return p, true
	}
	if id, ok := claudeAliases[model]; ok {
		model = id
	}
	p, ok := prices[model]
	return p, ok
}

// costOf prices one job's usage at p.
func costOf(c usage.Count, p tasks.ModelPrice) Money {
	long := c.Long
	if p.Over100K == nil {
		long = usage.Split{}
	}
	all := usage.Split{In: c.Input, Out: c.Output, CacheRead: c.CacheRead, CacheWrite: c.CacheWrite, CacheWrite1h: c.CacheWrite1h}
	short := usage.Split{In: all.In - long.In, Out: all.Out - long.Out, CacheRead: all.CacheRead - long.CacheRead,
		CacheWrite: all.CacheWrite - long.CacheWrite, CacheWrite1h: all.CacheWrite1h - long.CacheWrite1h}
	exact := rate(short, p)
	if p.Over100K != nil {
		exact += rate(long, *p.Over100K)
	}
	if rest := c.Total - c.CacheRead - c.CacheWrite; c.Input+c.Output == 0 && rest > 0 {
		uncached := float64(rest) / 1e6
		return Money{Low: exact + uncached*p.Input, High: exact + uncached*p.Output}
	}
	return Money{Low: exact, High: exact}
}

// rate is the USD price of t at p.
func rate(t usage.Split, p tasks.ModelPrice) float64 {
	hour := cmp.Or(p.CacheWrite1h, p.CacheWrite)
	return (float64(t.In)*p.Input + float64(t.Out)*p.Output + float64(t.CacheRead)*p.CacheRead +
		float64(t.CacheWrite-t.CacheWrite1h)*p.CacheWrite + float64(t.CacheWrite1h)*hour) / 1e6
}

// usd renders a Money as "$0.72" or "$0.12–0.60", and "<$0.01" for dust.
func usd(m Money) string {
	one := func(v float64) string {
		if v > 0 && v < 0.01 {
			return "<$0.01"
		}
		return fmt.Sprintf("$%.2f", v)
	}
	if m.Low == m.High {
		return one(m.Low)
	}
	return one(m.Low) + "–" + one(m.High)
}
