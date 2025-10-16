# Phase 3 Implementation Summary: Authentication Core

## Overview
Phase 3 implements the complete authentication system for WCE, including user registration, login, JWT token generation, and request authentication.

## Components Implemented

### 1. JWT Token Management (`internal/auth/jwt.go`)
- **JWTManager**: Handles JWT token generation and validation
- **Claims**: JWT payload structure containing user info, cenv ID, role, and session ID
- **Token Generation**: Creates signed JWT tokens with HMAC-SHA256
- **Token Validation**: Verifies signature, checks expiration, and extracts claims
- **Token Hashing**: SHA256 hashing for secure token storage and revocation

### 2. Authentication Core (`internal/auth/auth.go`)
- **Password Hashing**: Uses bcrypt with cost factor 12
- **Password Verification**: Secure password comparison
- **UUID Generation**: RFC 4122 compliant UUID v4 generation
- **Session ID Generation**: Cryptographically secure random session identifiers
- **User Management**:
  - `CreateUser()`: Creates new user with hashed password
  - `GetUserByUsername()`: Retrieves user by username
  - `UpdateLastLogin()`: Updates last login timestamp
- **Session Management**:
  - `CreateSession()`: Creates session with token hash
  - `IsSessionValid()`: Checks if session exists and hasn't expired
  - `RevokeSession()`: Revokes a single session
  - `RevokeAllUserSessions()`: Revokes all sessions for a user
  - `CleanupExpiredSessions()`: Removes expired sessions

### 3. Authentication Middleware (`internal/auth/middleware.go`)
- **AuthMiddleware**: Validates JWT tokens on requests
- **RequireRole**: Checks user has required role
- **Context Management**: Stores claims in request context for handlers

### 4. HTTP Endpoints (`internal/server/server.go`)

#### POST /new
Creates a new cenv with owner user:
- Request: `{"username": "admin", "password": "pass123", "email": "optional@email.com"}`
- Validates username and password (min 8 chars)
- Generates UUID for cenv
- Creates SQLite database with schema
- Creates owner user with bcrypt hash
- Response: `{"cenv_id": "...", "cenv_url": "...", "username": "...", "message": "..."}`

#### POST /{cenv-id}/login
Authenticates user and returns JWT token:
- Request: `{"username": "admin", "password": "pass123"}`
- Validates cenv exists
- Verifies username and password
- Checks if user is enabled
- Generates JWT token (24 hour expiry)
- Creates session record with token hash
- Updates last login timestamp
- Response: `{"token": "...", "expires_at": 1234567890, "user_id": "...", "username": "...", "role": "..."}`

#### GET/POST /{cenv-id}/*
Authenticated requests to cenv:
- Requires `Authorization: Bearer <token>` header
- Validates JWT token signature and expiration
- Verifies token is for the correct cenv
- Checks session hasn't been revoked
- Returns user claims on success
- Returns 401 on authentication failure

## Security Features

### Password Security
- Bcrypt hashing with cost factor 12
- Passwords stored as hashes, never in plaintext
- Constant-time password comparison

### Token Security
- JWT tokens signed with HMAC-SHA256
- Tokens scoped to specific cenv (can't be used across cenvs)
- Tokens include expiration time (24 hours default)
- Token hashes stored for revocation capability

### Session Security
- Sessions tracked in database
- Session validation on every request
- Ability to revoke individual sessions
- Ability to revoke all user sessions
- Expired sessions can be cleaned up

### Database Security
- File permissions set to 600 (owner only)
- All database queries use parameterized statements
- SQLite WAL mode with proper pragmas

## Test Coverage

### JWT Tests (`internal/auth/jwt_test.go`)
- Token generation
- Token validation
- Invalid token handling
- Wrong secret detection
- Expired token detection
- Token hash consistency

### Auth Tests (`internal/auth/auth_test.go`)
- Password hashing and verification
- UUID generation
- Session ID generation
- User creation and retrieval
- Duplicate username prevention
- Session creation and validation
- Session revocation
- Expired session cleanup

### Test Results
```
ok  	github.com/thetanil/wce/internal/auth	3.249s	coverage: 64.0% of statements
ok  	github.com/thetanil/wce/internal/cenv	0.056s	coverage: 76.7% of statements
```

## End-to-End Testing

Successfully tested complete authentication flow:

1. **Create Cenv**: POST /new with username/password → 201 Created
2. **Login**: POST /{cenv-id}/login → 200 OK with JWT token
3. **Authenticated Request**: GET /{cenv-id}/dashboard with token → 200 OK
4. **No Token**: GET /{cenv-id}/dashboard without token → 401 Unauthorized
5. **Invalid Token**: GET /{cenv-id}/dashboard with bad token → 401 Unauthorized
6. **Wrong Cenv**: Token from cenv A used on cenv B → 401 Unauthorized (or 404 if cenv doesn't exist)

## Database Schema Used

Phase 3 uses these tables from Phase 2:
- `_wce_users`: User authentication and management
- `_wce_sessions`: Session/token management and revocation

## Dependencies Added

- `golang.org/x/crypto/bcrypt` - Password hashing (allowed per CLAUDE.md)

## Commit Message Format

```
[Phase 3] Implement authentication core with JWT

- Add JWT token generation and validation
- Add user registration endpoint (POST /new)
- Add login endpoint (POST /{cenv-id}/login)
- Add JWT authentication middleware
- Add comprehensive auth tests
- Add password hashing with bcrypt
- Add session management and revocation
- Add end-to-end authentication flow
```

## Next Steps

Phase 4 will implement:
- Authorization & permissions (table-level and row-level)
- Permission checking middleware
- Query rewriting for row-level security
- Permission management endpoints

## Notes

- JWT secret is randomly generated on server start. In production, this should be loaded from environment variables or config.
- Session timeout defaults to 24 hours but can be configured per-cenv via `_wce_config` table.
- All authentication endpoints use JSON for requests and responses.
- Error messages are intentionally vague for security (don't reveal if user exists).
