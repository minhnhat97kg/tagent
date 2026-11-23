package usage

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"tagent/internal/transcript"
	"time"
)

// Count is the provider-reported cumulative usage of one native session.
// Total follows the provider's total where available; cache and reasoning
// are details, not extra tokens to add to Total.
type Count struct {
	Input, Output, CacheRead, CacheWrite, Reasoning, Total int64
}

func Read(harness, sessionID string) (Count, bool) {
	if sessionID == "" {
		return Count{}, false
	}
	switch harness {
	case "claude":
		t, ok := ForSession(sessionID)
		return Count{Input: t.In, Output: t.Out, CacheRead: t.CacheRead, CacheWrite: t.CacheWrite, Total: t.In + t.Out + t.CacheRead + t.CacheWrite}, ok
	case "pi":
		return readPi(sessionID)
	case "codex":
		return readCodex(sessionID)
	case "opencode":
		return readOpenCode(sessionID)
	}
	return Count{}, false
}

// readPi sums assistant usage across the session, including abandoned branches.
func readPi(sessionID string) (Count, bool) {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".pi", "agent")
	if d := os.Getenv("PI_CODING_AGENT_DIR"); d != "" {
		root = d
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	path, _ := transcript.Locate(ctx, filepath.Join(root, "sessions"), "_"+sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return Count{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var header struct{ Type, ID string }
	if !sc.Scan() || json.Unmarshal(sc.Bytes(), &header) != nil || header.Type != "session" || header.ID != sessionID {
		return Count{}, false
	}
	var total Count
	found := false
	for sc.Scan() {
		var row struct {
			Type    string
			Message struct {
				Role  string
				Usage *struct{ Input, Output, CacheRead, CacheWrite, TotalTokens int64 }
			}
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil || row.Type != "message" || row.Message.Role != "assistant" || row.Message.Usage == nil {
			continue
		}
		u := row.Message.Usage
		total.Input += u.Input
		total.Output += u.Output
		total.CacheRead += u.CacheRead
		total.CacheWrite += u.CacheWrite
		total.Total += u.TotalTokens
		found = true
	}
	return total, found && sc.Err() == nil
}

func codexSessionPath(sessionID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	return transcript.Find(ctx, "codex", sessionID)
}

func readCodex(sessionID string) (Count, bool) {
	f, err := os.Open(codexSessionPath(sessionID))
	if err != nil {
		return Count{}, false
	}
	defer f.Close()
	var latest Count
	found := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var row struct {
			Type    string `json:"type"`
			Payload struct {
				SessionID string `json:"session_id"`
				Thread    struct {
					Input      int64 `json:"input_tokens"`
					Output     int64 `json:"output_tokens"`
					CacheRead  int64 `json:"cached_input_tokens"`
					CacheWrite int64 `json:"cache_write_input_tokens"`
					Reasoning  int64 `json:"reasoning_output_tokens"`
					Total      int64 `json:"total_tokens"`
				} `json:"thread_token_usage"`
			} `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil || row.Type != "token_usage_record" || (row.Payload.SessionID != "" && row.Payload.SessionID != sessionID) {
			continue
		}
		u := row.Payload.Thread
		if u.Total >= latest.Total {
			latest = Count{u.Input, u.Output, u.CacheRead, u.CacheWrite, u.Reasoning, u.Total}
			found = true
		}
	}
	return latest, found && sc.Err() == nil
}

func openCodeDB() string {
	home, _ := os.UserHomeDir()
	db := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		db = filepath.Join(d, "opencode", "opencode.db")
	}
	return db
}

func readOpenCode(sessionID string) (Count, bool) {
	db := openCodeDB()
	if _, err := os.Stat(db); err != nil {
		return Count{}, false
	}
	id := strings.ReplaceAll(sessionID, "'", "''")
	sql := "select tokens_input,tokens_output,tokens_reasoning,tokens_cache_read,tokens_cache_write from session where id='" + id + "' limit 1"
	out, err := exec.Command("sqlite3", "-readonly", "-csv", db, sql).Output()
	if err != nil {
		return Count{}, false
	}
	fields, err := csv.NewReader(strings.NewReader(strings.TrimSpace(string(out)))).Read()
	if err != nil || len(fields) != 5 {
		return Count{}, false
	}
	var values [5]int64
	for i, field := range fields {
		if field == "" {
			continue
		}
		values[i], err = strconv.ParseInt(field, 10, 64)
		if err != nil {
			return Count{}, false
		}
	}
	in, outTok, reason, read, write := values[0], values[1], values[2], values[3], values[4]
	if in < 0 || outTok < 0 || reason < 0 || read < 0 || write < 0 {
		return Count{}, false
	}
	return Count{Input: in, Output: outTok, Reasoning: reason, CacheRead: read, CacheWrite: write, Total: in + outTok}, true
}
