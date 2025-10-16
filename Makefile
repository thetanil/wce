.PHONY: build test run clean coverage

# Build tags - FTS5 is always enabled
TAGS := fts5

# Build the binary
build:
	go build -tags=$(TAGS) -o wce ./cmd/wce

# Run all tests
test:
	go test -tags=$(TAGS) ./...

# Run tests with coverage
coverage:
	go test -tags=$(TAGS) -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Build and run
run: build
	./wce

# Clean build artifacts
clean:
	rm -f wce coverage.out coverage.html
	go clean -cache
