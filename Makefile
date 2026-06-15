GO ?= go
GOLANGCI_LINT ?= golangci-lint
BIN ?= oma
VERSION ?= dev
GIT_VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
GIT_DIRTY := $(shell test -z "$$(git status --porcelain 2>/dev/null)" || echo -dirty)
BUILD_VERSION ?= $(if $(filter dev,$(VERSION)),$(GIT_VERSION),$(VERSION))$(GIT_DIRTY)
LD_FLAGS := -s -w -X github.com/sean2077/oh-my-agents/internal/version.Version=$(BUILD_VERSION) -X github.com/sean2077/oh-my-agents/internal/version.Commit=$(GIT_COMMIT)$(GIT_DIRTY)

.DEFAULT_GOAL := help

.PHONY: help build install test vet fmt fmt-check lint check ci release clean

help:
	@printf '%s\n' \
		"Targets:" \
		"  build       Build stamped ./cmd/oma to ./$(BIN)" \
		"  install     Install ./cmd/oma using go install" \
		"  test        Run the full Go test suite" \
		"  vet         Run go vet" \
		"  fmt         Format Go sources in place" \
		"  fmt-check   Fail if Go sources need formatting" \
		"  lint        Run golangci-lint" \
		"  check       Run fmt-check, vet, test, and build" \
		"  ci          Run check plus lint" \
		"  release     Build release assets with VERSION=vX.Y.Z" \
		"  clean       Remove local build outputs"

build:
	$(GO) build -trimpath -ldflags '$(LD_FLAGS)' -o $(BIN) ./cmd/oma

install:
	$(GO) install -trimpath -ldflags '$(LD_FLAGS)' ./cmd/oma

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

fmt-check:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then \
		echo "files need gofmt:" >&2; \
		echo "$$out" >&2; \
		exit 1; \
	fi

lint:
	@command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1 || { \
		echo "missing $(GOLANGCI_LINT); install golangci-lint or set GOLANGCI_LINT=/path/to/golangci-lint" >&2; \
		exit 127; \
	}
	$(GOLANGCI_LINT) run

check: fmt-check vet test build

ci: check lint

release:
	@test "$(VERSION)" != "dev" || { echo "set VERSION=vX.Y.Z" >&2; exit 2; }
	scripts/build-release.sh "$(VERSION)"

clean:
	rm -rf dist $(BIN)
