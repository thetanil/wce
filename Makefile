.PHONY: build test run clean

# Build tags - FTS5 is always enabled
TAGS := fts5

# Build the binary
build:
	go build -tags=$(TAGS) -o wce ./cmd/wce

# Run all tests
test:
	go test -tags=$(TAGS) ./...

# Build and run
run: build
	./wce

# Clean build artifacts
clean:
	rm -f wce
	go clean -cache
