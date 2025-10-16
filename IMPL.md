# Implementation Guide

This document outlines the implementation steps for WCE, organized into small batches with clear dependencies. Each phase builds on the previous one, ensuring the system can be developed incrementally and tested at each stage.

## Phase 1: Foundation (Bootstrap) ✅ COMPLETED

**Goal**: Basic HTTP server that can route requests to different cenvs

### 1.1 Project Initialization ✅
- [x] Create `go.mod` with module name
- [x] Add dependencies: `mattn/go-sqlite3` v1.14.32, `go.starlark.net`, `golang.org/x/crypto`
- [x] Create basic project structure:
  ```
  /cmd/wce/main.go          - Entry point
  /internal/server/         - HTTP server
  /internal/cenv/           - Cenv management
  /internal/db/             - Database operations (used in Phase 2+)
  /internal/auth/           - Authentication (used in Phase 3+)
  /internal/starlark/       - Starlark integration (used in Phase 6+)
  /internal/template/       - Template engine (used in Phase 7+)
  /web/                     - Static assets (used in Phase 8+)
  ```

**Test**: `go build` succeeds ✅
**Note**: Starlark dependency is `go.starlark.net/starlark`, not `github.com/google/starlark-go`

### 1.2 Basic HTTP Server ✅
- [x] Implement HTTP server listening on port 5309
- [x] Graceful shutdown on SIGINT/SIGTERM (30s grace period)
- [x] Logging middleware (method, path, status, duration)
- [x] Health check endpoint: `GET /health`
- [x] Proper timeouts: ReadTimeout=15s, WriteTimeout=15s, IdleTimeout=60s

**Test**: Server starts, responds to `/health` ✅

**Implementation notes:**
- Used custom `responseWriter` wrapper to capture status codes for logging
- Logging happens via middleware for all requests
- Server uses channels for coordinating graceful shutdown

### 1.3 Cenv Routing ✅
- [x] Parse URL path to extract `{cenv-id}` using `r.PathValue()`
- [x] Validate cenv-id format (UUID) with regex
- [x] Route requests: `/{cenv-id}/{path...}` → cenv handler
- [x] Route requests: `/new` → cenv creation handler
- [x] Route requests: `GET /health` → health handler
- [x] 404 for invalid paths and invalid UUIDs

**Test**: Different cenv-ids route to correct handler ✅

**Implementation notes:**
- **IMPORTANT**: Use Go 1.22+ `http.ServeMux` pattern matching, NOT manual path parsing
- Pattern: `mux.HandleFunc("/{cenvID}/{path...}", handler)`
- Extract values: `r.PathValue("cenvID")` and `r.PathValue("path")`
- The `{path...}` wildcard captures remaining path segments
- UUID validation happens in handler, not in routing
- No need for manual `if r.URL.Path ==` checks - patterns handle it

**Files created:**
- `internal/cenv/cenv.go` - Manager, UUID validation, path parsing (mostly for testing)
- `internal/server/server.go` - HTTP server with pattern-based routing

### 1.4 Database File Management ✅
- [x] Configure cenv storage directory: `./cenvs/` (configurable via `WCE_STORAGE_DIR`)
- [x] Map cenv-id to file path: `./cenvs/{uuid}.db`
- [x] Check if database file exists: `Manager.Exists(cenvID)`
- [x] Create database file: `Manager.Create(cenvID)`
- [x] Set file permissions (600 - owner only) via `os.Chmod()`
- [x] Open database: `Manager.Open(cenvID)` with pragmas configured

**Test**: Database files created with correct permissions ✅

**Implementation notes:**
- `Manager.Create()` creates empty SQLite database and sets permissions
- `Manager.Open()` opens connection and sets pragmas:
  - `PRAGMA foreign_keys = ON`
  - `PRAGMA journal_mode = WAL`
  - `PRAGMA synchronous = NORMAL`
- Schema initialization deferred to Phase 2.2
- Storage directory created with 0700 permissions in `main.go`

**Files created:**
- `internal/cenv/cenv.go` - Added `Exists()`, `Create()`, `Open()` methods
- `internal/cenv/cenv_test.go` - Test suite (5 tests, all passing)

**Dependencies**: None
**Deliverable**: Server routes requests to cenv-specific handlers ✅
**Binary size**: ~12MB (includes SQLite driver)

---

## Phase 2: Database Schema ✅ COMPLETED

**Goal**: Initialize new cenvs with proper schema

