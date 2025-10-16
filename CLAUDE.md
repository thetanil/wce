# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WCE (Web Compute Environment) is a lightweight web application platform built in Go that combines embedded SQLite storage with Starlark scripting for runtime extensibility. Each user can provision isolated compute environments (cenvs), where each cenv is a standalone SQLite database with its own authentication, data, and logic.

**Core Philosophy**: The database IS the application. Each SQLite file is a complete, portable application.

## Development Commands

**IMPORTANT**: Always use the Makefile for building and testing. It automatically includes the required `fts5` build tag for SQLite FTS5 (Full-Text Search) support.

### Building
```bash
make build
```

### Testing
Tests use ONLY the Go standard library - no frameworks or assertion libraries:
```bash
make test
```

### Running
```bash
make run  # Builds and starts server on http://localhost:5309
```

### Cleaning
```bash
make clean  # Removes binary and clears build cache
```

**Note**: The `fts5` tag is REQUIRED because the document store uses SQLite FTS5 virtual tables for full-text search. The Makefile handles this automatically. See BUILD.md for manual build instructions if needed.

## Architecture

### Multi-tenancy Model
- Each cenv is a separate SQLite database file at `./cenvs/{uuid}.db`
- Accessed via `https://domain.com/{cenv-id}/`
- No platform database - users authenticate directly to their cenv
- Complete isolation between cenvs (no shared state)

### Key Components

1. **Storage & Events**: SQLite3 serves dual purpose as both data store and event bus. Commit hooks trigger Go callbacks for reactive behavior.

2. **Authentication**: Database-native authentication. Credentials stored in each cenv's `_wce_users` table. JWT tokens scoped per-cenv.

3. **Authorization**: Per-cenv permission system with table-level and row-level security policies. SQL queries are intercepted and rewritten based on permissions.

4. **Starlark Scripting**: Sandboxed Starlark interpreter for runtime extensibility. Users can define custom HTTP endpoints without recompilation.

5. **Template System**: Templates stored in SQLite with both edit (raw) and display (rendered) versions. Pages are pre-rendered on commit and cached before requests.

6. **Browser UI**: Monaco editor with monaco-vim integration for Vim-style editing in the browser.

### Event-Driven Architecture
```
Database Commit → Commit Hook → Go Callback → Template Re-render → Cache Update
```

This design ensures pages are ready on demand, separating content storage from rendered output.

## Project Structure

When implemented, the codebase will follow this structure:
```
wce/
├── cmd/wce/main.go          # Entry point
├── internal/
│   ├── server/              # HTTP server and routing
│   ├── cenv/                # Cenv management and isolation
│   ├── db/                  # Database operations and query rewriting
│   ├── auth/                # Authentication and JWT handling
│   ├── starlark/            # Starlark integration and sandboxing
│   └── template/            # Template engine and caching
├── web/                     # Static assets (HTML, CSS, JS, Monaco editor)
└── README.md
```

## Security Model

### Core Security Tables (auto-created in each cenv)
- `_wce_users`: User authentication (username, bcrypt hash, role)
- `_wce_sessions`: Session/token management and revocation
- `_wce_table_permissions`: Table-level access control
- `_wce_row_policies`: Row-level security policies with SQL conditions
- `_wce_config`: Per-cenv configuration (session timeout, registration mode, etc.)
- `_wce_audit_log`: Audit trail of all actions

### Permission Hierarchy
1. **Owner**: Full control, cannot be removed, can delete cenv
2. **Admin**: Can manage users, permissions, and all data
3. **Editor**: Can read/write data based on table permissions
4. **Viewer**: Read-only access based on table permissions

### Starlark Sandboxing
- No filesystem access (cannot read/write files outside cenv database)
- No network access (unless explicitly granted)
- CPU limits: max 5 seconds execution time per request
- Memory limits enforced
- Database access only via provided API with automatic permission checks

### Query Rewriting
All SQL queries are intercepted and rewritten to enforce permissions:
```
User submits: SELECT * FROM documents
    ↓
Platform checks: _wce_table_permissions and _wce_row_policies
    ↓
Rewritten: SELECT * FROM documents WHERE (owner_id = $user_id)
```

## Dependencies

### Strict Dependency Discipline
This project maintains STRICT dependency discipline to keep binary size minimal. Before adding ANY new dependency, open an issue for discussion.

