package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"tagent/internal/board"
	"tagent/internal/queue"
	"tagent/internal/schedule"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func init() { schedule.Dispatch = scheduleDispatch }

// scheduleDispatch sends a scheduled prompt, unless the agent already has one
// queued. ponytail: a short interval against a busy agent would pile prompts up; one queued prompt is enough.
func scheduleDispatch(a board.Action) error {
	if a.LongLived && a.Event.Task == "" {
		if pane := findBoardPane(tmux.QuerySessions(), tasks.Scope(a.Dir), a, boardPaneKey(a)); pane != "" && len(queue.List(pane)) > 0 {
			return fmt.Errorf("%w: %s already has a queued prompt", schedule.ErrSkip, a.Agent)
		}
	}
	return dispatchBoardAction(a)
}

// cmdSchedule: `schedule ls [--json]`, `schedule run|pause|resume NAME`, `schedule trust [--remove]`.
func cmdSchedule(args []string, out, errs io.Writer) int {
	wd, _ := os.Getwd()
	root := tasks.MainRoot(wd)
	if len(args) == 0 {
		return exitUsage
	}
	switch args[0] {
	case "trust":
		remove := len(args) == 2 && args[1] == "--remove"
		if len(args) > 1 && !remove {
			return exitUsage
		}
		if err := schedule.Trust(root, !remove); err != nil {
			fmt.Fprintln(errs, err)
			return exitFailure
		}
		if !remove {
			if err := board.Register(root); err != nil {
				fmt.Fprintln(errs, err)
				return exitFailure
			}
		}
		fmt.Fprintf(out, "%s %s\n", root, map[bool]string{true: "trusted", false: "untrusted"}[!remove])
		return exitOK
	case "ls":
		asJSON := len(args) == 2 && args[1] == "--json"
		if len(args) > 1 && !asJSON {
			return exitUsage
		}
		return scheduleLs(root, asJSON, out, errs)
	case "run", "pause", "resume":
		if len(args) != 2 {
			return exitUsage
		}
		e, err := findSchedule(root, args[1])
		if err != nil {
			fmt.Fprintln(errs, err)
			return exitFailure
		}
		if e.Err != nil {
			fmt.Fprintln(errs, e.Err)
			return exitFailure
		}
		now := time.Now()
		if args[0] == "run" {
			if err = trustLocal(root, e); err != nil {
				fmt.Fprintln(errs, err)
				return exitFailure
			}
			ticket, err := schedule.RunNow(e, now, schedule.Fires)
			if err != nil {
				fmt.Fprintln(errs, err)
				return exitFailure
			}
			if ticket != "" {
				fmt.Fprintln(out, ticket)
			}
			return scheduleScraper(errs)
		}
		on := args[0] == "resume"
		if on {
			if err = trustLocal(root, e); err != nil {
				fmt.Fprintln(errs, err)
				return exitFailure
			}
		}
		if err = schedule.Toggle(e, on, now); err != nil {
			fmt.Fprintln(errs, err)
			return exitFailure
		}
		fmt.Fprintf(out, "%s %s\n", args[1], map[bool]string{true: "resumed", false: "paused"}[on])
		if on {
			return scheduleScraper(errs)
		}
		return exitOK
	}
	return exitUsage
}

// trustLocal is the explicit consent of a run or resume from the repo: it trusts
// the repo's local schedules and registers it on the board. Global entries grant nothing.
func trustLocal(root string, e schedule.Entry) error {
	if e.Global {
		return nil
	}
	if err := schedule.Trust(root, true); err != nil {
		return err
	}
	return board.Register(root)
}

func scheduleScraper(errs io.Writer) int {
	if err := ensureBoardScraper(); err != nil {
		fmt.Fprintln(errs, err)
		return exitFailure
	}
	return exitOK
}

// scheduleSources are the files the current repo sees: its local file, then the global one.
func scheduleSources(root string) [][2]string {
	return [][2]string{{schedule.LocalFile(root), root}, {schedule.GlobalFile(), ""}}
}

func findSchedule(root, name string) (schedule.Entry, error) {
	for _, src := range scheduleSources(root) {
		entries, err := schedule.Load(src[0], src[1])
		if err != nil {
			return schedule.Entry{}, err
		}
		for _, e := range entries {
			if e.Name == name {
				return e, nil
			}
		}
	}
	return schedule.Entry{}, fmt.Errorf("no schedule named %s", name)
}

type scheduleRow struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	Rule    string `json:"rule"`
	Next    string `json:"next,omitempty"`
	Last    string `json:"last,omitempty"`
	Ticket  string `json:"ticket,omitempty"`
	Note    string `json:"note,omitempty"`
	Enabled bool   `json:"enabled"`
}

const stamp = "2006-01-02 15:04"

func scheduleLs(root string, asJSON bool, out, errs io.Writer) int {
	now := time.Now()
	trusted := slices.Contains(schedule.Trusted(), root)
	rows := []scheduleRow{}
	for i, src := range scheduleSources(root) {
		scope := "local"
		if i == 1 {
			scope = "global"
		}
		entries, err := schedule.Load(src[0], src[1])
		if err != nil {
			fmt.Fprintln(errs, "schedule:", err)
			continue
		}
		for _, e := range entries {
			st := schedule.StateOf(e)
			r := scheduleRow{Name: e.Name, Scope: scope, Rule: e.Rule(), Ticket: st.Ticket, Note: st.Note, Enabled: e.Enabled}
			if !st.Last.IsZero() {
				r.Last = st.Last.Format(stamp)
			}
			switch {
			case e.Err != nil:
				r.Note, r.Rule = e.Err.Error(), "invalid"
			case !e.Enabled || (!e.Global && !trusted):
			case st.Last.IsZero():
				r.Next = e.Next(now).Format(stamp) // first check arms it: its first run comes one slot on
			case e.Next(st.Last).After(now):
				r.Next = e.Next(st.Last).Format(stamp)
			default:
				r.Next = "due"
			}
			if r.Next == "" {
				r.Next = "-"
			}
			rows = append(rows, r)
		}
	}
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(rows)
		return exitOK
	}
	for _, r := range rows {
		last := r.Last
		if last == "" {
			last = "-"
		}
		target := "-"
		if r.Ticket != "" {
			target = "#" + r.Ticket
		}
		status := map[bool]string{true: "enabled", false: "paused"}[r.Enabled]
		switch {
		case r.Rule == "invalid":
			status = "invalid"
		case r.Scope == "local" && !trusted:
			status = "untrusted (run tagent schedule trust)"
		}
		fmt.Fprintf(out, "%s\t%s\t%s\tnext %s\tlast %s\t→ %s\t%s\t%s\n", r.Name, r.Scope, r.Rule, r.Next, last, target, status, r.Note)
	}
	return exitOK
}