### 2.1 Schema Definition ✅
- [x] Define SQL schema for all `_wce_*` tables:
  - `_wce_users` ✅
  - `_wce_sessions` ✅
  - `_wce_table_permissions` ✅
  - `_wce_row_policies` ✅
  - `_wce_config` ✅
  - `_wce_audit_log` ✅
- [x] Store schema as Go constant (use backtick strings for readability)
- [x] Schema includes indexes on foreign keys
- [x] Use proper SQLite data types (TEXT, INTEGER, BOOLEAN as INTEGER)

**Test**: Schema SQL parses correctly ✅

**Implementation:**
- Created `internal/db/schema.go` with complete schema as const
- Split into logical sections with comments
- 13 indexes created on foreign keys and common query fields
- Used `CREATE TABLE IF NOT EXISTS` for idempotency
- Includes default INSERT for `_wce_config` values

### 2.2 Database Initialization ✅
- [x] Connect to SQLite with appropriate pragmas (already done in Phase 1.4)
  - `PRAGMA foreign_keys = ON` ✅
  - `PRAGMA journal_mode = WAL` ✅
  - `PRAGMA synchronous = NORMAL` ✅
- [x] Add `Manager.Initialize(cenvID)` method
- [x] Check if database is initialized (check for `_wce_users` table)
- [x] Execute schema creation if not initialized
- [x] Insert default config values into `_wce_config`
- [x] Return error if initialization fails

**Test**: New cenv has all tables created ✅

**Implementation:**
- Modified `Manager.Create()` to call `Initialize()` automatically
- Uses transaction for schema creation (all or nothing)
- Queries `sqlite_master` to check if `_wce_users` exists
- Default config: `session_timeout_hours=24`, `allow_registration=false`, `max_users=10`
- Cleanup on failure: removes database file if initialization fails

### 2.3 Connection Pooling ✅
- [x] Use `sql.DB` built-in connection pooling (no custom implementation needed)
- [x] Set `db.SetMaxOpenConns(5)` per cenv
- [x] Set `db.SetMaxIdleConns(2)`
- [x] Set `db.SetConnMaxLifetime(30 * time.Minute)`
- [x] Set `db.SetConnMaxIdleTime(10 * time.Minute)` for idle connection cleanup
- [x] Store `*sql.DB` in Manager's sync.Map for cenv -> connection mapping
- [x] Add `Manager.GetConnection(cenvID)` method with lazy loading
- [x] Add `Manager.CloseConnection(cenvID)` for cleanup
- [x] Add `Manager.CloseAll()` for shutting down all connections

**Test**: Multiple concurrent requests use pooled connections ✅

**Implementation:**
- Go's `database/sql` package handles pooling automatically
- Used `sync.Map` for thread-safe cenv -> *sql.DB mapping
- `GetConnection()` lazy-loads connections on first access
- Connection reused from pool on subsequent calls
- `Open()` method still available for non-pooled connections (used internally)
- Tested with 10 concurrent goroutines successfully

**Files created:**
- `internal/db/schema.go` - Complete WCE schema definition
- Updated `internal/cenv/cenv.go` - Added Initialize(), GetConnection(), connection pooling
- Updated `internal/cenv/cenv_test.go` - Added TestSchemaInitialization, TestConnectionPooling

**Test results:**
- 7/7 tests passing
- All 6 system tables created
- 13 indexes created
- Default config values inserted
- Connection pooling working
- Concurrent access working

**Dependencies**: Phase 1 ✅
**Deliverable**: New cenvs initialize with complete schema ✅

**Database size**: ~115KB per empty cenv (includes schema + indexes + config)

**Phase 2 Extension Notes (for Phase 5):**
When implementing Phase 5 (Document Store), add these tables to the schema:
- `_wce_documents` - Core document storage (see Phase 5.1)
- `_wce_document_tags` - Document categorization
- `_wce_document_search` - FTS5 full-text search index
- `_wce_page_cache` - Rendered page cache (see Phase 7.3)

These tables will be added to `internal/db/schema.go` in Phase 5.

---

## Phase 3: Authentication Core ✅ COMPLETED

**Goal**: User registration, login, and JWT token generation

