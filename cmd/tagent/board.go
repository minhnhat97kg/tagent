package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"tagent/internal/board"
	"tagent/internal/queue"
	"tagent/internal/roles"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
)

func cmdBoard(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: tagent board run|init|validate|migrate [--dir DIR] [--dry-run]")
		return exitUsage
	}
	wd, _ := os.Getwd()
	fs := flag.NewFlagSet("board", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", wd, "repository directory")
	dryRun := fs.Bool("dry-run", false, "migrate: print the mapping without changing tickets")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return exitUsage
	}
	var err error
	switch args[0] {
	case "init":
		path := tasks.BoardConfigPath(*dir)
		err = os.MkdirAll(filepath.Dir(path), 0700)
		if err == nil {
			var f *os.File
			f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err == nil {
				_, err = f.WriteString(tasks.BoardExample)
				ce := f.Close()
				if err == nil {
					err = ce
				}
				if err == nil {
					fmt.Fprintln(stdout, path)
					err = ensureStarterWorkflow(*dir)
				}
			}
		}
	case "validate":
		var cfg tasks.BoardConfig
		cfg, err = tasks.LoadBoardConfig(*dir)
		if err == nil {
			for _, warning := range cfg.Warnings {
				fmt.Fprintln(stdout, "warning:", warning)
			}
			fmt.Fprintln(stdout, "Board configuration valid.")
		}
	case "migrate":
		var migrations []board.Migration
		migrations, err = board.Migrate(*dir, *dryRun)
		for _, m := range migrations {
			switch {
			case m.Skipped != "":
				fmt.Fprintf(stdout, "%s: left in %s: %s\n", m.ID, m.From, m.Skipped)
			case *dryRun:
				fmt.Fprintf(stdout, "%s: %s -> %s/%s (%s)\n", m.ID, m.From, m.Workflow, m.To, m.Job)
			default:
				fmt.Fprintf(stdout, "%s: %s -> %s/%s (%s)\n", m.ID, m.From, m.Workflow, m.To, m.Job)
			}
		}
	case "run":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err = board.Run(ctx, *dir, dispatchBoardAction)
	default:
		return exitUsage
	}
	if err != nil {
		fmt.Fprintln(stderr, "board:", err)
		return exitFailure
	}
	return exitOK
}

// ensureStarterWorkflow gives `board init`'s default_workflow a recipe to run.
func ensureStarterWorkflow(dir string) error {
	root := tasks.MainRoot(dir)
	if _, err := workflow.Find(root, "feature"); err == nil {
		return nil
	}
	path := filepath.Join(workflow.LocalDir(root), "feature.yml")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(workflow.Template("feature")), 0600)
}

var harnesses = map[string]bool{"claude": true, "codex": true, "pi": true, "opencode": true, "gemini": true, "copilot": true, "amp": true}

