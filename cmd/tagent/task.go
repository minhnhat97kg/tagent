package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"tagent/internal/board"
	"tagent/internal/hook"
	"tagent/internal/metrics"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func cmdTask(args []string, stdout, stderr io.Writer) int {
	wd, _ := os.Getwd()
	scope := tasks.Scope(wd)
	if len(args) == 0 || (len(args) == 1 && args[0] == "ls") {
		for _, t := range tasks.List(scope) {
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", t.ID, t.Status, strings.Join(t.Assignees, ","), t.Text)
		}
		return exitOK
	}
	usage := func() int {
		fmt.Fprintln(stderr, "usage: tagent task [ls] | add [--to name[,name]] [--needs id[,id]] <text> | done <id> [note] | move <id> <status> [comment] | retry <id> | start <id> <pane> | set <id> <configured-status> [note] | assign <id> <name[,name]|-> | show <id> | comment <id> [--attach path]... [--diff] <text> | due <id> <YYYY-MM-DD|-> | rm <id>")
		return exitUsage
	}
	var err error
	switch args[0] {
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
			cfg, e := tasks.LoadBoardConfig(wd)
			if e != nil {
				err = e
				break
			}
			ticket, e := tasks.Get(scope, args[1])
			if e != nil {
				err = e
				break
			}
			cfg, e = tasks.ConfigForTask(wd, cfg, ticket)
			if e != nil {
				err = e
				break
			}
			for _, s := range cfg.Statuses {
				if s.ID == args[2] && (s.Role != "" || s.Run != "") {
					if err = board.Register(wd); err == nil {
						err = ensureBoardScraper()
					}
					if err != nil {
						err = fmt.Errorf("entry persisted, dispatcher startup failed (not confirmed dispatched): %w", err)
					}
					break
				}
			}
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
		if brief {
			fmt.Fprint(stdout, tasks.Brief(t))
			if len(activity) > 0 {
				msg := []rune(activity[len(activity)-1].Message)
				if len(msg) > 400 {
					msg = append(msg[:400], '…')
				}
				fmt.Fprintf(stdout, "Dispatch: %s\n", string(msg))
			}
			break
		}
		fmt.Fprintf(stdout, "%s: %s\nStatus: %s\nAssignees: %s\nDue: %s\n\n%s\n", t.ID, t.Title, t.Status, strings.Join(t.Assignees, ","), t.Due, t.Text)
		if t.Workflow != "" {
			fmt.Fprintf(stdout, "Workflow: %s\nInputs: %v\nOutputs: %v\n", t.Workflow, t.Inputs, t.Outputs)
		}
		fmt.Fprintf(stdout, "Blocked by: %s (informational)\n", strings.Join(t.Needs, ","))
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
		for _, arg := range args[1:] {
			if arg == "-i" {
				return cmdTaskIntake(args[1:], os.Stdin, stdout, stderr)
			}
		}
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
