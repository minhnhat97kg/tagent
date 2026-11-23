# Ticket boards

## Attachments

`tagent task attachments ID` lists comment files (including long comments,
CI logs and `--diff`) and job file outputs. Existing
`tagent task comment ID --attach path` supports files-only comments.

The web ticket drawer lists all files together and beside their comments.
Click a row for a lazy preview: markdown, text/logs, coloured diffs, JSON or
raster images. Copy, Download, Open full screen and Load all are available;
text previews initially read only 256 KiB. HTML/SVG are never rendered as active
content. Use the paperclip picker, drag files onto the composer, or paste images.
Remove queued files by clicking their names. Uploads allow four files, each at
most 10 MiB (40 MiB total request, including multipart overhead). Errors retain
the comment and file queue.

In TUI ticket details, `a` opens the attachment selector (j/k, Enter, Escape),
and `A` prompts for a local file path. Text opens in `$PAGER` (default `less`),
with control characters removed and a 10 MiB snapshot limit. Images, PDF, audio and video open with the OS viewer (`open`/`xdg-open`, local machine only);
other binary files have a web-download fallback. Pager commands are split into arguments, not evaluated
as shell scripts; less runs without raw-control options.

Only recorded ticket files can be read. Stored files and all their ancestors
must not be symlinks. Output paths resolve against the selected project checkout,
not another worktree: absolute paths must remain inside that root. Missing or
unsafe paths remain visible as unavailable. Outputs have unknown authorship and
use filesystem modification time; comment files retain comment author/time.
Uploads become ordinary ticket comments, visible in web, TUI and CLI.

## Replying to blocked tickets

Blocked rows show the blocking question. `c` opens the existing comment editor
prefilled with the interrupted role. Send `@implementer answer` (also supported by
`tagent task comment ID "@implementer answer"`) to resume that exact step.
Another role routes to its first status in the ticket's flow. Unknown or multiple
distinct tags save the comment but keep the ticket blocked with a notice.
Without history or for a roleless step, type a valid role explicitly. An unchanged
prefill cancels. Resumed agents receive only the reply, not old comment history.
`max_loops` counts entries since the last exit from blocked. Agents must include
exactly one concrete question in their blocking move comment.

A ticket is the work state. Status entry dispatches work; comments carry handoffs.
Run `tagent board init` to exclusively create the example in the main
worktree's `.tasks/board.yaml`:

```yaml
roles:
  planner: {agent: pi, model: openai-codex/gpt-6.1-sol, effort: high}
  implementer: {agent: pi, model: openai-codex/gpt-6.1-sol}
  reviewer: {agent: claude}
statuses:
  - {id: todo, next: [plan]}
  - {id: plan, role: planner, next: [build, blocked]}
  - {id: build, role: implementer, next: [test, blocked]}
  - {id: test, run: "go test ./...", pass: review, fail: build}
  - {id: review, role: reviewer, next: [done, build, blocked], max_loops: 3}
  - {id: blocked}
  - {id: done}
max_fires_per_hour: 20
triggers:
  - on: mention
    when: {to: '*'}
    agent: '{{event.name}}'
```

Run `tagent board validate [--dir DIR]` to check the effective configuration
without creating or modifying files or running triggers. Like runtime, it reads
`.tasks/board.yaml` from the main worktree, then (only if absent)
`$XDG_CONFIG_HOME/tagent/board.yaml` (`~/.config/tagent/board.yaml`
when unset), then built-in defaults if neither exists. Local errors do not fall
back. Missing configuration is valid. Success prints a confirmation and exits 0;
invalid or unreadable configuration prints `board:` errors to stderr and exits 1.
Bad flags or extra arguments exit 2. This checks parser acceptance, not agent
availability or executable/trigger success.

## Board columns vs workflows

`board.yaml` columns define the default flow. A workflow overrides that flow for one ticket, attached with `s`/start. Choose board flow for routine work; choose a workflow when a ticket needs a special sequence of roles or checks.
## Board UI

`?` opens help for the focused panel or view only (tickets, Workflows, workflow
view, or task detail). Its title identifies the context. Press `a` in help
for all shortcuts, `j`/`k` to scroll, and `Esc` to close; reopening returns to
contextual help. Help also follows agent, definition, task and standalone focus.