### 3.1 User Registration (New Cenv Creation) ✅
- [x] Update `handleNewCenv` in `internal/server/server.go`
- [x] Parse JSON body: `{username, password, email?}` from `POST /new`
- [x] Validate username (required) and password (min 8 chars)
- [x] Generate UUID for new cenv (custom implementation using crypto/rand)
- [x] Call `Manager.Create(cenvID)` (already creates database)
- [x] Schema already initialized in Phase 2.2
- [x] Hash password with bcrypt (cost 12)
- [x] Generate UUID for user_id
- [x] Insert into `_wce_users` with role='owner', `enabled=1`
- [x] Return JSON: `{cenv_id, cenv_url, username, message}` with status 201

**Test**: Creating cenv returns valid URL, database contains owner user ✅

**Implementation:**
- Created `internal/auth/auth.go` with full user management
- Used custom UUID generation (RFC 4122 v4) with `crypto/rand` - no external dependency needed
- `CreateUser()` handles password hashing and user insertion
- Cenv URL includes scheme (http/https) detection from request
- Proper error handling for duplicate usernames, short passwords

### 3.2 Password Hashing ✅
- [x] Password functions in `internal/auth/auth.go`
- [x] Use `golang.org/x/crypto/bcrypt` (added to dependencies)
- [x] Function: `HashPassword(password string) (string, error)`
- [x] Function: `VerifyPassword(password, hash string) error`
- [x] Hash passwords with cost factor 12
- [x] Handle errors gracefully

**Test**: Hashing and verification work correctly ✅

**Implementation:**
- Bcrypt cost factor 12 provides good security/performance balance
- All tests pass including duplicate password verification
- Error messages don't leak password information

### 3.3 JWT Token Generation ✅
- [x] Created `internal/auth/jwt.go` with full JWT implementation
- [x] **Used standard library only** - no JWT dependencies
- [x] Implemented HMAC-SHA256 signing manually with `crypto/hmac`
- [x] JWT secret generated randomly at server startup (stored in Server struct)
- [x] Generate JWT with claims:
  - `user_id` (string)
  - `username` (string)
  - `cenv_id` (string)
  - `role` (string)
  - `jti` (session ID for tracking)
  - `iat` (issued at) - Unix timestamp
  - `exp` (expiration) - Unix timestamp
- [x] Sign with HS256 algorithm
- [x] Return token as base64url-encoded string

**Test**: Token generation and parsing work ✅

**Implementation:**
- Manual JWT implementation keeps binary size small
- `JWTManager` struct handles generation and validation
- `ValidateToken()` verifies signature and expiration
- `GetTokenHash()` provides SHA256 hash for session storage
- Default expiration: 24 hours (`auth.DefaultSessionTimeout`)

### 3.4 Login Endpoint ✅
- [x] Added route: `mux.HandleFunc("POST /{cenvID}/login", s.handleLogin)`
- [x] Parse JSON body: `{username, password}`
- [x] Extract cenvID from path: `r.PathValue("cenvID")`
- [x] Validate UUID format and cenv exists
- [x] Get database connection: `Manager.GetConnection(cenvID)`
- [x] Query `_wce_users` for username (prepared statement)
- [x] Verify password hash with `auth.VerifyPassword()`
- [x] Check `enabled` flag
- [x] Update `last_login` timestamp
- [x] Generate JWT token with 24h expiration
- [x] Generate session ID and insert into `_wce_sessions`
- [x] Store token hash (SHA256) for revocation capability
- [x] Capture IP address and user agent
- [x] Return JSON: `{token, expires_at, user_id, username, role}` with 200 status

**Test**: Valid credentials return token, invalid credentials return 401 ✅

**Implementation:**
- Token hash stored using `auth.GetTokenHash()` for revocation
- Session tracking includes IP address and user agent
- Generic error messages don't reveal if user exists
- Disabled accounts return 401 with specific message
- All database operations use prepared statements

### 3.5 Token Validation & Authentication ✅
- [x] Created `internal/auth/middleware.go` with reusable middleware
- [x] Integrated authentication directly into `handleCenvRequest`
- [x] Extract token from `Authorization: Bearer <token>` header
- [x] Parse and validate JWT signature using `JWTManager`
- [x] Verify `cenv_id` claim matches cenv from path
- [x] Verify token not expired (check `exp` claim)
- [x] Get database connection for cenv
- [x] Query `_wce_sessions` with token hash
- [x] Check session exists and not expired
- [x] Return user info in response for now (placeholder for future handlers)
- [x] Return 401 if any validation fails

**Test**: Authenticated requests pass, unauthenticated fail ✅

