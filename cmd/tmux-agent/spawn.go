package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tmux-agent/internal/checkpoint"
	"tmux-agent/internal/hook"
	"tmux-agent/internal/roles"
	"tmux-agent/internal/roster"
	"tmux-agent/internal/setup"
	"tmux-agent/internal/sidebar"
	"tmux-agent/internal/tasks"
	"tmux-agent/internal/tmux"
)

// spawnOpts is everything `new` (and fanout/plan/restore, which build on
// it) can ask for.
type spawnOpts struct {
	Name, Agent, Model, Effort, Cwd, Role string
	ConfigDir                             string
	Worktree                              bool
	Args                                  []string // extra agent arguments
	Wait                                  bool     // wait until the agent reports a status
}

// Pane options recording how an agent was started (restore, merge).
const (
	roleOption   = "@agent_role"
	wtRepoOption = "@agent_wt_repo"
)

// spawnAgent starts an agent in a pool window and returns its pane id, plus
// a warning worth showing (startup prompt, no status yet).
func spawnAgent(o spawnOpts) (pane, warn string, err error) {
	if o.Cwd == "" {
		o.Cwd, _ = os.Getwd()
	}

	if o.ConfigDir == "" {
		o.ConfigDir = o.Cwd
	}
	if o.Role != "" {
		profile := roles.ProfileFor(o.Role)
		if o.Agent == "" {
			o.Agent = profile.Harness
		}
		if o.Model == "" {
			o.Model = profile.Model
		}
	}
	if o.Agent == "" {
		o.Agent, _ = tmux.GetOption(tmux.AgentOption)
		if o.Agent == "" {
			o.Agent = "claude"
		}
	}
	effortArgs, err := nativeEffortArgs(roles.Harness(o.Agent), o.Effort)
	if err != nil {
		return "", "", err
	}
	var role roles.Definition
	if o.Role != "" {
		var ok bool
		if role, ok = roles.GetIn(o.ConfigDir, o.Agent, o.Role); !ok {
			return "", "", fmt.Errorf("unknown %s repo agent %q (have: %s)", roles.Harness(o.Agent), o.Role, strings.Join(roles.NamesIn(o.ConfigDir, o.Agent), ", "))
		}
		if o.Name == "" {
			o.Name = freeName(strings.ReplaceAll(o.Role, "/", "-"))
		}
	}
	if o.Name != "" && (strings.ContainsAny(o.Name, " \t|/") || tmux.ResolvePane(o.Name) != o.Name) {
		return "", "", fmt.Errorf("name %q is invalid or already used", o.Name)
	}
	if o.Model == "" {
		o.Model, _ = tmux.GetOption("@agent_model")
	}
	// Outside a pane (a script, a key binding), fall back to the window the
	// current client is looking at.
	target := selfPane()
	if target == "" && os.Getenv("TMUX") != "" {
		out, _ := tmux.RunTmux("display-message", "-p", "#{pane_id}")
		target = strings.TrimSpace(out)
	}
	session, spare := "", ""
	if target != "" {
		session = tmux.DisplayMessage(target, "#{session_id}")
	}
	if session == "" {
		// Standalone: outside tmux, agents live in a detached background session.
		if session, spare, err = hiddenSession(o.Cwd); err != nil {
			return "", "", err
		}
	}
	var repo, branch string
	if o.Worktree {
		if o.Name == "" {
			return "", "", errors.New("--worktree needs --name (it names the branch)")
		}
		if repo, branch, o.Cwd, err = addWorktree(o.Cwd, o.Name); err != nil {
			return "", "", err
		}
	}

	args := append(append([]string(nil), o.Args...), effortArgs...)
	roleEnv := ""
	if o.Role != "" && o.ConfigDir != o.Cwd && (role.Harness == "claude" || role.Harness == "opencode") {
		config, e := nativeRoleConfig(role)
		if e != nil {
			return "", "", e
		}
		switch roles.Harness(o.Agent) {
		case "claude":
			args = append(args, "--agents", config)
		case "opencode":
			roleEnv = "env OPENCODE_CONFIG_CONTENT=" + setup.ShellQuote(config) + " "
		}
	}
	if o.Model != "" {
		args = append([]string{"--model", o.Model}, args...)
	}
	if o.Role != "" {
		switch roles.Harness(o.Agent) {
		case "claude":
			if role.Harness == "global" {
				args = append(args, "--append-system-prompt", role.Instructions)
			} else {
				args = append(args, "--agent", o.Role)
			}
		case "opencode":
			if role.Harness == "global" {
				return "", "", fmt.Errorf("global OpenCode agents require a native definition in %s", roles.DirFor(o.ConfigDir, o.Agent))
			}
			args = append([]string{"mini", "--agent", o.Role}, args...)
		case "codex":
			// Codex has no standalone --agent flag; pass the repo agent's developer instructions.
			args = append(args, "-c", "developer_instructions="+strconv.Quote(role.Instructions))
		}
	}
	command := roleEnv + setup.ShellQuote(o.Agent)
	for _, a := range args {
		command += " " + setup.ShellQuote(a)
	}
	if pane, err = placePane(session, o.Cwd, command); err != nil {
		return "", "", err
	}
	if spare != "" { // the new background session's empty first window
		tmux.RunTmux("kill-window", "-t", spare)
	}
	if o.Name != "" {
		tmux.SetPaneOption(pane, tmux.PaneName, o.Name)
	}
	if o.Role != "" {
		tmux.SetPaneOption(pane, roleOption, o.Role)
	}
	if o.Worktree {
		tmux.SetPaneOption(pane, wtRepoOption, repo)
		tmux.SetPaneOption(pane, tmux.PaneWorktreeName, o.Name)
		tmux.SetPaneOption(pane, tmux.PaneWorktreeBranch, branch)
	}
	if !o.Wait {
		return pane, "", nil
	}
	if warn = waitReady(pane); warn != "" && strings.Contains(warn, "exited") {
		return "", "", errors.New(warn)
	}
	return pane, warn, nil
}

