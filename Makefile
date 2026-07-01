# Shinari developer tasks. Run `make help` for the list.
# The CI workflow (.github/workflows/main.yml) runs the same commands, so a
# green `make check` locally means a green build.

GO            ?= go
BIN           ?= shinari
GOBIN         := $(shell $(GO) env GOPATH)/bin
GOLANGCI      := $(GOBIN)/golangci-lint
GOLANGCI_VER  ?= v2.12.2

.PHONY: help build test test-race cover lint fmt vet check tools hooks demo clean

help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build the CLI (binary: shinari)
	$(GO) build -o $(BIN) ./cli

test: ## Run the unit test suite
	$(GO) test ./...

test-race: ## Run tests with the race detector
	$(GO) test -race ./...

cover: ## Run tests and write a coverage profile + summary
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1
	@echo "HTML report: $(GO) tool cover -html=coverage.out"

lint: ## Run golangci-lint (install with `make tools`)
	$(GOLANGCI) run ./...

fmt: ## Format all Go source with gofmt
	gofmt -w .

vet: ## Run go vet
	$(GO) vet ./...

check: lint test-race ## Run everything CI runs (lint + race tests)

tools: ## Install pinned dev tools (golangci-lint)
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VER)

hooks: ## Install the pre-commit git hook
	@ln -sf ../../scripts/hooks/pre-commit .git/hooks/pre-commit
	@chmod +x scripts/hooks/pre-commit
	@echo "Installed .git/hooks/pre-commit -> scripts/hooks/pre-commit"

demo: build ## Render the TUI homepage demo from docs/tapes/tui.tape (needs vhs, ttyd, ffmpeg, docker)
	@command -v vhs >/dev/null 2>&1 || { echo "vhs not found — install: go install github.com/charmbracelet/vhs@latest (also needs ttyd + ffmpeg)"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "docker not found — the recorded scenario brings up a Redis container"; exit 1; }
	@mkdir -p docs/assets/media
	vhs docs/tapes/tui.tape
	@echo "Rendered docs/assets/media/tui.{mp4,gif} — commit the assets to publish."

clean: ## Remove build/coverage artifacts
	rm -f $(BIN) coverage.out