**Implementation:**
- `AuthMiddleware()` function available for future use
- `RequireRole()` middleware for role-based access
- `GetClaimsFromContext()` helper for extracting user info
- Current implementation: inline auth in `handleCenvRequest`
- Tokens scoped to specific cenv (cannot use token from cenv A on cenv B)
- Session validation happens on every request

### 3.6 Session Management ✅
- [x] `CreateSession()` - Create session with token hash
- [x] `IsSessionValid()` - Check if session exists and not expired
- [x] `RevokeSession()` - Delete single session by token hash
- [x] `RevokeAllUserSessions()` - Delete all sessions for a user
- [x] `CleanupExpiredSessions()` - Remove expired sessions
- [x] Session tracking includes IP, user agent, timestamps

**Test**: Session management functions work correctly ✅

### 3.7 Integration Testing ✅
- [x] Created `internal/server/server_integration_test.go` (414 lines)
- [x] Tests complete flow: create cenv → login → authenticated requests
- [x] Tests error cases: wrong password, missing token, invalid token
- [x] Tests token isolation between cenvs
- [x] Tests multiple authenticated paths
- [x] All 9 sub-tests passing in ~2 seconds

**Test Coverage:**
- internal/auth: 64.0% (21 tests)
- internal/cenv: 76.7% (7 tests)
- internal/server: 49.3% (9 integration tests)
- **Total: 37 tests, all passing**

**Files created:**
- `internal/auth/auth.go` - User & session management (274 lines)
- `internal/auth/auth_test.go` - Auth unit tests (498 lines)
- `internal/auth/jwt.go` - JWT implementation (132 lines)
- `internal/auth/jwt_test.go` - JWT unit tests (136 lines)
- `internal/auth/middleware.go` - Auth middleware (98 lines)
- `internal/server/server_integration_test.go` - Integration tests (414 lines)
- Updated `internal/server/server.go` - HTTP handlers (+396 lines)

**Dependencies Added**:
- `golang.org/x/crypto` v0.43.0 (for bcrypt only - allowed in CLAUDE.md)

**Key Design Decisions:**
1. **No external JWT library** - Implemented JWT manually using stdlib to minimize dependencies
2. **Custom UUID generation** - Used crypto/rand instead of external package
3. **Session tracking** - Tokens hashed and stored for revocation capability
4. **Token scoping** - JWT claims include cenv_id to prevent cross-cenv token reuse
5. **Security first** - Generic error messages, bcrypt cost 12, prepared statements

**Security Features:**
- Bcrypt password hashing (never store plaintext)
- JWT tokens signed with HMAC-SHA256
- Session tracking with revocation capability
- Token expiration (24 hours default)
- Database files with 600 permissions
- All queries use prepared statements
- IP address and user agent tracking
- Token scoped to specific cenv

**Deliverable**: Users can register, login, and authenticate ✅

**Next Phase Notes:**
- Authentication is complete and tested
- Phase 4 (Authorization) can now build on this foundation
- Consider implementing logout endpoint in Phase 11 (uses `RevokeSession()`)
- JWT secret should be configurable via environment variable in production

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

## Phase 5: Document Store & Core Database Operations

**Goal**: SQLite-based document store with LiteStore-inspired design, plus safe database operations

### 5.1 Document Store Schema
- [ ] Create `_wce_documents` table:
  - `id` TEXT PRIMARY KEY (document path/key)
  - `content` TEXT (document body - JSON, HTML, markdown, etc.)
  - `content_type` TEXT (e.g., 'application/json', 'text/html', 'text/markdown')
  - `is_binary` INTEGER (0 = text, 1 = base64-encoded binary)
  - `searchable` INTEGER (1 = indexed for search, 0 = not indexed)
  - `created_at` INTEGER (Unix timestamp)
  - `modified_at` INTEGER (Unix timestamp)
  - `created_by` TEXT (user_id)
  - `modified_by` TEXT (user_id)
  - `version` INTEGER (incremental version number)
- [ ] Create `_wce_document_tags` table for categorization:
  - `id` INTEGER PRIMARY KEY AUTOINCREMENT
  - `document_id` TEXT (FK to _wce_documents)
  - `tag` TEXT (tag name)
  - UNIQUE(document_id, tag)
- [ ] Create `_wce_document_search` table using FTS5:
  - Full-text search index for searchable documents
  - Index document_id, content
- [ ] Create indexes on foreign keys and common query fields

**Test**: Document tables created successfully

