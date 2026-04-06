.PHONY: build run test lint fmt vet tidy docker-build docker-run setup clean

BINARY      := bin/dropper
MODULE      := github.com/vAudience/dropper
MAIN        := ./cmd/dropper
CONFIG      := configs/dropper.example.yaml
VERSION     := $(shell grep '  version:' versions.yaml | sed 's/.*"\(.*\)"/\1/')
BUILD_DATE  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS     := -ldflags "-s -w \
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
	docker build -t vaudience/dropper:latest .

docker-run:
	docker run --rm -p 8080:8080 \
		-v $(PWD)/configs/dropper.example.yaml:/etc/dropper/dropper.yaml:ro \
		-v $(PWD)/data:/data \
		-e DROPPER_SECRET=dev-secret-change-me \
		vaudience/dropper:latest --config /etc/dropper/dropper.yaml

setup:
	@echo "Setup script not yet implemented (Cycle 8)"

clean:
	rm -rf bin/ coverage.*
