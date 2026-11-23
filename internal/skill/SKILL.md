---
name: tagent
description: Coordinate coding agents in tmux panes, delegate repository tasks, and track tickets.
---

# tagent: tickets and agents

## Blocked tickets

Block with exactly one concrete question in the move comment. To resume, reply
with `tagent task comment ID "@implementer answer"`, or press `c` on the
blocked row (the editor prefills the interrupted role). The row shows the question.
The same role returns to its interrupted step; another role selects its first
status in the ticket's flow. Unknown or multiple distinct tags save without
unblocking and show a notice. Missing history or roleless steps require typing a
valid role. Resumed agents receive only the reply. `max_loops` resets on exit from
blocked; ordinary non-blocked mentions are unchanged.

## If you were dispatched a ticket
Everything needed is in the dispatch prompt; do not reload this skill.
Work only on the current status and verify the result.
For missing context: `{{BIN}} task show ID --brief`.
For human history: `{{BIN}} task show ID --all`.
Finish: `{{BIN}} task move ID NEXT "Checks, paths, next steps"`.
Requested outputs: `task move ID NEXT --output summary=value "Handoff"`.
Need a human: `task move ID blocked "Concrete question"`.
Keep handoffs short; put data in outputs/files, not comments.
Never delete tickets or send prompts to your own `$TMUX_PANE`.

## How work runs

Tickets are the only work state. `.tasks/board.yaml` defines statuses with roles
or shell `run:` checks. Entering a role dispatches `<role>#<ticket>`; re-entry
reuses its context and includes only new non-system comments. New panes receive
the description, handoff and outputs. Busy/approval/unknown panes queue until idle.
The scraper dispatches (`@agent_scrape` on); `board run` runs in the foreground.
Roles map to harness/agent definitions and optional models; models affect new
panes only. Assignees and ticket `needs` are informational, not prerequisites.

## Delegating

```sh
{{BIN}} board init
{{BIN}} task add "Implement the feature"
{{BIN}} task move ID plan "Requirements, acceptance criteria, relevant files"
{{BIN}} task show ID --brief
```

Use the board's actual entry status, not necessarily plan. `move` validates next;
omitted next allows any status, empty next allows none. `task set ID STATUS [note]`
and `task done ID [note]` bypass next for admin repair. `task start ID PANE` is
legacy manual dispatch requiring idle pane and configured doing status.
`task start ID WORKFLOW` attaches a workflow to a todo ticket and enters its first job;
`task start ID board` advances one board status. In the board, `s` opens this picker
(j/k select, Enter start, Esc cancel); Tasks panel uses the same ticket keys as the board.

## Worker handoffs

Prompts are self-contained. Use `task show --brief` only for missing context.
`task move ID NEXT [--output k=v]... "Handoff"` writes comment/status under one lock.
Comments over 600 characters become attachments with a short inline preview.
CI stores its full log as ci.log, retaining only a filtered summary inline.
Re-entering a limited status beyond its cap blocks with an explanation.

## Kanban comments and triggers

- `task [ls]`, `task show ID [--brief|--all]`: inspect tickets.
- `task comment ID [--attach path]... [--diff] "text"`: copied attachments;
  `@role` notifies when a mention trigger is configured.
- `task due ID YYYY-MM-DD` (`-` clears): edits trigger, not passage of time.
- `task assign ID role[,role]` (`-` clears): metadata unless triggers configured.
- Humans can use board `n`, `do "request"` (local approved quick commands), or `task add -i`; agents keep using plain `task add`.
- `task add [--to names] [--needs ids] "text"`; `task rm ID` for human cleanup.
- `board init|run [--dir DIR]`: shared config/dispatcher.

Main-worktree `.tasks/board.yaml` falls back to
`$XDG_CONFIG_HOME/tagent/board.yaml` (default `~/.config/tagent/board.yaml`).
Missing config yields roleless defaults. Todo/done are required; automation needs
blocked. CLI moves register automated tickets for discovery without a pane.
Events are reserved before firing: at-most-once, no replay after failure/crash.
Default cap: 20 fires/ticket/hour; capped events are consumed. Inspect
`dispatch.state` LastError and ticket history, then deliberately re-enter to retry.
Dispatch audits are events, not agent-visible comments; system events do not
recurse. Task replacement plus event append is not crash-atomic across files.

## Workflows

`workflow new NAME` creates a recipe; `workflow list` lists local/global recipes.
`workflow run NAME --input goal=value [--wait]` starts a ticket;
`task add --workflow NAME --input goal=value "Title"` stays in todo.
Main-worktree `.tasks/workflows` overrides global recipes. Jobs form one chain.
`${{ inputs.goal }}` and `${{ jobs.collect.outputs.file }}` expand once.
Command jobs publish key=value to `$TMUX_AGENT_OUTPUT`; agents use move --output.
Files resolve live. Interpolated shell inputs are trusted code; never publish
secrets. See docs/board.md for schema and failure routing.

## Manual panes, fanout and reports

- `list [--json]`, `agents [name]`, `read PANE [-n 50]`: inspect.
- `{{BIN}} new --repo-agent NAME [--agent HARNESS] [--model MODEL] [--worktree]`: start.
- `send PANE [--now] [--wait] [--timeout 30m] [--report FILE] TEXT`: send/queue.
- `wait PANE [--until stopped|idle|waiting] [--timeout 30m]`: pane state, not a
  specific queued task; verify results separately.
- `report wait FILE [--timeout 30m]`, `report show FILE`: worker reports.
- `fanout -n N [--role ROLE] [--wait] TEXT`: independent attempts;
  `compare PREFIX`, `diff NAME`, `merge NAME [--discard]`, `discard NAME` review.
- `handoff [PANE]`: return web-owned window sizing to tmux (current pane by default). Sidebar/dashboard Agents: `f`; browser-owned rows say `sized by web`. The tmux pane option `@agent_size_owner` is `web:<viewer-id>` while owned and empty after release. Web Handoff takes over; Release or closing the tab returns sizing.
- `name [PANE] NAME`, `kill PANE...`, `undo AGENT`, `stats [ls|clear]`.
- `ctx [--global]`, `ctx add TEXT`, `ctx rm ID`, `ctx clear --mine`: factual notes.

Never approve another agent's permission prompt without explicit user authority;
quote it to the human. For asynchronous waits use the harness background facility
and retain its handle. Detached processes cannot wake a finished turn. If no
notifications exist, say so and check when asked; do not simulate them with sleeps.
`events --json` is sampled, silent initially and has no replay; reconcile list/read.

Tasks panel and board share ticket controls: `n` intake, `s` workflow start picker,
`enter` detail, `e` edit, `C` comment, `H`/`L` status, `x` delete with confirmation,
`w` workflow, `?` help. Dashboard `Tab` changes focus; `b` opens the board.
