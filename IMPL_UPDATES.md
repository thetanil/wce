# IMPL.md Updates - Phase 3 Complete

## Summary of Changes

This document summarizes the updates made to IMPL.md after completing Phase 3 and incorporating the document store design inspired by LiteStore.

## Phase 3: Authentication Core - MARKED COMPLETE ✅

### What Was Implemented
All tasks from Phase 3 completed and documented:

1. **User Registration (3.1)** - Complete cenv creation with owner user
2. **Password Hashing (3.2)** - Bcrypt with cost factor 12
3. **JWT Generation (3.3)** - Manual implementation using stdlib only
4. **Login Endpoint (3.4)** - Full authentication flow with session tracking
5. **Token Validation (3.5)** - JWT validation and cenv-scoped tokens
6. **Session Management (3.6)** - Session CRUD with revocation support
7. **Integration Testing (3.7)** - Comprehensive end-to-end tests

### Key Metrics Added
- **Test Coverage**: 37 total tests (21 auth, 7 cenv, 9 integration)
- **Coverage Percentages**: 64% auth, 76.7% cenv, 49.3% server
- **Files Created**: 6 new files, 1552 lines of code + tests
- **Dependencies**: Only `golang.org/x/crypto` for bcrypt

### Implementation Highlights
- No external JWT library (stdlib only)
- Custom UUID generation (no uuid package)
- Token scoping per-cenv (prevents cross-cenv reuse)
- Session tracking with IP address and user agent
- Token hash storage for revocation capability

## New Design: Document Store (Phase 5)

### Inspiration
Based on LiteStore's approach to using SQLite as a document store, adapted for WCE's multi-tenant architecture.

### Core Tables Added to Phase 5
```
_wce_documents
  - id (document path/key like "pages/home")
  - content (text or base64-encoded binary)
  - content_type (MIME type)
  - is_binary, searchable flags
  - timestamps, versioning, creator tracking

_wce_document_tags
  - Categorization system for documents
  - Support for system and custom tags

_wce_document_search
  - SQLite FTS5 full-text search index
  - Searchable content indexed automatically
```

### Key Features
1. **Hierarchical Document IDs**: `pages/home`, `api/users`, `templates/header`
2. **Content Types**: JSON, HTML, Markdown, plain text, binary (base64)
3. **Full-Text Search**: Using SQLite FTS5 extension
4. **Versioning**: Track document version history
5. **Permissions**: Integrated with existing permission system
6. **Tags**: Flexible categorization and filtering

### Document Store Operations (Phase 5.2)
- CreateDocument - Insert new document
- GetDocument - Retrieve by ID
- UpdateDocument - Modify existing
- DeleteDocument - Remove document
- ListDocuments - Paginated listing with prefix filter
- SearchDocuments - Full-text search

### API Endpoints (Phase 5.3)
- `POST /{cenv-id}/documents` - Create
- `GET /{cenv-id}/documents/{doc-id}` - Read
- `PUT /{cenv-id}/documents/{doc-id}` - Update
- `DELETE /{cenv-id}/documents/{doc-id}` - Delete
- `GET /{cenv-id}/documents?prefix=...` - List
- `GET /{cenv-id}/documents/search?q=...` - Search

## Template System Updates (Phase 7)

### Document-Based Templates
Templates are now special documents rather than a separate table:
- Stored in `_wce_documents` with prefix `templates/`
- Content types: `text/html+template`, `text/markdown`
- Use document versioning for template history
- Tagged appropriately: `template`, `page`, `component`

### Template Rendering (7.2)
Enhanced rendering engine with access to:
- Database queries (permission-enforced)
- Other documents via `doc(id)` function
- Starlark functions
- Request context

Custom template functions:
- `doc(id)` - Load another document
- `query(sql, params)` - Execute query with permissions
- `markdown(text)` - Convert markdown to HTML
- `json(data)` - Serialize to JSON

### Page Cache System (7.3)
```sql
_wce_page_cache
  - document_id
  - rendered_html
  - rendered_at
  - etag (for cache validation)
```

