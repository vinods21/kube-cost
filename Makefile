GO ?= go
DOCKER ?= docker
HELM ?= helm
KIND ?= kind
BIN_DIR ?= bin
GO_CACHE_DIR ?= $(CURDIR)/.cache/go-build
GO_TMP_DIR ?= $(CURDIR)/.cache/gotmp

SERVICES := gateway identity tenant cluster-registry policy integrations pricing ingestion allocation recommendations query workflow export audit
OPERATORS := platform action-executor

ifeq ($(OS),Windows_NT)
MKDIR_GO_CACHE := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(GO_CACHE_DIR)','$(GO_TMP_DIR)' | Out-Null"
GO_BUILD := powershell -NoProfile -Command "$$env:GOCACHE='$(GO_CACHE_DIR)'; $$env:GOTMPDIR='$(GO_TMP_DIR)'; & '$(GO)' build ./..."
GO_TEST := powershell -NoProfile -Command "$$env:GOCACHE='$(GO_CACHE_DIR)'; $$env:GOTMPDIR='$(GO_TMP_DIR)'; & '$(GO)' test ./..."
GO_VET := powershell -NoProfile -Command "$$env:GOCACHE='$(GO_CACHE_DIR)'; $$env:GOTMPDIR='$(GO_TMP_DIR)'; & '$(GO)' vet ./..."
GO_PROTO := powershell -NoProfile -Command "$$env:GOCACHE='$(GO_CACHE_DIR)'; $$env:GOTMPDIR='$(GO_TMP_DIR)'; & '$(GO)' run ./tools/protogen"
GO_PROTO_TEST := powershell -NoProfile -Command "$$env:GOCACHE='$(GO_CACHE_DIR)'; $$env:GOTMPDIR='$(GO_TMP_DIR)'; & '$(GO)' test ./tests/contract"
DEV_UP := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev-up.ps1
DEV_DOWN := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev-down.ps1
HELM_INSTALL := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/helm-install.ps1
HELM_UNINSTALL := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/helm-uninstall.ps1
CLICKHOUSE_MIGRATE := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/clickhouse-migrate.ps1
CLICKHOUSE_BENCHMARK := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/clickhouse-benchmark.ps1
CLICKHOUSE_INTEGRATION_TEST := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-clickhouse-integration.ps1
COMPATIBILITY_CHECK := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/compatibility-check.ps1
else
MKDIR_GO_CACHE := mkdir -p "$(GO_CACHE_DIR)" "$(GO_TMP_DIR)"
GO_BUILD := GOCACHE="$(GO_CACHE_DIR)" GOTMPDIR="$(GO_TMP_DIR)" $(GO) build ./...
GO_TEST := GOCACHE="$(GO_CACHE_DIR)" GOTMPDIR="$(GO_TMP_DIR)" $(GO) test ./...
GO_VET := GOCACHE="$(GO_CACHE_DIR)" GOTMPDIR="$(GO_TMP_DIR)" $(GO) vet ./...
GO_PROTO := GOCACHE="$(GO_CACHE_DIR)" GOTMPDIR="$(GO_TMP_DIR)" $(GO) run ./tools/protogen
GO_PROTO_TEST := GOCACHE="$(GO_CACHE_DIR)" GOTMPDIR="$(GO_TMP_DIR)" $(GO) test ./tests/contract
DEV_UP := sh scripts/dev-up.sh
DEV_DOWN := sh scripts/dev-down.sh
HELM_INSTALL := sh scripts/helm-install.sh
HELM_UNINSTALL := sh scripts/helm-uninstall.sh
CLICKHOUSE_MIGRATE := sh scripts/clickhouse-migrate.sh
CLICKHOUSE_BENCHMARK := sh scripts/clickhouse-benchmark.sh
CLICKHOUSE_INTEGRATION_TEST := sh scripts/test-clickhouse-integration.sh
COMPATIBILITY_CHECK := sh scripts/compatibility-check.sh
endif

.PHONY: all go-cache-dirs build test fmt vet tidy proto-tools proto proto-test dev-up dev-down compose-up compose-down kind-up kind-down helm-install helm-uninstall helm-lint clickhouse-migrate clickhouse-benchmark clickhouse-integration-test compatibility-check clean

all: build test

go-cache-dirs:
	$(MKDIR_GO_CACHE)

build: go-cache-dirs
	$(GO_BUILD)

test: go-cache-dirs
	$(GO_TEST)

fmt:
	$(GO) fmt ./...

vet: go-cache-dirs
	$(GO_VET)

tidy:
	$(GO) mod tidy

proto-tools:
	$(GO) tool protoc-gen-go --version
	$(GO) tool protoc-gen-go-grpc --version

proto: go-cache-dirs
	$(GO_PROTO)

proto-test: go-cache-dirs
	$(GO_PROTO_TEST)

dev-up:
	$(DEV_UP)

dev-down:
	$(DEV_DOWN)

compose-up:
	$(DOCKER) compose -f deploy/compose/docker-compose.yaml up --detach --wait

compose-down:
	$(DOCKER) compose -f deploy/compose/docker-compose.yaml down

kind-up:
	$(KIND) create cluster --config deploy/kind/cluster.yaml

kind-down:
	$(KIND) delete cluster --name kube-cost

helm-install:
	$(HELM_INSTALL)

helm-uninstall:
	$(HELM_UNINSTALL)

helm-lint:
	$(HELM) lint deploy/helm/kube-cost-platform
	$(HELM) lint deploy/helm/kube-cost-agent

clickhouse-migrate:
	$(CLICKHOUSE_MIGRATE)

clickhouse-benchmark:
	$(CLICKHOUSE_BENCHMARK)

clickhouse-integration-test:
	$(CLICKHOUSE_INTEGRATION_TEST)

compatibility-check:
	$(COMPATIBILITY_CHECK)

clean:
	$(GO) clean
