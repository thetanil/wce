# Implementation Guide

This document outlines the implementation steps for WCE, organized into small batches with clear dependencies. Each phase builds on the previous one, ensuring the system can be developed incrementally and tested at each stage.

## Phase 1: Foundation (Bootstrap)

**Goal**: Basic HTTP server that can route requests to different cenvs

### 1.1 Project Initialization
- [ ] Create `go.mod` with module name
- [ ] Add dependencies: `mattn/go-sqlite3`, `github.com/google/starlark-go`
- [ ] Create basic project structure:
  ```
  /cmd/wce/main.go          - Entry point
  /internal/server/         - HTTP server
  /internal/cenv/           - Cenv management
  /internal/db/             - Database operations
  /internal/auth/           - Authentication
  /internal/starlark/       - Starlark integration
  /internal/template/       - Template engine
  /web/                     - Static assets
  ```

**Test**: `go build` succeeds

### 1.2 Basic HTTP Server
- [ ] Implement HTTP server listening on port 5309
- [ ] Graceful shutdown on SIGINT/SIGTERM
- [ ] Basic logging (requests, errors)
- [ ] Health check endpoint: `GET /health`

**Test**: Server starts, responds to `/health`

### 1.3 Cenv Routing
- [ ] Parse URL path to extract `{cenv-id}`
- [ ] Validate cenv-id format (UUID)
- [ ] Route requests: `/{cenv-id}/*` → cenv handler (namespace these with named accounts)
- [ ] Route requests: `/new` → cenv creation handler
- [ ] 404 for invalid paths

**Test**: Different cenv-ids route to correct handler

### 1.4 Database File Management
- [ ] Configure cenv storage directory (e.g., `./cenvs/`)
- [ ] Map cenv-id to file path: `./cenvs/{uuid}.db`
- [ ] Check if database file exists
- [ ] Create database file if needed
- [ ] Set file permissions (600 - owner only)

**Test**: Database files created with correct permissions

**Dependencies**: None
**Deliverable**: Server routes requests to cenv-specific handlers

---

## Phase 2: Database Schema

**Goal**: Initialize new cenvs with proper schema

### 2.1 Schema Definition
- [ ] Define SQL schema for all `_wce_*` tables:
  - `_wce_users`
  - `_wce_sessions`
  - `_wce_table_permissions`
  - `_wce_row_policies`
  - `_wce_config`
  - `_wce_audit_log`
- [ ] Store schema in Go constant or embed from `.sql` file
- [ ] Schema includes indexes on foreign keys

**Test**: Schema SQL parses correctly

### 2.2 Database Initialization
- [ ] Connect to SQLite with appropriate pragmas:
  - `PRAGMA foreign_keys = ON`
  - `PRAGMA journal_mode = WAL`
  - `PRAGMA synchronous = NORMAL`
- [ ] Check if database is initialized (check for `_wce_users` table)
- [ ] Execute schema creation if not initialized
- [ ] Insert default config values
- [ ] Close connection properly

**Test**: New cenv has all tables created

### 2.3 Connection Pooling
- [ ] Create connection pool per cenv (max 5 connections)
- [ ] Connection pool with idle timeout
- [ ] Lazy loading: create pool on first access
- [ ] Pool eviction after inactivity (e.g., 30 minutes)
- [ ] Thread-safe pool management

**Test**: Multiple concurrent requests use pooled connections

**Dependencies**: Phase 1
**Deliverable**: New cenvs initialize with complete schema

---

## Phase 3: Authentication Core

**Goal**: User registration, login, and JWT token generation

### 3.1 User Registration (New Cenv Creation)
- [ ] Endpoint: `POST /new` with username, password
- [ ] Generate UUID for new cenv
- [ ] Create database file
- [ ] Initialize schema
- [ ] Hash password with bcrypt (cost 12)
- [ ] Generate UUID for user_id
- [ ] Insert into `_wce_users` with role='owner'
- [ ] Return JSON: `{cenv_id, cenv_url}`