`Tab` switches focus between tickets and the bottom Workflows panel. In Workflows,
`j`/`k` selects a recipe, `Enter` (or `w`) opens its workflow view, `a` asks for a
new name and opens its generated template in `$EDITOR` (default `vi`), `e` edits
the selected file, and `x` asks for `y`/`n` deletion confirmation. Names accept
letters, digits, hyphens and underscores. New recipes are created exclusively in
the main worktree's `.tasks/workflows/<name>.yml`; existing files are never overwritten.
The list reloads after the editor exits (including errors) or a deletion. Editing
and deleting target the selected file, including global workflows marked `global`.

The sidebar opens on tickets grouped by configured status order. At pane widths of 120 columns or more it shows status columns, windowed around
the selection when necessary. `h/l` selects a column; `j/k` selects a card within
it. Narrow panes retain the list with status group headers. `H/L` still moves
the ticket's status. Enter opens the detail drawer; Escape closes it.

Cards show the current status role and its live pane state, comment count, and
repeated workflow-entry count when greater than one. Blocked tickets and tickets
waiting for human input are highlighted. Pane matching uses the exact
`role#ticket` BoardTarget for the current status; assignment is metadata only.

- `n`: new; `e`: edit; `x`: delete with confirmation.
- `m`: move using allowed next statuses (`next: []` allows none); `M`: explicit
  administrative override. Moves dispatch the same workflow events as the CLI.
- `c`: comment; `a`: assign; `o`: jump to the current-role pane.
- `P`: panes without a ticket; `G`: git drawer; `,`: settings; `?`: help.
- Approval option keys answer only the selected ticket's current-role pane and
  take precedence over ticket actions while an approval is visible.
- Standalone dashboard `t` focuses the selected ticket's terminal preview.

## Configuration

Local configuration replaces the global file in full. When absent, load
`$XDG_CONFIG_HOME/tagent/board.yaml` (default `~/.config/tagent/board.yaml`).
An invalid local file disables dispatch; it does not fall back. Linked worktrees
share the main worktree config and ticket store. With neither file, default
statuses are todo, doing, review, blocked, done, all roleless, with no triggers.

`columns: [todo, doing, done]` is accepted as roleless statuses. Do not combine it
with statuses. IDs must be unique; todo/done are required. Role or max_loops
statuses require blocked for escalation. A status cannot have both role and run.
Next, pass and fail must name existing statuses. Run requires both pass and fail.
Max_loops is nonnegative (0/omitted is unlimited); the fire cap must be positive.
Omitted next permits any move; explicit `next: []` permits none.

The TUI board uses one column per configured status, in configuration order.
Headers show the status, ticket count and role/run. Cards use two title lines
and one metadata line; blocked cards show the question. Done cards collapse to
one line.

A configured role requires agent, with optional model. Agent resolves as a
harness name or an existing agent definition. Unlisted roles resolve as their own
name. Pane identity stays logical-role#ticket, even if two roles use one harness.
Re-entry reuses that pane and context. Model/harness changes affect new panes only.

Triggers still support create, assign, status, comment, mention and due, filters
(to/from/actor), wildcard matching and the existing literal template variables.
Workflow/inputs trigger targets are no longer supported. Mention notifications
remain independent of the status role. System events are suppressed; ci events
are not. Explicit trigger self-target guards remain; built-in role dispatch does
not skip a role just because that role made the transition.

## Working a ticket

```sh
tagent task add "Implement the feature"  # prints ID
tagent task move ID plan "Acceptance criteria"
tagent task show ID                     # description and all comments
tagent task move ID build "Plan, files and verification strategy"
```

Move writes the optional comment and changed status under one task lock and one
event batch. It checks next; rejection writes nothing. Same-status moves may add
a comment but do not create a new entry. `task set ID STATUS [note]` is the admin
bypass used by TUI; `task done ID [note]` is admin shorthand for set done.
`task start ID WORKFLOW` attaches a workflow to a todo ticket and enters its first job.
`task start ID board` advances to the next board status. Board `s` opens a workflow
picker (j/k select, Enter start, Esc cancel). Existing pane names/IDs still select
legacy manual dispatch, which requires a doing status; Tasks panel shares the board ticket actions.
Needs (“blocked by”) and assignees are informational, never readiness gates.

