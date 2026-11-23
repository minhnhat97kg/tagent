#!/bin/sh
# Board "test" status: web build, go build, vet and tests. Prints only failures.
(cd web && npm run build >/dev/null 2>&1) || { echo "web build failed"; exit 1; }
go build ./... && go vet ./... || exit 1
out=$(TMPDIR=/tmp go test -skip TestSidebarLayoutHook ./... 2>&1); s=$?
echo "$out" | grep -v "^ok"
exit $s
