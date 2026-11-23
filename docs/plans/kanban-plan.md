# Kanban implementation plan

Planning only; based on `kanban-spec.md` and the current checkout. No new Go or npm dependencies: use existing `gopkg.in/yaml.v3`, Go stdlib, React/shadcn primitives and native drag-and-drop.

## Current integration points

- Task JSON lives in `sharedctx.Dir(scope)/tasks`, not repo `.tasks`. `tasks.Scope` shares a board across worktrees using git-common-dir; `workflow.LocalDir` uses the current worktree's `RepoRoot`.
- All task writes already use `tasks.withLock`; `write` atomically replaces a task. Existing mutations are `Add`, `Update`, `Edit`, `SetNote`, `Remove`.
- `startWorkflowAgent` calls `spawnAgent(spawnOpts{... Wait:true})`, selecting `Agent` for `harnesses` and `Role` for definitions. `spawnAgent` already resolves profiles/models/instructions, sets pane names, places panes and creates `hiddenSession` outside tmux.
- `runWorkflow` deliberately creates and closes a fresh agent per job; leave that lifecycle intact. Board agents should persist.
- `cmdSend` in `cmd/tmux-agent/agentcli.go` queues running/background/waiting panes via `queue.Push`, otherwise uses `tmux.TypeLine`. `internal/scrape/scrape.go` drains the queue when idle. Web `send` currently types directly, so it is not the dispatcher delivery path.
- Actual API is `POST /api/tasks` (plural). Route parsing is `web/src/hooks/useRoute.ts`; there is no `internal/hooks/route`. The installed agent skill source is `internal/skill/SKILL.md` (embedded by `skill.go`), not an assets skill file.

## 1. Define config and mutation contracts

Files: add `internal/tasks/config.go` and `config_test.go`; change `internal/tasks/tasks.go` and `tasks_test.go`.

Put the small shared config types/parser in `tasks`, exposed as `LoadBoardConfig(dir)`, `ParseBoardConfig`, `SaveBoardConfig`, `BoardConfigPath`, `DefaultColumns`. `internal/board` will consume them; tasks must not import board or workflow (workflow already imports tasks). Match `workflow.LocalDir` root resolution with git `--show-toplevel`; respect XDG config for the global file. Defaults are `todo,doing,review,blocked,done`, no triggers, limit 20. Local file replaces global config as a whole; a missing local file falls back to global. Missing fields receive defaults; an explicit empty triggers list disables triggers. Invalid config returns an error and disables dispatch; never silently dispatch using a fallback after a parse failure.

Validate unique nonempty columns (require `todo` and `done` initially), positive fire limit, known event/filter keys, exactly one agent/workflow target, nonempty target and valid template variables. Use existing YAML parser with known-field validation. Save valid config via temp+rename; editor retains invalid drafts and shows validation errors.

Preserve scope-only public calls using an optional `MutationOptions{Dir, Actor}` argument on existing mutators; add it to new mutators too. Production writers must pass Dir and Actor; scope-only calls use defaults for tests/legacy callers. Validate Dir maps to scope. Config is loaded per mutation so CLI/web/TUI agree after edits; do not use a process-global current-column map. `tasks.CallerActor()` resolves `$TMUX_PANE` through `tmux.QuerySessions`, choosing pane Name, then Agent, else user. Web explicitly supplies user, and dispatcher supplies system; do not attribute browser edits to the server's inherited pane.

Data formats (lowercase JSON field names):

- Task keeps all old fields including note/completed_at and adds `title`, optional `due` (`YYYY-MM-DD`), and `comments` (array). Read old records with title fallback to first line of text and empty comments. Initial title derives from text; explicit title edits remain independent of description thereafter.
- `Comment{ id, author, body, created, attachments:[{name,file}] }`; created is RFC3339 UTC. File is a relative basename under `<boardDir>/<task-id>/`, never a source path. Add `Comment`, `SetDue`, `Mentions`, and `EditDetails` for independent title/text changes; keep `Edit` compatibility.
- `Event{seq:int64,time:timestamp,task:id,kind,actor,from,to,comment_id,names}`; from/to are strings. Status events use old/new status; assign events emit once per added/removed assignee (`from:"",to:name` or `from:name,to:""`). Create emits create, then assign events for initial assignees so `add --to planner` can wake planner. Comment emits comment plus one mention event with deduplicated names; dispatcher expands names into individual `event.name`/`event.to` matches. Due records old/new date; it means a date change, not a scheduled alarm.
- Clarify the spec's “one line per mutation” as one line per semantic change: Update changing status and assignees and Comment containing mentions require multiple lines. No-op updates produce none. Extend logging to Edit/SetNote/Remove with non-triggering `edit`, `note`, `delete` kinds so every mutation is audited without accidentally waking comment triggers.

