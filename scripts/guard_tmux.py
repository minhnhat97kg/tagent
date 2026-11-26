#!/usr/bin/env python3
"""Claude Code PreToolUse hook: refuse Bash commands that can kill the user's tmux server.

Agents run inside that server, so $TMUX points at it and wins over TMUX_TMPDIR:
an agent's "isolated" `tmux kill-server` once ended every running agent.
Self-check: python3 scripts/guard_tmux.py --test

ponytail: reads the command text only; a kill hidden in `sh -c`, an alias or a
script file passes. A tmux wrapper on the agents' PATH is the next step if so.
"""

import json
import os
import shlex
import sys

MESSAGE = (
    "Blocked: this can kill the user's tmux server and every running agent. "
    "$TMUX overrides TMUX_TMPDIR. For an isolated test pass a private socket on EVERY "
    "tmux call: tmux -L mytest new-session ...; tmux -L mytest kill-server"
)


def words(line):
    lex = shlex.shlex(line, posix=True, punctuation_chars=True)
    lex.whitespace_split = True
    try:
        return list(lex)
    except ValueError:  # unbalanced quotes, e.g. one line of a multi-line string
        return line.split()


def dangerous(command):
    for line in command.replace("\\\n", " ").splitlines():
        tokens = words(line) + [";"]
        for i, token in enumerate(tokens):
            name = os.path.basename(token)
            rest = tokens[i + 1 :]
            if name in ("pkill", "killall"):
                for arg in rest:
                    if arg[:1] in ";&|()":
                        break
                    if arg == "tmux":
                        return True
            if name != "tmux":
                continue
            private = False
            j = 0
            while j < len(rest) and rest[j].startswith("-"):
                flag, value = rest[j][:2], rest[j][2:]
                if flag in ("-L", "-S", "-f", "-c", "-T"):
                    if not value:
                        j += 1
                        value = rest[j] if j < len(rest) else ""
                    if flag == "-L":
                        private = value != "default"
                    if flag == "-S":
                        private = "/tmp/tmux-" not in value
                j += 1
            if j < len(rest) and rest[j].startswith("kill-ser") and not private:
                return True
    return False


def test():
    blocked = [
        "tmux kill-server",
        "export TMUX_TMPDIR=/tmp/t && tmux -L default kill-server; tmux new -d; x; tmux kill-server",
        "cd /tmp\ntmux kill-server 2>/dev/null",
        "TMUX= tmux kill-server",
        "/usr/bin/tmux -f /dev/null kill-ser",
        "(tmux kill-server)",
        "tmux -S /private/tmp/tmux-501/default kill-server",
        "pkill -9 tmux",
        "killall tmux",
    ]
    allowed = [
        "tmux -L mytest kill-server",
        "tmux -Lmytest kill-server",
        'tmux -S "$d/s" -f /dev/null kill-server',
        "tmux kill-session -t t1",
        "tmux ls",
        'grep -rn "tmux kill-server" docs',
        "git commit -m 'block tmux kill-server'",
        "pkill -f tagent; tmux ls",
    ]
    for command in blocked:
        assert dangerous(command), "should block: " + command
    for command in allowed:
        assert not dangerous(command), "should allow: " + command
    print("ok")


if __name__ == "__main__":
    if sys.argv[1:] == ["--test"]:
        test()
    elif dangerous(json.load(sys.stdin).get("tool_input", {}).get("command", "")):
        print(MESSAGE, file=sys.stderr)
        sys.exit(2)  # exit 2 blocks the tool call and shows stderr to the agent