// nativeEffortArgs preserves the requested level; model support is checked by the harness.
func nativeEffortArgs(harness, effort string) ([]string, error) {
	if effort == "" {
		return nil, nil
	}
	if err := tasks.ValidateEffort(harness, effort); err != nil {
		return nil, err
	}
	switch harness {
	case "pi":
		return []string{"--thinking", effort}, nil
	case "claude":
		return []string{"--effort", effort}, nil
	case "codex":
		return []string{"-c", "model_reasoning_effort=" + strconv.Quote(effort)}, nil
	default:
		return nil, fmt.Errorf("effort is unsupported for %s", harness)
	}
}

// nativeRoleConfig supplies the live main-checkout definition without copying
// configuration into the execution tree or changing the harness's working directory.
func nativeRoleConfig(role roles.Definition) (string, error) {
	definition := map[string]any{}
	prompt := role.Instructions
	if strings.HasPrefix(prompt, "---\n") {
		front, body, ok := strings.Cut(prompt[4:], "\n---")
		if !ok {
			return "", fmt.Errorf("invalid agent frontmatter: %s", role.Path)
		}
		if err := yaml.Unmarshal([]byte(front), &definition); err != nil {
			return "", err
		}
		prompt = strings.TrimSpace(body)
	}
	delete(definition, "name")
	definition["prompt"] = prompt
	if role.Harness == "claude" {
		// Claude's markdown tools list is comma-separated; --agents expects an array.
		for _, key := range []string{"tools", "disallowedTools"} {
			if value, ok := definition[key].(string); ok {
				var tools []string
				for _, tool := range strings.Split(value, ",") {
					if tool = strings.TrimSpace(tool); tool != "" {
						tools = append(tools, tool)
					}
				}
				definition[key] = tools
			}
		}
		if _, ok := definition["description"]; !ok {
			definition["description"] = role.Name
		}
	}
	agents := map[string]any{role.Name: definition}
	var config any = agents
	if role.Harness == "opencode" {
		// Preserve explicitly supplied inline settings, overriding only this role.
		existing := map[string]any{}
		if content := os.Getenv("OPENCODE_CONFIG_CONTENT"); content != "" {
			if err := json.Unmarshal([]byte(content), &existing); err != nil {
				return "", err
			}
		}
		if prior, ok := existing["agent"].(map[string]any); ok {
			for name, value := range prior {
				if name != role.Name {
					agents[name] = value
				}
			}
		}
		existing["agent"] = agents
		config = existing
	}
	data, err := json.Marshal(config)
	return string(data), err
}

func isClaude(agent string) bool { return filepath.Base(strings.Fields(agent + " x")[0]) == "claude" }

// freeName returns base, or base-2, base-3… if taken.
func freeName(base string) string {
	name := base
	for i := 2; tmux.ResolvePane(name) != name; i++ {
		name = base + "-" + strconv.Itoa(i)
	}
	return name
}

