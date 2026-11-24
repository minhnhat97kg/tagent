#!/usr/bin/env bash
# Derived from hiroppy/tmux-agent-sidebar (MIT) — see NOTICE.

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -x "$PLUGIN_DIR/bin/tagent" ]]; then
    SIDEBAR_BINARY="$PLUGIN_DIR/bin/tagent"
elif command -v tagent &>/dev/null; then
    SIDEBAR_BINARY="$(command -v tagent)"
else
    tmux display-message "tagent: binary not built — run 'make build' in $PLUGIN_DIR"
    exit 0
fi

tmux set -g @agent_sidebar_bin "$SIDEBAR_BINARY"
# Settings saved by `tagent config` — before the defaults below, which
# only fill options still unset.
CONFIG_FILE="${XDG_CONFIG_HOME:-$HOME/.config}/tagent/config.conf"
[ -f "$CONFIG_FILE" ] && tmux source-file "$CONFIG_FILE"
tmux source-file "$PLUGIN_DIR/assets/agent-sidebar.conf"
