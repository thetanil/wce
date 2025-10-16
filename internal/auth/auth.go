package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// BcryptCost is the cost factor for bcrypt password hashing
	BcryptCost = 12

	// DefaultSessionTimeout is the default session timeout duration
	DefaultSessionTimeout = 24 * time.Hour

	// Role constants
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// User represents a user in the system
type User struct {
	UserID       string
	Username     string
	PasswordHash string
	Role         string
	Email        string
	CreatedAt    int64
	InvitedBy    string
	LastLogin    int64
	Enabled      bool
}

// Session represents an active session
type Session struct {
	SessionID string
	UserID    string
	TokenHash string
	CreatedAt int64
	ExpiresAt int64
	LastUsed  int64
	IPAddress string
	UserAgent string
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword verifies a password against a hash
func VerifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// GenerateUUID generates a random UUID (v4)
func GenerateUUID() (string, error) {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		return "", fmt.Errorf("failed to generate UUID: %w", err)
	}

	// Set version (4) and variant bits
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 10

	// Format as standard UUID string
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

// GenerateSessionID generates a random session ID
func GenerateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// CreateUser creates a new user in the database
func CreateUser(db *sql.DB, username, password, role, email, invitedBy string) (*User, error) {
	// Generate user ID
	userID, err := GenerateUUID()
	if err != nil {
		return nil, err
	}

	// Hash password
	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	// Insert user
	createdAt := time.Now().Unix()
	query := `
		INSERT INTO _wce_users (user_id, username, password_hash, role, email, created_at, invited_by, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)
	`

	var invitedByParam interface{}
	if invitedBy == "" {
		invitedByParam = nil
	} else {
		invitedByParam = invitedBy
	}

	var emailParam interface{}
	if email == "" {
		emailParam = nil
	} else {
		emailParam = email
	}

	_, err = db.Exec(query, userID, username, passwordHash, role, emailParam, createdAt, invitedByParam)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &User{
		UserID:       userID,
		Username:     username,
		PasswordHash: passwordHash,
		Role:         role,
		Email:        email,
		CreatedAt:    createdAt,
		InvitedBy:    invitedBy,
		Enabled:      true,
	}, nil
}

// GetUserByUsername retrieves a user by username
func GetUserByUsername(db *sql.DB, username string) (*User, error) {
	query := `
		SELECT user_id, username, password_hash, role, email, created_at, invited_by, last_login, enabled
		FROM _wce_users
		WHERE username = ?
	`

	var user User
	var email, invitedBy sql.NullString
	var lastLogin sql.NullInt64

	err := db.QueryRow(query, username).Scan(
		&user.UserID,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&email,
		&user.CreatedAt,
		&invitedBy,
		&lastLogin,
		&user.Enabled,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if email.Valid {
		user.Email = email.String
	}
	if invitedBy.Valid {
		user.InvitedBy = invitedBy.String
	}
	if lastLogin.Valid {
		user.LastLogin = lastLogin.Int64
	}

	return &user, nil
}

// UpdateLastLogin updates the last login timestamp for a user
func UpdateLastLogin(db *sql.DB, userID string) error {
	query := `UPDATE _wce_users SET last_login = ? WHERE user_id = ?`
	_, err := db.Exec(query, time.Now().Unix(), userID)
	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}
	return nil
}

// CreateSession creates a new session in the database
func CreateSession(db *sql.DB, userID, tokenHash, ipAddress, userAgent string, expiresIn time.Duration) (*Session, error) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	createdAt := now.Unix()
	expiresAt := now.Add(expiresIn).Unix()

	query := `
		INSERT INTO _wce_sessions (session_id, user_id, token_hash, created_at, expires_at, last_used, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = db.Exec(query, sessionID, userID, tokenHash, createdAt, expiresAt, createdAt, ipAddress, userAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &Session{
		SessionID: sessionID,
		UserID:    userID,
		TokenHash: tokenHash,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
		LastUsed:  createdAt,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}, nil
}

// IsSessionValid checks if a session is valid (not revoked and not expired)
func IsSessionValid(db *sql.DB, tokenHash string) (bool, error) {
	query := `
		SELECT COUNT(*) FROM _wce_sessions
		WHERE token_hash = ? AND expires_at > ?
	`

	var count int
	err := db.QueryRow(query, tokenHash, time.Now().Unix()).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check session: %w", err)
	}

	return count > 0, nil
}

// RevokeSession revokes a session by deleting it
func RevokeSession(db *sql.DB, tokenHash string) error {
	query := `DELETE FROM _wce_sessions WHERE token_hash = ?`
	_, err := db.Exec(query, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}
	return nil
}

// RevokeAllUserSessions revokes all sessions for a user
func RevokeAllUserSessions(db *sql.DB, userID string) error {
	query := `DELETE FROM _wce_sessions WHERE user_id = ?`
	_, err := db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke user sessions: %w", err)
	}
	return nil
}

// CleanupExpiredSessions removes expired sessions from the database
func CleanupExpiredSessions(db *sql.DB) error {
	query := `DELETE FROM _wce_sessions WHERE expires_at <= ?`
	_, err := db.Exec(query, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}
	return nil
}
