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
  fast:    ["Claude API:haiku@85", "claude:haiku"]   # Claude API until its quota reaches 85%
  smart:   [claude:opus@70, pi:openai-codex/gpt-6.1-sol]   # spill from Claude at 70%
```

A harness that routing names is defined in `harnesses.yml`. A usage command
prints the percentage used (`0` to `100`) and makes that harness its own quota:

```yaml
# ~/.config/tagent/harnesses.yml
harnesses:
  - name: Claude API
    command: claude-api                 # runs as the claude kind
    env: {CLAUDE_CONFIG_DIR: ~/.claude-api}
    usage:
      - {label: month, command: "my-usage-script"}   # prints e.g. 61
```

- Candidates are `harness:model`, `harness:model@N` or `harness:model@N#effort`,
  split at the first `:`. A trailing `#level` is the candidate's effort, chosen from
  its harness's effort list (see [Effort levels](#effort-levels)); it is split off
  first, so `claude:haiku@85#high` is haiku, spilled at 85%, at effort `high`.
  Strings without `#` load as before.
  Harness is any name listed in `$XDG_CONFIG_HOME/tagent/harnesses.yml`
  (`~/.config/tagent/harnesses.yml`), so names may contain spaces (`Claude API`).
  The one list is `harnesses.yml` when it exists, otherwise the built-ins
  (`claude`, `codex`, `opencode`, `gemini`, `pi`, `copilot`, `amp`). A file that
  lists names replaces the built-ins, so list every harness a tier uses (for
  example `claude`). The same list validates tiers and agents, and feeds the
  web pickers. A trailing `@N` (1 to 100) is that
  candidate's own threshold; without it the provider's `limits` value applies.
  Only a short numeric tail counts as `@N`: `pi:vertex/x@20250514` keeps its
  model id, and `pi:vertex/x@20250514@70` sets 70.
  `@0`, `@101`, `@0.5` and `claude:@70` (no model) are rejected. So is a model id
  whose short numeric tail is out of range, such as `pi:vertex/x@2025`.

- `limits` defaults to 90 per provider. Its keys are `claude`, `codex`, or a
  harness with `usage` commands (its own provider). Any other key is rejected,
  since a limit on a harness without usage would do nothing.
- `$XDG_CONFIG_HOME/tagent/routing.yaml` (default `~/.config/tagent/routing.yaml`).
  A missing file is an empty config. Unknown keys are an error.
- Tier names match `^[A-Za-z0-9][A-Za-z0-9_-]*$`; a file with another name does not load.

### Effort levels

Each harness declares the levels it accepts in `harnesses.yml`, and that list is the
only source: candidate validation, role saves, dispatch and the web pickers all use it.

```yaml
harnesses:
  - name: claude                       # no efforts key: the built-in levels of its kind
  - name: quick
    command: claude
    efforts: [fast, deep]              # exactly these levels, passed as --effort
  - name: plain
    command: opencode
    efforts: []                        # takes no effort
```

- Absent: the built-in levels of the kind its command runs (`claude`: low, medium,
  high, xhigh, max; `codex`: none, minimal, low, …, max; `pi`: off, minimal, …, max).
- Empty list: the harness takes no effort. Other kinds have no built-in levels.
- A custom list needs a `claude`, `codex` or `pi` command, since those are the only
  kinds whose launch can pass a level. Each level is lowercase letters or digits.
  Any other `efforts` value fails the whole `harnesses.yml`: the built-in list is
  served and launches report the error.
- A level not in the list is rejected when a candidate, role or agent is saved or
  loaded (`invalid effort "x" for claude`, or `effort is unsupported for plain`).
  Dispatch never passes an unlisted level.
- A tier role's own `effort` is only checked to be a level some harness lists, since
  its harness is chosen later. At dispatch it applies only if the chosen harness accepts it.

Effort is part of the pane route: a tier pane is reused only when harness, model and
effort all match. Effort is a launch argument, so a pane started at another level cannot
change it in place. Switching a tier's candidate effort therefore starts one new pane.

A candidate's effort is the `#level` at its end. Only a tail of lowercase letters
and digits is read as one; any other `#` stays part of the model id.

Board role:

```yaml
roles:
  dev: {tier: fast, effort: medium}
```

A tier role sets only `tier` and, optionally, `effort`. The role's `effort` is a
fallback: it applies only when the chosen candidate has no `#effort`. It cannot set `agent`, `model`,
`skills`, `mcp`, `tools`, `allow` or `deny`: the harness is chosen per dispatch,
and those lists are harness-specific.

## Selection

At dispatch, for each action of a tier role:

1. A step with its own `model:` wins. It uses the tier's first candidate's
   harness with that model, and no quota check runs.
