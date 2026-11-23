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
