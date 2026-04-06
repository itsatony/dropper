.PHONY: build run test lint fmt vet tidy docker-build docker-run docker-stop docker-logs setup smoke-test clean

BINARY         := bin/dropper
MODULE         := github.com/vAudience/dropper
MAIN           := ./cmd/dropper
CONFIG         := configs/dropper.example.yaml
DOCKER_IMAGE   := vaudience/dropper
DOCKER_TAG     := latest
CONTAINER_NAME := dropper-dev
VERSION        := $(shell grep '  version:' versions.yaml | sed 's/.*"\(.*\)"/\1/')
BUILD_DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS        := -ldflags "-s -w \
	-X github.com/itsatony/go-version.GitCommit=$(GIT_COMMIT) \
	-X github.com/itsatony/go-version.GitTag=v$(VERSION) \
	-X github.com/itsatony/go-version.BuildTime=$(BUILD_DATE)"

build:
	@mkdir -p bin
	go build $(LDFLAGS) -o $(BINARY) $(MAIN)

run:
	DROPPER_LOGGING_FORMAT=console DROPPER_LOGGING_LEVEL=debug \
	go run $(LDFLAGS) $(MAIN) --config $(CONFIG)

test:
	go test -race -cover -count=1 ./internal/... ./cmd/...

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

tidy:
	go mod tidy

docker-build:
	docker build \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_TAG=v$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-run:
	@mkdir -p data
	docker run --rm --name $(CONTAINER_NAME) \
		-p 8080:8080 \
		-v $(PWD)/configs/dropper.example.yaml:/etc/dropper/dropper.yaml:ro \
		-v $(PWD)/data:/data \
		-e DROPPER_SECRET=dev-secret-change-me \
		$(DOCKER_IMAGE):$(DOCKER_TAG) --config /etc/dropper/dropper.yaml

docker-stop:
	docker stop $(CONTAINER_NAME) 2>/dev/null || true

docker-logs:
	docker logs -f $(CONTAINER_NAME)

setup:
	bash scripts/setup.sh

smoke-test:
	bash scripts/smoke_test.sh

clean:
	rm -rf bin/ coverage.*
