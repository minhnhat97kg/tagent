.PHONY: build install reload test fmt vet clean update-frames web

BIN := bin/tmux-agent

build:
	go build -o $(BIN) ./cmd/tmux-agent
	@if [ "$$(uname -s)" = Darwin ]; then codesign --force --sign - $(BIN); fi

BIN_DIR ?= $(HOME)/.local/bin
# install.sh checkout that ~/.tmux.conf loads the plugin from
SHARE_DIR ?= $(HOME)/.local/share/tmux-agent

# Quick local install: build, link into BIN_DIR, integrate harnesses, reload sidebars.
install: build
	mkdir -p $(BIN_DIR)
	ln -sf $(CURDIR)/$(BIN) $(BIN_DIR)/tmux-agent
	@if [ -d $(SHARE_DIR) ]; then mkdir -p $(SHARE_DIR)/bin && ln -sf $(CURDIR)/$(BIN) $(SHARE_DIR)/bin/tmux-agent && rsync -a --delete assets/ $(SHARE_DIR)/assets/; fi
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
