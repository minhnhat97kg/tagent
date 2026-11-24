# tagent agent reference

Full CLI and board reference for agents and humans. The short agent guide is the installed skill (`internal/skill/SKILL.md`); MCP setup is in [mcp.md](mcp.md). Board schema and failure routing: [board.md](board.md).

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
For missing context: `tagent task show ID --brief`.
For human history: `tagent task show ID --all`.
Finish: `tagent task move ID NEXT "Checks, paths, next steps"`.
Requested outputs: `task move ID NEXT --output summary=value "Handoff"`.
Need a human: `task move ID blocked "Concrete question"`.
Keep handoffs short; put data in outputs/files, not comments.
Never delete tickets or send prompts to your own `$TMUX_PANE`.

## How work runs

Tickets are the only work state. `.tasks/workflows/*.yml` defines every flow
(jobs with an agent or shell `run:` check). `.tasks/board.yaml` holds roles,
triggers, `max_fires_per_hour` and `default_workflow`; board lanes are fixed at
todo, in progress, blocked, done. A ticket without a workflow starts
`default_workflow` when it leaves todo (`s`, `task start ID`, or a move).
Entering a role dispatches `<role>#<ticket>`; re-entry reuses its context and
includes only new non-system comments. New panes receive the description, handoff
and outputs. A comment that @mentions a role: a fresh pane gets the brief and the mention;
a reused pane gets only the comments since its cursor. Busy/approval/unknown panes queue until idle.
A role's harness, model and effort come from its `board.yaml` entry (never from a
harness directory); a model or effort change applies to panes started afterwards,
not to running ones. Instructions are plain markdown in `.tasks/agents/<role>.md`
(or `~/.config/tagent/agents/<role>.md`) and are included in the dispatch prompt
on every harness.
The scraper dispatches (`@agent_scrape` on); `board run` runs in the foreground.
Edit the board from the web Board header (Settings, `#/board/config`) or the TUI
board: `B` shows the default workflow, roles and triggers, `E` opens `$EDITOR`. Saves are
validated; an error names its line and keeps the loaded config.
Roles map to harness/agent instructions and optional models; models affect new
panes only. Legacy `.claude`/`.codex`/`.opencode` agent files are read-only. Assignees are metadata; ticket `needs` gate the start: a ticket with open dependencies cannot leave todo (CLI/TUI/web/dispatcher) and shows `waits #12` or `ready`.
Priority is high/normal/low; lanes and the busy/approval queue order by it.

## Delegating

```sh
tagent board init
tagent task add "Implement the feature"
tagent task move ID plan "Requirements, acceptance criteria, relevant files"
tagent task show ID --brief
```

Use the board's actual entry status, not necessarily plan. `move` validates next;
omitted next allows any status, empty next allows none. `task set ID STATUS [note]`
and `task done ID [note]` bypass next for admin repair. `task start ID PANE` is
legacy manual dispatch requiring idle pane and configured doing status.
`task start ID [WORKFLOW]` attaches a workflow to a todo ticket and enters its
first job; omit the name to use `default_workflow`. On a deprecated board,
`task start ID board` attaches the implicit `board` workflow. In the board, `s`
opens this picker (j/k select, Enter start, Esc cancel; the last row starts the
default). Tasks panel uses the same ticket keys as the board.

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
- Create tickets with plain `task add`.
- `task add [--to names] [--needs ids] [--priority high|normal|low] [--workflow NAME [--input k=v]] "text"`; `task needs ID ids|-`;
  `task priority ID high|normal|low`; `task rm ID` for human cleanup. Cycles/unknown ids are rejected.
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
- `tagent new --repo-agent NAME [--agent HARNESS] [--model MODEL] [--worktree]`: start.
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

Tasks panel and board share ticket controls: `s` workflow start picker,
`enter` detail, `e` edit, `C` comment, `H`/`L` status, `x` delete with confirmation,
`w` workflow, `?` help. Dashboard `Tab` changes focus; `b` opens the board.
