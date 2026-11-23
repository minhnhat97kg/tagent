# Terminal size handoff

Web agent headers offer **Handoff** and **Release**. Handoff sizes the pane's
window for this device, even with a Mac terminal attached. Only one viewer owns
a pane: another browser's Handoff takes over. Other viewers scale to fit and show
`Viewing at COLSxROWS · Handoff`. Background panes auto-handoff to an active viewer.

To reclaim sizing from a terminal, run `tagent handoff [PANE]` (defaults to
`$TMUX_PANE`), or press **f** on an agent in the sidebar/dashboard Agents list.
The key is specific to Agents; ticket and workflow shortcuts are unchanged.
Browser-owned rows show **sized by web**. Releasing or closing the owner's tab
also returns sizing to tmux's attached terminals.

Ownership is stored in the tmux pane option `@agent_size_owner`: `web:<viewer-id>`
for a browser, empty for terminal-controlled sizing. Web servers reread it before
resizing and when reporting ownership, so another server, CLI or TUI can revoke
ownership. A revoked viewer cannot silently reclaim it through passive resize
messages. The original local/inherited `window-size` policy is saved in
`@agent_size_policy` across takeovers, restored on release, and the window is
recomputed with `resize-window -A`. Sizing affects the whole window, not only a
split's display area. With no attached terminal, tmux chooses its default size.

Owners follow viewport/font/keyboard changes (12px phones, 13px otherwise).
Non-owners keep width-fit scaling with a 9px floor; xterm owns scrollback.
