# Agent Chat view

On the web, each agent has two display modes beside **Handoff**:

- **Terminal** — the agent's tmux pane as an xterm (unchanged).
- **Chat** — the conversation rendered from the harness's own session transcript.

The agent keeps running in tmux either way. Switching modes does not change who owns the pane size (Handoff/Release).

## Which agents

Chat works for `claude` and `pi` agents whose transcript can be found and read. For other harnesses (codex, opencode, …) the toggle is disabled with a tooltip and Terminal is used.

The mode is remembered per agent in this browser. The default is Chat on phones and Terminal elsewhere.

## What Chat shows

- User and assistant messages (Markdown), newest at the bottom. The feed follows new messages until you scroll up.
- Each tool call as one line (`Edit Board.tsx`, `Bash go test ./...`); tap to see its input and output. Reasoning is collapsed.
- A status line: working (with the current tool), waiting for approval, idle, or error.
- A composer: Enter sends and Shift+Enter adds a newline on desktop; the send button works on touch. Messages go through the same queue as `tagent send`: while the agent is busy or waiting, they wait and are delivered when it is idle. Queued messages are listed.

## Approvals

Chat never answers an approval. When the agent is waiting, Chat shows the prompt's screen excerpt and an **Open in Terminal** button. Answer it in Terminal.

## Limits (v1)

- Only the end of a long transcript is shown (the last 4 MB, at most 200 items per load). Older history stays in Terminal.
- Text is updated per transcript block, not per token. Updates arrive about every 1.5 seconds while the page is visible.
- Output of large tool results is cut (8 KB). Message text is cut at 32 KB.
- Not yet: approval buttons in Chat, codex and opencode transcripts, running agents through an SDK.

## How it works

`GET /api/agents/<pane>/transcript?after=<cursor>` reads the pane's session (id from the agent hooks) and returns the active branch of its transcript as items `{id, role, text, tool, time}`. `after` is the last item id the page has; the response holds only newer items, or `reset` when that id is gone (for example after `/clear`). Parsers live in `internal/transcript` (`claude.go`, `pi.go`). Sending goes to `POST /api/pane/send`.

## Per-ticket chat (backend)

A ticket's chat is an ordinary agent pane (claude or pi) dedicated to that
ticket; there is no separate harness. Chat stays on the same path as above:
messages go to `POST /api/pane/send`, the feed comes from the transcript
endpoint, and the pane keeps running in tmux.

- **Scope key**: `tasks.TicketScope(dir, id)` = `<project scope>#<ticket id>`,
  e.g. `tmux-agent-1a2b3c4d#12`. The project scope is shared by all worktrees.
  It is stored in the pane option `@pane_ticket`.
- **Spawn and reuse**: `tagent chat [--dir DIR] [--agent claude|pi] [--model M] [--clear] <id>`
  reuses the pane bound to the key, or starts one with the normal spawn path. The
  first message is `tasks.ContextFor(scope, id)`: ticket title, description,
  status and workflow step/next, assignees, due, note, inputs/outputs, comments
  and attachment paths. Reusing a pane never re-sends the context; `--clear`
  closes the pane, drops its queue and starts again from the context.
- **API**: `GET /api/tickets/<id>/chat?dir=<repo>` reports the pane
  (`{key,pane,harness,status,available,reason}`) without starting anything;
  `POST` with optional `{agent,clear}` starts or reuses it and returns the same
  shape plus `created`. A missing ticket is 404. `available:false` means a pane
  bound to the ticket runs a harness Chat cannot read (not claude/pi).
- **Tools bound to the ticket**: from a pane with `@pane_ticket`, `tagent task`
  accepts only that ticket's id (plus read-only `ls`); `add` and other tickets
  are refused. This guards the CLI, not a shell that unsets `TMUX_PANE` or runs
  elsewhere.

The conversation itself lives in the pane transcript, so it is the same for
Web and TUI and disappears when the pane is cleared. The older per-project
receptionist/intake harness was removed; this path replaces it.
