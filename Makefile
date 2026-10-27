# degoploy build tasks. Requires Go (see go.mod) and Node.js 24+.

# Use user-local toolchains (~/.local/share/toolchains/{go,node}) when present,
# so make works even if they are not on the shell PATH.
TOOLCHAINS := $(HOME)/.local/share/toolchains
ifneq ($(wildcard $(TOOLCHAINS)/go/bin/go),)
export PATH := $(TOOLCHAINS)/go/bin:$(PATH)
endif
ifneq ($(wildcard $(TOOLCHAINS)/node/bin/node),)
export PATH := $(TOOLCHAINS)/node/bin:$(PATH)
endif

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
ARCHES  ?= amd64 arm64
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN     := bin/degoploy
DIST    := internal/web/dist

.PHONY: all build web types deps dev dev-api dev-web test lint check dist clean

all: build

## build: single static binary with the web UI embedded
build: types web
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/degoploy

## web: build the SvelteKit SPA into internal/web/dist
web: web/node_modules
	cd web && npm run build

web/node_modules: web/package-lock.json
	cd web && npm ci
	@touch $@

## types: regenerate TypeScript API types from internal/api/types.go
types: web/node_modules
	go tool tygo generate
	cd web && npx prettier --write src/lib/api/types.gen.ts >/dev/null

## deps: install Go and npm dependencies
deps:
	go mod download
	cd web && npm ci

## dev: run API (127.0.0.1:8090) and Vite dev server (0.0.0.0:5173) together
dev:
	@$(MAKE) -j2 dev-api dev-web

dev-api: $(DIST)/index.html
	go run ./cmd/degoploy serve --config degoploy.dev.yaml --debug

dev-web: web/node_modules
	cd web && npm run dev

## test: Go tests (the embed dir must exist, so a stub is created if the UI is not built)
test: $(DIST)/index.html
	go test ./...

## check: vet, type-check and lint everything
check: $(DIST)/index.html
	go vet ./...
	cd web && npm run check && npm run lint

$(DIST)/index.html:
	@mkdir -p $(DIST)
	@echo '<!doctype html><title>degoploy</title><p>Web UI not built. Run <code>make web</code>.</p>' > $@

## dist: release tarballs (linux/$(ARCHES)) with binary, installer, unit and docs in release/
dist: types web
	@rm -rf release && mkdir -p release
	@set -e; for arch in $(ARCHES); do \
		name=degoploy-$(VERSION)-linux-$$arch; dir=release/$$name; \
		echo "building $$name"; mkdir -p $$dir; \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o $$dir/degoploy ./cmd/degoploy; \
		cp deploy/install.sh deploy/degoploy.service degoploy.example.yaml README.md docs/INSTALL.md $$dir/; \
		tar -C release -czf release/$$name.tar.gz $$name; rm -rf $$dir; \
	done
	cd release && sha256sum *.tar.gz > SHA256SUMS && cat SHA256SUMS

clean:
	rm -rf bin release $(DIST) web/.svelte-kit
