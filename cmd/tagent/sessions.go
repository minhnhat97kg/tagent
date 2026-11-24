package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"tagent/internal/tmux"
)

const ownerOption = "@tagent_owner"

// tagSession marks the session and pane as owned by a repo, so `sessions gc`
// knows which sessions tagent made.
func tagSession(pane, owner string) {
	if owner == "" {
		return
	}
	tmux.SetPaneOption(pane, ownerOption, owner)
	if s, ok := tmux.PaneSessionName(pane); ok {
		tmux.RunTmux("set", "-t", s, ownerOption, owner)
	}
}

type sessionPane struct {
	Agent, Ticket string
	Status        tmux.PaneStatus
	Window        string
	Cmd           string
}

type sessionInfo struct {
	Name, Owner string
	Created     time.Time
	Panes       []sessionPane
}

func (s sessionInfo) agents() (n int) {
	for _, p := range s.Panes {
		if p.Agent != "" {
			n++
		}
	}
	return
}

func isShell(cmd string) bool {
	switch strings.TrimPrefix(cmd, "-") {
	case "sh", "bash", "zsh", "fish":
		return true
	}
	return false
}

func (s sessionInfo) busy() bool {
	for _, p := range s.Panes {
		if p.Status == tmux.StatusRunning || p.Status == tmux.StatusWaiting || p.Agent == "" && p.Cmd != "" && !isShell(p.Cmd) {
			return true // a running agent, or a foreground command in a plain pane
		}
	}
	return false
}

// gcSelect picks the sessions gc may kill: managed (has an owner), not busy, and
// either its owner path is gone or it holds no agent pane. Unmanaged sessions are
// never selected.
func gcSelect(sessions []sessionInfo, exists func(string) bool) (kill []sessionInfo) {
	for _, s := range sessions {
		if s.Owner == "" || s.busy() {
			continue
		}
		if !exists(s.Owner) || s.agents() == 0 {
			kill = append(kill, s)
		}
	}
	return
}

func listSessions() ([]sessionInfo, error) {
	out, err := tmux.RunTmuxCapture("list-panes", "-a", "-F", strings.Join([]string{
		"#{session_name}", "#{session_created}", "#{" + ownerOption + "}", "#{window_id}",
		"#{q:" + tmux.PaneAgent + "}", "#{q:" + tmux.PaneStatusOpt + "}", "#{q:" + tmux.PaneTicket + "}", "#{pane_current_command}"}, "|"))
	if err != nil {
		return nil, err
	}
	byName := map[string]*sessionInfo{}
	var order []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "|")
		if len(f) != 8 {
			continue
		}
		s := byName[f[0]]
		if s == nil {
			sec, _ := strconv.ParseInt(f[1], 10, 64)
			s = &sessionInfo{Name: f[0], Owner: f[2], Created: time.Unix(sec, 0)}
			byName[f[0]] = s
			order = append(order, f[0])
		}
		s.Panes = append(s.Panes, sessionPane{Window: f[3], Agent: f[4], Status: tmux.ParsePaneStatus(f[5]), Ticket: f[6], Cmd: f[7]})
	}
	var res []sessionInfo
	for _, n := range order {
		res = append(res, *byName[n])
	}
	return res, nil
}

func sessionRow(s sessionInfo, now time.Time) []string {
	windows, tickets := map[string]bool{}, map[string]bool{}
	for _, p := range s.Panes {
		windows[p.Window] = true
		if _, id, ok := strings.Cut(p.Ticket, "#"); ok {
			tickets["#"+id] = true
		}
	}
	var ts []string
	for t := range tickets {
		ts = append(ts, t)
	}
	sort.Strings(ts)
	owner, status := s.Owner, "ok"
	if owner == "" {
		owner, status = "-", "not managed"
	} else if _, err := os.Stat(owner); err != nil {
		status = "orphan"
	} else if s.agents() == 0 {
		status = "empty"
	} else if s.busy() {
		status = "active"
	}
	age := now.Sub(s.Created).Round(time.Minute).String()
	return []string{s.Name, owner, strconv.Itoa(len(windows)), strconv.Itoa(s.agents()), strings.Join(ts, ","), age, status}
}

func cmdSessions(args []string, stdout, stderr io.Writer) int {
	usage := func() int {
		fmt.Fprintln(stderr, "usage: tagent sessions ls | tagent sessions gc [--dry-run]")
		return exitUsage
	}
	if len(args) == 0 {
		return usage()
	}
	sessions, err := listSessions()
	if err != nil {
		fmt.Fprintln(stderr, "sessions:", err)
		return exitFailure
	}
	switch {
	case args[0] == "ls" && len(args) == 1:
		w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SESSION\tOWNER\tWINDOWS\tAGENTS\tTICKET\tAGE\tSTATUS")
		for _, s := range sessions {
			fmt.Fprintln(w, strings.Join(sessionRow(s, time.Now()), "\t"))
		}
		w.Flush()
	case args[0] == "gc" && (len(args) == 1 || len(args) == 2 && args[1] == "--dry-run"):
		dry := len(args) == 2
		exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
		for _, s := range gcSelect(sessions, exists) {
			verb := "would remove"
			if !dry {
				verb = "removed"
				if _, ok := tmux.RunTmux("kill-session", "-t", "="+s.Name); !ok {
					verb = "failed to remove"
				}
			}
			fmt.Fprintf(stdout, "%s %s (%s)\n", verb, s.Name, s.Owner)
		}
	default:
		return usage()
	}
	return exitOK
}
