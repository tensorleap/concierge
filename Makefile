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

.PHONY: build test clean

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

test:
	go test ./...

clean:
	rm -rf $(BIN_DIR)
