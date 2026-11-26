#!/bin/sh
# ship: verify, commit, merge into main, build and reload.
#   scripts/ship.sh "commit message"   (no message needed when there is nothing to commit)
# It commits every change in the checkout, so check `git status` first when others share it.
set -eu
msg=${1:-}
cd "$(dirname "$0")/.."

[ "$(git rev-parse --git-dir)" = "$(git rev-parse --git-common-dir)" ] || { echo "ship: run it from the main checkout, not a worktree"; exit 1; }
[ -z "$(git status --porcelain)" ] || [ -n "$msg" ] || { echo "ship: uncommitted changes, give a commit message"; exit 1; }

echo "== verify"
./scripts/check.sh
(cd web && npm test >/dev/null 2>&1) || { echo "web tests failed (web: npm test)"; exit 1; }

if [ -n "$(git status --porcelain)" ]; then
  echo "== commit on $(git branch --show-current)"
  git add -A
  git diff --cached --stat
  git commit -q -m "$msg"
fi

if [ "$(git branch --show-current)" != main ]; then
  echo "== merge into main"
  # Fast-forward only, without leaving this branch. Does not push.
  git fetch -q . HEAD:main || echo "ship: WARNING: main not merged (not a fast-forward, or checked out elsewhere)"
fi

echo "== build"
make build

echo "== reload"
# A dedicated `tagent web --addr X` holds web.lock while the scraper waits for it.
pids() { pgrep -f "^([^ ]*/)?tagent $1" || true; }
mask() { sed -E 's/(token=)[^ &]+/\1***/g'; }
old=$(pids 'web( |$)' | head -n 1)
addr=${SHIP_WEB_ADDR:-}
if [ -z "$addr" ] && [ -n "$old" ]; then
  addr=$(ps -o command= -p "$old" | sed -nE 's/.*--?addr[= ]([^ ]+).*/\1/p')
  addr=${addr:-127.0.0.1:7777}
fi
if [ -z "$addr" ]; then
  # No dedicated web server: the new scraper serves it when @agent_web is on.
  for p in $(pids 'scrape$'); do kill "$p" 2>/dev/null || true; done
  bin/tagent reload
  exit 0
fi

log="${TMPDIR:-/tmp}"
log="${log%/}/tagent-web${addr##*:}.log"
url="http://$(echo "$addr" | sed 's/^0\.0\.0\.0:/127.0.0.1:/')/"
up=
for try in 1 2 3; do
  # Scrapers go first: otherwise one takes web.lock as soon as the old server
  # dies, serves on @agent_web_addr, and the new server only forwards to it.
  for p in $(pids 'scrape$') $(pids 'web( |$)'); do kill "$p" 2>/dev/null || true; done
  n=0
  while [ -n "$(pids 'web( |$)')" ] && [ "$n" -lt 50 ]; do sleep 0.1; n=$((n + 1)); done
  nohup "$PWD/bin/tagent" web --addr "$addr" >>"$log" 2>&1 </dev/null &
  new=$!
  sleep 2
  if kill -0 "$new" 2>/dev/null && curl -s -m 5 -o /dev/null "$url"; then up=1; break; fi
  echo "ship: web did not come up (try $try)"
done

# Always restart the scraper and sidebars, also when the web failed.
bin/tagent reload

if [ -z "$up" ]; then
  echo "ship: web is not serving on $addr; last log lines:"
  tail -n 5 "$log" | mask
  exit 1
fi
echo "ship: web is up on $addr (pid $new)"
