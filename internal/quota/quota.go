package quota

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"tagent/internal/apppath"
	"time"
)

type Window struct {
	Used  *float64        `json:"usedPercent"`
	Reset json.RawMessage `json:"resetsAt"`
}
type Provider struct {
	Five  Window
	Week  Window
	Stale bool
}
type Snapshot struct {
	Claude Provider
	Codex  Provider
}

func ParseClaude(b []byte) Provider {
	var x struct {
		Usage struct {
			Fetched int64 `json:"fetchedAtMs"`
			U       struct {
				Five struct {
					Utilization *float64        `json:"utilization"`
					Resets      json.RawMessage `json:"resets_at"`
				} `json:"five_hour"`
				Week struct {
					Utilization *float64        `json:"utilization"`
					Resets      json.RawMessage `json:"resets_at"`
				} `json:"seven_day"`
			} `json:"utilization"`
		} `json:"cachedUsageUtilization"`
	}
	json.Unmarshal(b, &x)
	u := x.Usage.U
	return Provider{Five: Window{u.Five.Utilization, u.Five.Resets}, Week: Window{u.Week.Utilization, u.Week.Resets}, Stale: x.Usage.Fetched > 0 && time.Since(time.UnixMilli(x.Usage.Fetched)) > 10*time.Minute}
}
func ParseClaudeOAuth(b []byte) (Provider, error) {
	var u struct {
		Five *struct {
			Used  *float64        `json:"utilization"`
			Reset json.RawMessage `json:"resets_at"`
		} `json:"five_hour"`
		Week *struct {
			Used  *float64        `json:"utilization"`
			Reset json.RawMessage `json:"resets_at"`
		} `json:"seven_day"`
	}
	if err := json.Unmarshal(b, &u); err != nil {
		return Provider{}, err
	}
	var p Provider
	if u.Five != nil {
		p.Five = Window{u.Five.Used, u.Five.Reset}
	}
	if u.Week != nil {
		p.Week = Window{u.Week.Used, u.Week.Reset}
	}
	return p, nil
}
func ParseCodex(b []byte) Provider {
	var x struct {
		Result struct {
			Limits struct {
				Primary   Window `json:"primary"`
				Secondary Window `json:"secondary"`
			} `json:"rateLimits"`
		} `json:"result"`
	}
	json.Unmarshal(b, &x)
	return Provider{Five: x.Result.Limits.Primary, Week: x.Result.Limits.Secondary}
}
func claude() Provider {
	home, _ := os.UserHomeDir()
	var cred []byte
	if b, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output(); err == nil {
		cred = b
	} else {
		dir := os.Getenv("CLAUDE_CONFIG_DIR")
		if dir == "" {
			dir = filepath.Join(home, ".claude")
		}
		cred, _ = os.ReadFile(filepath.Join(dir, ".credentials.json"))
	}
	var auth struct {
		OAuth struct {
			Token string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(cred, &auth) == nil && auth.OAuth.Token != "" {
		if v, ok := claudeAPI(auth.OAuth.Token); ok {
			return v
		}
	}
	p := os.Getenv("CLAUDE_CONFIG_JSON")
	if p == "" {
		p = filepath.Join(home, ".claude.json")
	}
	paths := []string{p}
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		paths = append(paths, filepath.Join(d, ".claude.json"))
	}
	for _, p := range paths {
		if strings.HasPrefix(p, "~/") {
			p = filepath.Join(home, p[2:])
		}
		b, e := os.ReadFile(p)
		if e == nil {
			v := ParseClaude(b)
			if v.Five.Used != nil || v.Week.Used != nil {
				return v
			}
		}
	}
	return Provider{}
}
func codex() Provider {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "codex", "app-server", "--stdio")
	in, e := cmd.StdinPipe()
	if e != nil {
		return Provider{}
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		return Provider{}
	}
	if cmd.Start() != nil {
		return Provider{}
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	enc := json.NewEncoder(in)
	if enc.Encode(map[string]any{"method": "initialize", "id": 1, "params": map[string]any{"clientInfo": map[string]string{"name": "ai-quota", "title": "ai-quota", "version": "1"}}}) != nil {
		return Provider{}
	}
	s := bufio.NewScanner(out)
	for s.Scan() {
		var m struct {
			ID int `json:"id"`
		}
		if json.Unmarshal(s.Bytes(), &m) != nil {
			continue
		}
		if m.ID == 1 {
			enc.Encode(map[string]any{"method": "initialized", "params": map[string]any{}})
			enc.Encode(map[string]any{"method": "account/rateLimits/read", "id": 2, "params": map[string]any{}})
		}
		if m.ID == 2 {
			return ParseCodex(s.Bytes())
		}
	}
	return Provider{}
}

// apiCache is shared by every tagent process (sidebars, scraper,
// standalone…): the usage endpoint answers 429 when each polls on its own.
type apiCache struct {
	Tried, Fetched int64
	Body           json.RawMessage
}

// claudeAPI asks the OAuth usage endpoint at most every 5 minutes across
// processes and otherwise reuses the last answer (stale after 15 minutes).
func claudeAPI(token string) (Provider, bool) {
	dir, _ := os.UserCacheDir()
	path := filepath.Join(apppath.Dir(dir), "claude-usage.json")
	var c apiCache
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &c)
	}
	save := func() {
		os.MkdirAll(filepath.Dir(path), 0o700)
		if b, err := json.Marshal(c); err == nil {
			os.WriteFile(path, b, 0o600)
		}
	}
	if time.Since(time.Unix(c.Tried, 0)) >= 5*time.Minute {
		c.Tried = time.Now().Unix()
		save() // claim this round before the slow request
		client := &http.Client{Timeout: 5 * time.Second}
		req, _ := http.NewRequest("GET", "https://api.anthropic.com/api/oauth/usage", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("anthropic-beta", "oauth-2025-04-20")
		if resp, err := client.Do(req); err == nil {
			b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if _, perr := ParseClaudeOAuth(b); err == nil && resp.StatusCode == 200 && perr == nil {
				c.Fetched, c.Body = c.Tried, b
				save()
			}
		}
	}
	if len(c.Body) == 0 {
		return Provider{}, false
	}
	p, err := ParseClaudeOAuth(c.Body)
	if err != nil {
		return Provider{}, false
	}
	p.Stale = time.Since(time.Unix(c.Fetched, 0)) > 15*time.Minute
	return p, true
}

