BINARY := alcampo
PKG := ./cmd/alcampo

.PHONY: build test lint live-test auth-live-test clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) $(PKG)

test:
	go test ./...

lint:
	go vet ./...

live-test: build
	tmpdir=$$(mktemp -d); \
	trap 'rm -rf "$$tmpdir"' EXIT; \
	ALCAMPO_CONFIG_DIR="$$tmpdir" ./$(BINARY) --help >/dev/null; \
	ALCAMPO_CONFIG_DIR="$$tmpdir" ./$(BINARY) set-market --region-id ac90d761-9d58-4918-a37d-dd14e1ce384a --retailer-region-id 5 --name Vaguada >/dev/null; \
	ALCAMPO_CONFIG_DIR="$$tmpdir" ./$(BINARY) search leche --limit 3; \
	ALCAMPO_CONFIG_DIR="$$tmpdir" ./$(BINARY) search leche --limit 3 --json >/dev/null; \
	ALCAMPO_CONFIG_DIR="$$tmpdir" ./$(BINARY) categories --json >/dev/null; \
	printf 'leche\narroz\nagua\n' | ALCAMPO_CONFIG_DIR="$$tmpdir" ./$(BINARY) batch -f -; \
	basket=$$(mktemp); \
	printf '54178 1\n205192 1\n' > "$$basket"; \
	ALCAMPO_CONFIG_DIR="$$tmpdir" ./$(BINARY) product 54178 --json >/dev/null; \
	ALCAMPO_CONFIG_DIR="$$tmpdir" ./$(BINARY) total -f "$$basket" --json >/dev/null; \
	rm -f "$$basket"

auth-live-test: build
	@if [ "$$ALCAMPO_LIVE_AUTH" != "1" ]; then \
		echo "set ALCAMPO_LIVE_AUTH=1 to run authenticated live checks"; \
		exit 2; \
	fi
	./$(BINARY) whoami --json
	./$(BINARY) addresses --json >/dev/null
	./$(BINARY) cart get --json >/dev/null
	@if [ -n "$$ALCAMPO_TEST_ADDRESS_ID" ]; then \
		./$(BINARY) checkout slots --address "$$ALCAMPO_TEST_ADDRESS_ID" --json >/dev/null; \
	else \
		./$(BINARY) checkout slots --json >/dev/null; \
	fi
	@if [ -n "$$ALCAMPO_TEST_ADD_REF" ]; then \
		./$(BINARY) cart add "$$ALCAMPO_TEST_ADD_REF" "$${ALCAMPO_TEST_ADD_QTY:-1}" --max "$${ALCAMPO_TEST_MAX_EUR:-20}" --json >/dev/null; \
	fi
	@if [ -n "$$ALCAMPO_TEST_SLOT_ID" ]; then \
		./$(BINARY) checkout select-slot --slot "$$ALCAMPO_TEST_SLOT_ID" --max "$${ALCAMPO_TEST_MAX_EUR:-50}" --json >/dev/null; \
	fi

clean:
	rm -f $(BINARY)
