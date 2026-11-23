# Ticket boards

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

## Local quick commands

For humans, board `n` and `tagent do "request"` use the local intake model to
classify requests: operations produce `CMD: tagent <args>`, work ideas use
questions/tickets, and design/review/judgement produces `ESCALATE: reason`.
The prompt includes up to 30 current tickets and workflow names/descriptions.
Commands are never automatic: popup Enter runs, e edits the command line (Enter
accepts the edit, then Enter approves), Esc cancels; CLI y runs, e edits, n aborts.
Output stays in the popup or terminal. Escalation suggests asking the lead (Claude)
and sends the original request to the clipboard with OSC 52 (terminal/tmux support required).

Allowed operations: list; task ls/show/move/set/comment/assign/due; workflow
list/run; reload; queue; ctx read/add; stats; kill (agent panes only). Commands
run the current binary directly, never a shell. Arguments support simple double
quotes. Shell metacharacters `; | & ` plus backticks, `$ ( ) > <`, newlines and
unbalanced quotes are refused, even inside quoted arguments. Other subcommands,
including task rm, merge, discard, install and config set, are refused before execution.
Workflow recipes remain trusted executable configuration: review the command before approval.

## Intake

Humans can press `n` on the board or run `tagent task add -i [--workflow NAME] ["idea"]`.
A local model asks at most three questions before proposing a ticket; nothing is created before approval.
Set `@agent_intake_llm` to `off` for plain add, or a custom stdin/stdout command (60-second timeout).
Default: `ollama run --nowordwrap --think=false bonsai-2:9b`. Errors fall back to plain add with a notice.
Set intake LLM off for plain creation. Draft keys: Enter create, r create+start, e editor, w cycle workflow, Esc cancel.
CLI: y create, r create+start, e editor, w NAME (or w none) change workflow, n abort.
Malformed replies are shown for correction in `$EDITOR` (default vi).
Workflow `description:` is an optional top-level string explaining when to choose the recipe;
intake sends every workflow as `name: description`. Without a workflow, r just creates.
One missing required workflow input receives the approved draft text. For multiple missing
inputs CLI prompts; TUI creates a plain todo ticket with a notice naming inputs and `workflow run`.
Starting a workflow moves the newly created ticket to its first job; it never creates a second ticket.

## Board UI

`?` opens help for the focused panel or view only (tickets, Workflows, workflow
view, task detail, or intake). Its title identifies the context. Press `a` in help
for all shortcuts, `j`/`k` to scroll, and `Esc` to close; reopening returns to
contextual help. Help also follows agent, definition, task, Chat and standalone focus. Chat replaces the terminal dashboard Git panel: Tab or click to focus, Enter to send, and Esc to cancel. Command execution requires explicit approval.

`Tab` switches focus between tickets and the bottom Workflows panel. In Workflows,
`j`/`k` selects a recipe, `Enter` (or `w`) opens its workflow view, `a` asks for a
new name and opens its generated template in `$EDITOR` (default `vi`), `e` edits
the selected file, and `x` asks for `y`/`n` deletion confirmation. Names accept
letters, digits, hyphens and underscores. New recipes are created exclusively in
the main worktree's `.tasks/workflows/<name>.yml`; existing files are never overwritten.
The list reloads after the editor exits (including errors) or a deletion. Editing
and deleting target the selected file, including global workflows marked `global`.

The sidebar opens on tickets grouped by configured status order. At dashboard
widths it shows Kanban columns, windowed around the selected ticket when the
workflow has more columns than fit. At 110 columns and wider, a ticket inspector
shares the screen. Enter opens the full comment timeline; Escape closes drawers
without leaving the board. Mouse clicks select cards; wheel or j/k changes selection.

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

The board shows few columns for many statuses, like Jira status categories.
Each status sits in its `column:`; by default `todo` and `done` are their own
columns and every other status is `in progress`. Cards show their status as a
badge, so `plan`, `build`, `test`, `review` and `blocked` share one column.

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
- **Goal:** Track a feature through its existing feature workflow and receive Telegram status updates.
- **Acceptance:** Assign an owner, move the ticket through the workflow, and verify that only `done`/`blocked` send a phone message, containing the ticket, old/new status and current assignees. Repeating the same status sends nothing.

Use the existing feature workflow (no notification-specific workflow):
```sh
tagent task add --workflow feature "Track delivery status updates"
tagent task assign ID builder
```
Assignment behavior is unchanged. Configure `notify_url` with the existing Telegram
`sendMessage` URL (including `chat_id`) or ntfy URL; empty disables delivery.
Only persisted status transitions alert, including workflow, sidebar and web edits;
creation, comments and assignment alone do not. Desktop enablement is independent.
The global tmux destination is shared by boards. Messages expose project scope,
ticket title/ID and assignees, with long display text truncated. Delivery is
best-effort after releasing the board lock, with a three-second timeout and no
retries or replay; concurrent transitions may arrive out of order.

## Telegram receptionist chat

Set `@agent_notify_url` to `https://api.telegram.org/bot<TOKEN>/sendMessage?chat_id=<ID>`
using a positive private-chat ID. The existing background scraper long-polls Telegram;
no webhook or public server is needed. Only that private chat is accepted. Keep the
URL/token secret. Existing outgoing task-status notifications are unchanged.

Telegram and dashboard Chat now share one durable receptionist conversation per task
scope, including the existing worktree/main-root scoping. This **replaces agent pane
control**: no terminal input, screen capture, agent completion bridge or transcript
migration. Old pane selections are cleared on upgrade.

- `/list`: registered and live-pane projects, deduplicated by task scope.
- `/chat <name|number>`: select a project; ambiguous names require a number.
- Plain text: ask the receptionist; a sole project is selected automatically.
  Multiple projects require explicit selection. Busy requests receive a retry message.
- Drafts and allowlisted commands require explicit **local approval in dashboard
  Chat**. Telegram cannot approve or execute them (`ok` and `/yes` only explain
  this boundary). `/no` cancels a proposal. Factual answers never execute commands.
  Agent terminal approval dialogs also require local input.
- `/read [n]`: bounded shared conversation history (default 30, maximum 48 entries).
- `/stop`: detach without cancelling or erasing the conversation.
- `/help`: command summary.

A clarification on either surface can be answered on the other. Chat keeps local
input/editor buffers separate; approval claims reject stale or competing proposals.
Interrupted model requests become retryable failures; interrupted mutation claims
report an uncertain outcome requiring inspection, and are never automatically rerun.

History is bounded and stored privately beneath the project's shared context directory
in `conversation/state.json`. It survives panel/poller restarts and may contain
sensitive user text. Old answers are quoted as untrusted context, not current facts.
Corrupt state reports an error rather than silently replacing history.

Only the owning scraper accepts input. Accepted text update IDs are deduplicated;
offsets advance after handling. Output cursors are endpoint/scope-qualified and
advance only after successful sends. Selection sends current pending state before
checkpointing; failed sends leave the selection update retryable. Old history is not replayed. HTTP
retries are ordered best-effort, not exactly-once delivery. Messages are plain text,
token-redacted and bounded below Telegram's 4096 UTF-16-unit limit.

Tasks panel and board share ticket controls: `n` intake, `s` workflow start picker,
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
