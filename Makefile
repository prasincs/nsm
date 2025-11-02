.PHONY: all test lint coverage clean build install-tools

# Tool versions - pinned for reproducibility
GOLANGCI_LINT_VERSION := v1.61.0

all: lint test coverage

# Install development tools with pinned versions
install-tools:
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

# Build all packages
build:
	go build -v ./...

# Run tests with race detection and coverage
test:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

# Run linting
lint:
	golangci-lint run --timeout=5m

# Generate and display coverage report
coverage: test
	go tool cover -func=coverage.out
	@echo ""
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Total coverage: $${COVERAGE}%"; \
	if [ "$$(echo "$${COVERAGE} < 60.0" | bc -l)" -eq 1 ]; then \
		echo "❌ Coverage $${COVERAGE}% is below minimum 60%"; \
		exit 1; \
	else \
		echo "✅ Coverage $${COVERAGE}% meets minimum threshold"; \
	fi

# Generate HTML coverage report
coverage-html: test
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run go vet
vet:
	go vet ./...

# Verify dependencies
verify:
	go mod verify
	go mod tidy -diff

# Clean build artifacts
clean:
	go clean -testcache
	rm -f coverage.out coverage.html

# Format code
fmt:
	go fmt ./...
	goimports -w .

# Run all checks (CI simulation)
ci: verify build vet lint test coverage
	@echo "✅ All CI checks passed"