Expose `BoardDir(scope)`, `ReadEvents(scope,after)` and a path-safe attachment opener for board/web use. Under `withLock`: load/validate, stage attachment copies, update task, append complete newline-terminated event records with monotonically increasing seq. Recover seq from the last complete record; tolerate only an incomplete final line. Return persistence errors. Copy files with generated collision-free storage basenames, retain display names, reject symlinks/nonregular files and traversal. `--diff` uses the same copy path. System fire comments append a comment event with actor system, but dispatcher explicitly ignores its own audit comments.

Acceptance: old files unchanged on read; custom status validation; done timestamp on entry and cleared on exit; date validation/clear; stable mention ordering/boundaries; assignment differences; concurrent increasing seq; no-op suppression; unsafe attachment rejection. See crash limitation below before implementing persistence ordering.

## 2. Add CLI commands and cover every writer

Files: change `cmd/tmux-agent/task.go` (`cmdTask`), `dispatch.go`, `help.go`, `internal/skill/SKILL.md`; add `cmd/tmux-agent/board.go`, `task_test.go`, `board_test.go`.

- Add show/comment/due; parse repeatable `--attach` and `--diff` without losing comment text. Run `exec.Command("git","diff","HEAD")` with cmd.Dir=cwd, check errors, stage output as diff.patch, clean temp files. Show full ticket and resolved attachment paths. Due `-` clears. Set help refers to configured columns.
- `cmdBoard`: `run [--dir DIR]` and `init [--dir DIR]`. Init exclusively creates `.tasks/board.yaml`, never overwrites; example lead → planner → implementer rules plus mention rule, using roles users can define. Register in command dispatch/help and document show/comment/due/mentions/attachments.
- Pass mutation options from CLI, `internal/web/server.go:taskOp`, `internal/sidebar/task_actions.go:handleTaskForm`, the task mutation branch in `internal/sidebar/model.go:Update`, and task mutation branches in `internal/watch/watch.go:updateKeys/updateInput`. Replace hardcoded status choices/hints/cycles in sidebar/watch and TaskDialog with configured columns; preserve metrics.Link/End and existing notifications.

Acceptance: CLI show includes comments/due/title; diff attachment matches git diff HEAD including staged/unstaged changes; clear assignment/due; all existing writers produce events and accept custom columns.

## 3. Implement dispatcher and bridge existing execution helpers

Files: add `internal/board/board.go`, `board_test.go`; extend `cmd/tmux-agent/board.go`; change `cmd/tmux-agent/web.go:cmdWeb`, `internal/web/server.go:Serve/server/state/project`, and `internal/workflow/workflow.go` for `StartDetached`.

`board.Run(ctx,dir,fire)` owns a nonblocking flock on `<boardDir>/dispatch.lock`, polls complete JSONL records, reloads valid config on changes, and persists `<boardDir>/dispatch.state` using temp+rename. Lock is separate from tasks .lock; never hold the task lock while launching agents. A single callback `fire(Action) error` bridges package board to main's unexported CLI helpers without moving spawn machinery or adding a service framework. `cmdWeb` passes this callback into web.Serve; `cmdBoard run` uses it directly.

State JSON: `{last_seq, pending:{seq,next_action}|null, fires:{task_id:[RFC3339 times]}, last_error}`. Process events in sequence and matching trigger order, including each mentioned name. Pending progress supports multiple actions per seq; do not dedupe solely by seq before all its actions complete. Persist rolling-hour reservations before firing; prune old timestamps. Skip actor equal to resolved agent target, system audit comments and over-limit actions. For workflows apply self guard to triggering actor versus workflow target name (not its temporary wf-* panes); the rate limit remains the final loop guard. Supply default prompts for every supported kind. Use a small literal `{{...}}` substitution function with an allowlist, no executable templates; assignees render comma-separated, absent comment values empty. Render agent/workflow/inputs/prompt consistently, reject unresolved variables. Reload task and locate comment by comment_id before rendering; deleted task is skipped with a diagnostic.

Main callback `dispatchBoardAction`:

