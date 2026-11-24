package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"tagent/internal/board"
	"tagent/internal/hook"
	"tagent/internal/metrics"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func cmdTask(args []string, stdout, stderr io.Writer) int {
	wd, _ := os.Getwd()
	scope := tasks.Scope(wd)
	if err := taskBinding(scope, args); err != nil {
		fmt.Fprintln(stderr, "task:", err)
		return exitFailure
	}
	if withUsage := len(args) == 2 && args[0] == "ls" && args[1] == "--usage"; len(args) == 0 || (len(args) == 1 && args[0] == "ls") || withUsage {
		for _, t := range tasks.List(scope) {
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s", t.ID, t.Status, strings.Join(t.Assignees, ","), t.Text)
			if withUsage {
				u := ticketUsage(wd, scope, t)
				col := "—"
				if len(u.Jobs) > 0 {
					col = fmt.Sprint(u.Total.Total)
					if u.Partial {
						col = "≥" + col
					}
				}
				fmt.Fprint(stdout, "\t", col)
			}
			fmt.Fprintln(stdout)
		}
		return exitOK
	}
	usage := func() int {
		fmt.Fprintln(stderr, "usage: tagent task [ls [--usage]] | add [--to name[,name]] [--needs id[,id]] [--priority high|normal|low] [--workflow name [--input k=v]...] <text> | done <id> [note] | move <id> <status> [comment] | retry <id> | start <id> [workflow|pane] | set <id> <configured-status> [note] | assign <id> <name[,name]|-> | needs <id> <id[,id]|-> | priority <id> <high|normal|low> | show <id> | attachments <id> | comment <id> [--attach path]... [--diff] <text> | due <id> <YYYY-MM-DD|-> | rm <id>")
		return exitUsage
	}
	var err error
	switch args[0] {
	case "attachments":
		if len(args) != 2 {
			return usage()
		}
		var list []tasks.TicketAttachment
		list, err = tasks.AttachmentList(scope, wd, args[1])
		if err == nil {
			for _, a := range list {
				fmt.Fprintf(stdout, "%s\t%s/%s\t%s\t%d bytes\t%s\t%s\t%s\n", a.Ref, a.Source, a.Kind, a.Name, a.Size, a.Author, a.Time.Format("2006-01-02 15:04"), a.Error)
			}
		}
	case "move", "retry":
		retry := args[0] == "retry"
		if retry {
			if len(args) != 2 {
				return usage()
			}
			t, e := tasks.Get(scope, args[1])
			if e != nil {
				err = e
				break
			}
			args = append(args, t.Status)
		}
		if len(args) < 3 {
			return usage()
		}
		rest := args[3:]
		opt := tasks.MutationOptions{}
		for len(rest) > 0 && rest[0] == "--output" {
			if len(rest) < 2 {
				return usage()
			}
			k, v, ok := strings.Cut(rest[1], "=")
			if !ok || k == "" {
				return usage()
			}
			if opt.Outputs == nil {
				opt.Outputs = map[string]string{}
			}
			opt.Outputs[k] = v
			rest = rest[2:]
		}
		if len(rest) > 1 {
			return usage()
		}
		body := strings.Join(rest, "")
		if retry {
			err = tasks.Retry(scope, args[1], opt)
		} else {
			err = tasks.Move(scope, args[1], args[2], body, opt)
		}
		if err == nil {
			err = dispatchEntry(wd, scope, args[1], args[2])
		}
	case "show":
		brief := len(args) == 3 && args[2] == "--brief"
		if len(args) != 2 && !(len(args) == 3 && (brief || args[2] == "--all")) {
			return usage()
		}
		var t tasks.Task
		t, err = tasks.Get(scope, args[1])
		if err != nil {
			break
		}
		if brief {
			var text string
			text, err = briefView(scope, t)
			fmt.Fprint(stdout, text)
			break
		}
		events, e := tasks.ReadEvents(scope, 0)
		if e != nil {
			err = e
			break
		}
		var activity []tasks.Event
		for _, ev := range events {
			if ev.Task == t.ID && ev.Kind == "dispatch" {
				activity = append(activity, ev)
			}
		}
		fmt.Fprintf(stdout, "%s: %s\nStatus: %s\nAssignees: %s\nDue: %s\n\n%s\n", t.ID, t.Title, t.Status, strings.Join(t.Assignees, ","), t.Due, t.Text)
		if t.Workflow != "" {
			fmt.Fprintf(stdout, "Workflow: %s\nInputs: %v\nOutputs: %v\n", t.Workflow, t.Inputs, t.Outputs)
		}
		if mark := tasks.PriorityMark(t.Priority); mark != "" {
			fmt.Fprintf(stdout, "Priority: %s %s\n", mark, tasks.NormalizePriority(t.Priority))
		}
		if len(t.Needs) > 0 {
			fmt.Fprint(stdout, "Dependencies:")
			for _, id := range t.Needs {
				status := "missing"
				if dep, e := tasks.Get(scope, id); e == nil {
					status = dep.Status
				}
				fmt.Fprintf(stdout, " #%s (%s)", id, status)
			}
			if state := tasks.CardState(scope, t); state != "" {
				fmt.Fprintf(stdout, "\nReadiness: %s\n", state)
			} else {
				fmt.Fprintln(stdout)
			}
		}
		if dependents := tasks.Dependents(scope, t.ID); len(dependents) > 0 {
			fmt.Fprint(stdout, "Blocks:")
			for _, d := range dependents {
				fmt.Fprintf(stdout, " #%s (%s)", d.ID, d.Status)
			}
			fmt.Fprintln(stdout)
		}
		if t.Note != "" {
			fmt.Fprintln(stdout, "Note:", t.Note)
		}
		for _, c := range t.Comments {
			fmt.Fprintf(stdout, "\n[%s] %s (%s)\n%s\n", c.ID, c.Author, c.Created.Format("2006-01-02T15:04:05Z07:00"), c.Body)
			for _, a := range c.Attachments {
				fmt.Fprintf(stdout, "  %s: %s\n", a.Name, filepath.Join(tasks.BoardDir(scope), t.ID, a.File))
			}
		}
		for _, ev := range activity {
			fmt.Fprintf(stdout, "\nDispatch [%d]: %s\n", ev.Seq, ev.Message)
		}
		if lines := ticketUsage(wd, scope, t).Lines(func(n int64) string { return fmt.Sprint(n) }); len(lines) > 0 {
			fmt.Fprintln(stdout, "\n"+strings.Join(lines, "\n"))
		}
	case "done":
		if len(args) < 2 {
			return usage()
		}
		if len(args) > 2 && args[2] == "--output" {
			return usage()
		}
		_, err = tasks.Update(scope, args[1], "done", nil)
		if err == nil && len(args) > 2 {
			err = tasks.SetNote(scope, args[1], strings.Join(args[2:], " "))
		}
		if err == nil {
			err = metrics.End(scope, args[1])
			if t, e := tasks.Get(scope, args[1]); e == nil {
				hook.Notify("task done", t.Text)
			}
		}
	case "due":
		if len(args) != 3 {
			return usage()
		}
		due := args[2]
		if due == "-" {
			due = ""
		}
		err = tasks.SetDue(scope, args[1], due)
	case "comment":
		if len(args) < 3 {
			return usage()
		}
		rest := args[2:]
		var paths []string
		diff := false
		for len(rest) > 0 {
			if rest[0] == "--attach" {
				if len(rest) < 2 {
					return usage()
				}
				paths = append(paths, rest[1])
				rest = rest[2:]
			} else if rest[0] == "--diff" {
				diff = true
				rest = rest[1:]
			} else if rest[0] == "--" {
				rest = rest[1:]
				break
			} else {
				break
			}
		}
		if diff {
			var b []byte
			b, err = exec.Command("git", "diff", "HEAD").Output()
			if err != nil {
				break
			}
			var dir string
			dir, err = os.MkdirTemp("", "tagent-diff-*")
			if err != nil {
				break
			}
			defer os.RemoveAll(dir)
			path := filepath.Join(dir, "diff.patch")
			err = os.WriteFile(path, b, 0600)
			if err != nil {
				break
			}
			paths = append(paths, path)
		}
		err = tasks.Comment(scope, args[1], tasks.CallerActor(), strings.Join(rest, " "), paths)
		if notice, ok := err.(*tasks.CommentNotice); ok {
			fmt.Fprintln(os.Stderr, notice)
			err = nil
		}
	case "start":
		if len(args) == 2 {
			err = board.Start(wd, args[1], "")
			break
		}
		if len(args) != 3 {
			return usage()
		}
		task, getErr := tasks.Get(scope, args[1])
		if getErr != nil {
			err = getErr
			break
		}
		paneID := tmux.ResolvePane(args[2])
		var pane tmux.PaneInfo
		found := false
		for _, s := range tmux.QuerySessions() {
			for _, w := range s.Windows {
				for _, p := range w.Panes {
					if p.PaneID == paneID {
						pane, found = p, true
					}
				}
			}
		}
		if !found {
			err = board.Start(wd, args[1], args[2])
			break
		}
		if pane.Status != tmux.StatusIdle {
			err = fmt.Errorf("agent %q is %s; choose an idle agent", args[2], pane.Status)
			break
		}
		if err = metrics.Link(scope, task.ID, pane); err != nil {
			break
		}
		name := pane.Name
		if name == "" {
			name = string(pane.Agent)
		}
		assignees := append([]string(nil), task.Assignees...)
		present := false
		for _, a := range assignees {
			if a == name {
				present = true
			}
		}
		if !present {
			assignees = append(assignees, name)
		}
		if _, err = tasks.Update(scope, task.ID, "doing", assignees); err != nil {
			_ = metrics.End(scope, task.ID)
			break
		}
		tmux.TypeLine(paneID, "Task "+task.ID+": "+strings.ReplaceAll(task.Text, "\n", " ")+". Set its status to done only after verifying the result.")
	case "add":
		rest := args[1:]
		var names []string
		opt := tasks.MutationOptions{Actor: tasks.CallerActor()}
		for len(rest) > 0 && strings.HasPrefix(rest[0], "--") {
			if len(rest) < 2 {
				return usage()
			}
			switch rest[0] {
			case "--to":
				names = strings.Split(rest[1], ",")
			case "--workflow":
				opt.Workflow = rest[1]
			case "--input":
				k, v, ok := strings.Cut(rest[1], "=")
				if !ok || k == "" {
					return usage()
				}
				if opt.Inputs == nil {
					opt.Inputs = map[string]string{}
				}
				opt.Inputs[k] = v
			case "--needs":
				opt.Needs = strings.Split(rest[1], ",")
			case "--priority":
				opt.Priority = rest[1]
			default:
				return usage()
			}
			rest = rest[2:]
		}
		if len(rest) == 0 {
			return usage()
		}
		var task tasks.Task
		task, err = tasks.Add(scope, strings.Join(rest, " "), names, opt)
		if err == nil {
			fmt.Fprintln(stdout, task.ID)
		}
	case "set":
		if len(args) < 3 {
			return usage()
		}
		if args[2] == "doing" && os.Getenv("TMUX_PANE") != "" {
			paneID := os.Getenv("TMUX_PANE")
			for _, s := range tmux.QuerySessions() {
				for _, w := range s.Windows {
					for _, p := range w.Panes {
						if p.PaneID == paneID && p.SessionID != "" {
							err = metrics.Link(scope, args[1], p)
						}
					}
				}
			}
			if err != nil {
				break
			}
		}
		_, err = tasks.Update(scope, args[1], args[2], nil)
		if err == nil && len(args) > 3 { // optional note: what was done / why blocked
			err = tasks.SetNote(scope, args[1], strings.Join(args[3:], " "))
		}
		if err == nil && args[2] != "doing" {
			err = metrics.End(scope, args[1])
		}
		if err == nil && (args[2] == "done" || args[2] == "blocked") {
			if t, e := tasks.Get(scope, args[1]); e == nil {
				hook.Notify("task "+args[2], t.Text)
			}
		}
	case "needs":
		if len(args) != 3 {
			return usage()
		}
		needs := []string{}
		if args[2] != "-" {
			needs = strings.Split(args[2], ",")
		}
		err = tasks.SetNeeds(scope, args[1], needs)
	case "priority":
		if len(args) != 3 {
			return usage()
		}
		err = tasks.SetPriority(scope, args[1], args[2])
	case "assign":
		if len(args) != 3 {
			return usage()
		}
		names := []string{}
		if args[2] != "-" {
			names = strings.Split(args[2], ",")
		}
		_, err = tasks.Update(scope, args[1], "", names)
	case "rm":
		if len(args) != 2 {
			return usage()
		}
		err = tasks.Remove(scope, args[1])
	default:
		return usage()
	}
	if err != nil {
		fmt.Fprintln(stderr, "task:", err)
		return exitFailure
	}
	return exitOK
}

