.PHONY: build-agent test test-verbose vet lint run-agent tidy

build-agent:
	go build -o bin/agent ./cmd/agent

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

tidy:
	go mod tidy