var mu sync.Mutex
var last time.Time
var cached Snapshot

// Get returns immediately, refreshing stale data in the background.
func Get() Snapshot {
	mu.Lock()
	defer mu.Unlock()
	if time.Since(last) >= time.Minute {
		last = time.Now()
		go func() { c := claude(); d := codex(); mu.Lock(); cached = Snapshot{c, d}; mu.Unlock() }()
	}
	return cached
}
func format(w Window) string {
	if w.Used == nil {
		return "n/a"
	}
	s := fmt.Sprintf("%.0f%%", *w.Used)
	var at time.Time
	var n int64
	var v string
	if json.Unmarshal(w.Reset, &n) == nil && n > 0 {
		at = time.Unix(n, 0)
	} else if json.Unmarshal(w.Reset, &v) == nil {
		at, _ = time.Parse(time.RFC3339, v)
	}
	if *w.Used > 80 && !at.IsZero() {
		s += " reset " + at.Local().Format("Jan 2 15:04")
		if at.Before(time.Now()) {
			s += " (stale)" // the snapshot predates this window's reset
		}
	}
	return s
}

// Lines is one row per tool: label, 5-hour window, weekly window.
func (s Snapshot) Lines() []string {
	return []string{"Claude " + func() string {
		if s.Claude.Stale {
			return "(stale) "
		}
		return ""
	}() + format(s.Claude.Five) + "·" + format(s.Claude.Week) + "  Codex " + format(s.Codex.Five) + "·" + format(s.Codex.Week)}
}