**Required Dependencies:**
- `mattn/go-sqlite3` - SQLite3 driver
- `google/starlark-go` - Starlark interpreter
- `golang.org/x/crypto/bcrypt` - Password hashing

**Browser Dependencies:**
- `monaco-vim` - Monaco editor with Vim keybindings

## Testing Philosophy

- Use ONLY Go standard library `testing` package
- NO test frameworks or assertion libraries
- Write tests alongside implementation (not after)
- Test files: `*_test.go`
- Aim for >80% code coverage
- Use in-memory SQLite (`:memory:`) for integration tests

## Implementation Phases

See IMPL.md for detailed implementation plan. The project is organized into 15 phases:

1. **Phase 1**: Foundation (HTTP server, cenv routing)
2. **Phase 2**: Database schema initialization
3. **Phase 3**: Authentication core (registration, login, JWT)
4. **Phase 4**: Authorization & permissions
5. **Phase 5**: Core database operations with commit hooks
6. **Phase 6**: Starlark integration
7. **Phase 7**: Template system
8. **Phase 8**: Web UI foundation
9. **Phase 9**: Advanced UI features
10. **Phase 10**: Security hardening
11. **Phase 11**: Session management
12. **Phase 12**: Configuration & admin
13. **Phase 13**: Error handling & logging
14. **Phase 14**: Testing
15. **Phase 15**: Documentation & polish

**Critical Path for MVP**: Phases 1-6, 8, 10

## Key Design Constraints

1. **Database is the application**: Each cenv must be a portable SQLite file containing all data, users, and logic
2. **No platform database**: Users authenticate directly to their cenv, no central registry
3. **Minimal dependencies**: Only essential libraries allowed
4. **Small binary size**: Keep compiled executable as compact as possible
5. **Zero external dependencies**: All functionality embedded in single binary
6. **Standard library testing**: No test frameworks
7. **Runtime extensibility**: Define endpoints and logic through Starlark in web UI

## Cenv Lifecycle

### Creating a New Cenv
1. User requests new cenv via `POST /new` with username/password
2. WCE generates UUID for cenv identifier
3. Creates new SQLite database file: `{uuid}.db`
4. Initializes schema with `_wce_*` tables
5. Hashes password (bcrypt cost 12) and stores in `_wce_users`
6. Returns cenv URL: `https://domain.com/{uuid}/`

### Authentication Flow
1. User navigates to `https://domain.com/{cenv-id}/`
2. WCE loads database file for `{cenv-id}`
3. User submits username + password
4. WCE queries `_wce_users` in THAT database
5. Verifies bcrypt hash
6. Issues JWT scoped to `{cenv-id}`
7. All subsequent requests validated against JWT

## Important Notes

- **File permissions**: Database files must have 600 permissions (owner only)
- **SQLite pragmas**: Always use `PRAGMA foreign_keys = ON`, `PRAGMA journal_mode = WAL`, `PRAGMA synchronous = NORMAL`
- **Parameterized queries**: NEVER allow string interpolation in SQL - always use parameterized statements
- **Error handling**: Don't crash on errors - recover gracefully with structured error types
- **Commit messages**: Format as `[Phase X.Y] Description` (e.g., `[Phase 3.1] Implement user registration endpoint`)

## Resource Limits (defaults)

Per-cenv quotas:
- Maximum database size: 100MB
- Maximum concurrent connections: 5
- Maximum query execution time: 30 seconds
- Maximum Starlark execution time: 5 seconds
- Maximum request rate: 100 requests/minute

## Security Headers (required in production)

All responses must include:
```
Content-Security-Policy: default-src 'self'
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: geolocation=(), microphone=(), camera=()
```

## SQLite Configuration

Connection pool settings:
- Max 5 connections per cenv
- Idle timeout: 30 minutes
- Lazy loading: create pool on first access
- Pool eviction after inactivity

## Starlark API

Scripts have access to:
- `db.query(sql, params)` - Execute SELECT queries (permissions enforced)
- `db.execute(sql, params)` - Execute write queries (permissions enforced)
- `request.method`, `request.path`, `request.query`, `request.headers`, `request.body`, `request.user`
- Response builder functions

Scripts CANNOT access:
- Filesystem operations (`open`, file I/O)
- Network operations (`http.get`, etc.)
- Go reflection or unsafe operations
- Native code execution
