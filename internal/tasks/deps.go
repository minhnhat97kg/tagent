package tasks

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Priorities are the three ordering ranks a ticket can have. Normal is the
// default and is stored as no value, so old tickets read as normal.
var Priorities = []string{"high", "normal", "low"}

// NormalizePriority maps empty or mixed case input to one of Priorities.
func NormalizePriority(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	if p == "" {
		return "normal"
	}
	return p
}

// ValidatePriority rejects anything but the three ranks.
func ValidatePriority(p string) error {
	switch NormalizePriority(p) {
	case "high", "normal", "low":
		return nil
	}
	return fmt.Errorf("priority must be high, normal or low")
}

// PriorityRank orders lanes and the dispatch queue: high first, then normal,
// then low.
func PriorityRank(p string) int {
	switch NormalizePriority(p) {
	case "high":
		return 0
	case "low":
		return 2
	}
	return 1
}

// PriorityMark is the one-cell card marker: ↑ high, ↓ low, nothing for normal.
func PriorityMark(p string) string {
	switch NormalizePriority(p) {
	case "high":
		return "↑"
	case "low":
		return "↓"
	}
	return ""
}

// NextPriority cycles normal → high → low → normal, the order the ticket key uses.
func NextPriority(p string) string {
	switch NormalizePriority(p) {
	case "normal":
		return "high"
	case "high":
		return "low"
	}
	return "normal"
}

// SetPriority changes only the priority. Normal clears the field.
func SetPriority(scope, id, priority string) error {
	if err := ValidatePriority(priority); err != nil {
		return err
	}
	p := NormalizePriority(priority)
	if p == "normal" {
		p = ""
	}
	return withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		if t.Priority == p {
			return nil
		}
		t.Priority, t.Updated = p, time.Now()
		return write(scope, t)
	})
}

// ValidateNeeds checks that every dependency exists, no id repeats and no
// dependency reaches back to id. Self is empty for a ticket that does not
// exist yet (task add), where a cycle is impossible.
func ValidateNeeds(scope, id string, needs []string) error {
	seen := map[string]bool{}
	for _, dep := range needs {
		if dep == "" {
			return fmt.Errorf("empty dependency")
		}
		if !validID(dep) {
			return fmt.Errorf("invalid dependency %q", dep)
		}
		if dep == id {
			return fmt.Errorf("ticket cannot depend on itself")
		}
		if seen[dep] {
			return fmt.Errorf("duplicate dependency %s", dep)
		}
		if _, err := Get(scope, dep); err != nil {
			return fmt.Errorf("task reference %s: unknown ticket", dep)
		}
		seen[dep] = true
	}
	if len(seen) == 0 || id == "" {
		return nil
	}
	byID := map[string]Task{}
	for _, t := range List(scope) {
		byID[t.ID] = t
	}
	var reaches func(from string, path map[string]bool) bool
	reaches = func(from string, path map[string]bool) bool {
		if from == id {
			return true
		}
		if path[from] {
			return false
		}
		path[from] = true
		for _, next := range byID[from].Needs {
			if reaches(next, path) {
				return true
			}
		}
		return false
	}
	for dep := range seen {
		if reaches(dep, map[string]bool{}) {
			return fmt.Errorf("dependency cycle: #%s waits for #%s", dep, id)
		}
	}
	return nil
}

// SetNeeds replaces the ticket's dependencies. An empty list clears them.
func SetNeeds(scope, id string, needs []string) error {
	cleaned := make([]string, 0, len(needs))
	for _, dep := range needs {
		if dep = strings.TrimSpace(dep); dep != "" {
			cleaned = append(cleaned, dep)
		}
	}
	if err := ValidateNeeds(scope, id, cleaned); err != nil {
		return err
	}
	return withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		if len(t.Needs) == 0 && len(cleaned) == 0 || slices.Equal(t.Needs, cleaned) {
			return nil
		}
		t.Needs, t.Updated = cleaned, time.Now()
		return write(scope, t)
	})
}

// OpenNeeds lists the dependencies that are not done yet. A dependency that
// was deleted no longer blocks anything.
func OpenNeeds(scope string, t Task) []string {
	var open []string
	for _, dep := range t.Needs {
		d, err := Get(scope, dep)
		if err == nil && d.Status != "done" {
			open = append(open, dep)
		}
	}
	return open
}

// WaitingReason is why a ticket may not leave todo, or "" when nothing blocks
// it: "waiting for #12, #15".
func WaitingReason(scope string, t Task) string {
	open := OpenNeeds(scope, t)
	ids := make([]string, len(open))
	for i, id := range open {
		ids[i] = "#" + id
	}
	if len(ids) == 0 {
		return ""
	}
	return "waiting for " + strings.Join(ids, ", ")
}

// CardState is the dependency badge a todo card shows: "waits #12, #15",
// "ready" once every dependency is done, or "" without dependencies.
func CardState(scope string, t Task) string {
	if len(t.Needs) == 0 || t.Status != "todo" {
		return ""
	}
	if reason := WaitingReason(scope, t); reason != "" {
		return strings.Replace(reason, "waiting for", "waits", 1)
	}
	return "ready"
}

// Dependents lists tickets that wait for id, in ticket (creation) order.
func Dependents(scope, id string) []Task {
	var out []Task
	for _, t := range List(scope) {
		if slices.Contains(t.Needs, id) {
			out = append(out, t)
		}
	}
	return out
}
