GO ?= go

.PHONY: build test openapi openapi-check

build:
	$(GO) build ./...

test:
	$(GO) test ./...

openapi:
	$(GO) run ./cmd/openapi-gen

openapi-check:
	$(GO) run ./cmd/openapi-gen -check
