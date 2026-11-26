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
  smart: [claude:opus@70, pi:openai-codex/gpt-6.1-sol]   # spill from Claude at 70%
```

- Candidates are `harness:model` or `harness:model@N`, split at the first `:`.
  Harness is `claude`, `pi` or `codex`. A trailing `@N` (1 to 100) is that
  candidate's own threshold; without it the provider's `limits` value applies.
  Only a short numeric tail counts as `@N`: `pi:vertex/x@20250514` keeps its
  model id, and `pi:vertex/x@20250514@70` sets 70.
  `@0`, `@101`, `@0.5` and `claude:@70` (no model) are rejected. So is a model id
  whose short numeric tail is out of range, such as `pi:vertex/x@2025`.
- `limits` defaults to 90 per provider. Unknown providers are rejected.
- `$XDG_CONFIG_HOME/tagent/routing.yaml` (default `~/.config/tagent/routing.yaml`).
  A missing file is an empty config. Unknown keys are an error.
- Tier names match `^[A-Za-z0-9][A-Za-z0-9_-]*$`; a file with another name does not load.

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
2. Otherwise the first candidate whose provider is under its threshold is used
   (its `@N`, else the provider's `limits` value). A provider is `claude` for
   Claude; `codex`, and `pi` with an `openai-codex/` model, for Codex; anything
   else is unknown and has no threshold.
3. A window counts as exhausted when `used >= threshold`. Either the 5h or the
   7d window is enough. A window whose reset time has passed is ignored (the
   snapshot is stale).
4. Unknown quota (no reading yet, or the read failed) counts as available.
5. All candidates exhausted: `first` runs the first candidate; `block` (default)
   moves the ticket to blocked with the windows and the earliest reset, e.g.
   `tier fast exhausted: claude:haiku 5h 95% >= 90; pi:openai-codex/gpt-6-luna 5h 95% >= 90, earliest reset Nov 25 18:00`.
   With `first`, the reason ends `; all exhausted, running first`.
6. Effort is passed only if the chosen harness accepts that level; otherwise it is dropped.

A pane is reused only if it runs the same harness and model as the choice;
otherwise a new pane is spawned. The dispatch event records `tier` and `route`
(e.g. `tier smart: claude:opus 5h 72% >= 70 -> pi:openai-codex/gpt-6.1-sol`;
`tier smart` when nothing was skipped; or `step model`), shown on the Run page.

If an exhausted tier is hit by a status step or any other trigger (comment,
create, assign, due), the ticket is moved to blocked with the reason.

A mention on a tier role with no candidate left is not a workflow step: it
is audited as `suppressed: ...` and the ticket does not move.

Manual `tagent new --role dev` on a tier role with no `--agent`/`--model` starts
on the first candidate, since a one-shot CLI has no quota reading.

## Web

The web UI edits the same file; the TUI does not.

- **Agents page**: the Model control lists "Tier" (the account's tiers) and "Fixed"
  models. A tier role hides Harness, disables the context fields and offers every
  effort level. Choosing a fixed model switches the harness back to `claude`.
- **Routing page** (`#/routing`, linked from Usage): quota per provider, the
  `Exhausted at` threshold, `when_exhausted`, and each tier's candidates (add,
  reorder, remove, create and delete tiers). A candidate's `Until %` field sets
  its own `@N` (empty removes it). Each action saves at once and shows the
  candidate the server selects now; it polls every 3 s.
- API: `GET` and `PUT /api/routing?dir=`. The server computes candidate state and
  the selection; the page only displays them. Each candidate carries `ref` (with
  `@N`), `state`, `why`, `until` (own threshold, null if inherited), `threshold`
  (effective, null for an unknown provider), `used` (highest counted window, %),
  `selected` and `spilled` (exhausted). PUT takes `{limits, when_exhausted, tiers}`,
  refuses a file that does not load (409), and refuses a body without `tiers` (400).
- A web save rewrites the file: YAML comments are lost, and `limits` and
  `when_exhausted` are written explicitly. Last write wins between two tabs.
- Deleting a tier used by a role blocks that role's tickets with `unknown tier`
  at dispatch. The "used by" list covers only the current project.

## Assumptions and limits

- Quota is read in the dispatcher's background refresh (about once a minute).
  A fresh dispatcher has no reading, so its first dispatches use the first candidate.
- Routed panes that no longer match stay running (they may be busy). Done closes them all, unless the ticket's worktree is kept for uncommitted changes.
- Not included: retry at reset time, switching candidates mid-step after a
  rate limit, quota for other pi providers, editing routing in the TUI, an LLM proxy.
- A binary older than this change rejects `tier` in board.yaml (strict parsing).
  Reinstall and restart the dispatcher before adding tiers.
- A binary older than the per-candidate thresholds reads `claude:opus@70` as a
  model name. Reinstall and restart the dispatcher and the web server before adding `@N`.
