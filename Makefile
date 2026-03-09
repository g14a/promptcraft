.PHONY: build run vet test clean install

BINARY  := better-prompter
VERSION := 1.0.0
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/better-prompter

vet:
	go vet ./...

test:
	go test -race ./...

clean:
	rm -f $(BINARY)

install:
	go install -ldflags="-s -w -X main.version=$(VERSION)" ./cmd/better-prompter
	@echo "Installed to $$(go env GOPATH)/bin/$(BINARY)"

# Quick smoke-test: send an initialize request and check the response.
smoke: build
	@echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}' \
	  | ./$(BINARY) 2>/dev/null | head -1
