BINARY := carrito
PKG := ./cmd/carrito
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X github.com/wachtermar/carrito/internal/cli.Version=$(VERSION) -X github.com/wachtermar/carrito/internal/cli.Commit=$(COMMIT) -X github.com/wachtermar/carrito/internal/cli.Date=$(DATE)

.PHONY: build install test lint check snapshot live-test auth-live-test clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY) $(PKG)

install:
	install -d $(BINDIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINDIR)/$(BINARY) $(PKG)

test:
	go test ./...

lint:
	go vet ./...

check: test lint
	go build -trimpath -o /tmp/$(BINARY)-check $(PKG)
	rm -f /tmp/$(BINARY)-check

snapshot:
	goreleaser release --snapshot --clean

live-test: build
	tmpdir=$$(mktemp -d); \
	trap 'rm -rf "$$tmpdir"' EXIT; \
	CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) --help >/dev/null; \
	CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) set-market --region-id ac90d761-9d58-4918-a37d-dd14e1ce384a --retailer-region-id 5 --name Vaguada >/dev/null; \
	CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) search leche --limit 3; \
	CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) search leche --limit 3 --json >/dev/null; \
	CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) categories --json >/dev/null; \
	printf 'leche\narroz\nagua\n' | CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) batch -f -; \
	basket=$$(mktemp); \
	printf '54178 1\n205192 1\n' > "$$basket"; \
	CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) product 54178 --json >/dev/null; \
	CARRITO_CONFIG_DIR="$$tmpdir" ./$(BINARY) total -f "$$basket" --json >/dev/null; \
	rm -f "$$basket"

auth-live-test: build
	@if [ "$$CARRITO_LIVE_AUTH" != "1" ]; then \
		echo "set CARRITO_LIVE_AUTH=1 to run authenticated live checks"; \
		exit 2; \
	fi
	./$(BINARY) whoami --json
	./$(BINARY) addresses --json >/dev/null
	./$(BINARY) cart get --json >/dev/null
	@if [ -n "$$CARRITO_TEST_ADDRESS_ID" ]; then \
		./$(BINARY) checkout slots --address "$$CARRITO_TEST_ADDRESS_ID" --json >/dev/null; \
	else \
		./$(BINARY) checkout slots --json >/dev/null; \
	fi
	@if [ -n "$$CARRITO_TEST_ADD_REF" ]; then \
		./$(BINARY) cart add "$$CARRITO_TEST_ADD_REF" "$${CARRITO_TEST_ADD_QTY:-1}" --max "$${CARRITO_TEST_MAX_EUR:-20}" --json >/dev/null; \
	fi
	@if [ -n "$$CARRITO_TEST_SLOT_ID" ]; then \
		./$(BINARY) checkout select-slot --slot "$$CARRITO_TEST_SLOT_ID" --max "$${CARRITO_TEST_MAX_EUR:-50}" --json >/dev/null; \
	fi

clean:
	rm -f $(BINARY)