// taskBinding keeps a per-ticket chat pane on its ticket: the pane option
// @pane_ticket names one <scope>#<id>, and every command that targets a ticket
// must name that same ticket. Listing is read-only and stays allowed; adding a
// ticket is not — the chat changes its own ticket, not the board. This guards
// the CLI only: a shell can still unset TMUX_PANE or change directory.
// briefView is the compact agent context (`task show --brief`) and the MCP task_show result.
func briefView(scope string, t tasks.Task) (string, error) {
	events, err := tasks.ReadEvents(scope, 0)
	if err != nil {
		return "", err
	}
	out := tasks.Brief(t)
	var last *tasks.Event
	for i := range events {
		if events[i].Task == t.ID && events[i].Kind == "dispatch" {
			last = &events[i]
		}
	}
	if last != nil {
		msg := []rune(last.Message)
		if len(msg) > 400 {
			msg = append(msg[:400], '…')
		}
		out += fmt.Sprintf("Dispatch: %s\n", string(msg))
	}
	return out, nil
}

// dispatchEntry starts the board dispatcher when a ticket enters a status that
// has a role or run. Shared by `task move` and the MCP task_move tool.
func dispatchEntry(wd, scope, id, to string) error {
	cfg, err := tasks.LoadBoardConfig(wd)
	if err != nil {
		return err
	}
	ticket, err := tasks.Get(scope, id)
	if err != nil {
		return err
	}
	cfg, err = tasks.ConfigForTask(wd, cfg, ticket)
	if err != nil {
		return err
	}
	for _, s := range cfg.Statuses {
		if s.ID == to && (s.Role != "" || s.Run != "") {
			if err := board.Register(wd); err != nil {
				return fmt.Errorf("entry persisted, dispatcher startup failed (not confirmed dispatched): %w", err)
			}
			if err := ensureBoardScraper(); err != nil {
				return fmt.Errorf("entry persisted, dispatcher startup failed (not confirmed dispatched): %w", err)
			}
			return nil
		}
	}
	return nil
}

