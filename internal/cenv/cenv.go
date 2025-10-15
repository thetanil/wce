package cenv

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// UUID regex pattern (RFC 4122 compliant)
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Manager handles cenv operations
type Manager struct {
	storageDir string
}

// NewManager creates a new cenv manager
func NewManager(storageDir string) *Manager {
	return &Manager{
		storageDir: storageDir,
	}
}

// IsValidUUID checks if a string is a valid UUID
func IsValidUUID(s string) bool {
	return uuidRegex.MatchString(strings.ToLower(s))
}

// ParsePath extracts the cenv ID from a request path
// Returns the cenv ID and the remaining path
// Example: "/abc-123/page/home" -> ("abc-123", "/page/home")
func ParsePath(path string) (cenvID string, remainingPath string, ok bool) {
	// Remove leading slash
	path = strings.TrimPrefix(path, "/")

	// Split into parts
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 {
		return "", "", false
	}

	cenvID = parts[0]

	// Check if it's a valid UUID
	if !IsValidUUID(cenvID) {
		return "", "", false
	}

	// Get remaining path
	if len(parts) == 2 {
		remainingPath = "/" + parts[1]
	} else {
		remainingPath = "/"
	}

	return cenvID, remainingPath, true
}

// GetDatabasePath returns the filesystem path for a cenv's database
func (m *Manager) GetDatabasePath(cenvID string) string {
	return fmt.Sprintf("%s/%s.db", m.storageDir, cenvID)
}

// Exists checks if a cenv database file exists
func (m *Manager) Exists(cenvID string) bool {
	dbPath := m.GetDatabasePath(cenvID)
	_, err := os.Stat(dbPath)
	return err == nil
}

// Create creates a new cenv database file with proper permissions
// This is a minimal implementation for Phase 1.4
// Schema initialization will be added in Phase 2
func (m *Manager) Create(cenvID string) error {
	dbPath := m.GetDatabasePath(cenvID)

	// Check if database already exists
	if m.Exists(cenvID) {
		return fmt.Errorf("cenv %s already exists", cenvID)
	}

	// Create the database file
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	defer db.Close()

	// Verify we can connect
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Set file permissions to 600 (owner read/write only)
	if err := os.Chmod(dbPath, 0600); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	return nil
}

// Open opens a connection to a cenv database
// Returns an error if the database doesn't exist
func (m *Manager) Open(cenvID string) (*sql.DB, error) {
	if !m.Exists(cenvID) {
		return nil, fmt.Errorf("cenv %s does not exist", cenvID)
	}

	dbPath := m.GetDatabasePath(cenvID)

	// Open database connection
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure SQLite pragmas
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	return db, nil
}
