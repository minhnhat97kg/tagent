<h1 align="center">tagent</h1>

<p align="center"><strong>A tmux-based coding agent.</strong></p>

<p align="center">
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <a href="go.mod"><img alt="Go 1.26" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&amp;logoColor=white"></a>
</p>

tagent brings Claude Code, Antigravity (AGY), Codex, OpenCode, Pi, Gemini, Copilot, Amp and other coding agents into one workspace. See what each agent is doing, find the ones waiting for you, and coordinate work across panes and projects.

## Demo

![tagent dashboard displaying Claude, Antigravity, Codex and OpenCode](examples/demo-four-harnesses.png)

*The demo uses simulated agent sessions and sample project data.*

[Install](#install) · [Get started](#get-started) · [Features](#features) · [Commands](#cli) · [Configuration](#configuration)

## One view for the whole team

**Watch the work.** A sidebar follows your tmux windows and shows each agent's status, harness color, elapsed time, and requests that need attention. Expand it into a dashboard to inspect the selected pane without losing your place.

**Coordinate across agents.** Send or queue prompts, share project notes, start agents defined by each repo, and assign work on a task board. The CLI is available to you and to agents with the corresponding integration installed.

**Keep control.** Review approval requests in the sidebar, inspect worktree changes before merging, and use checkpoints to return to earlier work. Notifications and the optional mascot help surface events without replacing your decisions.

## Features

| | What you can do |
|---|---|
| **Sidebar and dashboard** | Track agents by window, inspect live panes, and jump to an agent that needs you. |
| **Harness support** | Integrate Claude Code, Codex, Pi and OpenCode; detect supported agents without hooks through screen scraping. |
| **Team workflow** | Move tickets through role-owned statuses, hand off with comments, and run automatic CI checks. |
| **Worktrees and checkpoints** | Isolate changes, compare results, inspect diffs, merge work and restore snapshots. |
| **Customizable UI** | Choose a theme, colors, keybindings, mascot and optional language model through Settings. |

## Get started

After [installing](#install), open tmux and start a supported agent. The sidebar shows detected agents in your windows. Use the tmux prefix followed by **`e`** to toggle the sidebar and **`A`** to open the dashboard.

```sh
tagent list                         # see your agents
tagent ctx add "API contract: …"   # share a project note
tagent new --name tester --role tester
tagent send tester --wait "Run the tests and report failures"
```

See the [keybindings](#keys) and [CLI reference](#cli) for the full workflow.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/minhnhat97kg/tagent/main/install.sh | bash
```

The script clones into `~/.local/share/tagent` → builds → links
`~/.local/bin/tagent` → adds one `run-shell` line to `~/.tmux.conf` (only
once) → loads it into the running tmux → runs `tagent install` for every
harness it finds. Run it again to **update**.

Requirements: tmux ≥ 2.9, git, Go (the version in `go.mod`).

Options (environment variables): `TMUX_AGENT_DIR` (clone location),
`TMUX_AGENT_BIN_DIR` (where the binary is linked), `TMUX_AGENT_REF`
(branch/tag), `TMUX_AGENT_NO_TMUX=1` (don't touch tmux.conf),
`TMUX_AGENT_NO_HARNESS=1` (don't install hooks/skills), `TAGENT_NO_RTK=1`
(don't install rtk), `RTK_INSTALL_DIR` (rtk location, default the bin dir),
`RTK_EDITION` (default `oss`, the Apache-2.0 build).

<details>
<summary>Manual install</summary>

```sh
git clone <repo> ~/.local/share/tagent && cd ~/.local/share/tagent
make build && ln -s "$PWD/bin/tagent" ~/.local/bin/tagent
echo "run-shell '$PWD/assets/tagent.tmux'" >> ~/.tmux.conf
tmux source ~/.tmux.conf
tagent install
```
</details>

### Harness integration

```sh
tagent install --list          # support matrix + which harnesses are on this machine
tagent install                 # every harness found
tagent install claude pi       # specific ones
tagent install --dry-run       # show what would be written
tagent uninstall <harness>     # remove exactly what install wrote
```

| Harness | Exact status | Agent learns the CLI from |
|---|---|---|
| claude | hooks → `~/.claude/settings.json` | skill |
| codex | hooks → `~/.codex/hooks.json` | skill |
| pi | extension → `~/.pi/agent/extensions/tagent.ts` | skill |
| opencode | plugin → `~/.config/opencode/plugins/tagent.js` | block in `AGENTS.md` |
| gemini · copilot · amp | screen detection (scraper) | block in its instructions file |

**Token saver (rtk).** `tagent install` also installs
[rtk](https://github.com/rtk-ai/rtk), a CLI proxy that compresses command
output, and wires it into each harness it finds:

| Harness | rtk wiring |
|---|---|
| claude | `PreToolUse` hook (`rtk hook claude`) + `RTK.md` |
| codex | `RTK.md` + `@` line in `AGENTS.md` (no command hook) |
| pi | `~/.pi/agent/extensions/rtk.ts` |
| opencode | `~/.config/opencode/plugins/rtk.ts` |
| copilot | `~/.copilot/hooks/rtk-rewrite.json` |
| gemini | **skipped**: `rtk init` overwrites `GEMINI.md`, and uninstall deletes it |
| amp | **unsupported** by rtk |

The binary is installed by rtk's script (checksum-verified) from the upstream
`master` branch. A binary already on `PATH` is used as is. Wiring that existed
before is left alone. `tagent uninstall` reverts only what tagent added,
recorded in markers under `~/.config/tagent/` (`rtk-bin`, `rtk-<harness>`);
it does not remove rtk's history or filter data.

**Code Mode (claude).** `tagent install claude` sets
`CLAUDE_CODE_EXPERIMENTAL_CODE_MODE=1` in the `env` of `~/.claude/settings.json`,
so every claude session uses Code Mode. Claude asks for consent once; a person
must answer it (tagent never answers it). A value you set yourself is kept:
`CLAUDE_CODE_EXPERIMENTAL_CODE_MODE=0` opts out. `tagent uninstall claude`
removes only the key tagent added. Code Mode is experimental and needs Claude
Code 2.1.323 or newer.

Hooks are **merged** into the config (nothing else is overwritten; the
original is kept as `*.tagent.bak`); re-running doesn't duplicate them.
Agents without hooks (Cursor, Aider…) are still detected from the screen.

To let agents use the CLI (post to the board, start helpers) without asking
you each time, allow it in the harness — for Claude, in
`~/.claude/settings.json`: `"permissions": { "allow": ["Bash(tagent:*)"] }`.

## Usage

Ticket attachments: `tagent task attachments ID` lists comment files and job outputs.
Attach with `task comment ID --attach path`, the web composer, or `A` in the TUI
ticket detail (`a` opens the list). See [attachment limits and previews](docs/board.md#attachments).

### Keys

| Key | Where | Does |
|---|---|---|
| `prefix + e` / `E` | tmux | toggle the sidebar in this window / all windows |
| `prefix + A` | tmux | expand / collapse the sidebar into the dashboard (zoom, layout untouched) |
| `prefix + a` | tmux | quick action: go to the agent that needs you / send `@agent_done_prompt` to one that finished |
| `space` · click avatar | sidebar | same as `prefix + a` |
| `j` `k` · mouse | sidebar | move (wheel, click a row) |
| `enter` | sidebar | jump to the pane |
| `1` `2` `3` · `y` `n` | sidebar | answer the prompt of the selected waiting agent |
| `o` | sidebar / zoomed dashboard | zoom / unzoom the same view, keeping selection |
| `,` | sidebar / dashboard | open settings |
| `/` `r` `p` `v` `x` | sidebar | filter · repo filter · pin · select · delete worktree |
| `tab` | sidebar / dashboard | cycle agents, definitions, and tasks |
| `j` `k` | focused list | select an agent or task; selected task details stay visible |
| `s` / `T` / `x` | focused task | start with its type (picker if none) / type picker (todo) / confirm task deletion |
| `[` `]` | sidebar / dashboard | change chart range: 24 hours, 7 days, 30 days |
| `M` | dashboard | move and resize the panes: `h j k l` move, `H J K L` shrink width / grow height / shrink height / grow width, `tab` select, `enter` save, `esc` discard, `0` reset to the default |

The sidebar and zoomed dashboard use one view and one process. Narrow panes
stack analytics, agents, tasks, and detail. Wide panes place agents and tasks
side by side, with session history below.

Press `M` on the dashboard to change the layout, or drag with the mouse: press a
pane's title row and move to move it, press its bottom-right corner and move to
resize it. Releasing the mouse saves, like `enter`. Panes can overflow the
terminal (the dashboard scrolls to the selected pane) or leave gaps. The layout
is saved in `layout.json` next to `config.conf`, one for narrow (under 90
columns) and one for wide, and survives restarts.

The `Compact panes` setting (`@agent_compact`, in the settings screen and the
web Settings dialog) collapses the TUI panes to small rows and packs them upward.
The saved layout is kept. While it is on, `M` and mouse drag do nothing, and
nothing is saved. Turn it off to get the saved layout back. On the web home it
only tightens spacing.

Each pane picks what to show from its own size, so a pane that is shrunk
changes too, whatever the terminal width. Tasks: under 6 rows, lane counts;
narrower than 64 columns, the active lane's cards; 100 columns and 12 rows,
four lanes side by side (todo, doing, blocked, done, closed while a closed
ticket is selected); otherwise the lane list beside the selected ticket. Agents
under 5 rows show counts. Usage adds its sparkline and quota grid only when the
rows are there. Workflows at 80 columns or more show the selected workflow's
job flow beside its list.

### CLI

Run `tagent help` to see the commands and the agents defined in the current repo.

The harness list in **New agent** comes from `~/.config/tagent/harnesses.yml`; without
the file it is the built-in list. An entry runs `command` (default: its `name`) with its
`args` and `env`. Shell aliases do not work as `command`; spell them out. Restart the web server after editing it.

```yaml
harnesses:
  - name: claude                          # the binary to run
    args: [--permission-mode, auto]
  - name: Claude API                      # a second setup of the same binary
    command: claude
    env:
      CLAUDE_CONFIG_DIR: ~/.claude-api
  - name: pi
```

An entry may add `usage`: rows for the web **Usage** panel, each a `command` and an
optional `label`. The command runs with `sh -c` in your home directory, with the web
server's environment plus the entry's `env`. It must exit 0 within 5 seconds and print
only the percentage **used**, 0–100 (`42`, `42.5` or `42%`); anything else shows `error`
on that row. Results are cached for about 60 seconds and refreshed in the background.
Commands are read only from this file, and neither they nor `env` reach the browser.
The panel's checklist button chooses which rows this device shows.

```yaml
harnesses:
  - name: Claude API
    command: claude
    env:
      CLAUDE_CONFIG_DIR: ~/.claude-api
    usage:
      - label: month
        command: ~/bin/claude-api-usage   # your script; prints e.g. 42
      - label: disk
        command: df -P "$HOME" | awk 'NR==2 {print $5}'
```

| Command | Does |
|---|---|
| `tagent new --repo-agent <name> [--name <pane-name>] [--agent claude\|codex\|opencode] [--model model] [--worktree] [--ticket id]` | Start a repo agent defined in the selected harness's native format; harness/model are chosen by tagent config or flags. `--ticket` binds the pane to a ticket, as `tagent chat` does |
| `tagent agents [<name>]` | List repo agent definitions or print one; `roles` remains an alias |
| `tagent answer <pane> <1\|2\|…\|y\|n\|?>` | Answer an agent's approval prompt (also right in the sidebar) |
| `tagent diff <name> [--stat]` · `merge <name> [--discard]` · `discard <name>` | Review / merge / drop the changes of an agent running in a worktree |
| `tagent fanout -n 3 [--role r] [--wait] "<prompt>"` · `compare <prefix>` | Best-of-N: the same task to N agents in N worktrees, compare, keep the best |
| `tagent schedule ls [--json]` · `schedule run\|pause\|resume <name>` | Scheduled tickets and agent prompts from `.tasks/schedules.yml` and `~/.config/tagent/schedules.yml`; see [scheduled triggers](docs/board.md#scheduled-triggers) |
| `tagent task [ls]` · `task add --to <name> "<text>"` · `task start <id> <pane>` · `task set <id> <todo\|doing\|blocked\|done\|closed>` · `task close <id> [reason]` · `task purge --closed [--yes]` · `task assign <id> <name[,name]>` · `task rm <id>` | Create, dispatch, and track tasks on the repo board |
| `tagent chat [--dir DIR] [--agent claude\|pi] [--clear] <id>` | One assistant pane per ticket, reused until cleared. The first message is a ticket brief (summary, step, `tagent task show` for details); `tagent task` in that pane refuses other tickets |
| `tagent stats [ls\|clear]` | Show or delete local Claude, Codex, OpenCode and Pi usage history |
| `tagent save` · `restore [--dry-run]` | Save / bring back your agents after a restart (Claude resumes its session); auto-saved every 30s |
| `tagent tokens` | Tokens used per Claude agent + total |
| `tagent config [list\|get\|set\|reset\|path]` | Central configuration; no arguments = the settings screen (also `,` in the sidebar, or the Settings tab in the dashboard) |
| `tagent list [--json]` | pane · name · agent · status · window · cwd |
| `tagent name [<pane>] <name>` | Name an agent; the name works instead of a pane id everywhere |
| `tagent send <pane> [--now] [--wait] [--timeout D] [--report FILE] <prompt>` | Type a prompt + Enter. A busy agent (working, or waiting on you) gets it **queued** and delivered when idle; `--now` types right away |
| `tagent report wait <file> [--timeout D]` · `report show <file>` | Wait (file events, no polling) for a worker's JSON report and print a one-line summary; `send --report` asks the worker to write it |
| `tagent ask [--role r] [--wait] "<task>"` | Give a task to an idle agent of this repo (with that role), or start a new one if nobody is free |
| `tagent queue [pane]` · `queue clear <pane>` | Show / drop queued messages |
| `tagent handoff [PANE]` | Give web-owned sizing back to tmux; defaults to the current pane. In the sidebar/dashboard Agents list, press `f`. Browser-owned rows show `sized by web`. See [terminal handoff](docs/terminal-handoff.md). |
| `tagent checkpoints <agent>` · `undo <agent> [n]` | Every turn starts with a snapshot of the repo; `undo` rolls the files (and the branch, if the agent committed) back to before the last turn — or n turns back. `undo <agent> --redo <ref>` undoes the undo |
| `tagent wait <pane> [--until stopped\|idle\|waiting] [--timeout D]` | Wait for a status (exit 124 on timeout) |
| `tagent read <pane> [-n 50]` | Last N lines of the screen |
| `tagent events [--json]` | Stream status changes / tool calls (one event per line) |
| `tagent next` | The avatar's quick action (most urgent agent: waiting → error → just finished) |
| `tagent kill <pane…>` | Close agent panes (refuses non-agent panes) |
| `tagent ctx [--global] [ls] \| add <text> \| rm <id> \| clear [--mine] \| scope` | Shared context |
| `tagent reload` | After a rebuild: restart the sidebars + scraper |
| `tagent explain <pane>` | Why a pane has its status: hook vs screen, matched rule, CPU/tool evidence |
| `tagent install` / `uninstall` | Harness integration (see above) |

An orchestration example (agents can do this themselves through the skill):

```sh
tagent name lead
tagent ctx add "API contract: …"            # every agent of this repo can read it
tagent new --name tester --role tester
tagent send tester "Run the tests and fix failures"
```

For non-blocking monitoring of one agent, run `tagent wait tester --timeout
30m` once using your harness's background command tool, if it delivers command
completion notifications to the conversation. Use `list --json`, `queue
tester`, and `read tester -n 80` to verify the result: `wait` observes pane
status rather than a particular task. For several agents, `events --json` can
provide a continuous stream if the harness delivers streaming output. It
samples once per second, starts silently, and does not replay events. A
background process alone cannot wake an agent after a chat turn ends; without
harness notifications, check status when the user asks.

### Approvals, warnings, notifications

- **Approval inbox:** an agent waiting for approval shows its question and
  options right under it in the sidebar; put the cursor on it and press
  `1`/`2`/`3` (or `y`/`n`) to answer without jumping to its pane.
- **File conflicts:** two agents of the same project editing the same file
  within 10 minutes → a warning in the list and from the avatar, a note on the
  shared context, and a notification.
- **Stuck agents:** a `running` agent with no activity for
  `@agent_stuck_minutes` minutes (default 10) → a warning.
- **Undo a turn:** before each agent turn the repo's files are snapshotted
  (`@agent_checkpoints`, on by default; a throwaway index and refs under
  `refs/tagent/` — your index, branch and files aren't touched).
  `tagent undo <agent>` puts things back as they were before its last turn.
- **Phone:** `tagent config set notify_url https://ntfy.sh/<topic>` (or another
  plain-text POST webhook) to get a phone message when an agent needs
  approval and when a ticket reaches `done` or `blocked` (with assignees).
  Intermediate workflow steps and finished agent turns stay on the desktop.


Each agent in the web dashboard has **Terminal** and **Chat** views (Chat renders the conversation from the transcript; claude and pi agents). See [agent chat](docs/agent-chat.md).

A ticket's panel has its own **Chat** tab (web and TUI): one agent dedicated to
that ticket, started with the ticket's brief. A human can save a chat message
to the ticket as a comment, so what the conversation settled stays on the board.
The tab is disabled with the reason when its pane runs a harness Chat cannot
read.

The web dashboard is one page of split panes; by default Agents over Usage, the
selected agent's Terminal/Chat, and Tasks. Drag any divider to resize, or focus
it and use the arrow keys; double-click resets that row or stack. To move a pane,
drag its header or the grip in it (mouse, pen or finger). Over another pane, its
top, bottom, left or right quarter puts the pane on that side of it, and the
middle swaps the two. Along an edge that a group of panes shares, a thin strip
spans the whole group instead, and the outermost strip spans the full width or
height of the page (for example Tasks under Agents and Chat). The highlight and
its label show the area the pane will take; a place that would not fit the
window's minimum sizes is not offered. A group is panes that already sit together
in one row or stack, so spanning two of three columns can take two moves: dock
under both while they are the only columns, then add the third beside them.
Buttons, links, fields and tabs in a header work as usual. Escape or dropping
outside cancels. On a focused grip, up and down move the pane within its stack,
left and right into the neighbouring column; with nowhere to go it moves out to
span the next larger group on that side. A new neighbour takes half of the size
it docks on. A stacked Usage can be dragged shut, and its **Usage** button
reopens it. The layout is saved per device (`tagent.panes` in local storage;
earlier column layouts are converted, and the older pane grid is ignored) and
Settings > Reset layout restores the default. Phones keep their tabs.
Clicking an agent shows it in place. The Tasks
column has **Tasks** and **Workflows** tabs, and a ticket opens in a side panel.
The button at the right end of a pane header maximizes the pane (`#/pane/usage`,
`#/pane/agents`, `#/pane/chat`, `#/pane/tasks`, `#/pane/workflows`; with a mouse
it shows on hover or focus), and the same spot there minimizes it; the old
links (`#/agent/<pane>`, `#/workflows`, `#/board/config`) still work, and an agent
link selects that agent. A pane header is always one row: its title truncates,
the agent name in Chat shows in full on hover, and the Usage range (1d/7d/30d)
sits above its body. The top bar clears the iPad status bar and overlay through
the safe-area insets. Size changes what a pane shows: Tasks is a list under
600px and the board from 600px, its lanes scrolling sideways. Workflows lists them under 900px
and edits them from 900px. Agents show only counts under 120px of height, and
usage hides its token spend under 200px. Phones have **Agents** (with usage),
**Chat** and **Tasks** tabs; selecting an agent opens Chat.

Press `?` in the sidebar or dashboard for contextual help.

### Tickets and status handoffs

Workflows are the only flow definition: `.tasks/workflows/*.yml` lists jobs
(agent or `run:` check) and their statuses. `.tasks/board.yaml` holds roles,
triggers, `max_fires_per_hour` and `default_workflow`; board lanes are fixed at
todo, in progress, blocked, done. A ticket without a workflow starts
`default_workflow` when it leaves todo. Entering a role-owned job status
dispatches its agent; returning to that status reuses the same role/task pane and
context. Comments carry handoffs.

```sh
tagent board init
tagent board validate # read-only check of effective config
tagent task add "Implement the feature"  # prints ID
tagent task move ID plan "Requirements and acceptance criteria"
# Worker reads the entire ticket, then hands it on:
tagent task show ID
# Inspect dispatch errors, repair the cause, then explicitly retry current work:
tagent task retry ID
tagent task move ID build "Plan and verification strategy"
```

Jobs with `run:` execute shell checks asynchronously and comment as `ci`, then
move to their next/on-failure status. `max-loops` counts rejections (hand-backs to the on-failure status) and blocks at the one past the cap,
counting since the last exit from blocked.

Agent roles refresh their pane on entry 3 by default (entries 1–2 reuse, 3 replaces,
4 reuses, 5 replaces). Set `fresh_after` on a role to override the default 3
(deprecated board statuses may also set it). Explicit `0` disables refresh;
negative values are invalid. A `long_lived: true` role keeps one standing pane across
tickets and defaults to no refresh (see `docs/board.md`, Agents). Counts are per pane and ticket across all statuses
using the same logical role, including workflow statuses matched by ID. Mentions
and shell jobs do not count. Replacement loses agent context, comment cursor, and
queued prompts, but keeps the worktree and artifacts. `fresh_after: 1` replaces
on every subsequent entry; this is independent of `max_loops`.

Blocked rows show the blocking question. Press `c` to reply in the existing editor,
prefilled with the interrupted role, or run `tagent task comment ID "@implementer answer"`.
The tagged role resumes in the ticket's own flow with only that reply; another role
routes to its first status. Unknown or multiple distinct tags save the comment but
leave the ticket blocked with a notice. With missing history or a roleless step,
type the role explicitly. Agents must block with exactly one concrete question.
`task set` and `task done` remain admin overrides. Assignees are metadata; `needs`
is a start gate: a ticket with open dependencies cannot leave todo, and its card
shows `waits #12` until the last one is done, then `ready` (a human still starts it).
`task close ID [reason]` ends a ticket that will not be done (see docs/board.md).
`task needs ID ids|-` edits them; cycles and unknown ids are rejected. Priority is
`high`, `normal` (default) or `low` (`task add --priority`, `task priority`); lanes
and the dispatcher's busy/approval queue order by it. Mention triggers still notify.
Recipes support `workflow new|list|run`, `task workflow ID NAME|-` (todo tickets) sets a type, `workflow run NAME --input key=value --wait`,
and `task move ID NEXT --output key=value "Handoff"`.
`task show ID` ends with a usage block (tokens per job, harness/model/effort, loops, elapsed; `n/a` when a harness reports nothing, total marked partial) and `task ls --usage` adds total tokens per ticket. The usage block also shows an estimated cost per job and per ticket, labelled API-equivalent (list prices, overridable with `pricing:` in `board.yaml`; see docs/board.md). The web drawer and TUI detail show the same block.

`task show ID --brief` includes the latest dispatch outcome. Accepted is not
completed; an uncertain reservation may already have sent work, so retry can
duplicate side effects. Retry preserves hourly limits and workflow loop caps.
See the
[Workflows guide](docs/board.md#workflows) for trusted inputs, outputs and live resolution.
Agents that work better with typed tools can use `tagent mcp`, a stdio MCP server with `task_show`, `task_move`, `task_comment` and `task_add`; see [docs/mcp.md](docs/mcp.md).

See [docs/board.md](docs/board.md) for schema, migration and delivery guarantees.

### Central configuration

`,` in the sidebar opens the settings popup (also `tagent config`): theme,
avatar, sidebar position/width, default agent/model, stuck threshold and
notifications. Changes apply at once and are saved to
`~/.config/tagent/config.conf` (tmux syntax, loaded by the plugin at
startup). Less common options (avatar LLM/personality, key bindings…) are
not in the screen: `tagent config list` shows all of them, and
`tagent config set <key> <value>` changes any — short names work
(`avatar`, `width`, `notify_url`…).

### Workspaces

A workspace is a repo or folder that the board and the web UI show. It is not a
ticket's worktree.

- **Add:** run `tagent web` or bare `tagent` in a folder and it becomes a workspace
  (a git worktree adds its main repo). In the web app, use the header dropdown's
  *Add workspace…* or Settings → Workspaces: type or paste a path, or browse the
  server's folders in the picker. In the TUI settings (`,`), `a` adds by typed path.
- **Remove:** Settings → Workspaces → *Remove*, or `x` on the row in the TUI settings.
  This only hides the workspace from the lists (dropdown, Settings, TUI). Ticket
  files stay, and dispatch keeps running for tickets in flight. Adding it again
  shows it, and so does `tagent web` or bare `tagent` run in it.
- **Lists:** the dropdown and Settings show the server's folder, the repos with a
  running agent and the registered workspaces, minus the removed ones. Auto-created
  sidebars never add a workspace.
- **One web server per user:** `web.lock` in tagent's state directory
  (`~/.local/state/tagent/context/web.lock`) records the running server's pid,
  address and token. A second `tagent web` in another folder adds that folder to
  the running server, prints `added <dir> to running tagent: <url>` and exits 0.
  The lock is released when the server exits, even after a crash.
- **After upgrading:** if the sidebar shows `⊗ :<port>` because the old server still
  holds the port, run `tagent reload` twice.

### Repo agents and task board

Agent metadata is owned by tagent: `roles:` in `.tasks/board.yaml` names the harness, model and effort (optionally defaulted by `~/.config/tagent/agents/profiles.toml`). Instructions are plain markdown in `.tasks/agents/<name>.md`, or `~/.config/tagent/agents/<name>.md` for global agents; a harness directory implies neither. `tagent agents`, the sidebar below running panes, and the dashboard read every agent regardless of the process harness. To edit them in one place (repo, global and board.yaml roles), use the Agents page on the web (`#/agents`) or the dashboard's Agents card (`t` opens its Definitions tab); see `docs/board.md`. When a role starts a pane its instructions ride in the dispatch prompt on every harness, so pi gets them too. An optional per-agent memory file, `.tasks/memory/<name>.md`, is read on every dispatch and sent in the prompt above the ticket (a missing file adds nothing). It is never listed as a role and tagent never writes it; edit it by hand. Legacy `.claude/agents`, `.codex/agents` and `.opencode/agents` files are still read but never written; only those native files are loaded by their harness itself (Claude/OpenCode `--agent`, Codex `developer_instructions`) and are not repeated in the prompt.

Choose how they run in tagent configuration:

```sh
tagent config set default_agent codex
tagent config set model o3
tagent new --repo-agent reviewer
# or override one launch:
tagent new --repo-agent reviewer --agent claude --model sonnet
```

The task list and agent list remain visible in both widths. Press `tab` to focus tasks, then `j`/`k` to choose one; detail appears below. Press `s` to send it to the agent selected in the other list, or `x` to confirm deletion. A task is marked done only when an agent or user explicitly sets it. The board is shared by all worktrees of the same git repo. Create and update tasks from the CLI:

```sh
tagent task add --to reviewer "Review the auth change"
tagent task ls
tagent task start <id> <pane>
tagent task set <id> doing
tagent task set <id> blocked
tagent task set <id> done
tagent task close <id> "dropped"
tagent task purge --closed          # dry run; --yes deletes closed tickets and their worktrees
tagent task assign <id> reviewer,tester
tagent task rm <id>
tagent stats ls
tagent stats clear  # deletes local session and task-usage history
```

Pi stats sum assistant token usage across all session branches from
`~/.pi/agent/sessions` (or `$PI_CODING_AGENT_DIR/sessions`), including cache tokens.


### Statuses

| | Meaning |
|---|---|
| running | working |
| waiting | needs you (approval, a question, folder trust…) |
| background | the agent stopped but a background shell still runs |
| done | finished while you weren't looking at its pane — turns idle once you visit it |
| idle / error | idle / failed |

### Avatar

The top of the sidebar holds a small pixel character (drawn with braille:
28×16 pixels in 14 columns × 4 lines) that watches every agent and nudges you
like a friend:

> ▲ Hey, reviewer is done and everything passes — send it the commit prompt?

| Character (`@agent_avatar`) | |
|---|---|
| `slime` (default) | breathes asleep, bounces at work, jumps and squashes when happy, shakes when it needs you, melts when sad |
| `cat` | ear twitches, blinks, `^` eyes when happy, ears up when alarmed, down when sad |
| `sprout` | grows while working, blooms when done, waves its leaves when it needs you, wilts on errors |
| `ghost` | floats, rippling hem |
| `robot` | blinking antenna light, "talking" mouth while working |
| `off` | hidden |

**Its lines** are written by `claude -p --model haiku` (one short, friendly
sentence offering help — never a dump of the session), cached and shared by
every sidebar so one event costs one call; while it answers (a few seconds),
or with the LLM off, a built-in line in the same tone is shown. The default
language is English (`@agent_avatar_lang en`); set `vi` for Vietnamese. Panes
too short for the full avatar get a compact one.

**Quick action** (`space`, a click on the avatar, or `prefix + a` from
anywhere): a waiting or failed agent → jump to its pane (approvals are always
your call); a finished agent → send `@agent_done_prompt` if set, else jump to
it.

```tmux
set -g @agent_avatar sprout
set -g @agent_avatar_lang en                   # vi | en
set -g @agent_digest_llm "ollama run --nowordwrap --think=false qwen3:0.6b"   # writes the avatar's line on every agent event; prompt = last argument
set -g @agent_avatar_lines 6                   # text lines in the speech bubble (1–8, default 1)
set -g @agent_done_prompt "commit and summarize what you did"
set -g @agent_avatar_say_done "{name} is done!"      # your own line (overrides the LLM)
```

### Status line

```tmux
set -ga window-status-format         ' #{E:@agent_status_format}'
set -ga window-status-current-format ' #{E:@agent_status_format}'
set -ga status-right '#{E:@agent_session_status_format}'
set -ga status-right '#{E:@agent_hidden_status_format}'   # standalone agents
```

**Sessions:** every agent runs in a tmux session of its project, one window
per agent, named `tagent:<folder>-<4 hex>` (project = main git root; worktrees
share their repo's session; a folder outside git is its own project). This works
the same inside or outside tmux: `tmux ls`, then `tmux attach -t 'tagent:<name>-xxxx'`.
The last status-line format lists agents of sessions without a client.

Plain `tagent` outside tmux opens a split screen: the dashboard on the
left, the live screen of the selected agent on the right. `Enter` (or a click)
types into that agent (cursor, F-keys, ctrl/alt combos and — when the agent enables mouse reporting — mouse events are passed through), `ctrl+]` returns to the list, `N` starts a new agent in
the current directory. Panes of the background session are resized to fit; your
own tmux sessions are only viewed, never resized.

## Configuration

Set options with `set -g …` before loading the plugin, or use `tagent config`.

| Option | Default | |
|---|---|---|
| `@sidebar_width` | `15%` | sidebar width (columns or %) |
| `@sidebar_position` | `left` | `left` / `right` |
| `@sidebar_auto_create` | `on` | create the sidebar in new windows |
| `@sidebar_key` / `@sidebar_key_all` | `e` / `E` | toggle keys |
| `@agent_watch_key` | `A` | dashboard key |
| `@agent_action_key` | `a` | quick action key |
| `@agent_digest_llm` | `ollama run --nowordwrap --think=false qwen3:0.6b` | command writing the avatar's line on every agent event — who needs you, who works on what (prompt = last argument); a small local model is enough |
| `@agent_avatar` | `off` | `slime` / `cat` / `sprout` / `ghost` / `robot` / `off` |
| `@agent_avatar_lang` | `en` | `vi` / `en` |
| `@agent_avatar_lines` | `1` | text lines in the speech bubble (1–8) |
| `@agent_avatar_personality` | `cheerful, cute and encouraging` | how the avatar talks — pick a preset (tsundere, lazy office cat, sarcastic, pirate…) or describe your own |
| `@agent_avatar_llm` | `claude -p --model haiku` | command writing the dashboard's task summaries; `off` = none |
| `@agent_done_prompt` | — | prompt the quick action sends to a finished agent |
| `@agent_avatar_say_waiting` / `_done` / `_error` | | your own lines; `{name}`, `{action}` (tmux.conf only, not in the settings screen) |
| `@agent-sidebar-default-agent` | `claude` | default harness for `new` |
| `@agent_model` | — | model passed to the harness as `--model` |
| `@agent_claude_flags` | — | extra flags appended to every claude launch, e.g. `--strict-mcp-config` to skip MCP servers (leaner worker context) |
| `@agent_scrape` | `on` | screen detection for agents without hooks |
| `@agent_checkpoints` | `on` | snapshot the repo before each agent turn (for `undo`) |
| `@agent_stuck_minutes` | `10` | warn when a running agent shows no activity this long |
| `@agent_blocked_hours` | `24` | notify once when a ticket stays `blocked` (waiting on a human) this long |
| `@sidebar_notifications` | `on` | desktop notifications |
| `@agent_notify_url` | — | phone alerts and task-status transitions (ntfy or plain-text POST webhook); empty disables delivery |
| `@agent_telegram_token` | — | Telegram bot token; empty disables the gateway. Alerts go to every allowlisted chat (waiting agents get answer buttons); the scraper long-polls for replies, `@name text`, `/use <name>`, `/agents` |
| `@agent_telegram_chats` | — | comma-separated allowlisted chat IDs (groups are negative). Other chats are ignored and never answered. Anyone in an allowed chat can drive agents |
| `@agent_telegram_agent` | `receptionist` | offered in the agent list; a chosen target that is gone falls back here. Until `/use` picks a target, plain text is answered with help (also `/start`, `/help`) |
| `@agent_theme` | `light` | `dark` / `light`; `@agent_color_<token>`, `@agent_icon_<token>` override single colors/icons |

Put the settings in `~/.config/tagent/telegram.env` (tagent keeps it `0600`):

```
token=<bot token>
chats=<chat id>,<chat id>
agent=receptionist
```

A key missing from the file falls back to the tmux option. Prefer the file for the token: anyone who can run `tmux show -g` can read an option (agents running as your user can still read the file). The file applies to every tmux server, and only one of them polls at a time. Chat state (update offset, per-chat target, reply routing) is kept in `~/.config/tagent/telegram.json`.

## Development

```sh
make build && tagent reload   # build + reload the running sidebars/scraper
make test
make update-frames                # re-capture the sidebar's golden frames (check them by eye first)
```

tmux ≥ 2.9 is required because the query layer uses `#{q:…}` to escape `|` in
user data; verified on tmux 3.7b.

## Origin

The architecture and part of the assets derive from
[hiroppy/tmux-agent-sidebar](https://github.com/hiroppy/tmux-agent-sidebar) —
see [NOTICE](./NOTICE). The orchestration ideas come from
[herdr](https://herdr.dev).

## Contributing

Contributions, bug reports and feature requests are welcome. Please open an [issue](https://github.com/minhnhat97kg/tagent/issues) or a pull request.

## License

This project is licensed under the [MIT License](LICENSE). See [NOTICE](NOTICE) for attribution.

---

README structure adapted from [readme-md-generator](https://github.com/kefranabg/readme-md-generator).

Tasks panel and board share ticket controls: `s` starts with the ticket's type
(picker if it has none; last row: `default_workflow`), `T` type (todo), `enter` detail, `e` edit, `C` comment, `H`/`L`
status, `x` delete with confirmation, `w` workflow, `B` board config, `?` help.
Dashboard `Tab` changes focus; `Z` maximizes the focused card (`b` on Tasks too), `esc` restores.