// placePane splits a pool window with room or opens a new one (planPlacement).
func placePane(session, cwd, command string) (string, error) {
	maxPanes := defaultMaxPane
	if v, ok := tmux.GetOption(maxPanesOption); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPanes = n
		}
	}
	list, _ := tmux.RunTmux("list-panes", "-s", "-t", session, "-F",
		"#{window_id}|#{"+poolOption+"}|#{"+tmux.PaneRole+"}|#{pane_id}|#{pane_width}|#{pane_height}")
	if p := planPlacement(list, maxPanes); p.splitTarget != "" {
		return tmux.RunTmuxCapture("split-window", "-d", p.splitDir, "-t", p.splitTarget, "-c", cwd, "-P", "-F", "#{pane_id}", command)
	}
	out, err := tmux.RunTmuxCapture("new-window", "-d", "-t", session+":", "-n", poolWindowName, "-c", cwd, "-P", "-F", "#{pane_id} #{window_id}", command)
	if err != nil {
		return "", err
	}
	pane, win, _ := strings.Cut(out, " ")
	tmux.SetWindowOption(win, poolOption, "1")
	return pane, nil
}

// waitReady waits until a hook or the scraper reports a status; returns a
// warning ("" = ready).
func waitReady(pane string) string {
	deadline := time.Now().Add(newReadyTimeout)
	for time.Now().Before(deadline) {
		if _, alive := tmux.PaneSessionName(pane); !alive {
			return pane + " exited during startup"
		}
		switch tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt) {
		case "":
		case string(tmux.StatusWaiting):
			return pane + " is waiting on a startup prompt (e.g. folder trust) — answer it in the pane"
		default:
			return ""
		}
		time.Sleep(waitPoll)
	}
	return fmt.Sprintf("%s started but reported no status within %s", pane, newReadyTimeout)
}

// sendText types text into pane and presses Enter.
func sendText(pane, text string) { tmux.TypeLine(pane, text) }

// ─── worktrees ───────────────────────────────────────────────────────────

func git(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// worktreePaths: repo root of dir, and the branch/path an agent's
// worktree uses.
func worktreePaths(dir, name string) (repo, branch, path string, err error) {
	if repo, err = git(dir, "rev-parse", "--show-toplevel"); err != nil {
		return "", "", "", fmt.Errorf("%s is not in a git repo", dir)
	}
	return repo, "agent/" + name, filepath.Join(repo, ".worktrees", name), nil
}

// addWorktree creates (or reuses) .worktrees/<name> on branch agent/<name>,
// keeping .worktrees out of git status.
func addWorktree(dir, name string) (repo, branch, path string, err error) {
	if repo, branch, path, err = worktreePaths(dir, name); err != nil {
		return
	}
	validate := func() error {
		listing, e := git(repo, "worktree", "list", "--porcelain")
		if e != nil {
			return e
		}
		root, e := git(path, "rev-parse", "--show-toplevel")
		if e != nil {
			return e
		}
		common, e := git(path, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if e != nil {
			return e
		}
		expected, e := git(repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if e != nil {
			return e
		}
		common, e = filepath.EvalSymlinks(common)
		if e != nil {
			return e
		}
		expected, e = filepath.EvalSymlinks(expected)
		if e != nil {
			return e
		}
		if common != expected {
			return fmt.Errorf("foreign repository at %s", path)
		}
		if _, e = git(path, "rev-parse", "--verify", "HEAD^{commit}"); e != nil {
			return e
		}
		current, e := git(path, "symbolic-ref", "HEAD")
		if e != nil {
			return e
		}
		if root != path || current != "refs/heads/"+branch || !strings.Contains(listing, "worktree "+path+"\n") {
			return fmt.Errorf("invalid worktree at %s", path)
		}
		return nil
	}
	if _, statErr := os.Lstat(path); statErr == nil {
		err = validate()
		return
	} else if !os.IsNotExist(statErr) {
		err = statErr
		return
	}
	if _, err = git(repo, "worktree", "add", "-q", "-b", branch, path); err != nil {
		if validate() == nil {
			err = nil
		}
		return
	}
	excl, e := git(repo, "rev-parse", "--git-path", "info/exclude")
	if e != nil {
		err = e
		return
	}
	if !filepath.IsAbs(excl) {
		excl = filepath.Join(repo, excl)
	}
	if b, _ := os.ReadFile(excl); !strings.Contains(string(b), ".worktrees/") {
		if f, ferr := os.OpenFile(excl, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); ferr == nil {
			fmt.Fprintln(f, ".worktrees/")
			f.Close()
		}
	}
	return
}

// workflowWorktree keeps all jobs for a ticket in one retained checkout.
func workflowWorktree(dir, id string) (string, error) {
	if id == "" || strings.ContainsAny(id, "/\\. \t\n") {
		return "", fmt.Errorf("invalid workflow ticket ID %q", id)
	}
	_, _, path, err := addWorktree(tasks.MainRoot(dir), "workflow-"+id)
	return path, err
}

// agentWorktree finds an agent's worktree from the current repo.
func agentWorktree(name string) (repo, branch, path string, err error) {
	wd, _ := os.Getwd()
	if repo, branch, path, err = worktreePaths(wd, name); err != nil {
		return
	}
	if _, statErr := os.Stat(path); statErr != nil {
		err = fmt.Errorf("no worktree for %q at %s", name, path)
	}
	return
}

// cmdDiff shows what an agent changed in its worktree, committed or not,
// against the main checkout's HEAD.
func cmdDiff(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: tmux-agent diff <name> [--stat]")
		return exitUsage
	}
	repo, _, path, err := agentWorktree(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "diff:", err)
		return exitFailure
	}
	head, _ := git(repo, "rev-parse", "HEAD")
	git(path, "add", "-N", ".") // show new files too
	diffArgs := append([]string{"diff", head}, args[1:]...)
	out, err := git(path, diffArgs...)
	if err != nil {
		fmt.Fprintln(stderr, "diff:", err)
		return exitFailure
	}
	fmt.Fprintln(stdout, out)
	return exitOK
}

// cmdMerge commits an agent's leftover changes in its worktree and merges
// its branch into the main checkout; --discard then removes the worktree.
func cmdMerge(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: tmux-agent merge <name> [--discard]")
		return exitUsage
	}
	name := args[0]
	repo, branch, path, err := agentWorktree(name)
	if err != nil {
		fmt.Fprintln(stderr, "merge:", err)
		return exitFailure
	}
	if st, _ := git(path, "status", "--porcelain"); st != "" {
		if _, err := git(path, "add", "-A"); err == nil {
			git(path, "commit", "-q", "-m", "agent "+name+": work in progress")
		}
	}
	if out, err := git(repo, "merge", "--no-ff", "-m", "Merge "+branch, branch); err != nil {
		fmt.Fprintln(stderr, "merge:", err)
		return exitFailure
	} else if out != "" {
		fmt.Fprintln(stdout, out)
	}
	fmt.Fprintf(stdout, "merged %s into %s\n", branch, repo)
	if len(args) > 1 && args[1] == "--discard" {
		return cmdDiscard([]string{name}, stdout, stderr)
	}
	return exitOK
}