1. Enumerate `tmux.QuerySessions`; consider only agent panes whose `tasks.Scope(p.Path)` matches scope and whose Name matches target (unnamed harness match only when unambiguous). Prefer an idle pane; if the named pane is busy/waiting, reuse it via `cmdSend`, queuing rather than multiplying agents. Never use `--now`. Unknown/startup status must queue until positively idle, rather than cmdSend's current unknown-status direct send; add a narrowly scoped guard/helper in `agentcli.go` if necessary.
2. If none exists, mirror `startWorkflowAgent`'s harness/definition selection with `spawnOpts{Cwd:dir,Name:freeName(target),Wait:true}`, setting Agent or Role from `harnesses`. Call `spawnAgent`; retain warning/pane in diagnostics. Re-query scope/name before spawn to reduce races. If global name collision forces a suffix, record target→pane mapping in dispatch state and validate it before reuse. Use `cmdSend` for the initial prompt too. Do not use `runAgentJob`, which adds a report wait and workflow lifecycle.
3. Workflow action resolves `workflow.Find(dir,name)`, validates required inputs, then calls proposed `workflow.StartDetached(dir,file,inputs)`. Extract the executable/argv/Dir/Setsid/Start/Wait logic from web `workflowRun` into that helper and have both callers use it. Preserve `workflow.Load`/`LastRun` checks and argv-based `--input k=v`; no shell interpolation. Add a per-workflow launch lock spanning the actual run lifecycle to prevent web/board double-starts, since LastRun check alone races. `cmdWorkflow` still reaches unchanged sequential `runWorkflow`.
4. On accepted send/queue or successful process start, append `tasks.Comment(...,"system","▶ triggered ...",...)` with source seq/trigger identity in body; this records dispatch acceptance, not agent completion. Failures persist a diagnostic and stop that action for manual retry on next restart; no tight retry/spawn loop. Document replay behavior.

Web starts one cancellable dispatcher goroutine per tasks.Scope, for server dir immediately and roots discovered by state/project/board endpoints. A synchronized scope map prevents duplicate goroutines; background discovery ticker means dispatch does not depend on an open browser polling state. Stop on server shutdown; an already-held scope lock means another web/board run owns it. Queue delivery still requires the existing scraper; verify it is running for standalone board use and document/start via the existing scraper lifecycle if needed, rather than adding another queue consumer.

Acceptance: filter/from/actor/wildcard, templates/defaults, multi-mention dedupe, self guard, system suppression, persisted cap/restart cursor, competing dispatcher lock, partial final line, multi-trigger progress, queued busy/waiting pane, correct role/harness startup outside tmux, workflow input forwarding and launch exclusion. Use callback fakes for dispatcher tests and isolated tmux for launch integration.

## 4. Add authenticated board/task APIs

Files: change `internal/web/server.go:Serve/taskOp/project/workflowRun`; add `internal/web/board.go`, `board_test.go`.

- Keep POST /api/tasks; add POST /api/task alias for spec compatibility. JSON operations retain existing PascalCase request fields and add assign (empty array clears), due, comment, edit with Title/Text presence fields, and add with Status for column '+'. New status-at-create must be set in the same locked mutation, not add then set.
- Multipart comment fields: `Dir,Op=comment,ID,Text`, repeated `files`; author always server-resolved user, not client input. Bound total body (proposed 20 MiB), count (10), per-file (10 MiB); clean multipart temps on success/error. Task package handles copying/storage safety. Return resulting task JSON for writes; preserve old clients that ignore responses.
- GET /api/board?dir= returns `{columns,tasks,triggers,max_fires_per_hour}`; GET /api/board/config?dir= returns `{text,file,source,err?}`; PUT accepts `{Dir,Text}` and validates before atomic local save. Global fallback is visible but saving creates local override.
- GET /api/task/file?dir=&id=&name= verifies task ID, stored attachment membership, basename, resolved containment and no symlink escape. Serve only attachment bytes with safe disposition; patch/diff can be fetched as text/plain, all other downloads forced attachment with nosniff. No arbitrary filesystem path parameter.
- Use existing auth middleware for every new endpoint; validate directory exists and normalize via projectDir. Return config/mutation failures to clients. Include current columns in project response for sidebar controls.

Acceptance: httptest for JSON/multipart operations, assignment clears, invalid YAML/date/status, limits/cleanup, traversal/symlink/nonmember file refusal, auth coverage and old plural route compatibility.

## 5. Build board, ticket and config UI

