package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tagent/internal/checkpoint"
	"tagent/internal/hook"
	"tagent/internal/roles"
	"tagent/internal/roster"
	"tagent/internal/routing"
	"tagent/internal/setup"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workplace"
)

// spawnOpts is everything `new` (and fanout/plan/restore, which build on
// it) can ask for.
type spawnOpts struct {
	Name, Agent, Model, Effort, Cwd, Role string
	Ticket                                string // bind the pane to this ticket (per-ticket chat)
	ConfigDir                             string
	Worktree                              bool
	Args                                  []string // extra agent arguments
	Wait                                  bool     // wait until the agent reports a status
	Terminal                              bool     // a plain login shell, no harness; kept from the idle sweep
	Window                                string   // tmux window name (default: Name)
	PromptHasRole                         bool     // the caller puts the role's instructions in its prompt (dispatch)
	BoardRole, Preset                     string   // board.yaml role whose context applies; preset "scout" overrides it
	BrowserProfile                        string   // a browser ticket's Chrome profile dir; set on the browser MCP server's args
}

// Pane options recording how an agent was started (restore, merge).
const (
	roleOption   = "@agent_role"
	wtRepoOption = "@agent_wt_repo"
)

// spawnAgent starts an agent in a new window of its project session and
// returns its pane id, plus a warning worth showing (no status yet).
func spawnAgent(o spawnOpts) (pane, warn string, err error) {
	if o.Cwd == "" {
		o.Cwd, _ = os.Getwd()
	}
	if o.Terminal {
		// A terminal has no harness, role or model, and never reports a status.
		o.Agent, o.Model, o.Effort, o.Role, o.Args, o.Wait = "", "", "", "", nil, false
		o.BoardRole, o.Preset = "", ""
	}
	contextRole := cmp.Or(o.BoardRole, o.Role) // before a board-only role clears o.Role

	if o.ConfigDir == "" {
		o.ConfigDir = o.Cwd
	}
	if o.Ticket != "" {
		if _, err := tasks.Get(tasks.Scope(o.ConfigDir), o.Ticket); err != nil {
			return "", "", fmt.Errorf("ticket %q: %w", o.Ticket, err)
		}
	}
	// A role declared only in .tasks/board.yaml (e.g. pi roles) has no definition
	// file: start its harness/model directly, named after the role.
	if o.Role != "" {
		if cfg, err := tasks.LoadBoardConfig(o.ConfigDir); err == nil {
			if br, ok := cfg.Roles[o.Role]; ok {
				if _, def := roles.GetIn(o.ConfigDir, o.Role); !def {
					if br.Tier != "" && o.Agent == "" {
						// A one-shot CLI has no quota reading yet, so a tier role starts on its first candidate;
						// an explicit --model keeps its model.
						rc, err := routing.Load()
						if err != nil {
							return "", "", err
						}
						c, err := rc.First(br.Tier)
						if err != nil {
							return "", "", err
						}
						o.Agent, o.Model = c.Harness, cmp.Or(o.Model, c.Model)
						if tasks.ValidateEffort(o.Agent, br.Effort) != nil {
							br.Effort = ""
						}
					}
					o.Agent, o.Model, o.Effort = cmp.Or(o.Agent, br.Agent), cmp.Or(o.Model, br.Model), cmp.Or(o.Effort, br.Effort)
					if o.Name == "" {
						o.Name = freeName(o.Role)
					}
					o.Role = ""
				}
			}
		}
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
		if role, ok = roles.GetIn(o.ConfigDir, o.Role); !ok {
			return "", "", fmt.Errorf("unknown repo agent %q (have: %s)", o.Role, strings.Join(roles.NamesIn(o.ConfigDir), ", "))
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
	// One session per project, chosen from the cwd before any worktree switch.
	session, spare, err := tmux.ProjectSession(tasks.MainRoot(o.Cwd))
	if err != nil {
		return "", "", err
	}
	var repo, branch string
	if o.Worktree {
		if o.Name == "" {
			return "", "", errors.New("--worktree needs --name (it names the branch)")
		}
		if repo, branch, o.Cwd, err = workplace.Add(o.Cwd, o.Name); err != nil {
			return "", "", err
		}
	}

	contextArgs, err := roleArgs(o, contextRole)
	if err != nil {
		return "", "", err
	}
	args := append(append(append([]string(nil), o.Args...), effortArgs...), contextArgs...)
	if isClaude(o.Agent) {
		flags, _ := tmux.GetOption("@agent_claude_flags")
		args = append(args, strings.Fields(flags)...)
	}
	// Only a legacy file in the running harness's own agents directory is loaded by
	// that harness natively. App-owned instructions arrive in the dispatch prompt.
	native := o.Role != "" && role.Native && role.Harness == roles.Harness(o.Agent)
	roleEnv := ""
	if native && o.ConfigDir != o.Cwd && (role.Harness == "claude" || role.Harness == "opencode") {
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
	if native {
		switch roles.Harness(o.Agent) {
		case "claude":
			args = append(args, "--agent", o.Role)
		case "opencode":
			args = append([]string{"mini", "--agent", o.Role}, args...)
		case "codex":
			// Codex has no standalone --agent flag; pass the repo agent's developer instructions.
			args = append(args, "-c", "developer_instructions="+strconv.Quote(role.Instructions))
		}
	} else if text := strings.TrimSpace(role.Instructions); o.Role != "" && text != "" && !o.PromptHasRole {
		// App-owned instructions (.tasks/agents/<role>.md): no dispatch prompt
		// carries them here (`new`, ask, fanout, chat), so hand them to the harness.
		// ponytail: opencode/gemini/copilot/amp have no such flag; they still get them only via dispatch.
		switch roles.Harness(o.Agent) {
		case "claude", "pi":
			args = append(args, "--append-system-prompt", "Your role is "+o.Role+". Follow these instructions:\n\n"+text)
		case "codex":
			args = append(args, "-c", "developer_instructions="+strconv.Quote(text))
		}
	}
	command := roleEnv + setup.ShellQuote(o.Agent)
	for _, a := range args {
		command += " " + setup.ShellQuote(a)
	}
	if o.Terminal {
		command = setup.ShellQuote(cmp.Or(os.Getenv("SHELL"), "/bin/sh")) + " -l"
	}
	win := o.Window
	if win == "" {
		win = o.Name
	}
	if win == "" && o.Terminal {
		win = "terminal"
	}
	if win == "" {
		win = filepath.Base(o.Agent)
	}
	if pane, err = tmux.RunTmuxCapture("new-window", "-d", "-t", session+":", "-n", win, "-c", o.Cwd, "-P", "-F", "#{pane_id}", command); err != nil {
		return "", "", err
	}
	if spare != "" { // the new background session's empty first window
		tmux.RunTmux("kill-window", "-t", spare)
	}
	if o.Terminal {
		tmux.SetPaneOption(pane, tmux.PaneKeep, "1")
	}
	tagSession(pane, tasks.MainRoot(o.ConfigDir))
	if o.Name != "" {
		tmux.SetPaneOption(pane, tmux.PaneName, o.Name)
	}
	if o.Ticket != "" {
		// The project scope, not just the id: a tool call can come from a worktree
		// and must still resolve to the same board.
		tmux.SetPaneOption(pane, tmux.PaneTicket, tasks.TicketScope(o.ConfigDir, o.Ticket))
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

func git(dir string, args ...string) (string, error) { return workplace.Run(dir, args...) }

// agentWorktree finds an agent's worktree from the current repo.
func agentWorktree(name string) (repo, branch, path string, err error) {
	wd, _ := os.Getwd()
	if repo, branch, path, err = workplace.Paths(wd, name); err != nil {
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
		fmt.Fprintln(stderr, "usage: tagent diff <name> [--stat]")
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
		fmt.Fprintln(stderr, "usage: tagent merge <name> [--discard]")
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
		fmt.Fprintln(stderr, "usage: tagent discard <name>")
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
		fmt.Fprintln(stderr, "usage: tagent checkpoints <agent>")
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
	fmt.Fprintf(stdout, "\nundo the last turn: tagent undo %s · n turns back: tagent undo %s <n>\n", args[0], args[0])
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
		fmt.Fprintln(stderr, "usage: tagent undo <agent> [n]   |   tagent undo <agent> --redo <ref>")
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
		fmt.Fprintf(stderr, "undo: no checkpoint #%d for %s (see tagent checkpoints %s)\n", n, args[0], args[0])
		return exitFailure
	}
	backup, err := checkpoint.Restore(dir, list[n].Ref)
	if err != nil {
		fmt.Fprintln(stderr, "undo:", err)
		return exitFailure
	}
	fmt.Fprintf(stdout, "rolled %s back to %s (before turn #%d)\nchanged your mind? tagent undo %s --redo %s\n",
		dir, list[n].Time.Format("15:04:05"), n, args[0], backup)
	return exitOK
}
