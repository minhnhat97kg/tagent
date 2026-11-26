package web

import (
	"fmt"
	"net/http"
	"os"

	"tagent/internal/tasks"
	"tagent/internal/workflow"
	"tagent/internal/workplace"
)

// workplaceView is where a ticket's workflow keeps its work, as the UI shows it.
type workplaceView struct {
	Type     string `json:"type,omitempty"`
	Cleanup  string `json:"cleanup,omitempty"`
	Location string `json:"location,omitempty"`
	State    string `json:"state,omitempty"`
	Error    string `json:"error,omitempty"`
}

// workplaceKind names a resolved workplace and whether its cleanup keeps the output.
func workplaceKind(wp workplace.Workplace) (string, bool) {
	switch w := wp.(type) {
	case workplace.None:
		return "none", false
	case workplace.Folder:
		return "folder", w.Keep
	case workplace.Browser:
		return "browser", w.Keep
	case workplace.Remote:
		return "remote", true
	case workplace.Docker:
		return "docker", w.Keep
	case workplace.Git:
		return "git-worktree", w.Keep
	}
	return "", false
}

// cleanupLabel is what done does with the output: remote hosts are never cleaned.
func cleanupLabel(kind string, keep bool) string {
	switch {
	case kind == "none":
		return ""
	case kind == "remote" || keep:
		return "keep"
	}
	return "remove"
}

// workplaceState reports whether the output exists, from disk and the ticket's status.
func workplaceState(exists bool, status string) string {
	done := status == "done" || status == "closed"
	switch {
	case exists && done:
		return "kept"
	case exists:
		return "active"
	case status == "done":
		return "removed"
	}
	return "not created"
}

// workflowWorkplace is a recipe's workplace type and cleanup, as TicketWorkplace resolves them.
func workflowWorkplace(f workflow.Workflow) (string, string) {
	kind := f.Workplace
	if kind == "" {
		kind = "git-worktree"
	}
	return kind, cleanupLabel(kind, f.Cleanup == "keep")
}

// workplace answers on demand: one recipe lookup and one stat, no git or docker calls.
func (s *server) workplace(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	scope := tasks.Scope(dir)
	t, err := tasks.Get(scope, r.PathValue("id"))
	if err != nil {
		http.Error(w, "ticket not found", http.StatusNotFound)
		return
	}
	if t.Workflow == "" {
		http.Error(w, "ticket has no workflow", http.StatusNotFound)
		return
	}
	wp, err := tasks.TicketWorkplace(scope, t)
	if err != nil {
		writeJSON(w, workplaceView{Error: err.Error()})
		return
	}
	kind, keep := workplaceKind(wp)
	if kind == "" {
		writeJSON(w, workplaceView{Error: fmt.Sprintf("unknown workplace %T", wp)})
		return
	}
	out := workplaceView{Type: kind, Cleanup: cleanupLabel(kind, keep), Location: wp.Output(tasks.MainRoot(dir), t.ID)}
	if kind != "none" && kind != "remote" {
		if _, err := os.Stat(out.Location); err != nil && !os.IsNotExist(err) {
			out.Error = err.Error()
		} else {
			out.State = workplaceState(err == nil, t.Status)
		}
	}
	writeJSON(w, out)
}
