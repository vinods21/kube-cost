GO ?= go
DOCKER ?= docker
HELM ?= helm
KIND ?= kind
BIN_DIR ?= bin

SERVICES := gateway identity tenant cluster-registry policy integrations pricing ingestion allocation recommendations query workflow export audit
OPERATORS := platform action-executor

ifeq ($(OS),Windows_NT)
PROTO_GENERATE := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/generate-proto.ps1
DEV_UP := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev-up.ps1
DEV_DOWN := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev-down.ps1
HELM_INSTALL := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/helm-install.ps1
HELM_UNINSTALL := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/helm-uninstall.ps1
else
PROTO_GENERATE := sh scripts/generate-proto.sh
DEV_UP := sh scripts/dev-up.sh
DEV_DOWN := sh scripts/dev-down.sh
HELM_INSTALL := sh scripts/helm-install.sh
HELM_UNINSTALL := sh scripts/helm-uninstall.sh
endif

.PHONY: all build test fmt vet tidy proto-tools proto proto-test dev-up dev-down compose-up compose-down kind-up kind-down helm-install helm-uninstall helm-lint clean

all: build test

build:
	$(GO) build ./...

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

proto-tools:
	$(GO) tool protoc-gen-go --version
	$(GO) tool protoc-gen-go-grpc --version

proto:
	$(PROTO_GENERATE)

proto-test:
	$(GO) test ./tests/contract

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

clean:
	$(GO) clean
