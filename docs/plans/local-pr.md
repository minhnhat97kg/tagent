> **Dropped 2026-11-25 by the human.** Kept for reference only; not being built.

# Local PR (worktree → main)

Each workflow worktree (`agent/workflow-<ticket>`) is a local pull request into `main`.
A PR record says whether it is ready to merge. Only tagent decides that.

## Record (facts only)

Stored next to the ticket store (sharedctx), never inside the repo or a worktree.

```yaml
id: 81                # = ticket id
source: agent/workflow-81
target: {kind: local, branch: main}   # or {kind: github, repo: owner/name, branch: main}
checks:               # written only by the dispatcher after a run: job
  test: {sha: 9b7e4d2, result: pass, log: <path>}
approvals:            # agent review job pass, or the human via web/CLI
  - {by: qa, sha: 9b7e4d2}
  - {by: human, sha: 9b7e4d2}
changes_requested: [] # same shape; latest per reviewer wins
merged: {sha: ..., at: ..., forced: false}
```

## Derived on every read (never stored)

- head, base, ahead/behind: from git.
- conflicts: `git merge-tree --write-tree <target> <head>`; no checkout.
- stale: a check or approval whose `sha` != current head does not count.
- state: `merged` | `blocked` (reasons listed) | `ready`.

Ready when all hold on the current head:
1. ahead >= 1, no conflicts;
2. every `run:` job of the ticket's workflow passed;
3. the approvals the workflow requires exist. Each workflow declares it:

   ```yaml
   # .tasks/workflows/feature.yml
   approvals: [qa, human]   # agent role qa approved AND the human approved
   # .tasks/workflows/bugfix.yml
   approvals: [qa]          # AI review is enough
   ```

   Each entry is an agent role name (as in `board.yaml` `roles:` / `.tasks/agents`) or the
   reserved `human`. A role approves when a review job run by that role passes on head (or
   the role runs `tagent pr approve` from its own pane). `human` = a human approval on head.
   Unknown names fail workflow validation.
   Unset: the agents of the workflow's review jobs, else `[]`.
   Update `examples/workflows/*.yml` and `.tasks/workflows/*.yml`: feature → `[qa, human]`,
   bugfix/improvement/refactor → `[qa]`, docs/research → `[]`.

## Targets

- `local` (default, branch `main`): any local branch can be picked. tagent's gate decides ready;
  merge = existing rebase + ff onto that branch.
- `github` (follow-up ticket): same local gate must be ready first; then **Publish** pushes the
  source branch and runs `gh pr create --repo <repo> --base <branch>`. From then on GitHub is the
  source of truth: the UI mirrors its checks, reviews and mergeable state (`gh pr view --json`),
  and Merge calls `gh pr merge`. The record stores the PR URL/number.
- The target can be changed while the PR is open (not after merge/publish). Default target per
  repo: `pr_target:` in `board.yaml`, else `local/main`.

## Writers

- Dispatcher: check results (sha taken at job start), agent approval when the review job passes.
- Human: `tagent pr approve|request-changes <id>` and the web buttons. Agents cannot approve
  as human: the CLI refuses when called from an agent pane.
- No command writes `state`. No agent-facing MCP write tool.

## Merge gate

Moving a ticket to done (and `tagent merge`) re-derives state; not ready → refuse with reasons,
ticket stays. `--force` (human only) merges and records `forced: true`. Checks are trusted
when their sha matches head (no re-run at merge).

## Web (GitHub-like)

- `#/pulls`: list with title, `source → main`, check/review/conflict badges, state.
- `#/pulls/<id>`: header (title, state pill, `source → main`, ahead/behind), tabs
  Conversation (ticket comments + timeline: commits, check runs, reviews), Commits,
  Checks (logs), Files changed (diff). Bottom merge box: reasons list, Approve /
  Request changes, Merge (disabled unless ready), Force merge behind a confirm.

## Out of scope

Line comments, stacked PRs, PRs for hand-made worktrees, GitLab/other hosts,
syncing GitHub comments back into tickets, HMAC signing, harness deny rules.

## Decisions after research #78 (.tasks/research/78.md)

- Storage: a `PR` field on the ticket JSON (Option A), written under the tasks lock.
- C1: checks and approvals bind to a **tree id** of the worktree content at job start
  (`GIT_INDEX_FILE=<tmp> git add -A && git write-tree`), not HEAD. Stale = tree id changed.
  Conflicts: build a temp commit (`git commit-tree`, no ref) and `merge-tree` it against target.
- C2: the gate also requires the worktree is not behind the target. If behind, reason
  "behind <target> by N"; `tagent pr update <id>` commits, rebases and sends the ticket back to its
  first `run:` status so checks re-run. The merged content is then exactly the checked tree.
- C3: `tagent merge` (agent worktrees) stays ungated; workflow tickets merge only via done.
- C4/default: a role in `approvals` approves when a job run by that role passes on the current
  tree. No "review job" flag. Unset `approvals` = `[]`.
- C5: add `Approvals` to the workflow raw struct (KnownFields).
- C6: pre-existing-failure pass counts as pass, recorded with note `pre-existing failures`.
- C7: a gate refusal keeps the ticket in its current status and adds a comment with the reasons
  (also on the CI/FinishEntry path). For [qa, human]: qa's pass is recorded, ticket stays in
  reviewing; the human then approves and merges (task done / web Merge).
- C9: agent pane = `TMUX_PANE` set and (`@board_target` or `@pane_agent`). Advisory guard only.