**Implementation hints:**
- Similar to LiteStore's approach but adapted for WCE's multi-tenant model
- Documents are stored as-is for text, base64-encoded for binary
- Use hierarchical document IDs: `pages/home`, `api/users`, `templates/header`
- Enable SQLite JSON1 extension for JSON document queries
- Consider document size limits (e.g., 10MB per document)

### 5.2 Document CRUD Operations
- [ ] Create `internal/document/document.go`
- [ ] `CreateDocument(id, content, contentType)` - Insert new document
- [ ] `GetDocument(id)` - Retrieve document by ID
- [ ] `UpdateDocument(id, content)` - Update existing document
- [ ] `DeleteDocument(id)` - Remove document
- [ ] `ListDocuments(prefix, limit, offset)` - List with pagination
- [ ] `SearchDocuments(query, limit)` - Full-text search
- [ ] All operations enforce permissions (check user access)
- [ ] Automatic timestamps (created_at, modified_at)
- [ ] Track creator and modifier (created_by, modified_by)

**Test**: Document CRUD operations work correctly

**Implementation hints:**
- Documents are the primary storage mechanism for content
- HTML pages, API responses, templates all stored as documents
- Use prepared statements for all queries
- Return documents as JSON-serializable structs
- Handle binary data encoding/decoding automatically

### 5.3 Document API Endpoints
- [ ] `POST /{cenv-id}/documents` - Create document (requires write permission)
- [ ] `GET /{cenv-id}/documents/{doc-id}` - Get document (requires read permission)
- [ ] `PUT /{cenv-id}/documents/{doc-id}` - Update document (requires write permission)
- [ ] `DELETE /{cenv-id}/documents/{doc-id}` - Delete document (requires delete permission)
- [ ] `GET /{cenv-id}/documents` - List documents with optional prefix filter
- [ ] `GET /{cenv-id}/documents/search?q=query` - Search documents
- [ ] Support content negotiation (Accept header)
- [ ] Return proper HTTP status codes

**Test**: Document API endpoints work correctly

### 5.4 Parameterized Query Execution
- [ ] API: `Query(sql string, params ...interface{})`
- [ ] Always use parameterized statements
- [ ] Never allow string interpolation in SQL
- [ ] Return rows as JSON-serializable maps
- [ ] Handle NULL values correctly

**Test**: Parameterized queries prevent SQL injection

### 5.5 Transaction Support
- [ ] Begin transaction: `BeginTx()`
- [ ] Commit transaction: `Commit()`
- [ ] Rollback transaction: `Rollback()`
- [ ] Auto-rollback on panic
- [ ] Nested transaction handling (savepoints)

**Test**: Transactions roll back on error