// cmdDiscard closes an agent's pane and deletes its worktree and branch.
func cmdDiscard(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: tmux-agent discard <name>")
		return exitUsage
	}
	name := args[0]
	repo, branch, path, err := agentWorktree(name)
	if err != nil {
		fmt.Fprintln(stderr, "discard:", err)
		return exitFailure
	}
	if pane := tmux.ResolvePane(name); pane != name {
		cmdKill([]string{name}, io.Discard, io.Discard)
	}
	if _, err := git(repo, "worktree", "remove", "--force", path); err != nil {
		fmt.Fprintln(stderr, "discard:", err)
		return exitFailure
	}
	git(repo, "branch", "-D", branch)
	fmt.Fprintf(stdout, "discarded %s (%s)\n", name, branch)
	return exitOK
}

// ─── save / restore ──────────────────────────────────────────────────────

func cmdSave(stdout, stderr io.Writer) int {
	agents := roster.Collect(tmux.QuerySessions())
	if err := roster.Save(agents); err != nil {
		fmt.Fprintln(stderr, "save:", err)
		return exitFailure
	}
	fmt.Fprintf(stdout, "saved %d agent(s) to %s\n", len(agents), roster.File())
	return exitOK
}

// cmdRestore starts the remembered agents that aren't running — Claude
// resumes its old session.
func cmdRestore(args []string, stdout, stderr io.Writer) int {
	dry := len(args) > 0 && (args[0] == "--dry-run" || args[0] == "-n")
	snap, err := roster.Load()
	if err != nil {
		fmt.Fprintln(stderr, "restore: nothing saved yet (", roster.File(), ")")
		return exitFailure
	}
	missing := roster.Missing(snap.Agents, roster.Collect(tmux.QuerySessions()))
	if len(missing) == 0 {
		fmt.Fprintf(stdout, "all %d saved agent(s) are running\n", len(snap.Agents))
		return exitOK
	}
	code := exitOK
	for _, a := range missing {
		label := a.Name
		if label == "" {
			label = a.Agent + " in " + a.Cwd
		}
		if dry {
			fmt.Fprintf(stdout, "would restore %s (%s %s)\n", label, a.Agent, strings.Join(roster.ResumeArgs(a), " "))
			continue
		}
		pane, _, err := spawnAgent(spawnOpts{Name: a.Name, Agent: a.Agent, Cwd: a.Cwd, Role: a.Role, Args: roster.ResumeArgs(a)})
		if err != nil {
			fmt.Fprintf(stderr, "restore %s: %v\n", label, err)
			code = exitFailure
			continue
		}
		fmt.Fprintf(stdout, "restored %s → %s\n", label, pane)
	}
	return code
}

