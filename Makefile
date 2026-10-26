.DEFAULT_GOAL := help

GO ?= go
TOOL_MOD := tools/go.mod
gotool = "$(GO)" tool -modfile="$(TOOL_MOD)" $(1)

# Binary output directory and name
BIN_DIR := bin
OUTPUT_DIR := output
BINARY_NAME := $(BIN_DIR)/hyperfleet-e2e

# Version information
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_DIRTY ?= $(shell git diff --quiet 2>/dev/null || echo "-modified")
VERSION:=$(GIT_SHA)$(GIT_DIRTY)

# Go build flags
LDFLAGS := -X main.version=$(VERSION) \
           -X main.commit=$(GIT_COMMIT) \
           -X main.date=$(BUILD_DATE)

# Container tool (docker or podman)
CONTAINER_TOOL ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

# =============================================================================
# Image Configuration
# =============================================================================
IMAGE_REGISTRY ?= quay.io/openshift-hyperfleet
IMAGE_NAME ?= hyperfleet-e2e
IMAGE_TAG ?= $(VERSION)

# Dev image configuration - set QUAY_USER to push to personal registry
# Usage: QUAY_USER=myuser make image-dev
QUAY_USER ?=
DEV_TAG ?= dev-$(GIT_SHA)

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Code Generation

.PHONY: generate
generate: ## No-op (reserved for future code generation)
	@:

##@ Development

.PHONY: build
build: generate ## Build the hyperfleet-e2e binary
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/hyperfleet-e2e

.PHONY: install
install: ## Build and install binary to GOPATH/bin
	$(GO) install -ldflags "$(LDFLAGS)" ./cmd/hyperfleet-e2e

.PHONY: run
run: build ## Build and run with help
	./$(BINARY_NAME) --help

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	rm -rf $(OUTPUT_DIR)
	rm -f coverage.out coverage.html

##@ Testing

FLAKE_ATTEMPTS ?= 2

.PHONY: test
test: generate ## Run unit tests
	$(GO) test -v -race -cover -coverprofile=coverage.out ./pkg/... ./cmd/...

.PHONY: test-coverage
test-coverage: test ## Run tests and generate HTML coverage report
	$(GO) tool cover -html=coverage.out -o coverage.html

.PHONY: e2e
e2e: build ## Run all E2E tests
	TESTDATA_DIR=$(PWD)/testdata ./$(BINARY_NAME) test

PROCS ?= 4

.PHONY: e2e-ci
e2e-ci: ## Run E2E tests with CI configuration (parallel via ginkgo CLI)
	mkdir -p $(OUTPUT_DIR)
	TESTDATA_DIR=$(PWD)/testdata $(call gotool,ginkgo) \
		--procs=$(PROCS) \
		--flake-attempts=$(FLAKE_ATTEMPTS) \
		--junit-report=junit.xml \
		--output-dir=$(OUTPUT_DIR) \
		$(if $(GINKGO_LABEL_FILTER),--label-filter="$(GINKGO_LABEL_FILTER)") \
		$(if $(GINKGO_FOCUS),--focus="$(GINKGO_FOCUS)") \
		$(if $(GINKGO_SKIP),--skip="$(GINKGO_SKIP)") \
		./e2e
.PHONY: list-tests
list-tests: build ## List E2E tests by tier without executing (dry-run)
	@echo "=== tier0 ==="
	TESTDATA_DIR=$(PWD)/testdata ./$(BINARY_NAME) test --dry-run --label-filter=tier0
	@echo ""
	@echo "=== tier1 ==="
	TESTDATA_DIR=$(PWD)/testdata ./$(BINARY_NAME) test --dry-run --label-filter=tier1
	@echo ""
	@echo "=== tier2 ==="
	TESTDATA_DIR=$(PWD)/testdata ./$(BINARY_NAME) test --dry-run --label-filter=tier2

##@ Code Quality

.PHONY: fmt
fmt: ## Format Go code
	$(call gotool,golangci-lint) fmt ./...

.PHONY: fmt-check
fmt-check: ## Check if code is formatted
	@status=0; diff=$$($(call gotool,golangci-lint) fmt --diff ./...) || status=$$?; \
	if [ -n "$$diff" ]; then \
		echo "Code is not formatted. Run 'make fmt' to fix:"; \
		echo "$$diff"; \
		exit 1; \
	fi; \
	exit $$status

.PHONY: vet
vet: generate ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: ## Verify configuration schema and run golangci-lint
	$(call gotool,golangci-lint) config verify
	$(call gotool,golangci-lint) run ./...

.PHONY: tools
tools: ## Ensure tool dependencies are up to date
	cd tools && "$(GO)" mod tidy

.PHONY: verify-tools
verify-tools: tools ## Fail in CI if tool module drifted
	@git diff --exit-code HEAD -- tools/go.mod tools/go.sum || (echo "tool modules out of date; run 'make tools'" && exit 1)

.PHONY: verify
verify: generate fmt-check vet ## Run all verification checks

.PHONY: check
check: verify lint test verify-tools ## Run all checks (fmt, vet, lint, test, tools)

.PHONY: gofmt
gofmt: fmt ## Alias for fmt (used by the hyperfleet-gofmt pre-commit hook)

.PHONY: go-vet
go-vet: vet ## Alias for vet (used by the hyperfleet-go-vet pre-commit hook)

.PHONY: install-hooks
install-hooks: ## Install pre-commit hooks
	pre-commit install

##@ Container Images

.PHONY: image
image: ## Build container image with configurable registry/tag
	@echo "Building image $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) ..."
	$(CONTAINER_TOOL) build \
		--platform linux/amd64 \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) .
	@echo "Image built: $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: image-push
image-push: image ## Build and push container image
	@echo "Pushing image $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) ..."
	$(CONTAINER_TOOL) push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	@echo "Image pushed: $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: image-dev
image-dev: ## Build and push to personal Quay registry (requires QUAY_USER)
ifndef QUAY_USER
	@echo "Error: QUAY_USER is not set"
	@echo ""
	@echo "Usage: QUAY_USER=myuser make image-dev"
	@echo ""
	@echo "This will build and push to: quay.io/\$$QUAY_USER/$(IMAGE_NAME):$(DEV_TAG)"
	@exit 1
endif
	@echo "Building dev image quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG) ..."
	$(CONTAINER_TOOL) build \
		--platform linux/amd64 \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG) .
	@echo "Pushing dev image..."
	$(CONTAINER_TOOL) push quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG)
	@echo ""
	@echo "Dev image pushed: quay.io/$(QUAY_USER)/$(IMAGE_NAME):$(DEV_TAG)"