### 5.6 Commit Hook System
- [ ] Register Go callback functions for commit events
- [ ] Execute callbacks after successful commit
- [ ] Pass transaction details to callbacks
- [ ] Handle callback errors (log, don't fail commit)
- [ ] Support multiple hooks
- [ ] Use for document search index updates
- [ ] Use for cache invalidation

**Test**: Callbacks execute after commit

### 5.7 Audit Logging
- [ ] Log all write operations to `_wce_audit_log`
- [ ] Capture: user_id, action, resource, timestamp, IP, user-agent
- [ ] Log in same transaction as operation
- [ ] Background cleanup of old audit logs (optional)
- [ ] Include document operations in audit log

**Test**: All operations appear in audit log

**Dependencies**: Phase 4
**Deliverable**: Document store with full CRUD operations and secure database layer

**Document Store Design Notes:**
- Inspired by LiteStore's approach but adapted for multi-tenant cenvs
- Documents are the primary content storage mechanism
- Each document has a unique ID (path-like: `pages/home`, `api/users`)
- Content types: JSON, HTML, Markdown, plain text, binary (base64)
- Full-text search using SQLite FTS5 extension
- Tags for categorization and filtering
- Version tracking for document history
- Permission checks enforce access control

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

## Phase 7: Template & Page Rendering System

**Goal**: Document-based templates with rendering and caching

### 7.1 Template Document Storage
- [ ] Store templates as documents in `_wce_documents`
- [ ] Template document IDs use prefix: `templates/header`, `templates/footer`
- [ ] Content type: `text/html+template` for Go templates
- [ ] Content type: `text/markdown` for Markdown templates
- [ ] Store both source (edit version) and rendered (display version)
- [ ] Use document versioning for template history
- [ ] Tag templates: `template`, `page`, `component`, etc.

**Test**: Templates stored and retrieved as documents

**Implementation hints:**
- Templates are special documents with rendering capabilities
- Source stored in main document content
- Rendered version cached in `_wce_page_cache` table (see 7.3)
- Use document tags to identify template types

### 7.2 Template Rendering Engine
- [ ] Create `internal/template/template.go`
- [ ] Parse Go templates from document content
- [ ] Render with data from:
  - Database queries
  - Other documents
  - Starlark functions
  - Request context
- [ ] Auto-escape HTML by default
- [ ] Support custom functions (safe subset):
  - `doc(id)` - Load another document
  - `query(sql, params)` - Execute query (with permissions)
  - `markdown(text)` - Convert markdown to HTML
  - `json(data)` - Serialize to JSON
- [ ] Handle rendering errors gracefully

**Test**: Templates render correctly with data

**Implementation hints:**
- Use `html/template` for automatic XSS protection
- Provide sandbox for template functions
- Templates can include other templates
- Support layout templates (base + content)

### 7.3 Page Cache System
- [ ] Table: `_wce_page_cache` (document_id, rendered_html, rendered_at, etag)
- [ ] Store rendered output after successful render
- [ ] Generate ETag for cache validation
- [ ] Invalidate cache on document changes
- [ ] Set cache expiration (configurable per-cenv)
- [ ] Support cache headers (Cache-Control, ETag, Last-Modified)

**Test**: Cached pages serve faster, cache invalidates correctly

**Implementation hints:**
- Cache keyed by document ID + query parameters (if any)
- ETag generated from content hash
- Support `If-None-Match` header for 304 responses
- Cache miss triggers on-demand render

### 7.4 Commit Hook for Auto-Rendering
- [ ] Register commit hook in Phase 5's hook system
- [ ] Detect template document changes in commit
- [ ] Detect data changes that affect template output
- [ ] Trigger re-render for affected templates
- [ ] Update cache table asynchronously
- [ ] Handle rendering errors (log, don't fail commit)
- [ ] Update search index for searchable rendered content

**Test**: Commits trigger cache updates automatically

**Implementation hints:**
- Hook runs after commit succeeds
- Re-render can happen asynchronously
- Track dependencies between templates and data
- Consider incremental cache updates

### 7.5 Page Serving Endpoint
- [ ] Endpoint: `GET /{cenv-id}/pages/{path}` OR `GET /{cenv-id}/p/{path}`
- [ ] Map path to document ID: `pages/{path}`
- [ ] Check cache first (`_wce_page_cache`)
- [ ] Return cached HTML if fresh and ETag matches
- [ ] Fall back to on-demand render if cache miss
- [ ] Support content negotiation:
  - `Accept: text/html` → rendered page
  - `Accept: application/json` → document metadata + content
- [ ] Return proper HTTP headers (Content-Type, Cache-Control, ETag)
- [ ] Support 304 Not Modified responses

**Test**: Pages served efficiently from cache

### 7.6 Markdown Rendering
- [ ] Support Markdown documents: content_type = `text/markdown`
- [ ] Render Markdown to HTML on-demand or cached
- [ ] Use safe Markdown parser (no script injection)
- [ ] Support code highlighting
- [ ] Support custom extensions (tables, checkboxes, etc.)

**Test**: Markdown documents render correctly

**Dependencies**: Phase 5 (Document Store)
**Deliverable**: Document-based template system with rendering and caching

**Template System Design Notes:**
- Templates stored as special documents (not separate table)
- Source (edit) and rendered (display) versions both tracked
- Cache system provides fast page serving
- Commit hooks enable automatic cache updates
- Templates can reference other documents and query data
- Supports both Go templates and Markdown
- Permission system controls template access
- ETag-based cache validation for efficiency

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

1. **Phase 1-3**: Foundation + Authentication (critical) ✅ **COMPLETE**
2. **Phase 4**: Permissions system (important for multi-user cenvs)
3. **Phase 5**: Document Store + Database ops (critical - core feature)
4. **Phase 6**: Starlark integration (enables runtime extensibility)
5. **Phase 7**: Template rendering (enables HTML page serving)
6. **Phase 8**: Basic web UI (usability)
7. **Phase 10**: Security hardening (production requirement)

These phases provide a minimal viable product (MVP) that demonstrates the core concept.

**Current Status**: Phase 3 complete, 37 tests passing, ready for Phase 4

**Key Achievement**: Complete authentication with JWT, no external JWT library, minimal dependencies

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