**Test**: Creating cenv returns valid URL, database contains owner user

### 3.2 Password Hashing
- [ ] Use `golang.org/x/crypto/bcrypt`
- [ ] Hash passwords with cost factor 12
- [ ] Verify password against hash
- [ ] Handle errors gracefully

**Test**: Hashing and verification work correctly

### 3.3 JWT Token Generation
- [ ] Load JWT signing key from environment or config (32+ bytes)
- [ ] Generate JWT with claims:
  - `cenv_id` (string)
  - `user_id` (string)
  - `username` (string)
  - `role` (string)
  - `iat` (issued at)
  - `exp` (expiration)
- [ ] Sign with HS256 algorithm
- [ ] Return token as string

**Test**: Token generation and parsing work

### 3.4 Login Endpoint
- [ ] Endpoint: `POST /{cenv-id}/login` with username, password
- [ ] Query `_wce_users` for username
- [ ] Verify password hash
- [ ] Check `enabled` flag
- [ ] Update `last_login` timestamp
- [ ] Generate JWT token
- [ ] Insert session into `_wce_sessions`
- [ ] Return token (JSON or Set-Cookie)
- [ ] Log failed login attempts to `_wce_audit_log`

**Test**: Valid credentials return token, invalid credentials return 401

### 3.5 Token Validation Middleware
- [ ] Extract token from Authorization header or cookie
- [ ] Parse and validate JWT signature
- [ ] Verify `cenv_id` matches request path
- [ ] Verify token not expired
- [ ] Check session in `_wce_sessions` (not revoked)
- [ ] Update `last_used` in sessions table
- [ ] Inject user context into request
- [ ] Return 401 if invalid

**Test**: Authenticated requests pass, unauthenticated fail

**Dependencies**: Phase 2
**Deliverable**: Users can register, login, and authenticate

---

## Phase 4: Authorization & Permissions

**Goal**: Enforce table and row-level permissions

### 4.1 Permission Loading
- [ ] Load user's role from context
- [ ] Query `_wce_table_permissions` for user's access
- [ ] Query `_wce_row_policies` for applicable policies
- [ ] Cache permissions per request
- [ ] Helper functions: `canRead()`, `canWrite()`, `canDelete()`

**Test**: Permission queries return correct results

### 4.2 Query Interception
- [ ] Parse SQL statements (identify table, operation type)
- [ ] Detect operation: SELECT, INSERT, UPDATE, DELETE
- [ ] Check table permissions before execution
- [ ] Return 403 if permission denied

**Test**: Unauthorized queries rejected

### 4.3 Query Rewriting for Row Policies
- [ ] For SELECT: append WHERE clause from row policies
- [ ] For UPDATE/DELETE: append WHERE clause
- [ ] Combine multiple policies with AND
- [ ] Substitute `$user_id` with actual user_id
- [ ] Handle queries with existing WHERE clauses

**Test**: Row policies correctly filter results

### 4.4 Permission Management Endpoints
- [ ] `POST /{cenv-id}/admin/users` - Add user (admin only)
- [ ] `PUT /{cenv-id}/admin/users/{user-id}` - Update user role
- [ ] `POST /{cenv-id}/admin/permissions` - Grant table permission
- [ ] `DELETE /{cenv-id}/admin/permissions/{id}` - Revoke permission
- [ ] `POST /{cenv-id}/admin/policies` - Create row policy

**Test**: Admin can manage permissions, non-admin cannot

**Dependencies**: Phase 3
**Deliverable**: Fine-grained access control working

---

## Phase 5: Core Database Operations

**Goal**: Safe, audited database queries with commit hooks

### 5.1 Parameterized Query Execution
- [ ] API: `Query(sql string, params ...interface{})`
- [ ] Always use parameterized statements
- [ ] Never allow string interpolation in SQL
- [ ] Return rows as JSON-serializable maps
- [ ] Handle NULL values correctly

