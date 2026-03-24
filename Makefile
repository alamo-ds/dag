PROJECT_NAME := $(shell basename $(PWD))
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

test:
	@go vet ./...
	@go test -race -count=20 -cover ./...

bench:
	@go test -bench=.

gosec:
	@gosec -terse ./...

lint:
	@golangci-lint run --timeout=2m

ready: test lint gosec

.PHONY: test build gosec lint ready
