package transcript

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrUnsupported = errors.New("transcript: harness not supported")
	ErrUnreadable  = errors.New("transcript: transcript unreadable")
	ErrTooLarge    = errors.New("transcript: too large for a strict read")
)

const (
	windowBytes   = 4 << 20   // tail of the file that is read
	windowRecords = 4096      // strict reads fail closed above this
	recordBytes   = 256 << 10 // strict reads fail closed above this
	pageItems     = 200
	maxText       = 32 << 10
	maxOutput     = 8 << 10
	maxInput      = 4 << 10
)

// Tool is one tool call (Name, Input) or its result (CallID, Output).
type Tool struct {
	Name   string `json:"name,omitempty"`
	Label  string `json:"label,omitempty"`
	CallID string `json:"callId,omitempty"`
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
}

// Item is one block of the conversation. Role: user, assistant, tool, thinking.
type Item struct {
	ID   string `json:"id"`
	Role string `json:"role"`
	Text string `json:"text,omitempty"`
	Tool *Tool  `json:"tool,omitempty"`
	Time string `json:"time,omitempty"`
}

// Page is a slice of the active branch. Cursor is the last item the caller now has.
type Page struct {
	Items  []Item `json:"items"`
	Cursor string `json:"cursor"`
	Reset  bool   `json:"reset"`
	More   bool   `json:"more"`
}

// record is one transcript line: its tree links and the items it carries.
type record struct {
	ID, Parent string
	Items      []Item
}

type parser func(line []byte, sessionID string) (record, bool)

func parserFor(harness string) (parser, error) {
	switch harness {
	case "claude":
		return parseClaude, nil
	case "pi":
		return parsePi, nil
	}
	return nil, ErrUnsupported
}

// Read returns one page of the session's active branch after cursor ("" = latest page).
func Read(ctx context.Context, harness, sessionID, after string) (Page, error) {
	items, err := branch(ctx, harness, sessionID, false)
	if err != nil {
		return Page{}, err
	}
	return page(items, after), nil
}

// Latest returns the newest assistant text on the active branch, stopping at the
// user's last message. Strict budgets: an oversized transcript fails closed.
func Latest(ctx context.Context, harness, sessionID string) (Item, error) {
	items, err := branch(ctx, harness, sessionID, true)
	if err != nil {
		return Item{}, err
	}
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Role == "user" {
			break
		}
		if items[i].Role == "assistant" && items[i].Text != "" {
			return items[i], nil
		}
	}
	return Item{}, nil
}

func page(items []Item, after string) Page {
	start, reset := max(len(items)-pageItems, 0), false
	if after != "" {
		if i := indexOf(items, after); i >= 0 {
			start = i + 1
		} else {
			reset = true
		}
	}
	end := min(start+pageItems, len(items))
	p := Page{Items: items[start:end], Reset: reset, More: !reset && end < len(items)}
	if p.Items == nil {
		p.Items = []Item{}
	}
	p.Cursor = after
	if reset {
		p.Cursor = ""
	}
	if n := len(p.Items); n > 0 {
		p.Cursor = p.Items[n-1].ID
	}
	return p
}

func indexOf(items []Item, id string) int {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].ID == id {
			return i
		}
	}
	return -1
}

type cached struct {
	size  int64
	mod   time.Time
	items []Item
}

var (
	cacheMu sync.Mutex
	cache   = map[string]cached{}
)

// branch returns the active branch of a session, oldest first. Parsed results are
// cached per file version so polls with no new bytes cost a stat.
func branch(ctx context.Context, harness, sessionID string, strict bool) ([]Item, error) {
	parse, err := parserFor(harness)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ErrUnreadable
	}
	path := Find(ctx, harness, sessionID)
	if path == "" {
		return nil, ErrUnreadable
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrUnreadable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrUnreadable
	}
	key := fmt.Sprint(path, strict)
	cacheMu.Lock()
	c, ok := cache[key]
	cacheMu.Unlock()
	if ok && c.size == info.Size() && c.mod.Equal(info.ModTime()) {
		return c.items, nil
	}
	items, err := parseWindow(ctx, f, info.Size(), harness, sessionID, parse, strict)
	if err != nil {
		return nil, err
	}
	cacheMu.Lock()
	if len(cache) > 64 {
		cache = map[string]cached{}
	}
	cache[key] = cached{info.Size(), info.ModTime(), items}
	cacheMu.Unlock()
	return items, nil
}

