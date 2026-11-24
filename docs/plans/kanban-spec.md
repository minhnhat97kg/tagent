# Kanban board with triggers: spec

The goal is to turn the task board into a kanban that drives agents. For example:
- a lead agent reads a request and creates tasks;
- assigning a task to the planner wakes it, and the planner sets the due date and comments a plan;
- the task is then reassigned to the implementer, which comments with the git diff attached when it finishes;
- anyone can @mention an agent to ask a question, and the mentioned agent re-reads the ticket and replies in a comment.

Every trigger is configurable. A trigger can start an agent or run a workflow.

## Data (internal/tasks)
- Extend `Task` in a backward-compatible way, so old JSON files still load:
  - `Title` (first line of Text; keep using `Text` as the description)
  - `Due` (date string, optional)
  - `Comments []Comment`
- `Comment{ID, Author, Body, Created, Attachments []Attachment}`, where `Attachment{Name, File}`.
  - `File` is stored under `<boardDir>/<task-id>/` (the files dir).
  - The body can mention agents as `@name`, where name matches `[A-Za-z0-9_.-]+`.
- Columns (statuses) come from config. The default is `todo, doing, review, blocked, done`.
  - `Update` validates against the configured columns instead of the hard-coded map.
  - "done" stays special for CompletedAt.
- New functions: `Comment(scope, id, author, body string, attach []string /*source paths copied in*/)`, `SetDue`, `Mentions(body) []string`.
- Writes go through the existing `withLock`.

## Event log
- Every mutation appends one JSON line to `<boardDir>/events.jsonl`. This includes Add, Update (status and assignee changes, recorded separately), Comment and SetDue.
- Line shape: `{seq, time, task, kind: create|status|assign|comment|mention|due, actor, from, to, comment_id, names}`.
- Appending is done under the same lock.
- `actor` is the pane name taken from `$TMUX_PANE` (as tmux.PaneInfo.Name or the agent kind), else "user".
- This lets triggers react to changes from any writer (CLI from agents, web, sidebar TUI) without diffing.

## Triggers config
- Location: `.tasks/board.yaml` at the repo root (same root logic as workflow.LocalDir).
- Optional global file: `~/.config/tagent/board.yaml`.
- Missing files mean the default columns and no triggers.

```yaml
columns: [todo, doing, review, blocked, done]
triggers:
  - on: assign            # create | assign | status | comment | mention | due
    when: {to: planner}   # filters: to (new assignee / new status / mentioned name, "*" = any), from, actor
    agent: planner        # start/reuse an agent (definition name or harness)  -- OR --
    # workflow: code-review   # run a workflow; inputs below
    inputs: {task: "{{task.id}}"}
    prompt: |
      You were assigned task {{task.id}}: {{task.title}}.
      Read it with `tagent task show {{task.id}}`, add a plan as a comment, set a due date, then assign it to implementer.
  - on: mention
    when: {to: "*"}
    agent: "{{event.name}}"   # the mentioned agent
    prompt: "{{event.actor}} mentioned you on task {{task.id}}: {{comment.body}}. Re-read the ticket and reply with a comment."
```

- Template variables:
  - `task.id`, `task.title`, `task.text`, `task.status`, `task.due`, `task.assignees`
  - `event.kind`, `event.actor`, `event.from`, `event.to`, `event.name`
  - `comment.body`, `comment.author`
- If a trigger has no prompt, a sensible default prompt per event kind is used.

## Dispatcher
- New package `internal/board`. It tails `events.jsonl` from the last seen seq, stored in `<boardDir>/dispatch.state`, matches triggers and fires them.
- Agent target:
  1. If a running pane with that name is idle, send it the prompt (reuse the existing send/queue path, which queues when busy).
  2. Otherwise start a new agent from the definition, the same way `startWorkflowAgent` does, name it, and send the prompt.
- Workflow target: run it detached, the same way the web `workflowRun` does, and pass `inputs`.
- Loop guards:
  - Never fire on an event whose actor equals the target agent, except a mention from someone else.
  - Allow at most N fires per task per hour (default 20, configurable as `max_fires_per_hour`).
  - Dedupe by seq.
- Each fire is recorded as a system comment on the task, for example "▶ triggered planner (assign)", so the user can see it on the ticket.
- Where it runs:
  - inside `tagent web` (a goroutine per project scope that the server knows about);
  - as `tagent board run [--dir]` for TUI-only users.
  - A lock file prevents two dispatchers on one scope.

## CLI (cmd/tagent/task.go)
- `task show <id>`: title, status, assignees, due, text, comments with attachment paths. This is what agents read.
- `task comment <id> [--attach path]... [--diff] <text>`
  - `--diff` attaches `git diff HEAD` of the cwd as `diff.patch`.
  - The author is the caller pane's name.
- `task due <id> <YYYY-MM-DD|->`
- `task set` accepts the configured columns.
- `board run`
- `board init` writes an example board.yaml with lead → planner → implementer triggers.
- Update help text and the tagent skill (assets) so agents know these commands and how @mentions work.

## Web (web/, internal/web/server.go)
- Route `#/board`: a kanban view in `<main>`. It has one column per configured status, and task cards show title, assignees, due date and comment count.
  - Native HTML5 drag-and-drop changes status (no new dependency).
  - A "+" in a column adds a task.
- Clicking a card opens `#/task/<id>`, a detail view with:
  - an editable title/text;
  - status select, assignees (multi, from roles + running agent names) and a due date input;
  - a comment thread (rendering @mentions highlighted and attachments as links, with `.patch` and `.diff` shown inline in a pre);
  - a comment box with file upload (multipart) and @-autocomplete.
- Triggers editor: a YAML textarea for board.yaml, with save and validation errors shown, like the workflow editor.
- The TasksPanel in the sidebar gets a "Board" link.
- API:
  - `GET /api/board?dir=` returns columns, tasks and triggers.
  - `POST /api/task` gains the ops `assign`, `due` and `comment` (multipart for attachments).
  - `GET /api/task/file?dir=&id=&name=` downloads an attachment, with a path-safety check.
  - `GET` and `PUT /api/board/config`.
- Style: follow the existing Ink palette and shadcn components. Rebuild `internal/web/dist` with `make`.

## Tests
- tasks: comment/mention parsing, old JSON compatibility, event log appends.
- board: trigger matching, templating, loop guard.
- CLI: `task comment --diff` and `task show`.
- `TMPDIR=/tmp go test ./...` must pass. Note that `assets/sidebar_layout_test.go` is known to be failing and obsolete.
