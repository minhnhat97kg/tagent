package usage

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AssistantReply distinguishes unavailable storage from a known turn with no text.
// Cursor identifies assistant content, not file size (tool records cannot advance it).
type AssistantReply struct {
	Text, Cursor string
	Available    bool
}

func reply(text, id string) AssistantReply {
	return AssistantReply{text, fmt.Sprintf("%x", sha256.Sum256([]byte(id+"\x00"+text))), true}
}
func LastAssistantText(harness, sessionID string) string {
	return ReadAssistant(harness, sessionID).Text
}

// ReadAssistant returns text on the active conversation branch only, never tools
// or reasoning. A readable transcript without an answer must not trigger screen fallback.
func ReadAssistant(harness, sessionID string) AssistantReply {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	return readAssistant(ctx, harness, sessionID)
}

func readAssistant(ctx context.Context, harness, sessionID string) AssistantReply {
	if ctx.Err() != nil {
		return AssistantReply{Available: true}
	}
	if sessionID == "" || strings.ContainsAny(sessionID, "/\\") {
		return AssistantReply{}
	}
	if harness == "opencode" {
		db := openCodeDB()
		if _, err := os.Stat(db); err != nil {
			return AssistantReply{}
		}
		id := strings.ReplaceAll(sessionID, "'", "''")
		sql := `select json_object('id',m.id,'text',coalesce((select group_concat(json_extract(data,'$.text'),char(10)) from (select data from part where message_id=m.id and json_extract(data,'$.type')='text' order by time_created,id)),'')) from message m where session_id='` + id + `' and json_extract(data,'$.role')='assistant' and time_created>coalesce((select max(time_created) from message where session_id=m.session_id and json_extract(data,'$.role')='user'),0) order by time_created desc,id desc limit 1`
		out, err := exec.CommandContext(ctx, "sqlite3", "-readonly", "-noheader", db, "select substr(("+sql+"),1,65536)").Output()
		if err != nil {
			return AssistantReply{Available: ctx.Err() != nil}
		}
		var row struct{ ID, Text string }
		if len(strings.TrimSpace(string(out))) == 0 {
			return AssistantReply{Available: true}
		}
		if json.Unmarshal(out, &row) != nil {
			return AssistantReply{Available: true}
		}
		return reply(row.Text, row.ID)
	}
	root, suffix := "", ""
	switch harness {
	case "claude":
		root, suffix = Root(), string(filepath.Separator)+sessionID+".jsonl"
	case "codex":
		root, suffix = codexRoot(), sessionID+".jsonl"
	case "pi":
		home, _ := os.UserHomeDir()
		root, suffix = filepath.Join(home, ".pi", "agent", "sessions"), "_"+sessionID+".jsonl"
	default:
		return AssistantReply{}
	}
	path, exhausted := findSession(ctx, root, suffix)
	if exhausted {
		return AssistantReply{Available: true}
	}
	f, err := os.Open(path)
	if err != nil {
		return AssistantReply{}
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return AssistantReply{Available: true}
	}
	// Validate identity from the start independently of the bounded recent tail.
	if harness == "pi" {
		header := bufio.NewScanner(io.NewSectionReader(f, 0, 64<<10))
		header.Buffer(make([]byte, 4096), 64<<10)
		var row struct{ Type, ID string }
		if !header.Scan() || json.Unmarshal(header.Bytes(), &row) != nil || row.Type != "session" || row.ID != sessionID {
			return AssistantReply{Available: true}
		}
	}
	start := int64(0)
	if info.Size() > replyBytes {
		start = info.Size() - replyBytes
	}
	type entry struct {
		Type, ID, UUID, SessionID string
		ParentID                  *string
		ParentUUID                *string
		Message                   json.RawMessage
		Payload                   json.RawMessage
	}
	entries := map[string]entry{}
	leaf := ""
	header := true
	latest := AssistantReply{Available: true}
	sc := bufio.NewScanner(io.NewSectionReader(f, start, info.Size()-start))
	sc.Buffer(make([]byte, 64<<10), 256<<10)
	// Preserve CR bytes so Codex cursors use stable absolute record offsets.
	sc.Split(func(data []byte, eof bool) (int, []byte, error) {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			return i + 1, data[:i], nil
		}
		if eof && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	position := start
	if start > 0 && sc.Scan() {
		position += int64(len(sc.Bytes()) + 1)
	}
	line := 0
	for sc.Scan() {
		line++
		if line > replyRecords || ctx.Err() != nil {
			return AssistantReply{Available: true}
		}
		recordPosition := position
		position += int64(len(sc.Bytes()) + 1)
		var row entry
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if harness == "pi" && row.Type == "session" {
			if row.ID != sessionID {
				return AssistantReply{Available: true}
			}
			header = true
			continue
		}
		if harness == "claude" && row.SessionID != "" && row.SessionID != sessionID {
			return AssistantReply{Available: true}
		}
		if harness == "codex" {
			if row.Type != "response_item" {
				continue
			}
			role, text, id := assistantContent(row.Payload)
			if role == "user" {
				latest = AssistantReply{Available: true}
			} else if role == "assistant" && text != "" {
				latest = reply(text, fmt.Sprintf("%d:%s", recordPosition, id))
			}
			continue
		}
		id := row.ID
		if harness == "claude" {
			id = row.UUID
			row.ParentID = row.ParentUUID
		}
		if id != "" {
			entries[id] = row
			leaf = id
		}
	}
	if sc.Err() != nil || !header || ctx.Err() != nil {
		return AssistantReply{Available: true}
	}
	if harness == "codex" {
		return latest
	}
	visited := map[string]bool{}
	for leaf != "" && !visited[leaf] {
		if ctx.Err() != nil {
			return AssistantReply{Available: true}
		}
		visited[leaf] = true
		row, ok := entries[leaf]
		if !ok {
			break
		}
		role, text, id := assistantContent(row.Message)
		if role == "assistant" && text != "" {
			return reply(text, leaf+id)
		}
		if role == "user" {
			break
		}
		if row.ParentID == nil {
			break
		}
		leaf = *row.ParentID
	}
	return AssistantReply{Available: true}
}

func assistantContent(raw json.RawMessage) (role, text, id string) {
	var msg struct {
		Role, ID string
		Content  json.RawMessage
	}
	if json.Unmarshal(raw, &msg) != nil {
		return
	}
	role, id = msg.Role, msg.ID
	var plain string
	if json.Unmarshal(msg.Content, &plain) == nil {
		return role, plain, id
	}
	var parts []struct{ Type, Text string }
	json.Unmarshal(msg.Content, &parts)
	if role == "user" && len(parts) > 0 {
		onlyTools := true
		for _, p := range parts {
			onlyTools = onlyTools && p.Type == "tool_result"
		}
		if onlyTools {
			role = "toolResult"
		}
	}
	var texts []string
	for _, p := range parts {
		if p.Type == "text" || p.Type == "output_text" {
			texts = append(texts, p.Text)
		}
	}
	return role, strings.TrimSpace(strings.Join(texts, "\n")), id
}