The dispatcher reserves each event before side effects. A status entry fires its
role with instructions to read the full ticket, do that status's work, then
comment+move. Questions for a human go to blocked. Returning review -> build
fires again and reuses the implementer's pane. Busy/approval/startup panes queue.
CLI move registers automated tickets and ensures the scraper so no existing pane
is needed. `@agent_scrape off` disables scraper-based automation; use board run
explicitly if desired. Web/TUI discover repositories as before.

## Shell checks and limits

Run statuses execute `sh -c` asynchronously at the main repository root. Combined
stdout/stderr retains approximately 40 lines, bounded to 64 KiB. A ci comment
records exit code (startup errors use -1) and the tail, then ci moves to pass on
0 or fail otherwise; the next role dispatches normally. CI follows next too, so
include pass/fail if next is explicitly restricted. If the ticket has moved or
re-entered meanwhile, record a stale-result comment without overwriting it.
No execution timeout or cancellation scheduler is provided. Commands run with
local user privileges: only trust board configs you intend to execute.

Max_loops counts persisted status-change events into that status for the ticket,
through the current event (all actors), across restarts. Entry N+1 moves to blocked
with a system explanation instead of dispatching or running configured triggers
for that entry. Safety blocking bypasses next and the fire cap.

## Delivery and migration

At-most-once, not guaranteed delivery: LastSeq and per-task hourly fire reservations
are persisted before external side effects. A crash can lose a launch or the
remaining actions of that event. Rate-capped events are consumed, not deferred.
There is no automatic replay/resume. Inspect dispatch.state last_error and system/
ci comments. Inspect `task show <id>` (or `--brief` for the latest dispatch), repair
the reported error, then use `task retry <id>` for a fresh current-status entry.
Retry requires a configured role or run command, preserves ticket data, and does
not replay historical triggers. Accepted/queued means dispatched, not completed.
Hourly rate limits and max_loops still apply. A reservation with no outcome is
uncertain: retry may duplicate an already-delivered prompt or CI side effect.
Dispatch remains at-most-once; there is no automatic replay.
Task replacement and event append are best effort across files, not a crash-atomic
transaction or WAL. One-lock batches prevent interleaving concurrent mutations.

### Agents

One list of agent roles, in the TUI board and on the web (`#/agents`). Each row shows
the harness and model, whether the role is repo, global or board-only, the live panes
running it, and the statuses and workflow steps that use it.

- TUI: `t` in the board opens the Agents panel. `enter` shows detail, `e` edits the
  instructions in `$EDITOR`, `m` changes the model, `a` adds an agent, `x` deletes one (`y` confirms), `t` closes the panel.
- Web: harness, model, effort and instructions (with preview) save together. Global
  agents are read-only; **Copy to repo** makes an editable copy.

Harness, model and effort are written to the role's `board.yaml` entry, and comments
and other keys in that file are kept. Instructions go to the agent's file:
`.claude/agents/<name>.md` (the whole file, frontmatter included) or
`.codex/agents/<name>.toml` (only `developer_instructions` changes). Pi roles have no
file. Model and effort apply to new panes only. Running panes keep theirs.

## Workflows

Local `.tasks/workflows/*.yml` recipes override names in
`$XDG_CONFIG_HOME/tagent/workflows` (default `~/.config/tagent/workflows`).
Discovery and configuration use the main worktree root. Native Claude/OpenCode
role definitions are passed inline from that checkout, so main-only or edited
roles do not fall back to stale definitions in the ticket tree. Reuse validates
the worktree's repository identity, expected branch and committed HEAD.
Workflow execution lazily
creates `.worktrees/workflow-<ticket>` from committed HEAD (a Git repository with
a commit is required). All agent jobs, mentions, shell jobs and retries for that
ticket share this directory; relative files survive handoffs and dispatcher
restarts. Different tickets are isolated. Worktrees are retained on done/blocked;
no automatic merge, reset or cleanup occurs. Uncommitted files, ignored dependencies,
secrets and local-only scripts are not copied. Ordinary board execution is unchanged.
Creation failures are reported before launch; retry requires a new status entry. Recipes are resolved
live: editing affects existing tickets; missing or broken recipes report errors.

`tagent workflow new NAME` creates a starter and opens `$EDITOR`.
`workflow list` shows scope, jobs and in-progress tickets.
`workflow run NAME --input goal=value --wait "Title"` starts a ticket, prints its
ID and optionally waits (done exits 0, blocked exits 1).
`task add --workflow NAME --input goal=value "Title"` leaves the ticket in todo.

