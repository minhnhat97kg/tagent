# tagent MCP server

`tagent mcp` runs a stdio MCP server (newline-delimited JSON-RPC 2.0, no extra
dependencies). It exposes four typed ticket tools and two web-pane tools for agents that work better
with tools than with a CLI guide:

| Tool | Required params | Optional params | Wraps |
| --- | --- | --- | --- |
| `task_show` | `id` | | `tasks.Get` + brief view (same as `task show ID --brief`) |
| `task_move` | `id`, `to` | `comment`, `outputs` (object of string values) | `tasks.Move` + dispatcher registration |
| `task_comment` | `id`, `text` | | `tasks.Comment` |
| `task_add` | `text` | `to`, `priority` (enum high/normal/low), `workflow` | `tasks.Add` |
| `pane_add` | `name`, `html` | | Same as `tagent pane add NAME --file PATH`: writes `<config dir>/ui/<name>.html` ([custom panes](../README.md#custom-panes)) |
| `pane_list` | | | Same as `tagent pane ls`: each pane, and whether the user or an agent made it |
| `memory_search` | `query` | `limit` (1-50), `scope` (repo/shared; default both) | `tagent memory search` |
| `memory_show` | `id` | `scope` | `tagent memory show` (full node, `broken_ref` flag) |
| `memory_related` | `id` | `depth` (1-3), `limit` (default 20), `scope` | `tagent memory related` |
| `memory_add_node` | `label`, `kind` | `content`, `ref`, `lang`, `tags`, `scope` | `tagent memory add-node` |
| `memory_add_link` | `from`, `to`, `label` | `scope` | `tagent memory add-link` |
| `memory_edit_node` | `id` | `label`, `content`, `ref`, `lang`, `tags`, `scope` | `tagent memory edit-node` (your own nodes only) |
| `memory_edit_link` | `id`, `label` | `scope` | `tagent memory edit-link` (your own links only) |
| `memory_check` | | `scope` | `tagent memory check` (corrupt records, broken refs, dangling links) |

The memory tools use the graph described in [board.md](board.md#memory-graph). An agent can
search and edit only what it created; the user can edit or delete anything. An MCP call from a
tmux pane with no resolvable role is treated as an agent.

A pane an agent adds is **read-only** in the web UI: its frame may only `GET`, plus read and write its own
`/api/ext/<name>/data`. The user can add it to the layout and choose **Allow actions**; any later change to
its HTML (including your own update) revokes that. You can only change a pane you created, never one the user wrote. Names are letters, digits, `-` and `_`; HTML is limited to 256 KiB.

There is no delete tool, for tasks or memory. Deleting a memory node or link is user-only (`tagent memory rm-node|rm-link`). The server reads the working directory at start, so
start it from the repository (or worktree) that holds the board.

## Register it

`tagent install` registers the server for every harness it integrates (re-run
it to update the binary path; `tagent uninstall <harness>` removes the entry):

| Harness | Where |
| --- | --- |
| claude | `claude mcp add -s user tagent -- <bin> mcp` |
| codex | `codex mcp add tagent -- <bin> mcp` |
| opencode | `mcp.tagent` in `~/.config/opencode/opencode.json` |
| pi | `mcpServers.tagent` in `~/.pi/agent/mcp.json` (needs `pi-mcp-adapter`) |
| gemini | `mcpServers.tagent` in `~/.gemini/settings.json` |
| copilot | `mcpServers.tagent` in `~/.copilot/mcp-config.json` |
| amp | `amp.mcpServers.tagent` in `~/.config/amp/settings.json` |

Use the absolute path to the binary. The server inherits the harness environment
(including `TMUX_PANE`), so comments and new tickets record the same actor as the CLI.

## Harness support (checked 2026-11)

- **Claude Code**: native stdio MCP (`claude mcp add`). Verified against the installed CLI.
- **Codex CLI**: native stdio MCP (`codex mcp add`). Verified against the installed CLI.
- **opencode**: native MCP (`opencode mcp add`). Verified against the installed CLI.
- **pi**: no MCP in core. MCP comes from an extension package (`@earendil-works/pi-mcp`), installed in the local pi agent directory. Its config was not checked here, so wire it up by reading that package's docs.

Small models are the main target. The skill names the MCP tools first and keeps
the CLI as the fallback, so a harness without MCP still works.

## Not yet verified

The end-to-end check (a small model such as Haiku completing a dispatched ticket
with only `task_show` and `task_move`) has not been run. It needs a live harness
with the server registered and a dispatched ticket.
