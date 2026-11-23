#!/bin/sh
# Board "test" job: web build, go build, vet and tests. Prints only failures.
# A workflow worktree does not copy ignored dependencies, so install them first.
if [ ! -x web/node_modules/.bin/vite ]; then
  (cd web && npm ci >/dev/null 2>&1) || { echo "web deps install failed (web: npm ci)"; exit 1; }
fi
(cd web && npm run build >/dev/null 2>&1) || { echo "web build failed"; exit 1; }
go build ./... && go vet ./... || exit 1
out=$(TMPDIR=/tmp go test -skip TestSidebarLayoutHook ./... 2>&1); s=$?
echo "$out" | grep -v "^ok"
exit $s
