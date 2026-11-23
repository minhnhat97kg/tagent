# Web dashboard

Run `npm ci`, `npm test`, and `npm run build`; Vite embeds the build in `internal/web/dist`. `npm run dev` proxies `/api` to port 7777.

`#/chat` shares the TUI receptionist conversation and native tools. Enter sends, Shift+Enter adds a newline; `@` completes running names, panes and roles. Every change (including routed sends) needs approval; read-only tools do not. Commands: `/agents`, `/new`, `/board`, `/run <workflow>`, `/clear`.

New chat drops active history, not backups or completed actions. Interrupted approvals require inspection and acknowledgement before clearing. See [receptionist reference](../docs/board.md#web-receptionist) for tools, scope and configuration.
