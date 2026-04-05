APP      := ia-recuerdo
MODULE   := github.com/mdesantis1984/ia-recuerdo
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags="-s -w -X main.version=$(VERSION)"
BIN      := bin/$(APP)
BIN_LIN  := bin/$(APP)-linux-amd64

.PHONY: all build build-linux build-postgres run run-http run-stdio \
        test lint fmt vet tidy docker-build deploy-k8s clean help

all: build

## build: Compile for current OS (SQLite, dev)
build:
	go build $(LDFLAGS) -o $(BIN) ./cmd/$(APP)/

## build-linux: Cross-compile for Linux amd64 (SQLite)
build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_LIN) ./cmd/$(APP)/

## build-postgres: Compile with PostgreSQL driver (Linux, production)
build-postgres:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -tags postgres -o $(BIN_LIN)-postgres ./cmd/$(APP)/

## run: Run in HTTP mode with SQLite (dev)
run: build
	./$(BIN) -transport http -addr :7438 -db-driver sqlite -db-dsn ia-recuerdo.db

## run-http: Alias for run
run-http: run

## run-stdio: Run in STDIO mode (MCP drop-in for Engram)
run-stdio: build
	./$(BIN) -transport stdio -db-driver sqlite -db-dsn ia-recuerdo.db

## run-both: Run both STDIO and HTTP simultaneously
run-both: build
	./$(BIN) -transport both -addr :7438 -db-driver sqlite -db-dsn ia-recuerdo.db

## test: Run all tests
test:
	go test -v ./...

## test-race: Run tests with race detector (Linux/macOS only, requires CGO)
test-race:
	go test -v -race ./...

## test-cover: Run tests with coverage report
test-cover:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## fmt: Format code
fmt:
	gofmt -w .

## vet: Run go vet
vet:
	go vet ./...

## tidy: Tidy and vendor
tidy:
	go mod tidy

## docker-build: Build Docker image
docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		-t ia-recuerdo:$(VERSION) \
		-t ia-recuerdo:latest \
		-f deploy/docker/Dockerfile .

## deploy-k8s: Apply Kubernetes manifests
deploy-k8s:
	kubectl apply -k deploy/kubernetes/

## key-create: Create an API key (REST)
## Usage: make key-create ADDR=http://localhost:7438 NAME=my-agent
key-create:
	curl -s -X POST $(ADDR)/api/v1/keys \
		-H "X-Api-Key: $(ADMIN_KEY)" \
		-H "Content-Type: application/json" \
		-d '{"name":"$(NAME)","scopes":"read,write"}' | jq

## export: Export all observations to JSON (Engram migration)
## Usage: make export ADDR=http://localhost:7438 KEY=ir_xxx
export:
	curl -s -H "X-Api-Key: $(KEY)" $(ADDR)/api/v1/export > ia-recuerdo-export.json
	@echo "Exported to ia-recuerdo-export.json"

## import: Import observations from JSON (Engram migration)
## Usage: make import ADDR=http://localhost:7438 KEY=ir_xxx FILE=engram-export.json
import:
	curl -s -X POST -H "X-Api-Key: $(KEY)" \
		-H "Content-Type: application/json" \
		--data-binary @$(FILE) \
		$(ADDR)/api/v1/import | jq

## health: Check service health
## Usage: make health ADDR=http://localhost:7438
health:
	curl -s $(ADDR)/healthz | jq

## clean: Remove build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html ia-recuerdo.db

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
