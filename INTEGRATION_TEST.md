# Integration Test Documentation

## Overview
The integration test (`internal/server/server_integration_test.go`) provides comprehensive end-to-end testing of the WCE authentication system by starting a real HTTP server and making actual HTTP requests.

## Test Structure

### TestServerIntegration
Main test function that:
1. Creates a temporary storage directory for isolated test data
2. Starts an HTTP server on a random available port
3. Runs multiple sub-tests that exercise different aspects of the system
4. Cleans up temporary data after completion

### Server Startup
```go
// Server starts on random port (127.0.0.1:0)
// Port is communicated back via channel
// Server runs in background goroutine
```

The test properly handles:
- Port allocation (random to avoid conflicts)
- Server readiness signaling
- Graceful shutdown after tests complete
- Error handling if server fails to start

## Test Cases

### 1. CreateNewCenv
Tests the cenv creation endpoint (`POST /new`).

**What it tests:**
- Creating a new cenv with username/password/email
- Response status is 201 Created
- Response includes cenv_id, cenv_url, username
- User is properly created in database

**Example:**
```json
POST /new
{
  "username": "testuser",
  "password": "testpass123",
  "email": "test@example.com"
}

Response: 201 Created
{
  "cenv_id": "uuid-here",
  "cenv_url": "http://localhost:port/uuid-here/",
  "username": "testuser",
  "message": "..."
}
```

### 2. LoginAndAuthentication
Tests the login flow and JWT token generation.

**What it tests:**
- Creating a cenv
- Logging in with correct credentials
- Response status is 200 OK
- Response includes valid JWT token
- Token includes user_id, username, role, expires_at
- First user in cenv has "owner" role

**Example:**
```json
POST /uuid/login
{
  "username": "loginuser",
  "password": "loginpass123"
}

Response: 200 OK
{
  "token": "eyJhbGc...",
  "expires_at": 1234567890,
  "user_id": "uuid",
  "username": "loginuser",
  "role": "owner"
}
```

### 3. AuthenticationErrors
Tests various error scenarios to ensure security.

#### LoginWithWrongPassword
- Attempt login with incorrect password
- Expects 401 Unauthorized
- Error message doesn't reveal if user exists

#### LoginWithNonexistentUser
- Attempt login with username that doesn't exist
- Expects 401 Unauthorized
- Error message same as wrong password (security)

#### AccessWithoutToken
- GET request without Authorization header
- Expects 401 Unauthorized
- Error: "missing authorization header"

#### AccessWithInvalidToken
- GET request with malformed/invalid token
- Expects 401 Unauthorized
- Error: "invalid token"

#### CreateCenvWithShortPassword
- POST /new with password < 8 characters
- Expects 400 Bad Request
- Error: "password must be at least 8 characters"

### 4. CompleteFlow
Tests the entire authentication flow from start to finish.

**Flow:**
1. **Create cenv** - POST /new with credentials
2. **Login** - POST /{cenv-id}/login to get JWT token
3. **Make authenticated request** - GET /{cenv-id}/dashboard with token
4. **Test multiple paths** - Verify token works on different routes:
   - `/` (root)
   - `/dashboard`
   - `/settings`
   - `/api/data`
5. **Test token isolation** - Create second cenv, verify first token doesn't work

**Key assertions:**
- All authenticated requests return 200 OK
- Response includes user info (cenv_id, username, role)
- Token is properly scoped to specific cenv
- Cross-cenv requests with wrong token return 401

## Helper Functions

### createTestCenv
```go
func createTestCenv(t *testing.T, baseURL, username, password string) string
```
Convenience function that creates a cenv and returns its ID. Used by multiple test cases to set up test data.

## Test Execution

### Run integration test only:
```bash
go test -v ./internal/server -run TestServerIntegration
```

### Run all tests:
```bash
go test ./... -v
```

### Run with coverage:
```bash
go test ./... -cover
```

## Test Results

All tests pass successfully:
```
=== RUN   TestServerIntegration
=== RUN   TestServerIntegration/CreateNewCenv
=== RUN   TestServerIntegration/LoginAndAuthentication
=== RUN   TestServerIntegration/AuthenticationErrors
=== RUN   TestServerIntegration/AuthenticationErrors/LoginWithWrongPassword
=== RUN   TestServerIntegration/AuthenticationErrors/LoginWithNonexistentUser
=== RUN   TestServerIntegration/AuthenticationErrors/AccessWithoutToken
=== RUN   TestServerIntegration/AuthenticationErrors/AccessWithInvalidToken
=== RUN   TestServerIntegration/AuthenticationErrors/CreateCenvWithShortPassword
=== RUN   TestServerIntegration/CompleteFlow
--- PASS: TestServerIntegration (1.96s)
    --- PASS: TestServerIntegration/CreateNewCenv (0.23s)
    --- PASS: TestServerIntegration/LoginAndAuthentication (0.43s)
    --- PASS: TestServerIntegration/AuthenticationErrors (0.46s)
    --- PASS: TestServerIntegration/CompleteFlow (0.74s)
PASS
ok  	github.com/thetanil/wce/internal/server	1.966s	coverage: 49.3%
```

## Coverage Impact

The integration test significantly improves test coverage:
- **Server package**: 49.3% coverage (was 0%)
- Tests all HTTP endpoints
- Tests middleware and error handling
- Tests authentication flow end-to-end

## Test Data Isolation

Each test run:
1. Creates a fresh temporary directory
2. All test databases are isolated
3. Temporary directory is cleaned up after test completes
4. No interference between test runs
5. No leftover test data

## Future Enhancements

Potential additions for future phases:
- Test session revocation
- Test token expiration
- Test user management (add/remove users)
- Test permission system (once implemented)
- Test concurrent requests
- Test rate limiting (once implemented)
- Test database size limits (once implemented)

## Notes

- Server starts on random port to avoid conflicts with running instances
- Test uses real HTTP requests (not mocked)
- Test uses real SQLite databases (in-memory not suitable for server testing)
- All bcrypt operations run with real cost factor (slower but realistic)
- Test demonstrates proper HTTP client usage with Authorization headers
