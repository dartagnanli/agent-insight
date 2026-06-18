.PHONY: build test lint clean install version dist

BINARY_NAME := agent-insight
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.buildVersion=$(VERSION) -X main.buildCommit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)
GO := go
GOFLAGS := -trimpath

build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME) ./cmd/agent-insight/

test:
	CGO_ENABLED=0 $(GO) test -v -race -count=1 ./...

lint:
	golangci-lint run ./... || true

clean:
	rm -rf dist/

install: build
	cp dist/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

version:
	@echo $(BINARY_NAME) $(VERSION)

# 多平台交叉编译
dist:
	@mkdir -p dist
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-arm64 ./cmd/agent-insight/
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-amd64 ./cmd/agent-insight/
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-amd64 ./cmd/agent-insight/
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-arm64 ./cmd/agent-insight/
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-windows-amd64.exe ./cmd/agent-insight/
