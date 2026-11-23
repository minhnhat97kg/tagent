.PHONY: build install reload test fmt vet clean update-frames web

BIN := bin/tagent

build:
	go build -o $(BIN) ./cmd/tagent
	@if [ "$$(uname -s)" = Darwin ]; then codesign --force --sign - $(BIN); fi

BIN_DIR ?= $(HOME)/.local/bin
# install.sh checkout that ~/.tmux.conf loads the plugin from
SHARE_DIR ?= $(HOME)/.local/share/tagent

# Quick local install: build, link into BIN_DIR, integrate harnesses, reload sidebars.
install: build
	@# A worktree (e.g. a ticket agent) must not repoint the global install to its branch.
	@test "$$(git rev-parse --git-dir)" = "$$(git rev-parse --git-common-dir)" || { echo "make install: run it from the main checkout, not a worktree"; exit 1; }
	mkdir -p $(BIN_DIR)
	ln -sf $(CURDIR)/$(BIN) $(BIN_DIR)/tagent
	ln -sf tagent $(BIN_DIR)/tmux-agent
	ln -sf tagent bin/tmux-agent
	mkdir -p $(SHARE_DIR)/bin
	ln -sf $(CURDIR)/$(BIN) $(SHARE_DIR)/bin/tagent
	rsync -a --delete assets/ $(SHARE_DIR)/assets/
	bash assets/install-compat.sh "$(SHARE_DIR)"
	$(BIN) install
	-$(BIN) reload

reload: build
	$(BIN) reload

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

# Re-capture every golden frame. Only run after checking the frames by eye.
update-frames: build
	UPDATE_FRAMES=1 go test ./internal/frametest/... -v

clean:
	rm -rf bin

# Build the browser UI into internal/web/dist (embedded by the binary; commit the result).
web:
	cd web && npm install && npm run build
