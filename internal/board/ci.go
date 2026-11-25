package board

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"tagent/internal/tasks"
)

// A run job's result is a "ci" comment whose first line is "CI exit code N".
var (
	ciResult   = regexp.MustCompile(`(?m)^CI exit code (-?\d+)`)
	ciFailLine = regexp.MustCompile(`--- FAIL: (\S+)`)
	// Timings, paths and line numbers change between identical failures.
	volatile  = regexp.MustCompile(`\(?\d+(\.\d+)?s\)?|(/[\w.\-]+)+|:\d+`)
	skipWords = regexp.MustCompile(`(?i)\bskip (test|ci)\b`)
)

// ciLogNote points the agent at the full CI log attached to a failed run.
func ciLogNote(dir string, t tasks.Task, c tasks.TaskComment) string {
	for _, att := range c.Attachments {
		if att.Name == "ci.log" {
			return "Full CI log: " + filepath.Join(tasks.BoardDir(tasks.Scope(dir)), t.ID, att.File) + "; read only the failing part.\n"
		}
	}
	return ""
}

func isCIResult(c tasks.TaskComment) bool { return c.Author == "ci" && ciResult.MatchString(c.Body) }

func isCIFailure(c tasks.TaskComment) bool {
	m := ciResult.FindStringSubmatch(c.Body)
	return m != nil && m[1] != "0"
}

// NormalizeCI is the output of a run with its volatile parts removed, so two
// runs that fail the same way compare equal.
func NormalizeCI(body string) string { return volatile.ReplaceAllString(body, "") }

// FailedTests returns the top-level Go test names that failed in the output.
func FailedTests(output string) []string {
	var out []string
	for _, m := range ciFailLine.FindAllStringSubmatch(output, -1) {
		name, _, _ := strings.Cut(m[1], "/")
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// PreExisting reports whether every failing test also fails on the base commit.
func PreExisting(failing, onBase []string) bool {
	if len(failing) == 0 {
		return false
	}
	for _, f := range failing {
		if !slices.Contains(onBase, f) {
			return false
		}
	}
	return true
}

// repeatedFailure reports whether the CI result c is a failure identical to the
// CI failure before it on the same ticket.
func repeatedFailure(t tasks.Task, c tasks.TaskComment) bool {
	if !isCIFailure(c) {
		return false
	}
	prev := -1
	for i, x := range t.Comments {
		if x.ID == c.ID {
			break
		}
		if isCIResult(x) {
			prev = i
		}
	}
	return prev >= 0 && isCIFailure(t.Comments[prev]) && NormalizeCI(t.Comments[prev].Body) == NormalizeCI(c.Body)
}

// userSkip reports whether a user comment since the last run result asks to skip
// the run job. Agent comments never count. Any ci comment ends it: the run's
// result, or the "CI skipped" note a skipped run writes.
func userSkip(t tasks.Task) bool {
	for i := len(t.Comments) - 1; i >= 0; i-- {
		c := t.Comments[i]
		if c.Author == "ci" {
			return false
		}
		if c.Author == "user" && skipWords.MatchString(c.Body) {
			return true
		}
	}
	return false
}