**Test**: Parameterized queries prevent SQL injection

### 5.2 Transaction Support
- [ ] Begin transaction: `BeginTx()`
- [ ] Commit transaction: `Commit()`
- [ ] Rollback transaction: `Rollback()`
- [ ] Auto-rollback on panic
- [ ] Nested transaction handling (savepoints)

**Test**: Transactions roll back on error

### 5.3 Commit Hook System
- [ ] Register Go callback functions for commit events
- [ ] Execute callbacks after successful commit
- [ ] Pass transaction details to callbacks
- [ ] Handle callback errors (log, don't fail commit)
- [ ] Support multiple hooks

**Test**: Callbacks execute after commit

### 5.4 Audit Logging
- [ ] Log all write operations to `_wce_audit_log`
- [ ] Capture: user_id, action, resource, timestamp, IP, user-agent
- [ ] Log in same transaction as operation
- [ ] Background cleanup of old audit logs (optional)

**Test**: All operations appear in audit log

**Dependencies**: Phase 4
**Deliverable**: Secure database operations with auditing

---

## Phase 6: Starlark Integration

**Goal**: Embed Starlark with database access

### 6.1 Starlark Interpreter Setup
- [ ] Create interpreter instance per cenv
- [ ] Configure thread limits
- [ ] Set execution timeout (5 seconds default)
- [ ] Disable dangerous built-ins (open, etc.)
- [ ] Provide only whitelisted functions

**Test**: Simple Starlark script executes

### 6.2 Database API for Starlark
- [ ] Expose `db.query(sql, params)` function
- [ ] Enforce permission checks on queries
- [ ] Return results as Starlark values (dicts, lists)
- [ ] Expose `db.execute(sql, params)` for writes
- [ ] All operations use parameterized queries

**Test**: Starlark can query database with permissions enforced

### 6.3 HTTP Context for Starlark
- [ ] Provide `request` object with:
  - `method` (GET, POST, etc.)
  - `path`
  - `query` (parsed query params)
  - `headers` (dict)
  - `body` (parsed JSON or raw)
  - `user` (authenticated user info)
- [ ] Provide `response` builder

**Test**: Starlark can access request data

### 6.4 Endpoint Registration
- [ ] Table: `_wce_endpoints` (path, method, script)
- [ ] Load endpoints on server start
- [ ] Route matching: exact and pattern-based
- [ ] Execute Starlark script for matched endpoint
- [ ] Return script result as HTTP response
- [ ] Handle errors gracefully (500 with error message)

**Test**: Custom Starlark endpoints respond correctly

### 6.5 Script Management UI (Basic)
- [ ] Endpoint: `GET /{cenv-id}/admin/endpoints` - List scripts
- [ ] Endpoint: `POST /{cenv-id}/admin/endpoints` - Create/update script
- [ ] Store scripts in `_wce_endpoints` table
- [ ] Syntax validation before save
- [ ] Auto-reload on change

**Test**: Endpoints can be created via API

**Dependencies**: Phase 5
**Deliverable**: Starlark scripts can define endpoints

---

## Phase 7: Template System

**Goal**: Store and render templates with caching

### 7.1 Template Storage
- [ ] Table: `_wce_templates` (id, name, content, syntax)
- [ ] Support Go templates initially
- [ ] Store edit version (raw markdown/source)
- [ ] Store display version (rendered HTML)
- [ ] Version tracking (optional)

**Test**: Templates stored and retrieved

### 7.2 Template Rendering
- [ ] Parse Go templates
- [ ] Render with data from database
- [ ] Auto-escape HTML by default
- [ ] Support custom functions (safe subset)
- [ ] Handle rendering errors

**Test**: Templates render correctly

### 7.3 Render Caching
- [ ] Table: `_wce_page_cache` (path, html, rendered_at)
- [ ] Store rendered output after commit
- [ ] Serve from cache when available
- [ ] Invalidate cache on relevant data changes

**Test**: Cached pages serve faster

### 7.4 Commit Hook for Rendering
- [ ] Register commit hook to trigger re-rendering
- [ ] Detect which templates affected by commit
- [ ] Re-render affected templates
- [ ] Update cache table
- [ ] Handle errors without blocking commit

**Test**: Commits trigger cache updates

### 7.5 Page Serving
- [ ] Endpoint: `GET /{cenv-id}/pages/{path}`
- [ ] Check cache first
- [ ] Fall back to on-demand render if cache miss
- [ ] Return HTML with appropriate headers

**Test**: Pages served from cache

**Dependencies**: Phase 5
**Deliverable**: Template system with automatic caching

---

## Phase 8: Web UI Foundation

**Goal**: Browser-based interface with Monaco editor

### 8.1 Static File Serving
- [ ] Serve static files from `/web` directory
- [ ] HTML, CSS, JavaScript
- [ ] Set appropriate MIME types
- [ ] Cache headers for assets
- [ ] Embed assets in binary (go:embed)

**Test**: Static assets load in browser

### 8.2 Login Page
- [ ] HTML form: username, password
- [ ] JavaScript: POST to `/{cenv-id}/login`
- [ ] Store JWT in localStorage or cookie
- [ ] Redirect to dashboard on success
- [ ] Show error message on failure

**Test**: Login page works in browser

### 8.3 Dashboard Page
- [ ] Show current cenv info
- [ ] List tables in database
- [ ] Quick stats (user count, table count)
- [ ] Navigation to editor, admin, etc.
- [ ] Logout button

**Test**: Dashboard displays after login

### 8.4 Monaco Editor Integration
- [ ] Load Monaco Editor from CDN or bundled
- [ ] Create editor instance
- [ ] Set language (SQL, Starlark, Markdown)
- [ ] Load content from API
- [ ] Save content back to API

**Test**: Editor loads and displays content

### 8.5 Monaco-Vim Extension
- [ ] Load `monaco-vim` extension
- [ ] Enable Vim mode toggle
- [ ] Persist user preference
- [ ] Status line showing mode

**Test**: Vim keybindings work in editor

**Dependencies**: Phase 3
**Deliverable**: Basic web UI with editor

---

## Phase 9: Advanced UI Features

**Goal**: Full-featured web interface

### 9.1 Script Editor
- [ ] Page: `/{cenv-id}/editor/scripts`
- [ ] List all Starlark scripts
- [ ] Create new script button
- [ ] Monaco editor with Starlark syntax
- [ ] Save button → `POST /admin/endpoints`
- [ ] Test button → execute and show result
- [ ] Vim mode support

**Test**: Can create and edit Starlark scripts

### 9.2 SQL Console
- [ ] Page: `/{cenv-id}/console`
- [ ] Monaco editor with SQL syntax
- [ ] Execute button
- [ ] Results displayed as table
- [ ] Error messages displayed
- [ ] Query history

**Test**: Can execute SQL queries from browser

### 9.3 Template Editor
- [ ] Page: `/{cenv-id}/editor/templates`
- [ ] List all templates
- [ ] Split view: edit (markdown) / preview (HTML)
- [ ] Live preview
- [ ] Save button
- [ ] Vim mode support

**Test**: Can create and edit templates

### 9.4 User Management UI
- [ ] Page: `/{cenv-id}/admin/users`
- [ ] List all users
- [ ] Add user form
- [ ] Edit role
- [ ] Disable/enable user
- [ ] View permissions

**Test**: Admin can manage users via UI

### 9.5 Permission Management UI
- [ ] Page: `/{cenv-id}/admin/permissions`
- [ ] Table view of permissions
- [ ] Grant permission form (user, table, operations)
- [ ] Revoke button
- [ ] Row policy editor

**Test**: Permissions can be managed via UI

**Dependencies**: Phase 8
**Deliverable**: Complete web-based administration

---

## Phase 10: Security Hardening

**Goal**: Production-ready security features

### 10.1 Rate Limiting
- [ ] Per-IP rate limiting (100 req/min default)
- [ ] Per-cenv rate limiting
- [ ] Per-user rate limiting
- [ ] Configurable via `_wce_config`
- [ ] Return 429 when exceeded

**Test**: Rate limits enforced

### 10.2 Resource Quotas
- [ ] Maximum database size (100MB default)
- [ ] Maximum query execution time (30s)
- [ ] Maximum Starlark execution time (5s)
- [ ] Maximum concurrent connections (5)
- [ ] Configurable per-cenv

**Test**: Quotas enforced, operations fail when exceeded

### 10.3 Security Headers
- [ ] Content-Security-Policy
- [ ] X-Frame-Options: DENY
- [ ] X-Content-Type-Options: nosniff
- [ ] Referrer-Policy
- [ ] Permissions-Policy
- [ ] HSTS (if HTTPS)

**Test**: Security headers present in responses

### 10.4 CORS Configuration
- [ ] Default: CORS disabled
- [ ] Per-cenv: configure allowed origins in `_wce_config`
- [ ] Handle preflight requests
- [ ] Validate Origin header

**Test**: CORS works when configured

### 10.5 Input Validation
- [ ] Validate all user inputs
- [ ] Sanitize file paths (prevent directory traversal)
- [ ] Validate cenv-id format (UUID only)
- [ ] Limit request body size
- [ ] Validate JSON structure

**Test**: Invalid inputs rejected

**Dependencies**: Phase 5
**Deliverable**: Hardened security posture

---

## Phase 11: Session Management

**Goal**: Robust session handling

### 11.1 Session Lifecycle
- [ ] Create session on login
- [ ] Store in `_wce_sessions` table
- [ ] Update `last_used` on each request
- [ ] Expire based on `session_timeout_hours` config
- [ ] Clean up expired sessions periodically

**Test**: Sessions expire correctly

### 11.2 Token Revocation
- [ ] Endpoint: `POST /{cenv-id}/logout`
- [ ] Delete session from `_wce_sessions`
- [ ] Blacklist token (store SHA256 hash)
- [ ] Clear cookie/localStorage

**Test**: Revoked tokens rejected

### 11.3 Multiple Sessions
- [ ] Allow multiple active sessions per user
- [ ] List active sessions: `GET /{cenv-id}/sessions`
- [ ] Revoke specific session
- [ ] Revoke all sessions (force logout)

**Test**: Multiple sessions work, can be managed

### 11.4 Session Security
- [ ] HttpOnly cookies for tokens
- [ ] Secure flag when HTTPS
- [ ] SameSite=Strict
- [ ] CSRF token for state-changing operations

**Test**: Sessions secure against XSS/CSRF

**Dependencies**: Phase 3
**Deliverable**: Secure session management

---

## Phase 12: Configuration & Admin

**Goal**: System configuration and administration

### 12.1 Configuration Management
- [ ] Load config from `_wce_config` table
- [ ] Override with environment variables
- [ ] Admin UI for config: `/{cenv-id}/admin/config`
- [ ] Validate config values
- [ ] Apply changes without restart

**Test**: Configuration changes take effect

### 12.2 Cenv Provisioning Policy
- [ ] Environment variable: `ALLOW_NEW_CENVS` (true/false)
- [ ] If false, return 403 on `POST /new`
- [ ] Optional: approval workflow

**Test**: Cenv creation controlled by policy

### 12.3 Database Backup/Export
- [ ] Endpoint: `GET /{cenv-id}/admin/export`
- [ ] Stream database file as download
- [ ] Require owner role
- [ ] Use SQLite backup API for consistency

**Test**: Database can be exported

### 12.4 Database Import/Restore
- [ ] Upload database file
- [ ] Validate schema compatibility
- [ ] Replace existing database (with confirmation)
- [ ] Restore from backup

**Test**: Database can be imported

### 12.5 Audit Log Viewer
- [ ] Endpoint: `GET /{cenv-id}/admin/audit`
- [ ] Paginated audit log
- [ ] Filter by user, action, date
- [ ] Export as CSV/JSON

**Test**: Audit logs viewable

**Dependencies**: Phase 4
**Deliverable**: Full administrative control

---

## Phase 13: Error Handling & Logging

**Goal**: Robust error handling and observability

### 13.1 Error Handling
- [ ] Structured error types
- [ ] User-friendly error messages
- [ ] Detailed error logging (server-side)
- [ ] Error recovery (don't crash on errors)
- [ ] Panic recovery middleware

**Test**: Errors handled gracefully

### 13.2 Logging System
- [ ] Structured logging (JSON format)
- [ ] Log levels: DEBUG, INFO, WARN, ERROR
- [ ] Log to stdout by default
- [ ] Optional: log to file
- [ ] Include request ID in logs

**Test**: Logs are clear and useful

### 13.3 Health Checks
- [ ] `GET /health` - Basic health
- [ ] `GET /health/ready` - Readiness probe
- [ ] `GET /health/live` - Liveness probe
- [ ] Check database connectivity
- [ ] Return 503 if unhealthy

**Test**: Health checks work

### 13.4 Metrics (Optional)
- [ ] Request count, latency
- [ ] Active connections
- [ ] Database query times
- [ ] Expose as Prometheus metrics
- [ ] Endpoint: `GET /metrics`

**Test**: Metrics collected

**Dependencies**: Phase 1
**Deliverable**: Production-grade observability

---

## Phase 14: Testing

**Goal**: Comprehensive test coverage

### 14.1 Unit Tests
- [ ] Test authentication functions
- [ ] Test password hashing/verification
- [ ] Test JWT generation/validation
- [ ] Test permission checks
- [ ] Test query rewriting
- [ ] Use standard library only (no frameworks)

**Test**: `go test ./...` passes

### 14.2 Integration Tests
- [ ] Test full authentication flow
- [ ] Test database operations with permissions
- [ ] Test Starlark execution
- [ ] Test template rendering
- [ ] Use in-memory SQLite (`:memory:`)

**Test**: Integration tests pass

### 14.3 End-to-End Tests
- [ ] Spin up server
- [ ] Create new cenv via API
- [ ] Login and get token
- [ ] Create Starlark endpoint
- [ ] Access custom endpoint
- [ ] Verify response

**Test**: E2E tests pass

### 14.4 Security Tests
- [ ] Test SQL injection prevention
- [ ] Test XSS prevention
- [ ] Test CSRF protection
- [ ] Test rate limiting
- [ ] Test session security

**Test**: Security tests pass

### 14.5 Performance Tests
- [ ] Load test with multiple concurrent users
- [ ] Test query performance
- [ ] Test Starlark execution time
- [ ] Test connection pool behavior
- [ ] Identify bottlenecks

**Test**: Performance meets requirements

**Dependencies**: All phases
**Deliverable**: Well-tested codebase

---

## Phase 15: Documentation & Polish

**Goal**: Complete documentation and user experience

### 15.1 API Documentation
- [ ] Document all HTTP endpoints
- [ ] Request/response examples
- [ ] Error codes and meanings
- [ ] Authentication requirements
- [ ] OpenAPI specification (optional)

**Test**: API is documented

### 15.2 User Documentation
- [ ] Getting started guide
- [ ] Tutorial: Create first cenv
- [ ] Tutorial: Write Starlark endpoint
- [ ] Tutorial: Create template
- [ ] FAQ

**Test**: Docs are clear

### 15.3 Developer Documentation
- [ ] Architecture overview
- [ ] Code structure explanation
- [ ] How to add features
- [ ] How to run tests
- [ ] Contributing guide

**Test**: Devs can understand codebase

### 15.4 Example Cenvs
- [ ] Example: Blog system
- [ ] Example: Todo app
- [ ] Example: API with authentication
- [ ] Include SQL schemas and Starlark scripts

**Test**: Examples work

### 15.5 Final Polish
- [ ] Improve error messages
- [ ] Add helpful hints in UI
- [ ] Improve loading states
- [ ] Add animations (subtle)
- [ ] Test on different browsers

**Test**: UX is polished

**Dependencies**: All phases
**Deliverable**: Production-ready system

---

## Development Strategy

### Incremental Development
Each phase should result in a working, testable system:

1. After Phase 3: Can create cenvs and authenticate
2. After Phase 5: Can execute queries with permissions
3. After Phase 6: Can run Starlark scripts
4. After Phase 8: Have a working web UI
5. After Phase 10: Production-ready security

### Testing Strategy
- Write tests alongside implementation (not after)
- Use standard library `testing` package only
- Test files: `*_test.go`
- Run tests: `go test ./...`
- Aim for >80% code coverage

### Git Workflow
- Small, focused commits per task
- Commit message format: `[Phase X.Y] Description`
- Example: `[Phase 3.1] Implement user registration endpoint`
- Push working code frequently

### Validation Checkpoints

**Checkpoint 1 (After Phase 5)**: Core MVP
- Can create cenvs
- Can authenticate
- Can execute SQL with permissions
- Command-line or curl tests work

**Checkpoint 2 (After Phase 8)**: UI MVP
- Web interface functional
- Can login via browser
- Can use editor
- Manual testing works

**Checkpoint 3 (After Phase 12)**: Feature Complete
- All major features implemented
- Admin capabilities complete
- Security hardened
- Ready for beta testing

**Checkpoint 4 (After Phase 15)**: Production Ready
- Fully tested
- Documented
- Polished UX
- Ready for public release

---

## Priority Tasks (Critical Path)

If time is limited, focus on these essential tasks first:

1. **Phase 1-3**: Foundation + Authentication (critical)
2. **Phase 4-5**: Permissions + Database ops (critical)
3. **Phase 6**: Starlark (core feature)
4. **Phase 8**: Basic web UI (usability)
5. **Phase 10**: Security hardening (production requirement)

These phases provide a minimal viable product (MVP) that demonstrates the core concept.

---

## Optional Enhancements (Post-MVP)

Features that can be added later:

- Email integration for password reset
- OpenID/SAML federation per-cenv
- WebAuthn/passkey support
- Real-time collaboration features
- Built-in data import/export tools
- Visual query builder
- Mobile-responsive UI improvements
- CLI tool for cenv management
- Docker container image
- Kubernetes deployment manifests

---

## Dependencies Summary

```
Phase 1 (Foundation)
  ↓
Phase 2 (Schema)
  ↓
Phase 3 (Authentication) ←────────┐
  ↓                                │
Phase 4 (Authorization)            │
  ↓                                │
Phase 5 (Database Ops) ────────────┤
  ↓                   ↓            │
Phase 6 (Starlark)  Phase 7        │
  ↓                 (Templates)    │
  ↓                   ↓            │
Phase 8 (Web UI Foundation) ───────┤
  ↓                                │
Phase 9 (Advanced UI)              │
  ↓                                │
Phase 10 (Security) ───────────────┤
  ↓                                │
Phase 11 (Sessions) ───────────────┘
  ↓
Phase 12 (Admin)
  ↓
Phase 13 (Logging) ←────────┐
  ↓                          │
Phase 14 (Testing) ──────────┤
  ↓                          │
Phase 15 (Documentation) ────┘
```

Most phases can have some parallel development, but dependencies should be respected for stability.
