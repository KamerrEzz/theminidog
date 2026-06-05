.PHONY: build-agent build-server test test-verbose vet lint run-agent run-server tidy

build-agent:
	go build -o bin/agent ./cmd/agent

build-server:
	go build -o bin/server ./cmd/server

test:
	go test ./...

test-verbose:
	go test -v ./...

vet:
	go vet ./...

lint:
	golangci-lint run

run-agent:
	go run ./cmd/agent

run-server:
	go run ./cmd/server

tidy:
	go mod tidy
