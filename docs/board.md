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
`max_loops` counts rejections (hand-backs to the on-failure status) since the last exit from blocked; the step that hit the cap still runs once more, and its next rejection blocks. Agents must include
exactly one concrete question in their blocking move comment.

A comment alone does not reach the human. An agent that needs an answer moves
its ticket to `blocked` with the question. If a board agent goes idle while its
ticket still waits on its own status (no move, no blocked question), the watchdog
flags the pane as stalled after `@agent_stuck_minutes` (default 10): the pane
shows `stuck?` in the sidebar and one notification fires. The flag clears when
the ticket moves on or the pane resumes. Nothing moves the ticket.

A ticket is the work state. Status entry dispatches work; comments carry handoffs.
Flows are defined only by workflows (`.tasks/workflows/*.yml`, see [Workflows](#workflows)).
`board.yaml` holds agents and triggers. Run `tagent board init` to exclusively
create the example in the main worktree's `.tasks/board.yaml`:

```yaml
roles:
  planner: {agent: pi, model: openai-codex/gpt-6.1-sol, effort: high}
  implementer: {agent: pi, model: openai-codex/gpt-6.1-sol}
  reviewer: {agent: claude}
default_workflow: feature
max_fires_per_hour: 20
triggers:
  - on: mention
    when: {to: '*'}
    agent: '{{event.name}}'
```

`board init` also writes a starter `.tasks/workflows/feature.yml` when no
`feature` workflow exists, so `default_workflow` resolves immediately.

The board lanes are fixed: **todo, in progress, blocked, done, closed** (`closed` is built in, see below). Every workflow
job status lands in *in progress*; its card shows the job's real status
(`planning`, `building`, `testing`, ...). A ticket with no workflow gets
`default_workflow` when it leaves todo — `s` in the TUI, `task start ID`, or any
move out of todo. Moving to a job id also works: `task move ID test` enters that
job's `status` (`testing` by default; jobs can set `status:`). Lanes stay fixed
even when a workflow has many statuses.

Run `tagent board validate [--dir DIR]` to check the effective configuration
without creating or modifying files or running triggers. Like runtime, it reads
`.tasks/board.yaml` from the main worktree, then (only if absent)
`$XDG_CONFIG_HOME/tagent/board.yaml` (`~/.config/tagent/board.yaml`
when unset), then built-in defaults if neither exists. Local errors do not fall
back. Missing configuration is valid. Success prints a confirmation and exits 0;
invalid or unreadable configuration prints `board:` errors to stderr and exits 1.
Bad flags or extra arguments exit 2. This checks parser acceptance, not agent
availability or executable/trigger success.

### Usage cost (`pricing:`)

`task show ID --all`, `task ls --usage`, the TUI detail and the web drawer price
each job's tokens at API list prices and sum them per ticket, labelled
**API-equivalent**: an estimate, not a bill, also for subscription logins. Built-in
prices cover the Claude 5.5 models (`claude-opus-5-5`, `claude-sonnet-5-5`,
`claude-haiku-5-5`; the roles' `sonnet`/`haiku`/`opus` aliases map to them).
Add or override a model with USD per million tokens:

```yaml
pricing:
  claude-sonnet-5-5: {input: 2, output: 10, cache_read: 0.10, cache_write: 2.50}
  my-model:          {input: 1, output: 4, cache_read: 0.1, cache_write: 1.25}
```

A job whose model has no price shows its tokens only. A job with no input/output
split shows a range (all input .. all output). Haiku 5.5's long-context tier
(prompts over 100K, 5x) is not applied.

## Workflows vs board.yaml

Workflows are the only flow definition. `board.yaml` keeps `roles`, `triggers`,
`max_fires_per_hour` and `default_workflow`. Start a ticket with `s`, which runs its
type; a ticket without one opens the picker. `task start ID [WORKFLOW]` does the
same, and with no name it falls back to `default_workflow`. A todo ticket's type
is the Workflow line in the web Details tab, `T` in the TUI, or `task workflow ID
NAME|-`. Editing a workflow changes existing tickets too
(recipes resolve live), so there is one place to edit a flow.

## Workspaces

Each workspace is a repo whose `.tasks` board tagent dispatches. The web and TUI
lists add and remove them (see the README's Workspaces section). Removing one only
hides it from the lists: its tickets are untouched and dispatch keeps running for
them, so removing a workspace never stalls work in flight. Adding it back shows it.

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

The sidebar has five fixed lanes: todo, in progress, blocked, done, closed. The
Tasks pane picks what it shows from its own size (see the README): lane counts
under 6 rows, the active lane's cards under 64 columns, the lane list beside the
selected ticket, and at 100 columns and 12 rows four lanes side by side (done
gives way to closed while a closed ticket is selected). `h/l` selects a lane;
`j/k` selects a card within it. `H/L` still moves the ticket's
status within its flow. Enter opens the detail drawer; Escape closes it. At 100
 columns or more it is a right pane beside the tickets, narrower panes show it
 full width. The drawer has four tabs: **Details** (description, attachments, note, agent runs),
 **Chat** (the ticket's own agent conversation), **Activity** (the comment box on top, comments and the
 folded dispatch noise) and **Run** (the ticket's run, same data as the web).
 On Activity, `c` opens the comment box: typing `@` suggests roles from `board.yaml`
 and the board's agents (`tab` completes), `enter` posts. A role that was mentioned
 and has a pane working on the ticket shows as `role replying…` until its reply lands.
 `Tab`/`l` and `Shift-Tab`/`h` cycle the tabs; `1`, `2`, `3`, `4` jump to
 Details, Chat, Activity, Run. Opening the drawer always starts on Details.

Chat is the pane bound to the ticket (`tagent chat <id>`, the same conversation
as the web's Chat tab). `enter` starts it when there is none; `i` messages the
agent (queued while it is busy, like `tagent send`); `S` saves the newest reply
to the ticket as a comment, so what was settled survives the pane. When the
bound pane runs a harness whose transcript Chat cannot read (anything but claude
or pi), the tab is disabled and the summary line says why.

Cards show the current status role and its live pane state, comment count, and
repeated workflow-entry count when greater than one. Blocked tickets and tickets
waiting for human input are highlighted. A todo card with dependencies shows
`waits #12` (dim) or `ready`; a non-normal priority adds a small `↑`/`↓` marker.
`p` cycles priority (normal → high → low) and `D` opens the dependency picker
(j/k select, space toggles, Enter saves). Pane matching uses the exact
`role#ticket` BoardTarget for the current status; assignment is metadata only.

On the web board, each lane is only as tall as its cards (the pane caps it and the cards scroll). Lanes have a minimum width; when they do not all fit, the lanes scroll sideways and the right edge fades while more lanes are hidden. A card's badges wrap onto a second row whole, so a dot never splits from its status text, and a badge too long for its row ends in an ellipsis.

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
share the main worktree config and ticket store. With neither file the board has
the fixed lanes, no roles, no triggers and no default workflow: set
`default_workflow` (and a workflow) before moving tickets out of todo.

`default_workflow` must name a local or global workflow with at least one job;
an unknown name is an error at its line. The fire cap must be positive. A
configured role requires an agent, or a `tier` (see [routing](routing.md)); a
trigger requires an agent and valid template variables (see below).

The TUI board has the five fixed lanes, and its four-lane view shows todo, in
progress, blocked and done (closed in place of done while a closed ticket is
selected). Headers show the lane and ticket count; a card shows its real status. Blocked cards show the question;
done and closed cards collapse to one line. The web closed lane starts collapsed.

### Deprecated statuses and migration

Old `board.yaml` files with `statuses` (or `columns`) still load. They become an
implicit workflow named `board` with the same status ids, so running tickets keep
their exact status names and `next`/`pass`/`fail` semantics. `columns` and
`status:`'s `column:` are ignored (lanes are fixed). `tagent board validate`
prints a deprecation warning; `board init` and the web editor write the new
format only. Non-linear edges a linear workflow cannot express are reported as
warnings.

`tagent board migrate [--dry-run]` moves in-flight tickets onto
`default_workflow`: each old status maps to the job with that **id** first, then
to the job whose `status` matches. For the `feature` recipe, `plan -> planning`,
`build -> building`, `test -> testing`, `review -> reviewing`. The ticket gets the
workflow's inputs and a normal `migrated to workflow X` move event. Unmatched
tickets are listed and left untouched. Migration is idempotent and explicit;
`--dry-run` prints the mapping without changing tickets. Run it before removing
`statuses` from `board.yaml`, then restart `board run` (a long-running dispatcher
keeps the config it started with). Loop counters restart after migration because
status ids change (`build` -> `building`).

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

### Editing the configuration

`#/board/config` (the Settings icon in the Tasks header or the Board header) edits the raw
`.tasks/board.yaml`. The readable half previews the draft as-is: the default
workflow, roles with harness and model, and triggers. A **Default workflow**
select lists local and global workflows and writes only the `default_workflow`
key (comments and key order survive); the YAML half reloads from the response.
Workflow pages edit flows; this page never lists statuses. The board header shows
`default: <name>`.
The right half is the YAML editor with line numbers; an error is marked on its
line and can be clicked to jump there. Save validates first and writes atomically,
so comments and key order survive. Revert discards the draft and **Restore
defaults** loads the `board init` template. A `run:` command executes locally on
the machine when that status is entered.

In the TUI board, `B` opens the same summary from the same parsed config (default
workflow, roles, triggers); `j`/`k` scroll. `Enter` on the Default workflow row
opens the same workflow picker as `s` and saves the choice to `board.yaml`
(comments and key order survive; the last row removes the default). `E` opens
`.tasks/board.yaml` in `$EDITOR`. After the editor exits, the file is validated;
an error keeps the loaded config and shows the line in the notice bar instead of
reloading a broken config. The board header shows `default: <name>`.

## Working a ticket

```sh
tagent task add "Implement the feature"  # prints ID
tagent task move ID plan "Acceptance criteria"   # job id: enters planning
tagent task show ID                     # description and all comments
tagent task move ID build "Plan, files and verification strategy"
```

Move writes the optional comment and changed status under one task lock and one
event batch. It checks next; rejection writes nothing. Same-status moves may add
a comment but do not create a new entry. `task set ID STATUS [note]` is the admin
bypass used by TUI; `task done ID [note]` is admin shorthand for set done.
`task start ID [WORKFLOW]` attaches a workflow to a todo ticket and enters its
first job; no name uses `default_workflow`. Board `s` opens a workflow picker
(j/k select, Enter start, Esc cancel); its last row starts the default workflow.
A ticket that already has a workflow opens with it preselected. Board `n` picks the
new ticket's type first (default: `default_workflow`; last row: none), then opens `$EDITOR`.
For a deprecated board, `task start ID board` attaches the implicit `board`
workflow. Existing pane names/IDs still select
legacy manual dispatch, which requires a doing status; Tasks panel shares the board ticket actions.
Assignees are metadata; `needs` are a start gate (see below).

### Closing and purging tickets

`closed` is a built-in status for work that will not be done. `task close ID
[reason]` closes a ticket from any status; so do `task set ID closed`, a web
status change, and a web drag onto the closed lane. A closed ticket never fires
triggers, and `start`, auto-start and the dispatcher all refuse it. Its agent
panes are closed; its worktree and branch stay. Open dependents keep waiting:
their badge reads `waits closed #12`. The TUI's `H/L` and `m` picker never close
a ticket by accident. An agent can close one through MCP `task_move`.

A closed ticket reopens only to the todo lane: `task set ID todo`. Other moves
are refused with `reopen it with task set ID todo`.

`task purge --closed` is a dry run: it lists every closed ticket and what
removing it discards (uncommitted changes, unmerged branches). `task purge
--closed --yes` deletes them with their worktrees, branches and attachments, and
it is refused from an agent pane. The web closed lane offers the same purge
behind a second click. A closed ticket that a live ticket still needs is kept
and reported, since deleting it would unblock that ticket. Event history stays,
so ticket ids are never reused. Archiving or exporting closed tickets is not
covered.

### Dependencies and priority

`--needs` (on `task add`) and `task needs ID ids|-` record the tickets this one
waits for. Dependencies gate the start: a ticket whose dependencies are not all
done cannot leave todo — `start`, `move` and `set` refuse with
`waiting for #12, #15`, the dispatcher never fires it, and the card shows a dim
`waits #12` badge. A closed dependency never finishes: the badge reads `waits closed #12`. When the last dependency reaches done the card turns to
`ready`. A ticket can be set to auto-start (`task add --auto-start`, or
`task auto ID on|off`, the TUI key `U` in the ticket detail or lane, or the
Auto-start checkbox in the web Details; default off). A card with auto-start on
shows an `auto` badge. When its last dependency reaches done, an
auto-start ticket that has a workflow starts itself: its first job fires and a
comment names the dependency (`auto-started: #12 done`). A ticket waiting on
several dependencies starts only when the last one is done. A ticket without a
workflow, or with auto-start off, still waits for a human to start it.
Auto-start does not run for a blocked or failed dependency. Cycles, itself and unknown ids are rejected when
saved; a deleted dependency stops blocking.
The Details tab lists dependencies and dependents with their status (clickable
in the web). In the web, **+ Add** picks a dependency from the board's tickets
(filter by id or title; done tickets sort last) and × removes one; no id is typed.

Priority is `high`, `normal` (default) or `low` (`task add --priority`, `task
priority ID ...`). Lanes sort by priority and then by the usual ticket order, and
busy/approval dispatch queues deliver higher priority first. A card shows only a
small `↑`/`↓` marker when the priority is not normal. In the TUI, `p` cycles the
priority and `D` opens the dependency picker (space toggles, Enter saves).

The dispatcher reserves each event before side effects. A status entry fires its
role with instructions to read the full ticket, do that status's work, then
comment+move. Questions for a human go to blocked. Returning review -> build
fires again and reuses the implementer's pane. Busy/approval/startup panes queue.
CLI move registers automated tickets and ensures the scraper so no existing pane
is needed. `@agent_scrape off` disables scraper-based automation; use board run
explicitly if desired. Web/TUI discover repositories as before.

## Scheduled triggers

`schedules.yml` fires a workflow ticket (the same path as `tagent workflow run`)
or queues a prompt to an agent, on a daily time or an interval:

```yaml
- name: morning-ci-triage     # unique per file
  at: "08:00"                 # daily local time, or every: 30m (min 1m); exactly one
  workflow: bugfix            # or send: {agent: receptionist, prompt: "..."}
  title: "Triage CI failures"
  inputs: {bug: "CI red on main"}
  # dir: ~/projects/x         # required for a global workflow; forbidden in a repo
  # enabled: false
```

- Local: `<repo>/.tasks/schedules.yml` runs only for trusted repos, listed one per line in `~/.config/tagent/schedule-trust`. A clone's file does not run until you `tagent schedule run|resume|trust` in it (`trust --remove` undoes it), so a repo cannot start work on its own.
- Global: `~/.config/tagent/schedules.yml`.
- A global `workflow` needs its `dir` registered on the board already: run `tagent workflow run` there once, or the schedule fails with a note. A global schedule never trusts or registers a repo.
- A blocked ticket (waiting on a human) longer than `@agent_blocked_hours` (default 24) gets one notification per blocked spell.
- State (last run, last ticket, note): `schedules.state.json` beside each file.
- Checked by the scraper once a minute, so it needs the scraper running (`@agent_scrape off` stops schedules too).
- The first check arms a new entry: it runs at its first slot after that, not at once. A run missed while the machine was off fires once, not once per missed slot.
- A workflow whose previous ticket is still open (not done or closed) is skipped; the state notes it.
- `send` reuses the agent's standing pane in that repo, spawns it when absent, and queues while it is busy.
- An invalid entry shows `file:line: problem` in `tagent schedule ls` and is disabled; its siblings still run.
- `tagent schedule pause|resume NAME` only changes `enabled` in the file.

## Shell checks and limits

Workflow jobs with `run:` execute `sh -c` asynchronously at the main repository root. Combined
stdout/stderr retains approximately 40 lines, bounded to 64 KiB. A ci comment
records exit code (startup errors use -1) and the tail, then ci moves to pass on
0 or fail otherwise; the next role dispatches normally. CI follows next too, so
include pass/fail if next is explicitly restricted. If the ticket has moved or
re-entered meanwhile, record a stale-result comment without overwriting it.
No execution timeout or cancellation scheduler is provided. Commands run with
local user privileges: only trust board configs you intend to execute.

`max-loops` counts rejections, not entries: status-change events from a looped step
to its on-failure status (all actors, across restarts, since the last exit from blocked).
A task retry is not a rejection and never blocks: it re-dispatches the current step. The step that hit the cap still runs once more, and the rejection that exceeds the
cap moves the ticket to blocked with a system reason such as
`review rejected 4/3; last findings: ...`. Safety blocking bypasses next and the fire cap.

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

tagent owns agent metadata. A role's harness, model and effort live only in
`.tasks/board.yaml` (`roles:`); the global default is
`~/.config/tagent/agents/profiles.toml`, and the board entry wins. A harness
directory never implies any of it. Instructions are separate: plain markdown
without frontmatter, in `.tasks/agents/<name>.md` (repo) or
`~/.config/tagent/agents/<name>.md` (global). Repo files shadow global ones.

One list of agent roles, in the TUI board and on the web (`#/agents`), merges
`roles:` entries and instruction files. It is never filtered by the process
harness: every agent shows under claude, pi, codex and opencode. An agent with no
`roles:` entry shows the process harness as its current harness.

- TUI: on the Agents card, `t` opens the Definitions tab (`t` again goes back to Running). `enter` shows detail, `esc` (or `enter`
  again) closes the detail and returns to the list, `e` edits the
  instructions in `$EDITOR`, `m` changes the model, `a` adds an agent, `x` deletes one (`y` confirms).
- Web: harness, model, effort and instructions (with preview) save together. Global
  and legacy agents are read-only; **Copy to repo** makes an editable `.tasks/agents` copy.

A role with `long_lived: true` in `roles:` keeps one standing pane for all tickets
instead of one per ticket. Its pane is keyed `<role>#` with no ticket, so the next
dispatch to the role reuses it, and a ticket reaching done kills only its own
`<role>#<ticket>` panes, never the standing one. It is not refreshed by default
(`fresh_after` still overrides), each dispatch sends its full prompt (no comment
cursor carries over from another ticket), and it runs in the main checkout, not the
ticket's worktree.

Everything the TUI, web and CLI write goes to `board.yaml` and
`.tasks/agents/<name>.md`; a new agent starts from a plain markdown template. A
legacy `.claude/agents`, `.codex/agents` or `.opencode/agents` file is still read
(shown as local) but never written or deleted. Model and effort apply to new panes
only. Running panes keep theirs.

When a role starts a pane, its instructions are included in the dispatch prompt for
every harness. A legacy file in the running harness's own directory is the only
case where the harness already loads the same file natively; it is not sent twice.

A role's memory file, `.tasks/memory/<role>.md`, is optional and is prepended to the
dispatch prompt on every dispatch, including a reused long-lived pane, so the role
carries over what it learned. A missing file adds nothing. It is never listed as a
role, and tagent never writes it: edit it by hand (`.tasks/*` is git-ignored, so it
stays local).

## Workflows

Local `.tasks/workflows/*.yml` recipes override names in
`$XDG_CONFIG_HOME/tagent/workflows` (default `~/.config/tagent/workflows`).
Discovery and configuration use the main worktree root. Native Claude/OpenCode
role definitions are passed inline from that checkout, so main-only or edited
roles do not fall back to stale definitions in the ticket tree. Reuse validates
the worktree's repository identity, expected branch and committed HEAD.
A recipe's top-level `workplace:` key is `git-worktree` (the default), `none`, `folder`, `browser`, `remote` or `docker`;
any other value fails validation on its line.
A top-level `cleanup:` key is `remove`
(the default) or `keep`, validated the same way; `keep` leaves a done ticket's worktree and
branch in place (its work is still merged). The web UI shows a ticket's workplace (type,
state, cleanup and location) in its Details tab, and a workflow's workplace and cleanup in
its header. The ticket view asks `GET /api/tasks/<id>/workplace` and reports a recipe that
cannot be resolved as an error, not as a worktree. With the default, workflow
execution lazily creates `.worktrees/workflow-<ticket>` from committed HEAD (a Git
repository with a commit is required). All agent jobs, mentions, shell jobs and retries for that
ticket share this directory; relative files survive handoffs and dispatcher
restarts. Different tickets are isolated. Moving a ticket to done commits any uncommitted work in its worktree on
`agent/workflow-<ticket>`, rebases that branch onto the main checkout's HEAD if
needed, and fast-forwards the main checkout to it (never forced or reset). When it rebased, the ticket's workflow run jobs (their `run:` commands, in workflow order) run again in the rebased worktree first, through `sh -c` with the output kept as the ticket's `merge failed` comment; a failing one blocks the ticket and leaves main unchanged. A rebase is needed only when main moved ahead, and a workflow with no `run:` job merges without a re-run. A rebase that stops only on conflicts under `internal/web/dist/` (both sides rebuilt the generated UI) takes the branch's side of those files and continues, so it needs no human. After any rebase, when the repo tracks `internal/web/dist`, `make web` rebuilds it from the rebased `web/src` before the run jobs, and the result is committed as `rebuild web dist` if it changed; so the merged dist matches a fresh build even when the rebase merged cleanly. A failed build blocks the ticket like a failed check. Any other conflict still aborts the rebase and blocks the ticket. Then the
worktree and merged branch are removed; one with untracked or modified files, or a detached HEAD, is kept (`cleanup: keep` skips this). If the merge fails, the ticket moves
to blocked with a `merge failed` comment and the worktree and branch are kept. The merge runs before done is recorded, so a failed merge leaves the ticket blocked, the command exits with an error, no done notice is sent and dependents do not start. Uncommitted files, ignored dependencies,
secrets and local-only scripts are not copied. Ordinary board execution is unchanged.
With `workplace: none` there is no worktree, branch or merge: the ticket's jobs run in
the main root, and its panes are reused like any other role's. Edits they leave are
uncommitted in the main checkout for a human to commit. On done, the ticket's output files
are copied into its attachments (shown by `tagent task show`), as with the default.
With `workplace: folder` there is no git: the ticket runs in its own directory
`<context>/<repo scope>/folder/<ticket>` (under the tagent context directory, outside the repo),
with no worktree or branch and no merge. Its panes run there and are reused. On done, every
regular file in it, dot files and subdirectories included, is attached to the ticket under its
base name (symlinks are skipped), whether or not a job declared it as an output, and then the
directory is removed. `cleanup: keep` attaches the files and keeps the directory. If an attach
or the removal fails, the directory and the panes are kept and a `cleanup: kept folder` note is
recorded; done still happens. Tools run inside the folder, such as `tagent task move`, act on the
repo's board. A harness may ask to trust each new folder on its first run; a human must answer that prompt.
With `workplace: browser` there is no git either: the ticket runs in the main root (no worktree, branch or merge),
and its dispatch prompt names two directories under `<context>/<repo scope>/browser/<ticket>`: `profile/`, a
Chrome profile (mode 0700, outside the repo so cookies cannot be committed), and `captures/`, for screenshots and
extracted text. On done, every regular file in `captures/` is attached under its base name, as with `folder`, and
the whole `browser/<ticket>` directory is removed, profile included; nothing in `profile/` is attached.
`cleanup: keep` attaches the captures and keeps both directories, since the profile holds sign-ins. A failed attach
or removal keeps the directory and panes with a `cleanup: kept folder` note. tagent does not start a browser; it
passes the profile to the `playwright` MCP server as `--user-data-dir=<profile>`, but only when the ticket is a browser
ticket and the role lists `playwright` in `mcp:` (the server must be in `.mcp.json` or `~/.claude.json`). Other servers
are unchanged. The profile flag is set when a pane starts, so a reused pane keeps its earlier args.
With `workplace: remote` the harness runs over ssh on another host, in the directory set by `remote:` in
`board.yaml` (one host and dir for all remote tickets of the repo, e.g. `remote: {host: devbox, dir: /srv/project}`).
The pane runs locally as `exec ssh -t -o BatchMode=yes <host> 'cd <dir> && exec <harness> …'`. `BatchMode=yes`
means key or agent auth and a known host key are required: a password or host-key prompt fails the pane instead
of receiving the prompt. The host needs the harness on its non-interactive PATH, a POSIX shell, and the repo at
`dir`. ssh keepalives end the pane after about 45 s of silence. The first non-blank screen counts as up, so an ssh banner can show idle before the harness starts; the first prompt may then be typed early. Status comes only from the local screen scraper, so `@agent_scrape` must not be `off`. There are no tagent MCP/hooks on remote agents:
their dispatch prompt says so, they cannot run `tagent task`, and a human moves the ticket. `run:` jobs are rejected in a remote recipe, and `long_lived` is ignored for remote tickets. Nothing is
copied and no git runs; on done the panes are closed and the ticket records `cleanup: remote files stay in
<host>:<dir>`, so files on the host are removed by hand. Remote tickets share `dir` and can overwrite each other.
With `workplace: docker` the harness runs in a container of the image set by `docker:` in `board.yaml` (e.g. `docker: {image: devbox:latest}`; the image must already exist, tagent builds nothing). The local `docker` CLI creates the ticket's container on first dispatch (`docker create --name tagent-<id> -v <dir>:/work -w /work <image> tail -f /dev/null`, then `docker start`; a re-dispatch only starts it again), and each pane runs `docker exec -it -w /work tagent-<id> sh -c '<harness> …'`. `<dir>` is the ticket's own directory under the shared context (`folder`-style, with its files attached on done). The image must hold the harness and its auth: tagent does not put them there. Like remote tickets there are no tagent MCP/hooks, `run:` jobs are rejected, and `long_lived` is ignored. On done the container is removed with its directory (`cleanup: keep` keeps both and records `cleanup: kept container tagent-<id>`, which keeps the panes open). A failed `docker rm` keeps the container and its directory and says so on the ticket.
Hooks and MCP are not supported for remote or docker agents yet.
On done, the ticket's chat and role panes are closed after the worktree removal, never
during it. A worktree kept for uncommitted changes keeps its panes, since its agents may still
be working in it. Only the done step closes panes; the dispatcher does not.
`tagent workplace prune` is a dry run unless `--yes` is given. It covers every ticket with a
`.worktrees/workflow-*` checkout or an `agent/workflow-*` branch. It removes one only when the
ticket is done or missing and its branch, if any, is merged into the main checkout's HEAD.
A usable git worktree must be on `agent/workflow-<ticket>` (a detached HEAD is kept) with no
untracked or modified files; files deleted from it are removed, since they are in the merged
branch. A checkout that is not a usable git worktree (a removal that stopped partway, or an
orphan left by a moved repository) is deleted directly once its branch is merged; the dry run labels it
`merged, unusable dir (untracked files not checked)`. It ignores
`cleanup: keep`. Anything else is skipped with its reason (not merged, uncommitted changes,
HEAD is not the ticket branch, ticket not done or running). Non-git
workplaces are out of scope. Folders are not pruned: they are removed on done.
Creation failures are reported before launch; retry requires a new status entry. Recipes are resolved
live: editing affects existing tickets; missing or broken recipes report errors.

`tagent workflow new NAME` creates a starter and opens `$EDITOR`.
`workflow list` shows scope, jobs and in-progress tickets.
`workflow run NAME --input goal=value --wait "Title"` starts a ticket, prints its
ID and optionally waits (done exits 0, blocked exits 1).
`task add --workflow NAME [--input goal=value] "Title"` sets the ticket's type and leaves it in todo. Without `--input`, the first required input takes the title text. The web Tasks add form has the same Type select.

The web Workflows page edits YAML with line numbers, syntax colours and soft wrap. A validation error is marked on its line; click the error bar to jump there. Clicking a flow card jumps to that job in the YAML, and moving the cursor into a job highlights its card. Global workflows are read-only there. The page edits recipes only; it does not show a ticket's run.

### Run view

A ticket's run is the drawer's **Run** tab (web) and the third tab of the TUI
detail drawer, from the same events: steps ran, loops and history. `#/runs/<ticket>?dir=<project>`
still opens the same run as a full page in that project (without `dir` it uses the picked project;
old `#/workflows/<wf>/<ticket>` links land there too). It shows that ticket's run only:

- header: title, workflow name, inputs used, overall status, total time, and **Edit workflow** to the editor;
- a compact flow strip of the steps with entry counts and the current one;
- one row per entry into a step, in run order: step and loop (`build #2 after review: changes`), role · harness · model, pane link to the agent view while that pane is alive, start/end/duration, result (pass, fail, changes requested, blocked, retry, moved or current), the handoff comment, published outputs, and the CI exit code with its `ci.log`.

Each row folds out the prompt that was actually sent. Prompt, pane, outputs and handoff are recorded per entry from the events log, so loops keep their own values. Runs recorded before that was stored show status moves, times, results and CI exits only; the page says so.

Jobs form one needs chain; parallel jobs are not supported. Each job has `run`
or `agent` with optional `prompt`, optional `status` (defaults to job ID),
`on-failure` (job/status, default blocked), `max-loops`, `model`, and `outputs`.
Agent-only jobs use generated handoff guidance. Model overrides apply only to
new panes; reused role panes retain their model.

Expressions `${{ inputs.goal }}`, `${{ ticket.id }}`, `${{ ticket.title }}`,
`${{ jobs.collect.outputs.file }}`, `${{ next }}` (the statuses this job may move the ticket to) and
`${{ status }}` (the job's own status) expand once at dispatch; unknown values are
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

Ticket rows include workflow names. `w` opens the selected workflow (or the
fixed lanes when the ticket has none), `[`/`]` browse recipes, and `e`/`c` edit
the displayed recipe.
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
`w` workflow, `?` help. Dashboard `Tab` changes focus; `Z` maximizes the focused card (`b` on Tasks too), `esc` restores.

### Role context

A role may limit what its agent loads, in `board.yaml` or on the Agents page
(Context block). Unset keeps the harness default; `[]` means none.

- `skills: []` (none) or `mcp: [names]`, `tools: [..]`, `allow: [..]`, `deny: [..]`.
- Claude: `--disable-slash-commands`, `--strict-mcp-config --mcp-config=FILE`,
  `--tools=`, `--allowedTools=`, `--disallowedTools=`. MCP names come from the
  project's `.mcp.json`, then `~/.claude.json`; `tagent` is generated.
- Pi: `--no-skills`, `--no-mcp`, `--tools=`, `--exclude-tools=`.
- Codex, OpenCode, Gemini, Copilot and Amp have no mapping: setting any field is a
  config error. Pi has no per-server or per-skill allowlist, so skill and MCP
  lists and `allow` are rejected there; Claude has no per-skill flag, so a skill
  list is rejected too.
- Plugin-provided MCP servers cannot be selected. Context applies to new panes only.
- Browser tickets: the `playwright` server gets `--user-data-dir` for the ticket's profile (see the browser workplace above).
An unsupported entry is a board config error for the whole `board.yaml`, as an
invalid role effort already is: every role's spawn fails until it is fixed.
This is intended, so a bad restriction never starts an agent with less
restriction than asked. The Agents page and API refuse such a save. Only a
hand-edited file can reach the error. A save that carries no context keys leaves
the role's context unchanged; the Agents page always sends all five.

The scout job uses a fixed Claude preset: Read, Grep, Glob, no skills, only the
`tagent` MCP server, and of its tools only `task_show`, `task_move` and
`task_comment` (`task_add` is denied). Checked live on Claude 2.1.332 with
stream-json init: `--tools` does not hide MCP tools, so the deny list is what
removes `task_add`. Scout does not apply on pi.

### Scout and skip rule

`uses: scout` expands to a job with status `scouting`, role `ba`, output `scout`
and the built-in prompt. It is skipped (advances without running) when the
ticket's title, text or inputs name a source file (`*.go`, `*.ts`, ...) or a
function (`name()`), or when the job already has a `scout` output (a CI or QA
bounce reuses it). This is a regex heuristic: it can false-positive (e.g. "Node.js")
or miss a name, costing one skipped or one extra cheap scout. A scout output over
8000 characters is refused: shorten it and resubmit.

### CI bounce and Haiku context

A CI failure sent back to a build status reuses the existing dev pane (no fresh
pane) and gets the full log path (`Full CI log: ...`). A dead pane still starts
fresh. Identical repeated CI failures are still blocked by `repeatedFailure`.
Haiku panes keep the harness's default compaction window. A forced 100K window
(tried in #67) made Claude Code's autocompact thrash on a real UI ticket (#68): the
fixed context plus the files being edited refilled it within three turns. Keep Haiku
contexts small by splitting tickets and starting from the scout/plan output instead.

### Rollout (after merge)

The change is inert until the binary is installed and the workflows are copied.
1. Install the new binary; restart the board dispatcher and web.
2. Copy `examples/workflows/*.yml` into `.tasks/workflows/` (merge local edits).
3. Optionally set role context on the Agents page.
4. Re-run one feature and one bugfix ticket; compare with `task show ID --all`:
   fewer CI loops, lower lead tokens, and scout
   skipped when files are named.

Verified in development: claude 2.1.332 tool and MCP flags (stream-json init
event). Not verified: the Haiku 100K compaction window with `/context`, and the
token acceptance numbers, which need the rollout runs above.

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