Files: change `web/src/hooks/useRoute.ts:Sel/parseSel`, `App.tsx`, `types.ts`, `components/panels/TasksPanel.tsx`; add `components/BoardView.tsx`, `TaskView.tsx`, `BoardConfigView.tsx`. Change `api.ts` only if a tiny FormData helper improves clarity.

- Routes `#/board`, `#/task/<encoded-id>`, `#/board/config`; App mounts in main for selected dir, keyed by dir/id. Preserve existing agent/workflow routes. Reload/usePoll at existing 2s cadence, without overwriting dirty form state. Keep selected project explicit when following task links; on project switch use that project's board or show missing task (IDs alone do not encode scope).
- Board columns from API; cards show title fallback, assignees/due/comment count. Native drag/drop posts status and reloads; status select in ticket provides keyboard/touch alternative. Column '+' posts initial status atomically. Render old tasks whose status disappeared in a visible warning section so config edits never hide tasks.
- TaskView has independent title/text editing, dynamic status select, assignee checkboxes from project roles plus running names in the same task scope (retain already-assigned names), native date input, comments and FormData upload. Highlight mentions as React text spans; no raw HTML. Fetch patch/diff with bounded inline text previews in pre; other attachments link to the protected file endpoint. @autocomplete uses the same names with keyboard support. Preserve note, delete and missing-task feedback.
- TasksPanel adds Board link and opens routed ticket instead of local modal. Remove `TaskDialog.tsx` if no callers remain; otherwise pass configured columns to it and delete its hardcoded STATUSES. BoardConfigView copies WorkflowView's YAML/dirty/save/error interaction with a textarea; no YAML editor package and no need to modify WorkflowView itself.

Acceptance: manual desktop/mobile/keyboard checks for board navigation, drag/status fallback, add in custom column, edit independent fields, multiassign/clear, due/clear, mentions/autocomplete, upload/download/patch display, invalid config and project switching.

## 6. Integrate and verify

Run focused tests for tasks/board/CLI/web, then `TMPDIR=/tmp go test ./...`; report any failure with actual output. The spec calls `assets/sidebar_layout_test.go` obsolete; do not silently delete/skip it or claim full green if it fails. Run `npm run lint` and `npm run build` in web, and `make web` to regenerate committed `internal/web/dist` with the existing Makefile (no dependency changes intended). Review lockfile drift before retaining it. Finish with `git diff --check`.

In an isolated repo and tmux server, prove: create/assign → planner starts or reuses → comment/due/reassign → implementer → diff comment; external mention queues a response; same actor/system comments do not loop; restart does not replay completed actions; web and standalone dispatchers cannot both own scope. Verify scraper queue draining and startup approvals remain intact. No install/reload/push is part of this implementation plan.

## Risks and decisions to confirm before implementation

1. Worktree ambiguity: shared task scope versus worktree-local board.yaml. Proposed policy is a persisted owner root in dispatch.state, chosen by first dispatcher; reject a different worktree owner with a clear error and have all writers validate against the owner's config. Owner registration/read belongs under task lock; absent owner uses caller dir. Decide whether canonical main-worktree config is preferable before phase 1; do not let lock acquisition silently choose conflicting trigger rules.
2. Task replacement, JSONL append, dispatch-state save and external send/start are separate filesystem/process operations. Flock is concurrency control, not a transaction. Proposed minimal guarantee is best-effort at-most-once dispatch: persist action reservation before execution; an interrupted reservation is marked uncertain and never auto-replayed. A crash can lose a fire or leave a task changed without its event if append fails. If durable no-loss mutations are required, add a small write-ahead pending-mutation record and recovery under withLock; exactly-once external delivery still needs recipient idempotency. Agree this guarantee explicitly rather than promise exactly-once.
3. Spec contradictions resolved provisionally above: busy named agents queue rather than spawn duplicates; multiple semantic changes require multiple events; edit/note/delete need non-triggering audit kinds. System audit events must bypass triggers or wildcard comment rules recurse. A due change is not a clock-based reminder.
4. Title starts as first line of Text, but independent editable title requires persistent override semantics. Removing configured columns leaves existing tasks visible but invalid for new writes; no automatic migration. Requiring todo/done simplifies compatibility; confirm arbitrary columns without these are required before changing Add/completion semantics.
5. Existing workflow state/report paths are keyed by name and job; concurrent same-name runs collide. Enforce the proposed run lock rather than invent run IDs here. Agent name lookup is global while tasks are scope-specific; suffix mapping and scope checks are required. Queue delivery depends on scraper runtime, and initial unknown agent status must not accept typed prompts.
