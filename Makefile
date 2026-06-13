GO ?= go
DOCKER ?= docker
HELM ?= helm
KIND ?= kind
BIN_DIR ?= bin

SERVICES := gateway identity tenant cluster-registry policy integrations pricing ingestion allocation recommendations query workflow export audit
OPERATORS := platform action-executor

.PHONY: all build test fmt vet tidy proto compose-up compose-down kind-up kind-down helm-lint clean

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

proto:
	protoc -I proto --go_out=proto/gen/go --go_opt=paths=source_relative --go-grpc_out=proto/gen/go --go-grpc_opt=paths=source_relative proto/cost/v1/common/common.proto proto/cost/v1/agent/agent.proto proto/cost/v1/pricing/pricing.proto proto/cost/v1/query/query.proto proto/cost/v1/recommendation/recommendation.proto proto/cost/v1/events/events.proto

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
