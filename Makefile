GO ?= go
DOCKER ?= docker
HELM ?= helm
KIND ?= kind
BIN_DIR ?= bin

SERVICES := gateway identity tenant cluster-registry policy integrations pricing ingestion allocation recommendations query workflow export audit
OPERATORS := platform action-executor

ifeq ($(OS),Windows_NT)
PROTO_GENERATE := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/generate-proto.ps1
else
PROTO_GENERATE := ./scripts/generate-proto.sh
endif

.PHONY: all build test fmt vet tidy proto-tools proto proto-test compose-up compose-down kind-up kind-down helm-lint clean

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

compose-up:
	$(DOCKER) compose -f deploy/compose/docker-compose.yaml up -d

compose-down:
	$(DOCKER) compose -f deploy/compose/docker-compose.yaml down

kind-up:
	$(KIND) create cluster --config deploy/kind/cluster.yaml

kind-down:
	$(KIND) delete cluster --name kube-cost

helm-lint:
	$(HELM) lint deploy/helm/kube-cost-platform
	$(HELM) lint deploy/helm/kube-cost-agent

clean:
	$(GO) clean
