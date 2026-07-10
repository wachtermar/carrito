BINARY := carrito
PKG := ./cmd/carrito
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X github.com/wachtermar/carrito/internal/cli.Version=$(VERSION) -X github.com/wachtermar/carrito/internal/cli.Commit=$(COMMIT) -X github.com/wachtermar/carrito/internal/cli.Date=$(DATE)

.PHONY: build install fmt test lint installer-test check live-read-test auth-read-test clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY) $(PKG)

install:
	install -d $(BINDIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINDIR)/$(BINARY) $(PKG)

fmt:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -print))" || { gofmt -l $$(find . -name '*.go' -print); exit 1; }

test:
	go test -count=1 ./...

lint:
	go vet ./...

installer-test:
	bash testdata/installer_clone_smoke.sh

check: fmt test lint installer-test
	go build -trimpath -o /tmp/$(BINARY)-check $(PKG)

live-read-test: build
	tmpdir=$$(mktemp -d); \
	trap 'rm -rf "$$tmpdir"' EXIT; \
	CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) set-market --region-id ac90d761-9d58-4918-a37d-dd14e1ce384a --retailer-region-id 5 --name Vaguada >/dev/null; \
	CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) search leche --limit 3 --json >/dev/null; \
	CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) product 54178 --json >/dev/null

auth-read-test: build
	@if [ "$$CARRITO_LIVE_AUTH" != "1" ]; then echo "set CARRITO_LIVE_AUTH=1 to run authenticated reads"; exit 2; fi
	./$(BINARY) whoami --json
	./$(BINARY) cart get --json >/dev/null

clean:
	rm -f $(BINARY)
