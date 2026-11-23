// Package report is the structured hand-off from a worker agent to its lead:
// one JSON file, awaited with OS file events instead of scraping panes.
package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrTimeout = errors.New("report: timed out")

type Report struct {
	Contract string         `json:"contract"`
	Role     string         `json:"role"`
	Status   string         `json:"status"`
	Files    []string       `json:"files,omitempty"`
	Checks   map[string]any `json:"checks,omitempty"`
	Findings []string       `json:"findings,omitempty"`
	Question *string        `json:"question"`
	Outputs  map[string]any `json:"outputs,omitempty"` // workflow job outputs
}

var statuses = map[string]bool{"DONE": true, "BLOCKED": true, "PASS": true, "CHANGES_REQUESTED": true}

func Parse(b []byte) (Report, error) {
	var r Report
	if err := json.Unmarshal(b, &r); err != nil {
		return Report{}, fmt.Errorf("report: %w", err)
	}
	if r.Contract == "" || r.Role == "" || r.Status == "" {
		return Report{}, errors.New("report: contract, role and status are required")
	}
	if !statuses[r.Status] {
		return Report{}, fmt.Errorf("report: invalid status %q", r.Status)
	}
	return r, nil
}

func Summary(r Report) string {
	keys := make([]string, 0, len(r.Checks))
	for k := range r.Checks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	checks := make([]string, len(keys))
	for i, k := range keys {
		checks[i] = fmt.Sprintf("%s=%v", k, r.Checks[k])
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s files=%d checks=%s findings=%d", r.Contract, r.Role, r.Status, len(r.Files), strings.Join(checks, ","), len(r.Findings))
	if r.Question != nil && *r.Question != "" {
		fmt.Fprintf(&b, " question=%s", *r.Question)
	}
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "\n- %s", f)
	}
	return b.String()
}

func Instruction(path string) string {
	return "When finished (or blocked), write exactly one JSON report to " + path + ".tmp, then rename it to " + path + " (atomic). " +
		`Schema: {"contract":str,"role":"builder|reviewer|planner","status":"DONE|BLOCKED|PASS|CHANGES_REQUESTED","files":[str],"checks":{name:number|str},"findings":[str],"question":str|null}. ` +
		"Keep findings to at most 5 one-line strings. Do not print the report anywhere else."
}

// Wait blocks on OS file events (no polling) until path holds a valid report.
func Wait(path string, timeout time.Duration) (Report, error) {
	deadline := time.Now().Add(timeout)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Report{}, err
	}
	w, err := newWatcher(dir)
	if err != nil {
		return Report{}, err
	}
	defer w.close()
	for {
		// Re-arm the file watch before reading so a write between the
		// read and the block still raises an event.
		w.watchFile(path)
		if b, err := os.ReadFile(path); err == nil {
			if r, err := Parse(b); err == nil {
				return r, nil
			}
		}
		left := time.Until(deadline)
		if left <= 0 {
			return Report{}, ErrTimeout
		}
		if err := w.wait(left); err != nil {
			return Report{}, err
		}
	}
}