2. Otherwise the first candidate whose provider is under its threshold is used
   (its `@N`, else the provider's `limits` value). Quota comes from the harness
   entry:
   - An entry with `usage` commands is its own provider, named by the entry.
     Each command's percentage is a window, and any one reaching the threshold
     exhausts it.
   - An entry without `usage` keeps the quota of the kind its command runs
     (`claude` for Claude, `codex`, and `pi` with an `openai-codex/` model, for
     Codex) only when it sets no `env`, so it shares that account. With `env`
     it is unknown.
   - Anything else is unknown and has no threshold.
3. A window counts as exhausted when `used >= threshold`. Either the 5h or the
   7d window is enough. A window whose reset time has passed is ignored (the
   snapshot is stale).
4. Unknown quota (no reading yet, or the read failed) counts as available.
   A usage command that failed or has not run yet adds no window.
5. All candidates exhausted: `first` runs the first candidate; `block` (default)
   moves the ticket to blocked with the windows and the earliest reset, e.g.
   `tier fast exhausted: claude:haiku 5h 95% >= 90; pi:openai-codex/gpt-6-luna 5h 95% >= 90, earliest reset Nov 25 18:00`.
   With `first`, the reason ends `; all exhausted, running first`.
6. Effort: the chosen candidate's `#effort` if it has one, else the role's `effort`.
   It is passed only if the chosen harness accepts that level; otherwise it is dropped.

A pane is reused only if it runs the same harness, model and effort as the choice;
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
  models. A tier role hides Harness and Effort and disables the context fields.
  A fixed role's Effort lists its harness's served levels. Choosing a fixed model
  switches the harness back to `claude`.
- **Routing page** (`#/routing`, linked from Usage): one card per tier. A node
  chart at the top shows the flow as chips joined by arrows, e.g. `Claude opus
  until 70%` → `Codex gpt-6.1-sol until 90%` → `block` (the end node is `run
  first` when `when_exhausted` is `first`); the running candidate is highlighted.
  Candidates are listed below in priority order, joined by a dashed spine, with the 5h and 7d quota bars;
  a tick marks the candidate's `Spill at %`, which is edited in place (empty
  means the provider limit). The candidate running now shows `Running now`; an
  exhausted one shows `Spilled - resets <time>`. Reorder by dragging the grip or
  with the up and down buttons; each candidate has an Effort picker with its
  harness's levels (default: none). The add form takes an effort too. Remove, add a candidate, and delete the tier from
  the card, which also lists the roles that use it. `when_exhausted` shows at the
  end of every flow and is set under `Defaults`, with the provider limits. Each
  action saves at once and shows what the server selects now; it polls every 3 s.
- The candidate pickers offer the same harness list, served as `harnesses` by
  the API. The Agents page Harness picker uses it too. Provider cards cover
  only Claude and Codex; a usage-backed harness shows its percentages in the
  Usage panel and on its candidates, with no card of its own.
- API: `GET` and `PUT /api/routing?dir=`. The server computes candidate state and
  the selection; the page only displays them. Each candidate carries `ref` (with
  `@N`), `state`, `why`, `until` (own threshold, null if inherited), `threshold`
  (effective, null for an unknown provider), `used` (highest counted window, %),
  `selected` and `spilled` (exhausted). PUT takes `{limits, when_exhausted, tiers}`,
  refuses a file that does not load (409), and refuses a body without `tiers` (400).
- Usage commands live only in the global `harnesses.yml`; the API never returns
  their text or `env`.
- A web save rewrites the file: YAML comments are lost, and `limits` and
  `when_exhausted` are written explicitly. Last write wins between two tabs.
- Deleting a tier used by a role blocks that role's tickets with `unknown tier`
  at dispatch. The "used by" list covers only the current project.

## Assumptions and limits

- Quota is read in the dispatcher's background refresh (about once a minute).
  A fresh dispatcher has no reading, so its first dispatches use the first candidate.
  Usage commands (of a `harnesses.yml` entry) run at most once a minute, in the
  web server and the dispatcher alike; a command that is slow or fails counts as unknown.
- A harness removed from `harnesses.yml` while a tier still names it makes the
  routing config fail to load: dispatch blocks with the error, and the web save is refused (409).
- Routed panes that no longer match stay running (they may be busy). Done closes them all, unless the ticket's worktree is kept for uncommitted changes.
- Not included: retry at reset time, switching candidates mid-step after a
  rate limit, quota for other pi providers, editing routing in the TUI, an LLM proxy.
- A binary older than this change rejects `tier` in board.yaml (strict parsing).
  Reinstall and restart the dispatcher before adding tiers.
- A binary older than the per-candidate thresholds reads `claude:opus@70` as a
  model name. Reinstall and restart the dispatcher and the web server before adding `@N`.
