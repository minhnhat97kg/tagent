package board

import (
	"regexp"

	"tagent/internal/tasks"
)

// A ticket that already names files or functions needs no scout. ponytail: a
// regex heuristic; it can false-positive (e.g. "Node.js") or miss a name, and a
// wrong call costs one cheap scout or one skipped scout.
var (
	scoutFile = regexp.MustCompile(`[\w./-]+\.(go|ts|tsx|js|jsx|py|rs|sh|css)\b`)
	scoutFunc = regexp.MustCompile(`\b\w+\(\)`)
)

// scoutSkip returns why the scout job can be skipped, or "" when it must run.
func scoutSkip(t tasks.Task, job string) string {
	if len(t.Outputs[job]) > 0 {
		return "a scout output already exists"
	}
	text := t.Title + "\n" + t.Text
	for _, v := range t.Inputs {
		text += "\n" + v
	}
	if scoutFile.MatchString(text) || scoutFunc.MatchString(text) {
		return "the ticket names files or functions"
	}
	return ""
}
