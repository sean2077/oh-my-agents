GO ?= go
GIT ?= git
PYTHON ?= python
GOLANGCI_LINT ?= golangci-lint
ifneq ($(findstring .exe,$(SHELL)),)
BASH ?= $(subst /bin/sh.exe,/bin/bash.exe,$(subst /usr/bin/sh.exe,/bin/bash.exe,$(SHELL)))
else
BASH ?= bash
endif
BIN ?= oma
VERSION ?= dev

# GNU Make 3.81 on Windows cannot reliably run $(shell ...) when its configured
# POSIX SHELL path contains spaces. Resolve provenance inside the recipe shell,
# which also keeps build/install on one fail-closed stamping path.
define stamped_go
	@set -eu; \
	git_version="$$($(GIT) describe --tags --always 2>/dev/null || printf '%s' dev)"; \
	git_commit="$$($(GIT) rev-parse --short HEAD 2>/dev/null || printf '%s' none)"; \
	git_dirty=""; \
	if [ -n "$$($(GIT) status --porcelain 2>/dev/null)" ]; then git_dirty="-dirty"; fi; \
	build_version="$(VERSION)"; \
	if [ "$$build_version" = dev ]; then build_version="$$git_version"; fi; \
	build_version="$$build_version$$git_dirty"; \
	$(GO) $(1) -trimpath \
		-ldflags "-s -w -X github.com/sean2077/oh-my-agents/internal/version.Version=$$build_version -X github.com/sean2077/oh-my-agents/internal/version.Commit=$$git_commit$$git_dirty" \
		$(2) ./cmd/oma
endef

.DEFAULT_GOAL := help

.PHONY: help build install test vet fmt fmt-check agent-check tooling-check lint check ci release clean hooks

help:
	@printf '%s\n' \
		"Targets:" \
		"  build       Build stamped ./cmd/oma to ./$(BIN)" \
		"  install     Install ./cmd/oma using go install" \
		"  test        Run the full Go test suite" \
		"  vet         Run go vet" \
		"  fmt         Format Go sources in place" \
		"  fmt-check   Fail if Go sources need formatting" \
		"  agent-check Verify agent-harness links and generated projections" \
		"  tooling-check Reconcile committed command surfaces with the manifest" \
		"  lint        Run golangci-lint" \
		"  check       Run agent-check, tooling-check, fmt-check, vet, test, and build" \
		"  ci          Run check plus lint" \
		"  release     Build release assets with VERSION=vX.Y.Z" \
		"  clean       Remove local build outputs" \
		"  hooks       Enable repo git hooks (harness + content guards)"

build:
	$(call stamped_go,build,-o "$(BIN)")

install:
	$(call stamped_go,install,)

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

agent-check:
	$(PYTHON) .agents/symlink-manager.py verify --repo .
	$(PYTHON) .agents/tools/generate-subagents.py --check

tooling-check:
	"$(BASH)" tools/manifest-check.sh

lint:
	@command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1 || { \
		echo "missing $(GOLANGCI_LINT); install golangci-lint or set GOLANGCI_LINT=/path/to/golangci-lint" >&2; \
		exit 127; \
	}
	$(GOLANGCI_LINT) run

check: agent-check tooling-check fmt-check vet test build

ci: check lint

release:
	@test "$(VERSION)" != "dev" || { echo "set VERSION=vX.Y.Z" >&2; exit 2; }
	"$(BASH)" tools/release/build-release.sh "$(VERSION)"

clean:
	rm -rf dist $(BIN)

hooks:
	git config core.hooksPath tools/git-hooks
	@echo "git hooks enabled (tools/git-hooks); harness, tooling, and content guards active on commit."
