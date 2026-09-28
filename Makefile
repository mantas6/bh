VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS       := -s -w -X main.version=$(VERSION)
GOLANGCI_LINT ?= golangci-lint

.PHONY: all build test race lint vet fmt fmt-check clean

all: build

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bh ./cmd/bh

test:
	CGO_ENABLED=0 go test ./...

race:
	go test -race ./...

lint:
	$(GOLANGCI_LINT) run ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi

clean:
	rm -f bh
	rm -rf dist
