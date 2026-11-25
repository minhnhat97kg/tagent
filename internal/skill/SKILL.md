---
name: tagent
description: Read this first for any question or action on tickets, tasks or the board (how many are open, list, show, move, comment, add). Uses the tagent CLI or its MCP tools.
---

# tagent tickets

## Do NOT
- Approve or answer another agent's permission or trust prompt. Quote it to the human.
- Sleep or poll in a loop. Use harness notifications; if there are none, say so.
- Delete tickets. There is no delete tool for agents; `task rm` is for humans.
- Send prompts to your own `$TMUX_PANE`.
- Start other agents, subagents or panes unless your prompt says to.

## Tools first
If the `tagent` MCP tools are listed (`task_show`, `task_move`, `task_comment`, `task_add`), call them. Otherwise run the CLI below. `{{BIN}}` is the tagent binary.

## Example: a dispatched ticket
Your prompt gives the ticket ID, your status and the allowed next statuses. Work only on that status.

```sh
{{BIN}} task show 12 --brief
# ...do the work in this session and verify it...
{{BIN}} task move 12 review --output summary="fixed login redirect" "Tests: go test ./internal/auth passes. Changed: internal/auth/redirect.go. Next: review the diff."
```

MCP equivalent:
- `task_show {"id": "12"}`
- `task_move {"id": "12", "to": "review", "outputs": {"summary": "fixed login redirect"}, "comment": "Tests: go test ./internal/auth passes. Changed: internal/auth/redirect.go. Next: review the diff."}`

Need a human decision? `{{BIN}} task move 12 blocked "Should the redirect keep the old path?"`

## Other questions
- Open (not done or closed) tickets, one per line `id<TAB>status<TAB>assignees<TAB>title`: `{{BIN}} task ls | awk -F'\t' '$2!="done" && $2!="closed"'`
- One ticket: `{{BIN}} task show ID`. Comment: `{{BIN}} task comment ID "text"`. New ticket: `{{BIN}} task add --desc "context, steps, acceptance" "Short title under 80 chars"` (MCP: `task_add {"title": ..., "description": ...}`). Never put the details in the title.

## Rules of thumb
- Handoffs are short: checks run, changed paths, next step. Put data in outputs or files, not long comments.
- Blocked means exactly one concrete question.
- Use `task_show` or `task show ID --brief` only for missing context.
- Mention another role with `@role` in a comment to notify it.

## Reference
Full CLI, workflows, triggers, fanout and board editing: `docs/agent-reference.md` in the tagent repo. MCP setup: `docs/mcp.md`. Board schema: `docs/board.md`.
