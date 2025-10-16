# Build Instructions

## Requirements

- Go 1.21 or later
- gcc (for compiling SQLite with C extensions)
- make (for using the Makefile)

## Quick Start

### Building

```bash
make build
```

This produces a `wce` binary in the project root.

### Testing

```bash
make test
```

Runs all tests with FTS5 support enabled.

### Running

```bash
make run
```

Builds and runs the server on http://localhost:5309

### Cleaning

```bash
make clean
```

Removes the binary and clears the build cache.

## Why FTS5?

WCE uses SQLite's FTS5 virtual table module for full-text search of documents. This provides:

- Fast full-text search across document content
- BM25 ranking algorithm
- Phrase queries and boolean operators
- Minimal memory overhead

The Makefile automatically includes the `fts5` build tag required for FTS5 support.

## Production Builds

For production deployments, use the Makefile or specify the build tag manually:

```bash
# Using Makefile (recommended)
make build

# Or manually with all flags
CGO_ENABLED=1 go build -tags=fts5 -o wce ./cmd/wce
```

Cross-compilation requires the appropriate C toolchain for the target platform.

## Binary Size

The FTS5-enabled binary includes the SQLite FTS5 module:

- Expected size: ~10-12 MB

This is acceptable given the significant search capabilities it provides.

## Troubleshooting

### Compilation errors

If you encounter C compilation errors:
1. Ensure gcc is installed: `gcc --version`
2. Ensure CGO is enabled: `go env CGO_ENABLED` (should be "1")
3. On macOS, install Xcode Command Line Tools: `xcode-select --install`