Features:
- ETag-based cache validation
- Support for `If-None-Match` (304 responses)
- Automatic invalidation on document changes
- Configurable expiration per-cenv

### Commit Hooks (7.4)
Auto-rendering system:
- Detect template document changes
- Detect data changes affecting templates
- Trigger re-render asynchronously
- Update cache table
- Update search index for rendered content

### Page Serving (7.5)
- Endpoint: `GET /{cenv-id}/pages/{path}` or `GET /{cenv-id}/p/{path}`
- Check cache first with ETag validation
- Fall back to on-demand render
- Content negotiation support
- Proper cache headers

## Phase 2 Extension Notes Added

Added forward-looking note to Phase 2 about tables that will be added in Phase 5:
- `_wce_documents`
- `_wce_document_tags`
- `_wce_document_search`
- `_wce_page_cache`

These will extend the existing schema defined in `internal/db/schema.go`.

## Priority Tasks Updated

**Before:**
1. Phase 1-3: Foundation + Authentication (critical)
2. Phase 4-5: Permissions + Database ops (critical)
3. Phase 6: Starlark (core feature)
4. Phase 8: Basic web UI (usability)
5. Phase 10: Security hardening (production requirement)

**After:**
1. Phase 1-3: Foundation + Authentication ✅ **COMPLETE**
2. Phase 4: Permissions system (important)
3. **Phase 5: Document Store + Database ops (CRITICAL - core feature)**
4. Phase 6: Starlark integration (enables extensibility)
5. Phase 7: Template rendering (enables HTML pages)
6. Phase 8: Basic web UI (usability)
7. Phase 10: Security hardening (production)

**Current Status**: Phase 3 complete, 37 tests passing, ready for Phase 4

## Why Document Store?

### Benefits Over Separate Tables
1. **Unified Storage**: All content (pages, templates, API responses) in one place
2. **Flexible Schema**: No need to alter tables for new content types
3. **Full-Text Search**: Built-in FTS5 for all searchable content
4. **Versioning**: Track history of all documents uniformly
5. **Permissions**: Single permission model applies to all documents
6. **Tags**: Flexible categorization without schema changes

### Inspired by LiteStore
- Uses SQLite as NoSQL document database
- Text stored as-is, binary as base64
- Hierarchical document organization
- FTS4/FTS5 for full-text search
- JSON1 extension for JSON queries
- Minimal dependencies

### Adapted for WCE
- Multi-tenant (per-cenv isolation)
- Integrated with WCE permission system
- Document IDs are hierarchical paths
- Commit hooks for automatic cache updates
- RESTful API following WCE patterns

## Implementation Sequence

With these changes, the recommended implementation order is:

1. ✅ **Phase 1-3**: Foundation, Database, Authentication (DONE)
2. **Phase 4**: Authorization & Permissions (enables multi-user)
3. **Phase 5**: Document Store (CRITICAL - enables content storage)
4. **Phase 6**: Starlark (enables runtime extensibility)
5. **Phase 7**: Template Rendering (enables HTML serving)
6. **Phase 8**: Web UI (enables browser-based management)

After Phase 5, you'll have a working document store that can be used via the API. Templates and UI build on top of this foundation.

## Next Steps

1. **Phase 4** (if multi-user needed): Implement permissions
2. **Phase 5** (recommended next): Implement document store
   - Add tables to schema
   - Implement document CRUD operations
   - Create API endpoints
   - Write tests
3. **Use document store**: Start storing content as documents
4. **Phase 7**: Add template rendering on top of documents

## Files Modified

- `IMPL.md` - Updated Phase 3 to complete, added document store design to Phase 5, updated Phase 7 for document-based templates, updated priority tasks

## Commits

1. `a669189` - [Phase 3] Implement authentication core with JWT
2. `e74f1d1` - Add comprehensive integration test for authentication flow
3. `317ba63` - Add integration test documentation
4. `e83fd0a` - Update IMPL.md with Phase 3 completion and document store design
