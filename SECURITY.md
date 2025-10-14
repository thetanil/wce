# Security Model

## Overview

WCE operates as a multi-tenant platform where each user can provision isolated
compute environments (cenvs). Each cenv is a separate SQLite database with its
own authentication, authorization, and user management system.

## Threat Model

### Deployment Context

- Single binary deployed on public web server
- Multiple users, each with one or more cenvs
- Users can define custom endpoints via Starlark
- Users can execute arbitrary SQL within their cenv
- Users can grant access to other users

### Security Boundaries

1. **Cenv Isolation**: Each cenv is a separate SQLite database file with independent security context
2. **User Isolation**: Users cannot access other cenvs unless explicitly granted permission
3. **Code Execution**: Starlark scripts execute in sandboxed environment per cenv
4. **Data Access**: Row-level and table-level permissions managed within each cenv

## Authentication Architecture

### Model: Direct Database Authentication

**No platform database.** Users authenticate directly to their cenv database. Each cenv is completely autonomous for authentication and authorization.

#### Core Principles

1. **Database-native authentication**: Credentials stored in each cenv's `_wce_users` table
2. **URL-based cenv selection**: `https://domain.com/{cenv-id}/` routes to specific database
3. **Per-database sessions**: Session tokens scoped to individual cenv
4. **Truly portable**: Each cenv is a standalone SQLite file with all auth data
5. **No shared state**: Zero coordination between cenvs

#### Creating a New Cenv

```
1. User requests new cenv via public endpoint
   ↓
2. WCE generates UUID for cenv identifier
   ↓
3. Creates new SQLite database file: {uuid}.db
   ↓
4. Initializes schema with _wce_* tables
   ↓
5. Prompts user to set initial admin password
   ↓
6. Hashes password (bcrypt) and stores in _wce_users
   ↓
7. Returns cenv URL: https://domain.com/{uuid}/
```

#### Authentication Flow

```
1. User navigates to https://domain.com/{cenv-id}/
   ↓
2. WCE loads database file for {cenv-id}
   ↓
3. User submits username + password
   ↓
4. WCE queries _wce_users in THAT database
   ↓
5. Verifies bcrypt hash
   ↓
6. Issues JWT scoped to {cenv-id}
   ↓
7. All subsequent requests include JWT
   ↓
8. JWT validated against _wce_users in {cenv-id} database
```

#### Session Management

- **JWT tokens** include: `{cenv_id, user_id, role, issued_at, expires_at}`
- **Scope validation**: Token for cenv-A cannot access cenv-B
- **Token storage**: httpOnly cookies or localStorage (user choice)
- **Expiration**: Configurable per-cenv (default 24 hours)
- **Refresh tokens**: Optional, stored in `_wce_sessions` table in each cenv

#### Multi-User Access

Users can grant access to their cenv to others:

1. Owner invites user by setting username/password in `_wce_users`
2. Owner shares cenv URL and credentials out-of-band
3. Invited user logs in with those credentials
4. Multiple users can authenticate to same cenv with different credentials

**Alternative**: Owner can enable "registration mode" where cenv accepts new signups via web form.

### Why This Approach?

**Advantages:**
- ✅ No platform database to manage or back up separately
- ✅ Each cenv is truly independent and portable
- ✅ Can copy/move/backup databases as simple files
- ✅ No coordination or shared state between cenvs
- ✅ Cenv can be moved between WCE instances seamlessly
- ✅ Simpler architecture with fewer moving parts
- ✅ Aligns with "database is the application" philosophy

**Considerations:**
- User must remember different URLs for different cenvs
- No single-sign-on across cenvs (each has separate login)
- User may have different credentials in different cenvs
- Password resets require cenv owner intervention (or email config per-cenv)

## Authorization Model

### Per-Cenv Permission System

Each cenv implements its own authorization using built-in tables:

#### Core Security Tables