func dispatchBoardAction(a board.Action) error {
	scope := tasks.Scope(a.Dir)
	if a.Close {
		panes, err := tmux.BoardPanes()
		if err != nil {
			return err
		}
		for _, p := range panes {
			_, ticket, ok := strings.Cut(p.BoardTarget, "#")
			if ok && ticket == a.Event.Task && tasks.Scope(p.Path) == scope {
				if _, err = tmux.RunTmuxCapture("kill-pane", "-t", p.PaneID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	priority := 1
	if a.Event.Task != "" {
		ticket, err := tasks.Get(scope, a.Event.Task)
		if err != nil {
			return err
		}
		priority = tasks.PriorityRank(ticket.Priority)
		if ticket.Workflow != "" {
			a.ExecDir, err = workflowWorktree(a.Dir, ticket.ID)
			if err != nil {
				return err
			}
		}
	}
	if a.Run != "" {
		// The command is what the user sees for this entry; no pane is involved.
		if a.Event.Task != "" {
			prompt := a.Run
			if a.Skip {
				prompt = "skipped by user comment: " + a.Run
			}
			_ = tasks.RecordDispatch(scope, a.Event.Task, tasks.Event{To: a.Event.To, Role: "ci", Prompt: tasks.CapText(prompt, 8000)})
		}
		go func() {
			if err := runStatus(a); err != nil {
				if a.OnError != nil {
					a.OnError(err)
				}
				fmt.Fprintln(os.Stderr, "board run:", err)
			}
		}()
		return nil
	}
	// A dedicated target option keeps suffixed names reusable across restarts.
	// One pane per (role, task), refreshed at the destination entry threshold.
	key := a.Agent + "#" + a.Event.Task
	pane := ""
	for _, s := range tmux.QuerySessions() {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.Agent == "" || tasks.Scope(p.Path) != scope || (a.ExecDir != "" && p.Path != a.ExecDir) {
					continue
				}
				if p.BoardTarget == key {
					pane = p.PaneID
					break
				}
			}
		}
	}
	entries := 0
	if pane != "" && a.ResumePrompt != nil {
		count, err := strconv.Atoi(tmux.GetPaneOptionValue(pane, "@agent_role_entries"))
		entries = count
		if err != nil || entries < 0 {
			entries = 0
		}
		if a.FreshAfter > 0 && entries+1 >= a.FreshAfter {
			if _, err := tmux.RunTmuxCapture("kill-pane", "-t", pane); err != nil {
				return err
			}
			queue.Clear(pane)
			pane, entries = "", 0
		}
	}
	if pane != "" && a.ResumePrompt != nil {
		if count, err := strconv.Atoi(tmux.GetPaneOptionValue(pane, "@agent_comment_cursor")); err == nil {
			a.Prompt = a.ResumePrompt(count)
		}
	}
	if pane == "" {
		name := a.Agent
		if t := a.Event.Task; t != "" {
			name += "-" + t[max(0, len(t)-4):]
		}
		o := spawnOpts{Name: freeName(name), Cwd: a.Dir, ConfigDir: tasks.MainRoot(a.Dir), Wait: true}
		if a.Role != "" && a.Event.Task != "" {
			o.Window = a.Role + "#" + a.Event.Task
		}
		if a.ExecDir != "" {
			o.Cwd = a.ExecDir
		}
		target := a.Role
		if target == "" {
			target = a.Agent
		}
		o.Model = a.Model
		o.Effort = a.Effort
		if harnesses[target] {
			o.Agent = target
		} else {
			o.Role = target
		}
		// Instructions are app-owned, so only a legacy native file is loaded by
		// the harness itself; every other role gets them in this prompt.
		mainRoot := tasks.MainRoot(a.Dir)
		harness := target
		if !harnesses[harness] {
			harness = roleHarness(mainRoot, target)
		}
		// Only a role started as o.Role can have its file handed to the harness
		// (--agent, developer_instructions); a board entry starts o.Agent directly.
		if text := roleInstructions(mainRoot, a.Agent, harness, !harnesses[target]); text != "" {
			a.Prompt = withRoleInstructions(a.Agent, text, a.Prompt)
		}
		var warn string
		var err error
		pane, warn, err = spawnAgent(o)
		if err != nil {
			return err
		}
		if warn != "" {
			fmt.Fprintln(os.Stderr, "board:", warn)
		}
		tmux.SetPaneOption(pane, tmux.BoardTarget, key)
	}
	// Record what was actually sent; the run view reads these fields. Best effort:
	// the audit line below still records delivery.
	if a.Event.Task != "" {
		harness := tmux.GetPaneOptionValue(pane, tmux.PaneAgent)
		if harness == "" && harnesses[a.Role] {
			harness = a.Role
		}
		_ = tasks.RecordDispatch(scope, a.Event.Task, tasks.Event{To: a.Event.To, Role: a.Agent, Harness: harness, Model: a.Model, Effort: a.Effort, Pane: pane, PaneName: tmux.GetPaneOptionValue(pane, tmux.PaneName), Prompt: tasks.CapText(a.Prompt, 8000)})
	}
	// cmdSend handles active/approval states. Unknown startup states must queue too.
	status := tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt)
	if status != string(tmux.StatusIdle) {
		if err := ensureBoardScraper(); err != nil {
			return err
		}
		_, _, err := queue.PushRank(pane, a.Prompt, priority)
		if err == nil && a.ResumePrompt != nil {
			tmux.SetPaneOption(pane, "@agent_comment_cursor", strconv.Itoa(a.CommentCount))
			tmux.SetPaneOption(pane, "@agent_role_entries", strconv.Itoa(entries+1))
		}
		return err
	}
	// System send: no sender, so the prompt is never signed, even when this process inherited TMUX_PANE.
	if code := sendPrompt([]string{pane, a.Prompt}, "", io.Discard, os.Stderr); code != exitOK {
		return fmt.Errorf("send to %s failed", pane)
	}
	if a.ResumePrompt != nil {
		tmux.SetPaneOption(pane, "@agent_comment_cursor", strconv.Itoa(a.CommentCount))
		tmux.SetPaneOption(pane, "@agent_role_entries", strconv.Itoa(entries+1))
	}
	return nil
}

// roleHarness resolves the harness a role runs on, mirroring spawnAgent: the
// board.yaml agent, then the global profile, then the process harness.
func roleHarness(dir, role string) string {
	if cfg, err := tasks.LoadBoardConfig(dir); err == nil {
		if r, ok := cfg.Roles[role]; ok && r.Agent != "" {
			return roles.Harness(r.Agent)
		}
	}
	if p := roles.ProfileFor(role); p.Harness != "" {
		return roles.Harness(p.Harness)
	}
	h, _ := tmux.GetOption(tmux.AgentOption)
	return roles.Harness(h)
}

// roleInstructions is the instructions to put in a dispatch prompt. It is empty
// when the role has none, or when nativeLoad says spawn hands that very legacy
// file to the harness itself (a native definition must not be sent twice).
func roleInstructions(dir, role, harness string, nativeLoad bool) string {
	def, ok := roles.GetIn(dir, role)
	if !ok || strings.TrimSpace(def.Instructions) == "" {
		return ""
	}
	if nativeLoad && def.Native && def.Harness == roles.Harness(harness) {
		return ""
	}
	return strings.TrimSpace(def.Instructions)
}

func withRoleInstructions(role, instructions, prompt string) string {
	return "Your role is " + role + ". Follow these instructions:\n\n" + instructions + "\n\n" + prompt
}

// Reuse the existing singleton scraper and its queue consumer.
func ensureBoardScraper() error {
	if v, _ := tmux.GetOption("@agent_scrape"); v == "off" {
		return nil
	}
	v, _ := tmux.GetOption("@agent_scrape_pid")
	pid, _ := strconv.Atoi(v)
	if pid > 0 && syscall.Kill(pid, 0) == nil {
		return nil
	}
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, "scrape")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
