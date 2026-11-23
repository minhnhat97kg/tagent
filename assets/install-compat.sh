#!/usr/bin/env bash
# Preserve the plugin entrypoint referenced by existing tmux configurations.
set -eu
canonical="$1"
legacy="$HOME/.local/share/tmux-agent/assets/tmux-agent.tmux"
mkdir -p "$(dirname "$legacy")"
# Replace only our old plugin or our forwarding shim; preserve unrelated files.
if [ ! -e "$legacy" ] || grep -qE 'Derived from hiroppy/tmux-agent-sidebar|tagent compatibility shim' "$legacy"; then
  printf '#!/usr/bin/env bash\n# tagent compatibility shim\nexec bash %q\n' "$canonical/assets/tagent.tmux" > "$legacy"
fi
