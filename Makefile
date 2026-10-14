# jailwick build. `make` builds the Linux agents (embedded) and the jailwick binary into bin/.
# The version is in the VERSION file (embedded: every build knows it); Go stamps the commit.
VERSION := $(shell cat VERSION)
LDFLAGS := -s -w
# The agent carries no version or VCS stamp: its bytes change only when its code does, so a new
# jailwick build doesn't make running sandboxes "pending" for an agent that does the same thing.
AGENT_FLAGS := -trimpath -buildvcs=false -ldflags "-s -w"
AGENT_DIR := internal/assets/agent

.PHONY: all agent build app app-windows winres web docs test lint install clean

HOST_OS := $(shell go env GOOS)
# One minimum macOS for Go and the cgo parts (otherwise the linker warns that objects were built
# for a newer macOS than the binary claims to need).
export MACOSX_DEPLOYMENT_TARGET ?= 13.0

# docs/CLI.md is generated from the command definitions (a test checks it's current).
docs:
	go run ./tools/gendocs docs/CLI.md

all: build

# The front end (web/, Svelte + Panda CSS; needs Node 22+) builds into internal/webui/dist, which
# the Go binaries embed. dist is not committed: the targets that compile Go build it first, and
# again whenever web/ changes. `make web` always rebuilds it.
WEB_DIST := internal/webui/dist/index.html
WEB_SRC := $(shell find web/src web/public -type f) \
	$(wildcard web/index.html web/*.json web/*.ts web/*.js web/*.cjs)

web:
	cd web && npm ci --no-audit --no-fund && npm run build

$(WEB_DIST): $(WEB_SRC)
	$(MAKE) web

agent:
	@go run ./tools/agentinfo $(AGENT_DIR)/agent-info.json
	@for arch in arm64 amd64; do \
	  CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build $(AGENT_FLAGS) \
	    -o $(AGENT_DIR)/jailwick-agent-linux-$$arch ./cmd/jailwick-agent || exit 1; \
	done

build: agent $(WEB_DIST)
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/jailwick ./cmd/jailwick

# The desktop app (Wails; needs cgo, and on macOS Xcode's command line tools), for this OS:
#   macOS    bin/jailwick.app, a regular app (Launchpad, Spotlight, the Dock); install copies it
#            to ~/Applications
#   Linux    bin/jailwick-app; install adds a desktop entry and its icon
#   Windows  `make app-windows` (from any OS): bin/jailwick-app.exe, with its icon
# `jw app` opens it; without it, the same app runs in the browser.
APP_PKG := cmd/jailwick-app/packaging
APP_BUNDLE := bin/jailwick.app

app: agent $(WEB_DIST)
	CGO_ENABLED=1 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/jailwick-app ./cmd/jailwick-app
ifeq ($(HOST_OS),darwin)
	rm -rf $(APP_BUNDLE)
	mkdir -p $(APP_BUNDLE)/Contents/MacOS $(APP_BUNDLE)/Contents/Resources
	cp bin/jailwick-app $(APP_BUNDLE)/Contents/MacOS/jailwick-app
	cp $(APP_PKG)/darwin/jailwick.icns $(APP_BUNDLE)/Contents/Resources/jailwick.icns
	sed 's/>VERSION</>$(VERSION)</' $(APP_PKG)/darwin/Info.plist > $(APP_BUNDLE)/Contents/Info.plist
	codesign --force --sign - $(APP_BUNDLE)
endif

app-windows: agent $(WEB_DIST)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS) -H windowsgui" \
	  -o bin/jailwick-app.exe ./cmd/jailwick-app

# The Windows app's icon and manifest, as .syso files that `go build` links in (committed; rebuild
# after the icon changes).
winres:
	go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64,arm64 --manifest gui \
	  --icon cmd/jailwick-app/icons/app-night.png --product-name jailwick --file-description jailwick \
	  --original-filename jailwick-app.exe --product-version $(VERSION) --file-version $(VERSION) \
	  --out cmd/jailwick-app/rsrc

test: $(WEB_DIST)
	go test -race ./...

lint: $(WEB_DIST)
	gofmt -l . | (! grep .)
	go vet ./...
	@if command -v shellcheck >/dev/null; then \
	  shellcheck -s sh -x -P SCRIPTDIR internal/assets/images/project/*.sh internal/assets/images/stacks/*.sh; \
	else echo "shellcheck not installed: image scripts not checked"; fi

# Installs the CLI to ~/.local/bin (override with PREFIX=/usr/local) and, once built with
# `make app`, the desktop app: macOS to ~/Applications (override with APPS=/Applications), Linux
# to PREFIX with a desktop entry.
PREFIX ?= $(HOME)/.local
APPS ?= $(HOME)/Applications
install: build
	install -d $(PREFIX)/bin
	install -m 0755 bin/jailwick $(PREFIX)/bin/jailwick
ifeq ($(HOST_OS),darwin)
	if [ -d $(APP_BUNDLE) ]; then mkdir -p $(APPS) && rm -rf $(APPS)/jailwick.app && cp -R $(APP_BUNDLE) $(APPS)/; fi
else
	if [ -f bin/jailwick-app ]; then \
	  install -m 0755 bin/jailwick-app $(PREFIX)/bin/jailwick-app && \
	  install -d $(PREFIX)/share/applications $(PREFIX)/share/icons/hicolor/512x512/apps && \
	  sed 's|^Exec=.*|Exec=$(PREFIX)/bin/jailwick-app|' $(APP_PKG)/linux/jailwick.desktop \
	    > $(PREFIX)/share/applications/jailwick.desktop && \
	  install -m 0644 $(APP_PKG)/linux/jailwick.png $(PREFIX)/share/icons/hicolor/512x512/apps/jailwick.png; \
	fi
endif

clean:
	rm -rf bin $(AGENT_DIR)/jailwick-agent-linux-* $(AGENT_DIR)/agent-info.json
