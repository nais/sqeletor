ENVTEST_K8S_VERSION = 1.29.0

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: ## Generate ClusterRole objects.
	$(CONTROLLER_GEN) rbac:roleName=sqeletor paths="./..." output:rbac:dir=config

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests fmt vet  ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

.PHONY: lint
lint: ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests fmt vet ## Build sqeletor binary.
	go build -o bin/sqeletor cmd/main.go

.PHONY: run
run: manifests fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen
ENVTEST ?= go run sigs.k8s.io/controller-runtime/tools/setup-envtest
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/cmd/golangci-lint