The web Workflows page edits YAML with line numbers, syntax colours and soft wrap. A validation error is marked on its line; click the error bar to jump there. Clicking a flow card jumps to that job in the YAML, and moving the cursor into a job highlights its card. Global workflows are read-only there.

Jobs form one needs chain; parallel jobs are not supported. Each job has `run`
or `agent` with optional `prompt`, optional `status` (defaults to job ID),
`on-failure` (job/status, default blocked), `max-loops`, `model`, and `outputs`.
Agent-only jobs use generated handoff guidance. Model overrides apply only to
new panes; reused role panes retain their model.

Expressions `${{ inputs.goal }}`, `${{ ticket.id }}`, `${{ ticket.title }}` and
`${{ jobs.collect.outputs.file }}` expand once at dispatch; unknown values are
empty. Recipes and interpolated shell inputs are trusted executable code, not
shell-escaped data. Never interpolate untrusted inputs or publish secrets.

A collection command can call an API and publish its data path:
```yaml
name: collect-report
description: Collect a report from a URL and analyze it
inputs:
  url: {required: true}
jobs:
  collect:
    run: |
      curl -fsS '${{ inputs.url }}' > .tasks/data.json
      echo 'file=.tasks/data.json' >> "$TMUX_AGENT_OUTPUT"
  analyze:
    needs: collect
    agent: analyst
    prompt: 'Analyze ${{ jobs.collect.outputs.file }}'
    outputs: [summary]
```
Commands inherit the environment plus `TMUX_AGENT_TICKET` and
`TMUX_AGENT_OUTPUT`. Output reads are capped at 64 KiB; lines without `=` or
with empty keys are ignored; split at first `=`, last duplicate wins. Outputs
appear in the CI comment. Agents finish with
`task move ID done --output summary=value "Findings"`. Outputs belong to the
source job; stale command results cannot overwrite them. Completing a repeated
job replaces its output map.

Ticket rows include workflow names. `w` opens the selected workflow (or board
statuses), `[`/`]` browse recipes, and `e`/`c` edit the displayed recipe.
Legacy flat outputs still load but are not converted into job outputs.
Old assigned todo tickets do not auto-run; move them deliberately.


### Status-update tracking task

Reusable definition:
- **Title:** Track delivery status updates
- **Goal:** Track a feature through its existing feature workflow and receive phone status updates.
- **Acceptance:** Assign an owner, move the ticket through the workflow, and verify that only `done`/`blocked` send a phone message, containing the ticket, old/new status and current assignees. Repeating the same status sends nothing.

Use the existing feature workflow (no notification-specific workflow):
```sh
tagent task add --workflow feature "Track delivery status updates"
tagent task assign ID builder
```
Assignment behavior is unchanged. Configure `notify_url` with an ntfy URL or plain-text POST webhook; empty disables delivery.
Only persisted status transitions alert, including workflow, sidebar and web edits;
creation, comments and assignment alone do not. Desktop enablement is independent.
The global tmux destination is shared by boards. Messages expose project scope,
ticket title/ID and assignees, with long display text truncated. Delivery is
best-effort after releasing the board lock, with a three-second timeout and no
retries or replay; concurrent transitions may arrive out of order.

Tasks panel and board share ticket controls: `s` workflow start picker,
`enter` detail, `e` edit, `C` comment, `H`/`L` status, `x` delete with confirmation,
`w` workflow, `?` help. Dashboard `Tab` changes focus; `b` opens the board.

### Role effort

Optional `roles.<name>.effort` selects native reasoning effort for new launches:

- Pi: `--thinking LEVEL`: `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`.
- Claude: `--effort LEVEL`: `low`, `medium`, `high`, `xhigh`, `max`.
- Codex: `-c model_reasoning_effort="LEVEL"`: `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`.

Other harnesses reject nonempty effort. Omit it to preserve harness defaults.
Codex's installed schema permits model-advertised strings; these known spellings
are accepted here, but model/version support still applies for all harnesses.
Levels are not converted (`off` is not Codex `none`). Explicit effort is passed
as a native option without rewriting model strings. Existing panes retain their
original model and effort; changes apply only to newly launched panes. Status
model overrides do not override role effort. Mention-trigger launches use the
same role effort.
