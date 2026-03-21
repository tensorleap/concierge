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
PYTHON ?= python3
QA_DIR ?= QA
QA_TEST_DIR ?= $(QA_DIR)/tests
REPO ?=
QA_STEP ?=
QA_ARGS ?=
QA_IMAGE_MODE ?=

.PHONY: build install test test-qa-loop test-fixtures test-live-claude clean fixtures-prepare fixtures-mutate-cases fixtures-verify fixtures-reset fixtures-checks qa

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

install: build
	@cp $(BIN_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME) 2>/dev/null \
		|| cp $(BIN_DIR)/$(BINARY_NAME) $(HOME)/go/bin/$(BINARY_NAME) 2>/dev/null \
		|| (echo "Installing to /usr/local/bin (may require sudo)..." && sudo install -m 755 $(BIN_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME))

test:
	go test $(UNIT_TEST_PACKAGES)
	$(PYTHON) -m unittest discover -s $(QA_TEST_DIR) -p 'test_*.py' -v

test-qa-loop:
	$(PYTHON) -m unittest discover -s $(QA_TEST_DIR) -p 'test_*.py' -v

test-live-claude:
	CONCIERGE_LIVE_CLAUDE=1 go test ./internal/agent ./internal/cli -run 'LiveClaude' -v

test-fixtures: fixtures-prepare fixtures-verify
	go test ./internal/e2e/fixtures -v

qa:
	bash scripts/qa_fixture_run.sh $(if $(strip $(REPO)),--repo "$(REPO)") $(if $(strip $(QA_STEP)),--step "$(QA_STEP)") $(if $(strip $(QA_IMAGE_MODE)),--image-mode "$(QA_IMAGE_MODE)") -- $(QA_ARGS)

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
