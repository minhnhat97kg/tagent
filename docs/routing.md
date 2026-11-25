# Quota routing (tiers)

A board role can name a **tier** instead of a harness and model. When the
dispatcher starts that role, it picks the harness and model from the account's
routing config, using the quota left for each provider.

Three layers:

- **Workflow**: a step names a role. Unchanged.
- **Role** (`.tasks/board.yaml`): `agent` + `model` (fixed), or `tier` (routed).
- **Routing** (`~/.config/tagent/routing.yaml`, global): each tier's candidates in order.

## Config

```yaml
limits: {claude: 90, codex: 90}   # % used (5h or 7d window) that counts as exhausted
when_exhausted: block              # block | first
tiers:
  fast:  [claude:haiku, pi:openai-codex/gpt-6-luna]
  smart: [claude:opus, pi:openai-codex/gpt-6.1-sol]
```

- Candidates are `harness:model`, split at the first `:`. Harness is `claude`,
  `pi` or `codex`.
- `limits` defaults to 90 per provider. Unknown providers are rejected.
- `$XDG_CONFIG_HOME/tagent/routing.yaml` (default `~/.config/tagent/routing.yaml`).
  A missing file is an empty config. Unknown keys are an error.

Board role:

```yaml
roles:
  dev: {tier: fast, effort: medium}
```

A tier role sets only `tier` and `effort`. It cannot set `agent`, `model`,
`skills`, `mcp`, `tools`, `allow` or `deny`: the harness is chosen per dispatch,
and those lists are harness-specific.

## Selection

At dispatch, for each action of a tier role:

1. A step with its own `model:` wins. It uses the tier's first candidate's
   harness with that model, and no quota check runs.
2. Otherwise the first candidate whose provider is under its limit is used.
   A provider is `claude` for Claude; `codex`, and `pi` with an `openai-codex/`
   model, for Codex; anything else is unknown.
3. A window counts as exhausted when `used >= limit`. A window whose reset time
   has passed is ignored (the snapshot is stale).
4. Unknown quota (no reading yet, or the read failed) counts as available.
5. All candidates exhausted: `first` runs the first candidate; `block` (default)
   moves the ticket to blocked with the windows and the earliest reset, e.g.
   `tier fast exhausted: Claude 5h 95%, earliest reset Nov 25 18:00`.
6. Effort is passed only if the chosen harness accepts that level; otherwise it is dropped.

A pane is reused only if it runs the same harness and model as the choice;
otherwise a new pane is spawned. The dispatch event records `tier` and `route`
(e.g. `tier fast (Claude 5h 93%)` or `step model`), shown on the Run page.

If an exhausted tier is hit by a status step or any other trigger (comment,
create, assign, due), the ticket is moved to blocked with the reason.

A mention on a tier role with no candidate left is not a workflow step: it
is audited as `suppressed: ...` and the ticket does not move.

Manual `tagent new --role dev` on a tier role with no `--agent`/`--model` starts
on the first candidate, since a one-shot CLI has no quota reading.

## Assumptions and limits

- Quota is read in the dispatcher's background refresh (about once a minute).
  A fresh dispatcher has no reading, so its first dispatches use the first candidate.
- Routed panes that no longer match stay running (they may be busy). Done closes them all, unless the ticket's worktree is kept for uncommitted changes.
- Not included: retry at reset time, switching candidates mid-step after a
  rate limit, quota for other pi providers, editing routing in the TUI or web,
  an LLM proxy.
- A binary older than this change rejects `tier` in board.yaml (strict parsing).
  Reinstall and restart the dispatcher before adding tiers.
- Until the web Agents page supports tiers, edit board.yaml by hand.