func taskBinding(scope string, args []string) error {
	boundScope, bound, ok := tasks.CallerTicket()
	if !ok {
		return nil
	}
	if boundScope != scope {
		return fmt.Errorf("this pane is bound to %s; refusing tasks from %s", boundScope, scope)
	}
	if len(args) == 0 || args[0] == "ls" {
		return nil
	}
	target := ""
	switch args[0] {
	case "attachments", "assign", "comment", "done", "due", "move", "needs", "priority", "retry", "rm", "set", "show", "start":
		if len(args) > 1 {
			target = args[1]
		}
	}
	if target == bound {
		return nil
	}
	if target == "" {
		return fmt.Errorf("this pane is bound to ticket %s; it may not run %q", bound, args[0])
	}
	return fmt.Errorf("this pane is bound to ticket %s; refusing ticket %s", bound, target)
}

// ticketUsage reads stored session usage and attributes it to one ticket.
func ticketUsage(wd, scope string, t tasks.Task) metrics.TicketUsage {
	cfg, _ := tasks.LoadBoardConfig(wd)
	if c, err := tasks.ConfigForTask(wd, cfg, t); err == nil {
		cfg = c
	}
	events, _ := tasks.ReadEvents(scope, 0)
	return metrics.TicketUsageOf(metrics.Read(scope), t, cfg, events, time.Now().UTC())
}
