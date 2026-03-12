BINARY_NAME ?= concierge
BIN_DIR ?= bin
CMD_PATH ?= ./cmd/concierge

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w \
	-X github.com/tensorleap/concierge/internal/buildinfo.Version=$(VERSION) \
	-X github.com/tensorleap/concierge/internal/buildinfo.Commit=$(COMMIT) \
	-X github.com/tensorleap/concierge/internal/buildinfo.Date=$(DATE)

UNIT_TEST_PACKAGES := $(shell go list ./... | grep -v '/internal/e2e/fixtures$$')

.PHONY: build test test-fixtures test-live-claude clean fixtures-prepare fixtures-mutate-cases fixtures-verify fixtures-reset fixtures-checks

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

test:
	go test $(UNIT_TEST_PACKAGES)

test-live-claude:
	CONCIERGE_LIVE_CLAUDE=1 go test ./internal/agent ./internal/cli -run 'LiveClaude' -v

test-fixtures: fixtures-prepare fixtures-verify
	go test ./internal/e2e/fixtures -v

clean:
	rm -rf $(BIN_DIR)

fixtures-prepare:
	bash scripts/fixtures_prepare.sh

fixtures-mutate-cases: fixtures-prepare
	bash scripts/fixtures_mutate_cases.sh

fixtures-verify: fixtures-mutate-cases
	bash scripts/fixtures_verify.sh

fixtures-reset: fixtures-prepare fixtures-verify

fixtures-checks: test-fixtures
