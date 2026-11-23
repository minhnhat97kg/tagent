package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"tagent/internal/queue"
	"tagent/internal/tmux"
)

// ─── fanout / compare: best-of-N ─────────────────────────────────────────

// cmdFanout runs the same prompt in N agents, each in its own worktree, so
// you can compare the attempts and merge the best one.
func cmdFanout(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fanout", flag.ContinueOnError)
	fs.SetOutput(stderr)
	n := fs.Int("n", 3, "number of attempts")
	base := fs.String("name", "try", "name prefix: <name>-1, <name>-2, …")
	role := fs.String("role", "", "role for every attempt")
	agent := fs.String("agent", "", "agent command")
	wait := fs.Bool("wait", false, "wait for all, then print the comparison")
	timeout := fs.Duration("timeout", 30*time.Minute, "with --wait: give up after")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	prompt := strings.Join(fs.Args(), " ")
	if prompt == "" || *n < 1 {
		fmt.Fprintln(stderr, `usage: tagent fanout [-n 3] [--name try] [--role r] [--wait] "prompt"`)
		return exitUsage
	}
	var panes []string
	for i := 1; i <= *n; i++ {
		name := fmt.Sprintf("%s-%d", *base, i)
		pane, warn, err := spawnAgent(spawnOpts{Name: name, Agent: *agent, Role: *role, Worktree: true, Wait: true})
		if err != nil {
			fmt.Fprintf(stderr, "fanout: %s: %v\n", name, err)
			return exitFailure
		}
		if warn != "" {
			fmt.Fprintf(stderr, "fanout: %s\n", warn)
		}
		panes = append(panes, pane)
		fmt.Fprintf(stdout, "%s → %s\n", name, pane)
	}
	for _, p := range panes {
		sendText(p, prompt)
	}
	if !*wait {
		fmt.Fprintf(stdout, "running; compare with: tagent compare %s\n", *base)
		return exitOK
	}
	deadline := time.Now().Add(*timeout)
	for _, p := range panes {
		if st := waitStopped(p, time.Until(deadline)); st == "" {
			fmt.Fprintf(stderr, "fanout: timed out waiting for %s\n", p)
			return exitTimeout
		}
	}
	return cmdCompare([]string{*base}, stdout, stderr)
}

// cmdCompare lists the attempts <base>-* side by side: status and what each
// changed, with the commands to keep one.
func cmdCompare(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: tagent compare <name-prefix>")
		return exitUsage
	}
	wd, _ := os.Getwd()
	repo, _, _, err := worktreePaths(wd, "x")
	if err != nil {
		fmt.Fprintln(stderr, "compare:", err)
		return exitFailure
	}
	dirs, _ := filepath.Glob(filepath.Join(repo, ".worktrees", args[0]+"-*"))
	if len(dirs) == 0 {
		fmt.Fprintf(stderr, "compare: no worktrees %s-* in %s\n", args[0], repo)
		return exitFailure
	}
	head, _ := git(repo, "rev-parse", "HEAD")
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ATTEMPT\tSTATUS\tCHANGES")
	for _, d := range dirs {
		name := filepath.Base(d)
		status := "closed"
		if pane := tmux.ResolvePane(name); pane != name {
			status = displayStatus(tmux.ParsePaneStatus(tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt)), tmux.GetPaneOptionValue(pane, tmux.PaneUnseen) == "1")
		}
		git(d, "add", "-N", ".")
		stat, _ := git(d, "diff", "--shortstat", head)
		if stat == "" {
			stat = "no changes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", name, status, strings.TrimSpace(stat))
	}
	w.Flush()
	fmt.Fprintf(stdout, "\nreview:  tagent diff <attempt>\nkeep:    tagent merge <attempt> --discard\ndrop:    tagent discard <attempt>\n")
	return exitOK
}

// waitStopped waits until pane has run and stopped (idle/done/error/
// waiting); "" on timeout or when the pane is gone.
func waitStopped(pane string, timeout time.Duration) tmux.PaneStatus {
	start := time.Now()
	sawActive := false
	for time.Since(start) < timeout {
		if _, alive := tmux.PaneSessionName(pane); !alive {
			return ""
		}
		st := tmux.ParsePaneStatus(tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt))
		sawActive = sawActive || st == tmux.StatusRunning || st == tmux.StatusBackground
		if waitReached("stopped", st) && (sawActive || time.Since(start) > waitGrace) {
			return st
		}
		time.Sleep(waitPoll)
	}
	return ""
}

// ─── ask: hand a task to whoever is free ─────────────────────────────────

// cmdAsk gives a task to an idle agent working in the same repo (with the
// role, if --role), or starts a new one when nobody is free.
func cmdAsk(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	fs.SetOutput(stderr)
	role := fs.String("role", "", "only agents with this role (a new one gets it too)")
	agent := fs.String("agent", "", "agent command for a new agent")
	wait := fs.Bool("wait", false, "wait until it's done")
	timeout := fs.Duration("timeout", 30*time.Minute, "with --wait: give up after")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	task := strings.Join(fs.Args(), " ")
	if task == "" {
		fmt.Fprintln(stderr, `usage: tagent ask [--role r] [--wait] "task"`)
		return exitUsage
	}
	wd, _ := os.Getwd()
	root := wd
	if r, err := git(wd, "rev-parse", "--show-toplevel"); err == nil {
		root = r
	}
	pane, name := pickIdle(tmux.QuerySessions(), root, *role)
	if pane == "" {
		base := *role
		if base == "" {
			base = "helper"
		}
		var warn string
		var err error
		name = freeName(base)
		if pane, warn, err = spawnAgent(spawnOpts{Name: name, Role: *role, Agent: *agent, Cwd: wd, Wait: true}); err != nil {
			fmt.Fprintln(stderr, "ask:", err)
			return exitFailure
		}
		if warn != "" {
			fmt.Fprintln(stderr, "ask:", warn)
		}
		fmt.Fprintf(stdout, "nobody free — started %s (%s)\n", name, pane)
	} else {
		fmt.Fprintf(stdout, "→ %s (%s)\n", name, pane)
	}
	tmux.TypeLine(pane, task)
	if *wait {
		return cmdWait([]string{pane, "--timeout", timeout.String()}, stdout, stderr)
	}
	return exitOK
}

// pickIdle finds an idle agent under root (its cwd in the repo), with role
// if given and nothing queued for it. Returns its pane and display name.
func pickIdle(sessions []tmux.SessionInfo, root, role string) (pane, name string) {
	for _, s := range sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.Status != tmux.StatusIdle || !(p.Path == root || strings.HasPrefix(p.Path, root+string(os.PathSeparator))) {
					continue
				}
				if role != "" && tmux.GetPaneOptionValue(p.PaneID, roleOption) != role {
					continue
				}
				if len(queue.List(p.PaneID)) > 0 || p.PaneID == selfPane() {
					continue
				}
				name = p.Name
				if name == "" {
					name = p.PaneID
				}
				return p.PaneID, name
			}
		}
	}
	return "", ""
}
