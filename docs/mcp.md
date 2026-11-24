# tagent MCP server

`tagent mcp` runs a stdio MCP server (newline-delimited JSON-RPC 2.0, no extra
dependencies). It exposes four typed ticket tools for agents that work better
with tools than with a CLI guide:

| Tool | Required params | Optional params | Wraps |
| --- | --- | --- | --- |
| `task_show` | `id` | | `tasks.Get` + brief view (same as `task show ID --brief`) |
| `task_move` | `id`, `to` | `comment`, `outputs` (object of string values) | `tasks.Move` + dispatcher registration |
| `task_comment` | `id`, `text` | | `tasks.Comment` |
| `task_add` | `text` | `to`, `priority` (enum high/normal/low), `workflow` | `tasks.Add` |

There is no delete tool. The server reads the working directory at start, so
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
