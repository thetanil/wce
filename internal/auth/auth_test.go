package auth

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create the _wce_users table
	_, err = db.Exec(`
		CREATE TABLE _wce_users (
			user_id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			email TEXT,
			created_at INTEGER NOT NULL,
			invited_by TEXT,
			last_login INTEGER,
			enabled INTEGER DEFAULT 1
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create _wce_users table: %v", err)
	}

	// Create the _wce_sessions table
	_, err = db.Exec(`
		CREATE TABLE _wce_sessions (
			session_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			last_used INTEGER,
			ip_address TEXT,
			user_agent TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create _wce_sessions table: %v", err)
	}

	return db
}

func TestHashPassword(t *testing.T) {
	password := "test-password-123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	if hash == "" {
		t.Error("Hash should not be empty")
	}

	if hash == password {
		t.Error("Hash should not equal plain password")
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "test-password-123"
	wrongPassword := "wrong-password"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// Test correct password
	err = VerifyPassword(password, hash)
	if err != nil {
		t.Errorf("Failed to verify correct password: %v", err)
	}

	// Test wrong password
	err = VerifyPassword(wrongPassword, hash)
	if err == nil {
		t.Error("Expected error for wrong password, got nil")
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid, err := GenerateUUID()
	if err != nil {
		t.Fatalf("Failed to generate UUID: %v", err)
	}

	// Check format (36 characters with dashes)
	if len(uuid) != 36 {
		t.Errorf("Expected UUID length 36, got %d", len(uuid))
	}

	// Generate another and ensure they're different
	uuid2, err := GenerateUUID()
	if err != nil {
		t.Fatalf("Failed to generate second UUID: %v", err)
	}

	if uuid == uuid2 {
		t.Error("Two generated UUIDs should be different")
	}
}

func TestGenerateSessionID(t *testing.T) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("Failed to generate session ID: %v", err)
	}

	if sessionID == "" {
		t.Error("Session ID should not be empty")
	}

	// Generate another and ensure they're different
	sessionID2, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("Failed to generate second session ID: %v", err)
	}

	if sessionID == sessionID2 {
		t.Error("Two generated session IDs should be different")
	}
}

func TestCreateUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	username := "testuser"
	password := "test-password-123"
	role := RoleAdmin
	email := "test@example.com"

	user, err := CreateUser(db, username, password, role, email, "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user.UserID == "" {
		t.Error("User ID should not be empty")
	}

	if user.Username != username {
		t.Errorf("Expected username %s, got %s", username, user.Username)
	}

	if user.Role != role {
		t.Errorf("Expected role %s, got %s", role, user.Role)
	}

	if user.Email != email {
		t.Errorf("Expected email %s, got %s", email, user.Email)
	}

	if !user.Enabled {
		t.Error("User should be enabled by default")
	}

	// Verify password hash was created
	if user.PasswordHash == "" {
		t.Error("Password hash should not be empty")
	}

	if user.PasswordHash == password {
		t.Error("Password hash should not equal plain password")
	}
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	username := "testuser"
	password := "test-password-123"

	// Create first user
	_, err := CreateUser(db, username, password, RoleAdmin, "", "")
	if err != nil {
		t.Fatalf("Failed to create first user: %v", err)
	}

	// Try to create second user with same username
	_, err = CreateUser(db, username, password, RoleAdmin, "", "")
	if err == nil {
		t.Error("Expected error when creating user with duplicate username")
	}
}

func TestGetUserByUsername(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	username := "testuser"
	password := "test-password-123"
	role := RoleEditor

	// Create user
	createdUser, err := CreateUser(db, username, password, role, "", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Get user by username
	user, err := GetUserByUsername(db, username)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if user.UserID != createdUser.UserID {
		t.Errorf("Expected user ID %s, got %s", createdUser.UserID, user.UserID)
	}

	if user.Username != username {
		t.Errorf("Expected username %s, got %s", username, user.Username)
	}

	if user.Role != role {
		t.Errorf("Expected role %s, got %s", role, user.Role)
	}
}

func TestGetUserByUsername_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := GetUserByUsername(db, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent user")
	}
}

func TestUpdateLastLogin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create user
	user, err := CreateUser(db, "testuser", "password123", RoleAdmin, "", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Update last login
	err = UpdateLastLogin(db, user.UserID)
	if err != nil {
		t.Fatalf("Failed to update last login: %v", err)
	}

	// Get user and verify last login was updated
	updatedUser, err := GetUserByUsername(db, user.Username)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if updatedUser.LastLogin == 0 {
		t.Error("Last login should be set")
	}
}

func TestCreateSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create user first
	user, err := CreateUser(db, "testuser", "password123", RoleAdmin, "", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	tokenHash := "test-token-hash"
	ipAddress := "127.0.0.1"
	userAgent := "test-agent"
	expiresIn := 1 * time.Hour

	session, err := CreateSession(db, user.UserID, tokenHash, ipAddress, userAgent, expiresIn)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if session.SessionID == "" {
		t.Error("Session ID should not be empty")
	}

	if session.UserID != user.UserID {
		t.Errorf("Expected user ID %s, got %s", user.UserID, session.UserID)
	}

	if session.TokenHash != tokenHash {
		t.Errorf("Expected token hash %s, got %s", tokenHash, session.TokenHash)
	}

	if session.IPAddress != ipAddress {
		t.Errorf("Expected IP address %s, got %s", ipAddress, session.IPAddress)
	}

	if session.UserAgent != userAgent {
		t.Errorf("Expected user agent %s, got %s", userAgent, session.UserAgent)
	}
}

func TestIsSessionValid(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create user first
	user, err := CreateUser(db, "testuser", "password123", RoleAdmin, "", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	tokenHash := "test-token-hash"
	expiresIn := 1 * time.Hour

	// Create session
	_, err = CreateSession(db, user.UserID, tokenHash, "127.0.0.1", "test-agent", expiresIn)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Check if valid
	valid, err := IsSessionValid(db, tokenHash)
	if err != nil {
		t.Fatalf("Failed to check session validity: %v", err)
	}

	if !valid {
		t.Error("Session should be valid")
	}

	// Check non-existent session
	valid, err = IsSessionValid(db, "non-existent-hash")
	if err != nil {
		t.Fatalf("Failed to check session validity: %v", err)
	}

	if valid {
		t.Error("Non-existent session should not be valid")
	}
}

func TestIsSessionValid_Expired(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create user first
	user, err := CreateUser(db, "testuser", "password123", RoleAdmin, "", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	tokenHash := "test-token-hash"
	expiresIn := -1 * time.Hour // Already expired

	// Create expired session
	_, err = CreateSession(db, user.UserID, tokenHash, "127.0.0.1", "test-agent", expiresIn)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Check if valid
	valid, err := IsSessionValid(db, tokenHash)
	if err != nil {
		t.Fatalf("Failed to check session validity: %v", err)
	}

	if valid {
		t.Error("Expired session should not be valid")
	}
}

func TestRevokeSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create user first
	user, err := CreateUser(db, "testuser", "password123", RoleAdmin, "", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	tokenHash := "test-token-hash"

	// Create session
	_, err = CreateSession(db, user.UserID, tokenHash, "127.0.0.1", "test-agent", 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Revoke session
	err = RevokeSession(db, tokenHash)
	if err != nil {
		t.Fatalf("Failed to revoke session: %v", err)
	}

	// Check if still valid
	valid, err := IsSessionValid(db, tokenHash)
	if err != nil {
		t.Fatalf("Failed to check session validity: %v", err)
	}

	if valid {
		t.Error("Revoked session should not be valid")
	}
}

func TestRevokeAllUserSessions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create user
	user, err := CreateUser(db, "testuser", "password123", RoleAdmin, "", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create multiple sessions
	tokenHash1 := "test-token-hash-1"
	tokenHash2 := "test-token-hash-2"

	_, err = CreateSession(db, user.UserID, tokenHash1, "127.0.0.1", "test-agent", 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	_, err = CreateSession(db, user.UserID, tokenHash2, "127.0.0.1", "test-agent", 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	// Revoke all user sessions
	err = RevokeAllUserSessions(db, user.UserID)
	if err != nil {
		t.Fatalf("Failed to revoke all sessions: %v", err)
	}

	// Check if both sessions are invalid
	valid1, _ := IsSessionValid(db, tokenHash1)
	valid2, _ := IsSessionValid(db, tokenHash2)

	if valid1 || valid2 {
		t.Error("All user sessions should be revoked")
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create user
	user, err := CreateUser(db, "testuser", "password123", RoleAdmin, "", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create expired session
	expiredTokenHash := "expired-token-hash"
	_, err = CreateSession(db, user.UserID, expiredTokenHash, "127.0.0.1", "test-agent", -1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create expired session: %v", err)
	}

	// Create valid session
	validTokenHash := "valid-token-hash"
	_, err = CreateSession(db, user.UserID, validTokenHash, "127.0.0.1", "test-agent", 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create valid session: %v", err)
	}

	// Cleanup expired sessions
	err = CleanupExpiredSessions(db)
	if err != nil {
		t.Fatalf("Failed to cleanup expired sessions: %v", err)
	}

	// Check that expired session is gone
	valid, _ := IsSessionValid(db, expiredTokenHash)
	if valid {
		t.Error("Expired session should be cleaned up")
	}

	// Check that valid session still exists
	valid, _ = IsSessionValid(db, validTokenHash)
	if !valid {
		t.Error("Valid session should not be cleaned up")
	}
}