// ─── checkpoints / undo ──────────────────────────────────────────────────

// paneDir is where an agent works: the hook-reported cwd, else the pane's.
func paneDir(pane string) string {
	if d := tmux.GetPaneOptionValue(pane, tmux.PaneCwd); d != "" {
		return d
	}
	return tmux.DisplayMessage(pane, "#{pane_current_path}")
}

func cmdCheckpoints(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: tmux-agent checkpoints <agent>")
		return exitUsage
	}
	pane := tmux.ResolvePane(args[0])
	dir := paneDir(pane)
	list, err := checkpoint.List(dir, hook.CheckpointKey(pane))
	if err != nil || len(list) == 0 {
		fmt.Fprintf(stdout, "no checkpoints for %s (they're taken before each turn, in a git repo)\n", args[0])
		return exitOK
	}
	for i, c := range list {
		stat, _ := git(dir, "diff", "--shortstat", c.Ref)
		if stat == "" {
			stat = "same as now"
		}
		fmt.Fprintf(stdout, "%d\t%s\t%s\n", i, c.Time.Format("15:04:05"), strings.TrimSpace(stat))
	}
	fmt.Fprintf(stdout, "\nundo the last turn: tmux-agent undo %s · n turns back: tmux-agent undo %s <n>\n", args[0], args[0])
	return exitOK
}

// cmdUndo rolls an agent's repo back to the start of its last turn (or n
// turns earlier); --redo <ref> brings back what an undo replaced.
func cmdUndo(args []string, stdout, stderr io.Writer) int {
	if len(args) == 3 && args[1] == "--redo" {
		dir := paneDir(tmux.ResolvePane(args[0]))
		if _, err := checkpoint.Restore(dir, args[2]); err != nil {
			fmt.Fprintln(stderr, "undo --redo:", err)
			return exitFailure
		}
		fmt.Fprintln(stdout, "redone")
		return exitOK
	}
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(stderr, "usage: tmux-agent undo <agent> [n]   |   tmux-agent undo <agent> --redo <ref>")
		return exitUsage
	}
	pane := tmux.ResolvePane(args[0])
	if st := tmux.ParsePaneStatus(tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt)); st == tmux.StatusRunning {
		fmt.Fprintf(stderr, "undo: %s is working right now — stop it first (Esc in its pane)\n", args[0])
		return exitFailure
	}
	n := 0
	if len(args) == 2 {
		if v, err := strconv.Atoi(args[1]); err == nil && v >= 0 {
			n = v
		} else {
			fmt.Fprintln(stderr, "undo: n must be a number ≥ 0")
			return exitUsage
		}
	}
	dir := paneDir(pane)
	list, err := checkpoint.List(dir, hook.CheckpointKey(pane))
	if err != nil || n >= len(list) {
		fmt.Fprintf(stderr, "undo: no checkpoint #%d for %s (see tmux-agent checkpoints %s)\n", n, args[0], args[0])
		return exitFailure
	}
	backup, err := checkpoint.Restore(dir, list[n].Ref)
	if err != nil {
		fmt.Fprintln(stderr, "undo:", err)
		return exitFailure
	}
	fmt.Fprintf(stdout, "rolled %s back to %s (before turn #%d)\nchanged your mind? tmux-agent undo %s --redo %s\n",
		dir, list[n].Time.Format("15:04:05"), n, args[0], backup)
	return exitOK
}

// hiddenSession returns the id of the background session, creating it (and
// the tmux server, if none runs) on first use. @agent_hidden marks it for
// the status-line format (@agent_hidden_status_format).
func hiddenSession(cwd string) (id, spare string, err error) {
	if id := tmux.DisplayMessage("="+sidebar.HiddenSession+":", "#{session_id}"); id != "" {
		return id, "", nil
	}
	out, err := tmux.RunTmuxCapture("new-session", "-d", "-s", sidebar.HiddenSession, "-c", cwd, "-P", "-F", "#{session_id} #{window_id}")
	id, spare, _ = strings.Cut(strings.TrimSpace(out), " ")
	if err != nil || id == "" {
		return "", "", fmt.Errorf("not inside tmux and could not start a background session: %v", err)
	}
	tmux.RunTmux("set-option", "-t", id, "@agent_hidden", "1")
	return id, spare, nil
}