func parseWindow(ctx context.Context, f *os.File, size int64, harness, sessionID string, parse parser, strict bool) ([]Item, error) {
	if harness == "pi" && !piHeader(f, sessionID) {
		return nil, ErrUnreadable
	}
	start := max(size-windowBytes, 0)
	data, err := io.ReadAll(io.NewSectionReader(f, start, size-start))
	if err != nil {
		return nil, ErrUnreadable
	}
	lines := bytes.Split(data, []byte{'\n'})
	lines = lines[:len(lines)-1] // the last line is unterminated while the agent writes it
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // the window may begin inside a record
	}
	if strict && len(lines) > windowRecords {
		return nil, ErrTooLarge
	}
	recs := map[string]record{}
	leaf := ""
	for _, line := range lines {
		if ctx.Err() != nil {
			return nil, ErrUnreadable
		}
		if strict && len(line) > recordBytes {
			return nil, ErrTooLarge
		}
		if rec, ok := parse(line, sessionID); ok {
			recs[rec.ID] = rec
			leaf = rec.ID
		}
	}
	// Walk from the newest record to the root; abandoned branches are never visited.
	var chain [][]Item
	seen := map[string]bool{}
	for id := leaf; id != "" && !seen[id]; {
		seen[id] = true
		rec, ok := recs[id]
		if !ok {
			break
		}
		chain = append(chain, rec.Items)
		id = rec.Parent
	}
	var items []Item
	for i := len(chain) - 1; i >= 0; i-- {
		items = append(items, chain[i]...)
	}
	return items, nil
}

// piHeader checks that the file belongs to the session (its first line names it).
func piHeader(f *os.File, sessionID string) bool {
	line, _ := bufio.NewReaderSize(io.NewSectionReader(f, 0, 64<<10), 64<<10).ReadBytes('\n')
	var h struct{ Type, ID string }
	return json.Unmarshal(line, &h) == nil && h.Type == "session" && h.ID == sessionID
}

// newRecord numbers a record's items so each keeps a stable, append-only id.
func newRecord(id, parent, ts string, items []Item) record {
	for i := range items {
		items[i].ID = fmt.Sprintf("%s.%d", id, i)
		items[i].Time = ts
	}
	return record{ID: id, Parent: parent, Items: items}
}

// block is one content part; claude and pi differ only in field names.
type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Arguments json.RawMessage `json:"arguments"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// blocks reads message content: a plain string is one text block.
func blocks(raw json.RawMessage) []block {
	var bs []block
	if json.Unmarshal(raw, &bs) == nil {
		return bs
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []block{{Type: "text", Text: s}}
	}
	return nil
}

func toItems(role string, bs []block) []Item {
	var out []Item
	for _, b := range bs {
		switch b.Type {
		case "text", "output_text":
			if t := strings.TrimSpace(b.Text); t != "" && (role == "user" || role == "assistant") {
				out = append(out, Item{Role: role, Text: clip(t, maxText)})
			}
		case "thinking":
			if t := strings.TrimSpace(b.Thinking); t != "" {
				out = append(out, Item{Role: "thinking", Text: clip(t, maxText)})
			}
		case "tool_use", "toolCall":
			args := b.Input
			if len(args) == 0 {
				args = b.Arguments
			}
			out = append(out, Item{Role: "tool", Tool: &Tool{Name: b.Name, CallID: b.ID, Label: label(b.Name, args), Input: clip(string(args), maxInput)}})
		case "tool_result":
			out = append(out, Item{Role: "tool", Tool: &Tool{CallID: b.ToolUseID, Output: clip(resultText(b.Content), maxOutput)}})
		}
	}
	return out
}

// resultText flattens a tool result: a string, or text blocks.
func resultText(raw json.RawMessage) string {
	var parts []string
	for _, b := range blocks(raw) {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// label is the one-line summary shown on a tool card.
func label(name string, args json.RawMessage) string {
	var in struct {
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
		Command  string `json:"command"`
		Pattern  string `json:"pattern"`
	}
	json.Unmarshal(args, &in)
	switch strings.ToLower(name) {
	case "read", "edit", "write", "multiedit", "notebookedit":
		if f := firstNonEmpty(in.FilePath, in.Path); f != "" {
			return name + " " + filepath.Base(f)
		}
	case "bash":
		if c := strings.TrimSpace(strings.SplitN(in.Command, "\n", 2)[0]); c != "" {
			return name + " " + clip(c, 80)
		}
	case "grep", "glob":
		if in.Pattern != "" {
			return name + " " + clip(in.Pattern, 80)
		}
	}
	return name
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// clip truncates to n bytes without splitting a UTF-8 sequence.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
