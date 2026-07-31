# Development tasks for lazypr.
#
# The formatting standard is gofmt plus goimports with a local-prefix group, so
# imports split into up to three groups, always ordered stdlib, third-party, then
# this module. goimports is pinned because an unpinned tool would let the standard
# drift silently between machines and CI.

GOIMPORTS_VERSION ?= v0.48.0
GOIMPORTS := go run golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
LOCAL_PREFIX := github.com/DreadPirateRob/LazyPrReview

# Tracked files only: never reach into build output or the module cache.
GO_FILES := $(shell git ls-files '*.go')
BIN := bin/lazypr

# Bare `make` verifies and builds. `help` is defined first for readability, so the
# default goal must be set explicitly rather than inherited from target order.
.DEFAULT_GOAL := all

.PHONY: help all fmt fmt-check vet test e2e build check clean

help: ## List available targets
	@grep -hE '^[a-z0-9-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*##/\t/' | expand -t20

all: check build ## Verify everything, then build

fmt: ## Format every tracked Go file
	gofmt -w $(GO_FILES)
	$(GOIMPORTS) -local $(LOCAL_PREFIX) -w $(GO_FILES)

fmt-check: ## Fail on formatting drift (never writes)
	@drift="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$drift" ]; then \
		echo "gofmt drift - run 'make fmt':"; echo "$$drift"; exit 1; \
	fi
	@drift="$$($(GOIMPORTS) -local $(LOCAL_PREFIX) -l $(GO_FILES))"; \
	if [ -n "$$drift" ]; then \
		echo "import-group drift - run 'make fmt':"; echo "$$drift"; exit 1; \
	fi
	@echo "format clean"

vet: ## Run go vet
	go vet ./...

test: ## Run the unit suite
	go test ./...

e2e: ## Run the live E2E smoke test (needs an authenticated gh)
	LAZYPR_E2E=1 go test ./cmd/lazypr -run TestE2EReadOnlySmoke -count=1

build: ## Build the binary into bin/
	go build -o $(BIN) ./cmd/lazypr

check: fmt-check vet test ## Everything CI enforces

clean: ## Remove build output
	rm -rf bin