```sql
-- Created automatically in each new cenv
CREATE TABLE _wce_users (
    user_id TEXT PRIMARY KEY,  -- UUID generated on creation
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,  -- bcrypt hash
    role TEXT NOT NULL DEFAULT 'viewer', -- owner, admin, editor, viewer
    email TEXT,  -- Optional, for password reset
    created_at INTEGER NOT NULL,
    invited_by TEXT,  -- user_id of inviter
    last_login INTEGER,
    enabled BOOLEAN DEFAULT 1
);

CREATE TABLE _wce_sessions (
    session_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    token_hash TEXT NOT NULL,  -- SHA256 of JWT for revocation
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    last_used INTEGER,
    ip_address TEXT,
    user_agent TEXT,
    FOREIGN KEY (user_id) REFERENCES _wce_users(user_id)
);

CREATE TABLE _wce_table_permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    table_name TEXT NOT NULL,
    user_id TEXT NOT NULL,
    can_read BOOLEAN DEFAULT 0,
    can_write BOOLEAN DEFAULT 0,
    can_delete BOOLEAN DEFAULT 0,
    can_grant BOOLEAN DEFAULT 0,  -- Can grant permissions to others
    FOREIGN KEY (user_id) REFERENCES _wce_users(user_id)
);

CREATE TABLE _wce_row_policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    table_name TEXT NOT NULL,
    user_id TEXT,  -- NULL means applies to all
    policy_type TEXT NOT NULL,  -- 'read', 'write', 'delete'
    sql_condition TEXT NOT NULL  -- e.g., "owner_id = $user_id"
);

CREATE TABLE _wce_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    updated_by TEXT,
    FOREIGN KEY (updated_by) REFERENCES _wce_users(user_id)
);

-- Default config entries
INSERT INTO _wce_config (key, value, updated_at) VALUES
    ('session_timeout_hours', '24', strftime('%s', 'now')),
    ('allow_registration', 'false', strftime('%s', 'now')),
    ('max_users', '10', strftime('%s', 'now'));
```

#### Permission Hierarchy

1. **Owner**: Full control, cannot be removed, can delete cenv
2. **Admin**: Can manage users, permissions, and all data
3. **Editor**: Can read/write data based on table permissions
4. **Viewer**: Read-only access based on table permissions

### SQL Query Rewriting

The platform intercepts and rewrites queries based on permissions:

```
User submits: SELECT * FROM documents
                ↓
Platform checks: _wce_table_permissions for 'documents'
                ↓
Platform checks: _wce_row_policies for additional conditions
                ↓
Rewritten query: SELECT * FROM documents WHERE (id IN (SELECT id FROM documents WHERE owner_id = ?))
                ↓
Execute with user context
```

## Starlark Sandboxing

### Execution Restrictions

Each Starlark script executes in a sandboxed environment:

- **No filesystem access**: Cannot read/write files outside cenv database
- **No network access**: Cannot make external HTTP requests (unless explicitly granted)
- **CPU limits**: Maximum execution time per request (e.g., 5 seconds)
- **Memory limits**: Maximum heap size per execution context
- **No native code**: Cannot call arbitrary Go functions
- **Database access**: Only via provided API with permission checks

### Allowed Operations

Starlark scripts can:
- Query the cenv database (with permission enforcement)
- Define HTTP endpoints
- Transform data using pure functions
- Call whitelisted standard library functions

### Example Starlark Context

```python
# Available to scripts
def my_endpoint(request):
    # db.query() automatically enforces permissions
    results = db.query("SELECT * FROM my_table WHERE public = 1")
    return {"data": results}

# NOT available (will fail)
def bad_endpoint(request):
    open("/etc/passwd")  # Error: 'open' is not defined
    http.get("http://evil.com")  # Error: 'http' is not defined
```

## Isolation Mechanisms

### Cenv Isolation

1. **Filesystem**: Each cenv is a separate SQLite file with restricted permissions (600)
2. **Process**: All cenvs run in the same Go process but with isolated contexts
3. **Memory**: Separate connection pools per cenv
4. **Starlark**: Separate interpreter instances per cenv with isolated globals

### Resource Limits

Per-cenv quotas (configurable):
- Maximum database size: 100MB (default)
- Maximum concurrent connections: 5
- Maximum query execution time: 30 seconds
- Maximum Starlark execution time: 5 seconds
- Maximum request rate: 100 requests/minute

## Attack Scenarios & Mitigations

### 1. SQL Injection

**Attack**: User crafts malicious SQL via Starlark
**Mitigation**:
- All database queries use parameterized statements
- Starlark db API only accepts parameterized queries
- Query rewriting happens after parameterization

### 2. Cross-Cenv Access

**Attack**: User tries to access another user's cenv
**Mitigation**:
- Every request validated against _wce_users table IN THAT CENV
- JWT includes cenv_id; tokens for cenv-A rejected by cenv-B
- Cenv file paths use unpredictable identifiers (UUIDs)
- OS-level file permissions prevent direct file access

### 3. Resource Exhaustion (DoS)

**Attack**: User creates expensive queries or infinite loops
**Mitigation**:
- Per-cenv rate limiting
- Query timeout enforcement
- Database size quotas
- Starlark execution time limits
- Connection pool limits

