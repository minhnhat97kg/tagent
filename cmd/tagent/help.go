package main

import (
	"fmt"
	"io"
	"os"
	"tagent/internal/roles"
)

const helpText = `tagent commands

Agents
  list [--json]                Running panes and their status
  agents [name]                List repo agents or show one definition (roles is an alias)
  new --repo-agent NAME        Start a repo agent; --agent and --model choose the harness
  chat [--dir DIR] [--agent claude|pi] [--clear] ticket
                               One assistant pane per ticket, bound to it (see task)
  name [pane] name             Name a running pane
  read pane [-n lines]         Read a pane's screen
  send pane [options] text     Send or queue a prompt
  wait pane [options]          Wait for a status
  answer pane option           Answer a prompt
  next                         Jump to the agent needing attention
  kill pane...                 Close agent panes

Board and coordination
  task [ls]                    List tasks: id, status, assignees, text
  task add [--to names] [--needs ids] text  Create a ticket (needs is informational)
  task done id [note]          Admin shorthand for set done
  workflow list|new NAME|run NAME [--input k=v] [--wait] [title]
task add --workflow NAME [--input k=v] title
task move id status [--output k=v]... [comment]  Handoff along configured transitions
  task retry id               Fresh current-status dispatch after repairing a failure
  task set id status           Admin status change (bypasses next)
  task start id [workflow]      Start a ticket (default_workflow when omitted; or pass an idle pane to link it)
  task assign id names         Assign comma-separated agent names (- clears)
  task show id [--brief|--all]  Compact agent context / full human history, including dispatch activity
  task comment id [--attach path]... [--diff] text  Comment / @mention an agent
  task attachments id         List comment and job-output attachments, sizes and availability
  task due id YYYY-MM-DD|-      Set or clear a due date
  board init|run|validate|migrate [--dir DIR] [--dry-run]
                               Create config / run triggers / validate / move tickets to default_workflow
                                     board.yaml roles support agent, model, effort (edit in the board's Agents panel or the web Agents page)
  task rm id                   Delete a task
  task [ls] from a chat pane   Listing is read-only; a chat pane may only change its own ticket
  stats [ls|clear]             View or clear Claude/Codex/OpenCode/Pi usage
  ctx [--global] ...           Share notes (ls, add, rm, clear, scope)
  ask [--role name] text       Delegate to an idle agent or start one
  queue [pane]                 Show queued prompts (queue clear pane)
  report wait|show file        Read a worker report
  events [--json]              Stream agent events
  tokens                       Show token usage

Worktrees and attempts
  spawn task                   Start an agent in a worktree
  diff name                    Show changes (also --stat)
  merge name [--discard]       Merge a worktree agent's changes
  discard name                 Drop a worktree agent
  handoff [PANE]               Return web-owned pane sizing to this terminal
  checkpoints name            List snapshots; undo name [n] restores one
  fanout -n N text             Run parallel attempts; compare prefix reviews them
  save | restore               Save and restore running agents

UI and configuration
  watch                        Open dashboard
  toggle | toggle-all          Show or hide sidebar panes
  expand                       Expand or collapse sidebar
  config [list|get|set|reset|path]  View or change settings
  install | uninstall          Manage harness hooks and skills
  setup [harness]              Print hook setup instructions
  reload                       Restart sidebars after an update
  explain <pane|name>          Why a pane has its status (hook/screen/CPU evidence)
  version                      Print version
`

func cmdHelp(stdout io.Writer) int {
	fmt.Fprint(stdout, helpText)
	wd, _ := os.Getwd()
	names := roles.NamesIn(wd)
	fmt.Fprintf(stdout, "Agents defined in this repo:\n")
	if len(names) == 0 {
		fmt.Fprintf(stdout, "  (none; add %s/<name>.md)\n", roles.LocalDir(wd))
	} else {
		for _, name := range names {
			fmt.Fprintf(stdout, "  %s\n", name)
		}
	}
	return exitOK
}
