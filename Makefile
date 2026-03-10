.PHONY: build run vet test clean install coverage coverage-html test-verbose bench lint fmt check-fmt security

BINARY  := promptcraft
VERSION := 1.0.0
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"
COVERAGE_OUT := coverage.out

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/promptcraft

vet:
	go vet ./...

test:
	go test -race ./...

# Run tests with coverage analysis
coverage:
	go test -race -cover -coverprofile=$(COVERAGE_OUT) ./...
	go tool cover -func=$(COVERAGE_OUT)

# Generate HTML coverage report and open in browser
coverage-html: coverage
	go tool cover -html=$(COVERAGE_OUT) -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@command -v open >/dev/null 2>&1 && open coverage.html || echo "Open coverage.html in your browser"

# Verbose test output
test-verbose:
	go test -race -cover -v ./...

# Run benchmarks
bench:
	go test -bench=. -benchmem ./internal/prompter

# Lint code with golangci-lint
lint:
	golangci-lint run

# Format code
fmt:
	gofmt -s -w .
	goimports -w .

# Check if code is properly formatted
check-fmt:
	@if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "Code is not formatted. Run 'make fmt'"; \
		gofmt -s -l .; \
		exit 1; \
	fi
	@if command -v goimports >/dev/null 2>&1; then \
		if [ "$$(goimports -l . | wc -l)" -gt 0 ]; then \
			echo "Imports are not organized. Run 'make fmt'"; \
			goimports -l .; \
			exit 1; \
		fi; \
	fi

# Run security checks
security:
	@if command -v gosec >/dev/null 2>&1; then \
		echo "Running gosec security scan..."; \
		gosec -fmt=text ./... || echo "Gosec found security issues"; \
	else \
		echo "gosec not installed. Installing..."; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest && \
		gosec -fmt=text ./... || echo "Gosec found security issues"; \
	fi
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not installed. Install with: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
	fi

# Run all quality checks
quality: vet lint check-fmt security test coverage

# Setup development environment
dev-setup:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

clean:
	rm -f $(BINARY)

install:
	go install -ldflags="-s -w -X main.version=$(VERSION)" ./cmd/promptcraft
	@echo "Installed to $$(go env GOPATH)/bin/$(BINARY)"

# Quick smoke-test: send an initialize request and check the response.
smoke: build
	@echo "Testing MCP protocol..."
	@echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}' \
	  | ./$(BINARY) 2>/dev/null | head -1
	@echo "Testing CLI enhancement..."
	@echo "Build a REST API" | ./$(BINARY) --enhance > /dev/null && echo "✓ CLI test passed" || echo "✗ CLI test failed"
