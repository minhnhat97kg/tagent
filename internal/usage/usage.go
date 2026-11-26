// Package usage reads token usage of Claude Code sessions from their
// transcripts (~/.claude/projects/*/<session>.jsonl): every assistant
// message carries a usage block. A message is logged on several lines (one
// per content block), so lines are deduplicated by message id, keeping the
// last — the final count.
package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"tagent/internal/transcript"
	"time"
)

// longPrompt is the prompt size above which some models charge a higher rate.
const longPrompt = 100_000

// Tokens is a session's usage.
type Tokens struct {
	In, Out, CacheRead, CacheWrite int64
	CacheWrite1h                   int64 // the part of CacheWrite kept for an hour, billed higher
	// Long is the part of the fields above from requests whose prompt was over longPrompt.
	Long Split
}

// Split is a token count by billed rate.
type Split struct{ In, Out, CacheRead, CacheWrite, CacheWrite1h int64 }

// Add sums b into s.
func (s *Split) Add(b Split) {
	s.In += b.In
	s.Out += b.Out
	s.CacheRead += b.CacheRead
	s.CacheWrite += b.CacheWrite
	s.CacheWrite1h += b.CacheWrite1h
}

// Add sums b into t.
func (t *Tokens) Add(b Tokens) {
	t.In += b.In
	t.Out += b.Out
	t.CacheRead += b.CacheRead
	t.CacheWrite += b.CacheWrite
	t.CacheWrite1h += b.CacheWrite1h
	t.Long.Add(b.Long)
}

// Short renders "12.3k in · 4.1k out · 1.2M cached".
func (t Tokens) Short() string {
	return fmt.Sprintf("%s in · %s out · %s cached", human(t.In+t.CacheWrite), human(t.Out), human(t.CacheRead))
}

func human(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

// Root is where Claude Code keeps transcripts.
func Root() string { return transcript.ClaudeRoot() }

type cached struct {
	mod  time.Time
	size int64
	t    Tokens
}

var (
	mu    sync.Mutex
	cache = map[string]cached{}
)

// ForSession returns a Claude session's usage; ok=false when its
// transcript isn't found. Re-reads only when the file changed.
func ForSession(sessionID string) (Tokens, bool) {
	if sessionID == "" {
		return Tokens{}, false
	}
	matches, _ := filepath.Glob(filepath.Join(Root(), "*", sessionID+".jsonl"))
	if len(matches) == 0 {
		return Tokens{}, false
	}
	path := matches[0]
	st, err := os.Stat(path)
	if err != nil {
		return Tokens{}, false
	}
	mu.Lock()
	c, hit := cache[path]
	mu.Unlock()
	if hit && c.mod.Equal(st.ModTime()) && c.size == st.Size() {
		return c.t, true
	}
	t, err := parse(path)
	if err != nil {
		return Tokens{}, false
	}
	mu.Lock()
	cache[path] = cached{st.ModTime(), st.Size(), t}
	mu.Unlock()
	return t, true
}

func parse(path string) (Tokens, error) {
	f, err := os.Open(path)
	if err != nil {
		return Tokens{}, err
	}
	defer f.Close()
	type usage struct {
		In         int64 `json:"input_tokens"`
		Out        int64 `json:"output_tokens"`
		CacheRead  int64 `json:"cache_read_input_tokens"`
		CacheWrite int64 `json:"cache_creation_input_tokens"`
		Creation   struct {
			Hour int64 `json:"ephemeral_1h_input_tokens"`
		} `json:"cache_creation"`
	}
	var line struct {
		Type    string `json:"type"`
		Message struct {
			ID    string `json:"id"`
			Usage *usage `json:"usage"`
		} `json:"message"`
	}
	byID := map[string]usage{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20) // transcript lines can be huge
	for sc.Scan() {
		line.Type, line.Message.ID, line.Message.Usage = "", "", nil
		if json.Unmarshal(sc.Bytes(), &line) != nil || line.Type != "assistant" || line.Message.Usage == nil {
			continue
		}
		byID[line.Message.ID] = *line.Message.Usage
	}
	var t Tokens
	for _, u := range byID {
		one := Tokens{In: u.In, Out: u.Out, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, CacheWrite1h: u.Creation.Hour}
		// Each request is priced on its own prompt size, cache reads and writes included.
		if u.In+u.CacheRead+u.CacheWrite > longPrompt {
			one.Long = Split{u.In, u.Out, u.CacheRead, u.CacheWrite, u.Creation.Hour}
		}
		t.Add(one)
	}
	return t, sc.Err()
}