### 4. Privilege Escalation

**Attack**: Viewer tries to gain admin access
**Mitigation**:
- Role changes require admin permission
- All permission checks happen in platform code (not Starlark)
- Owner role cannot be transferred or removed
- Audit log of all permission changes

### 5. Data Exfiltration

**Attack**: Malicious user granted read access attempts bulk export
**Mitigation**:
- Row-level policies enforced on every query
- Rate limiting prevents rapid bulk access
- Audit logging of all queries
- Optional: webhook notifications for bulk operations

### 6. Starlark Escape

**Attack**: Script attempts to break sandbox
**Mitigation**:
- No access to Go reflection or unsafe operations
- Whitelist of allowed built-in functions
- No file I/O or network primitives exposed
- Regular security audits of exposed APIs

### 7. Template Injection

**Attack**: User injects malicious code via templates
**Mitigation**:
- Templates parsed and validated before storage
- Auto-escaping of user content by default
- Content Security Policy headers
- Separate rendering context per request

## Network Security

### TLS/HTTPS

**Production deployment requires:**
- TLS 1.3 (minimum TLS 1.2)
- Valid certificate (Let's Encrypt recommended)
- HSTS headers
- Secure cipher suites only

### Headers

All responses include:
```
Content-Security-Policy: default-src 'self'
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: geolocation=(), microphone=(), camera=()
```

### CORS

- Default: CORS disabled
- Per-cenv: Owners can configure allowed origins
- Stored in: `_wce_config` table

## Audit Logging

### What Gets Logged

Per-cenv audit table (`_wce_audit_log`):
- User authentication attempts (success/failure)
- Permission grants/revokes
- Table creation/deletion
- Bulk data operations (>100 rows)
- Starlark script modifications
- Configuration changes

### Log Format

```sql
CREATE TABLE _wce_audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER NOT NULL,
    user_id TEXT NOT NULL,
    username TEXT NOT NULL,  -- Denormalized for convenience
    action TEXT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    details TEXT,  -- JSON
    ip_address TEXT,
    user_agent TEXT,
    FOREIGN KEY (user_id) REFERENCES _wce_users(user_id)
);
```

## Backup and Recovery

### Cenv Backup

Each cenv is a single SQLite file:
- Owners can export entire database
- Point-in-time backups via WAL mode
- Platform can enforce backup policies

### Cenv Discovery

Without a platform database, how do users find their cenvs?

**Options:**

1. **Bookmark URLs**: Users bookmark their cenv URLs
2. **Cenv Registry Page**: Optional static HTML page listing known cenvs
3. **Subdomain Mapping**: DNS CNAME records for friendly names (myapp.domain.com → domain.com/{uuid}/)
4. **File-based Registry**: Optional `registry.json` served at root listing public cenvs

This keeps the system simple while allowing organizational flexibility.

## Secure Deployment Checklist

- [ ] Run behind reverse proxy (nginx/caddy) with TLS
- [ ] Set restrictive file permissions on cenv directory (700)
- [ ] Configure resource limits in deployment environment
- [ ] Enable audit logging
- [ ] Set up automated backups
- [ ] Configure rate limiting at reverse proxy
- [ ] Set strong JWT signing key (min 32 random bytes)
- [ ] Decide on new cenv provisioning policy (open/restricted)
- [ ] Set reasonable per-cenv quotas
- [ ] Monitor resource usage
- [ ] Keep Go and dependencies updated
- [ ] Review Starlark exposed APIs regularly

## Future Enhancements

1. **E2E Encryption**: Optional client-side encryption of cenv data
2. **Federated Auth per Cenv**: Each cenv can configure OpenID/SAML if desired
3. **Email Integration per Cenv**: Password reset via email (config in _wce_config)
4. **Audit Streaming**: Real-time audit events to external SIEM
5. **Anomaly Detection**: ML-based detection of suspicious patterns
6. **Compliance Modes**: Pre-configured settings for GDPR, HIPAA, etc.
7. **Magic Links**: Passwordless authentication via email tokens
8. **WebAuthn/Passkeys**: FIDO2 authentication stored in _wce_users

## Reporting Security Issues

Please report security vulnerabilities to: [security contact TBD]

Do not open public issues for security problems.

## References

- SQLite Security: https://www.sqlite.org/security.html
- Starlark Security: https://github.com/google/starlark-go/blob/master/doc/spec.md#security
- OWASP Top 10: https://owasp.org/www-project-top-ten/
