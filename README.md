# WCE - Web Compute Environment

A lightweight, extensible web application platform built in Go that combines embedded SQLite storage with Starlark scripting for runtime extensibility.

## Overview

WCE is designed to be a minimal yet powerful web compute environment that compiles to a small, self-contained executable. Each user can provision isolated compute environments (cenvs), where each cenv is a standalone SQLite database with its own authentication, data, and logic.

**Key Features:**

- **Isolated Compute Environments**: Each cenv is a separate SQLite database with complete autonomy
- **Database-native Authentication**: Users authenticate directly to their cenv (no platform database)
- **Event-driven Architecture**: Commit hooks trigger callbacks for reactive behavior
- **Runtime Extensibility**: Embedded Starlark scripting without recompilation
- **Browser-based UI**: Monaco editor with Vim support for in-browser development
- **Dynamic Endpoints**: Define API endpoints via Starlark through the web interface

## Architecture

### Core Components

- **Multi-tenancy Model**: Each cenv is a separate SQLite database file accessible via `https://domain.com/{cenv-id}/`
- **Storage & Events**: SQLite3 serves dual purpose as both data store and event bus, using commit hooks to trigger Go callbacks
- **Authentication**: Direct database authentication with credentials stored in each cenv's `_wce_users` table
- **Document Database**: Pages (edit and display modes) stored and retrieved via SQL queries with a schema designed for document-style access
- **Template Engine**: Efficient template rendering with templates and values stored in SQLite; pages pre-rendered on commit and cached before requests
- **Scripting Layer**: Embedded Starlark interpreter for extending functionality without recompilation
- **Web Server**: HTTP server running on port 5309 by default
- **Browser Editor**: [monaco-vim](https://github.com/brijeshb42/monaco-vim) integration for Vim-style editing in the browser

### Design Principles

1. **Database is the Application**: Each cenv is a portable SQLite file containing all data, users, and logic
2. **No Platform Database**: Users authenticate directly to their cenv, no central registry
3. **Minimal Dependencies**: Only essential libraries (mattn/go-sqlite3, starlark-go)
4. **Small Binary Size**: Keep the compiled executable as compact as possible
5. **Zero External Dependencies**: All functionality embedded in single binary
6. **Standard Library Testing**: No test frameworks or assertion libraries
7. **Runtime Extensibility**: Define endpoints and logic through Starlark in the web UI

## Dependencies

### Go Dependencies (Minimal)

- [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) - SQLite3 driver with FTS5 support
- [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) - Bcrypt password hashing only

**Future**: [starlark-go](https://github.com/google/starlark-go) will be added for Starlark integration (Phase 6)

### Browser Dependencies (Planned)

- [monaco-vim](https://github.com/brijeshb42/monaco-vim) - Monaco editor with Vim keybindings

**Note**: We maintain strict dependency discipline. Additional dependencies require explicit approval to keep binary size minimal (~10-12 MB).

## Current Status

### ✅ Implemented (Phases 1-5)

- **Foundation** (Phase 1): HTTP server with cenv routing and database management
- **Database Schema** (Phase 2): Complete schema with security, users, permissions, and documents
- **Authentication** (Phase 3): JWT-based auth, session management, user registration
- **Authorization** (Phase 4): Role-based permissions and row-level security policies
- **Document Store** (Phase 5): Full CRUD operations with FTS5 search, REST API, and tags
  - Hierarchical document IDs (`pages/home`, `api/users`)
  - Full-text search with BM25 ranking
  - Version tracking and user auditing
  - Binary content support (base64 encoding)
  - Tag-based categorization

### 🚧 In Progress

- **Test Coverage**: 97 tests across 5 packages, 64-79% coverage
- **Build System**: Makefile with `build`, `test`, `coverage` commands
- **Documentation**: Implementation plan (IMPL.md), build guides

### 📋 Planned (Phases 6+)

- **Starlark Integration** (Phase 6): Runtime extensibility without recompilation
- **Template System** (Phase 7): Document-based templates with rendering and caching
- **Web UI** (Phase 8-9): Browser-based management interface
- **Security Hardening** (Phase 10): Production-ready security measures
- **Admin Features** (Phase 11-12): Session management and configuration

## Development

### Building

```bash
make build
```

### Testing

```bash
make test
```

### Running

```bash
make run
```

### Cleaning

```bash
make clean
```

### Coverage

```bash
make coverage
```

Generates a coverage report in `coverage.html` that you can open in a browser.

**Note**: The Makefile automatically includes the `fts5` build tag required for SQLite FTS5 support.

### Project Structure

```
wce/
├── README.md
├── go.mod
├── main.go
└── ... (to be organized)
```

## Usage

### Starting the Server

```bash
# Run the web server
./wce
```

Server starts on `http://localhost:5309` (or configured port).

### Creating a New Cenv

1. Navigate to `http://localhost:5309/new`
2. Set initial admin username and password
3. Receive your unique cenv URL: `http://localhost:5309/{cenv-id}/`
4. Bookmark this URL - it's how you access your cenv

### Accessing Your Cenv

Each cenv has a unique URL:
```
http://localhost:5309/{cenv-id}/
```

- Authenticate with your username/password
- JWT token scoped to your specific cenv
- Each cenv is completely isolated from others

### Multi-User Access

Cenv owners can invite other users:

1. Add user to `_wce_users` table with username/password
2. Share cenv URL and credentials
3. Optional: Enable registration mode in `_wce_config` to allow signups

## Extending with Starlark

Define custom endpoints and logic directly from the web interface using Starlark:

```python
# Example: Define a custom endpoint
def handle_request(ctx):
    return {"message": "Hello from Starlark!"}
```

## Architecture Details

### Page Rendering Model

Pages are stored in two modes (edit and display) and retrieved via SQL queries:

1. **On Commit**: When database changes are committed, commit hooks trigger page re-rendering
2. **Pre-caching**: Rendered pages are cached before being requested when possible
3. **Event-driven**: The commit hook → callback → render → cache pipeline ensures pages are ready on demand

This design separates content storage from rendered output, allowing:
- Fast page serving from pre-rendered cache
- Consistent rendering triggered by data changes
- Efficient resource usage through event-driven updates

### Database Schema

Each cenv includes built-in security and system tables:

**Security Tables:**
- `_wce_users`: User authentication (username, password_hash, role)
- `_wce_sessions`: Session/token management and revocation
- `_wce_table_permissions`: Table-level access control
- `_wce_row_policies`: Row-level security policies
- `_wce_config`: Per-cenv configuration (session timeout, registration, etc.)
- `_wce_audit_log`: Audit trail of all actions

**Application Tables:**
- Page storage (edit and display versions)
- Template definitions
- Template values/data
- Starlark scripts
- Rendered page cache

User-created tables can be added dynamically and protected with granular permissions.

## Philosophy

WCE embraces minimalism and composability:

- **Database is the application**: Each SQLite file is a complete, portable application
- **One binary, full platform**: Everything needed runs from a single executable
- **No platform database**: Each cenv manages its own users and authentication
- **Store everything**: Templates, data, configuration, users all in SQLite
- **Extend at runtime**: No recompilation needed for new functionality
- **Events over polling**: Commit hooks drive reactive behavior
- **Script over compile**: Starlark for user-defined logic
- **Render on write**: Pre-cache pages on commit, not on request
- **True isolation**: Cenvs have no shared state, truly independent

## Security

Each cenv is an isolated security domain:

- Users authenticate directly to their cenv database
- JWT tokens scoped per-cenv (cannot cross boundaries)
- Table-level and row-level permissions
- Starlark sandboxing prevents filesystem/network access
- Per-cenv resource limits and rate limiting
- Complete audit logging in each database

See [SECURITY.md](SECURITY.md) for detailed security model and threat analysis.

## License

MIT License - See LICENSE file for details

## Contributing

This project maintains strict dependency discipline. Before adding any new dependency, please open an issue for discussion.
