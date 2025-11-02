.PHONY: all test lint coverage clean build

all: lint test coverage

# Build all packages
build:
	go build -v ./...

# Run tests with race detection and coverage
test:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

# Run linting
lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0 run --timeout=5m

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
	go run golang.org/x/tools/cmd/goimports@latest -w .

# Run all checks (CI simulation)
ci: verify build vet lint test coverage
	@echo "✅ All CI checks passed"