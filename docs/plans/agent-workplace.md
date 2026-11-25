# Agent → Task → Workplace (draft)

Goal: tagent becomes a general agent workspace (like Hermes Agent / OpenClaw), not only a
coding board. **tmux stays the root**: agents are tmux panes, so any harness (claude, codex,
pi, opencode, ...) can be swapped in. Web, TUI and Telegram stay front-ends.

## Model

```
Agent      named, long-lived instance of a role (harness, model, memory, skills)
 └─ Task   has a type (code | research | ops | chat | ...) → picks workflow + workplace type
     └─ Workplace?  0..1, typed:
          git-worktree  output: diff      gate: merge on done succeeds
          folder        output: files     gate: always passes (implemented, ticket 93)
          remote        output: logs/files (ssh/docker)
          browser       output: captures      gate: always passes (implemented, ticket 97; no MCP wiring yet)
          none          output: comment/report, no gate
```

A workplace type defines: create, cleanup, what its output is, and its review gate.
Today's `.worktrees/workflow-<ticket>` becomes the `git-worktree` type.
Local PR feature dropped (#79–#83, #87 closed); the `git-worktree` gate is just "merge on done succeeds".

## Open questions (for research)

- Current package map and where ticket ↔ worktree ↔ pane are coupled.
- Phases that keep the running board working after each merge.

## Decisions after research #86 (.tasks/research/86.md)

- Task type = the ticket's workflow (already called "type" in task add). The workflow gets a
  `workplace:` key (`git-worktree` default | `none`). No separate Task.Type.
- New package `internal/workplace`: interface Create / Cleanup / Output / Gate, impls
  `git-worktree` (wraps today's code; gate = merge succeeds) and `none` (exec dir = main root, gate always ready).
- Phases: (0 dropped) 1 seam (no behaviour change) → 2 gate hook + no done on failed merge
  → 3 `none` + `workplace:` key → 4 outputs per workplace. Persisted Agent entity, remote/browser
  types: later, only when a ticket needs them. The `folder` type is done (ticket 93): a plain directory
  per ticket outside git; every file is attached on done, then the directory is removed. The `browser`
  type is B1 done (ticket 97): a main-root run with a per-ticket Chrome profile and captures; only the
  captures are attached. B2 (pointing an MCP browser at the profile) is open.
- Local PR feature dropped by the human (2026-11-25).
