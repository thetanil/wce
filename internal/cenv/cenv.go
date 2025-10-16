package cenv

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/thetanil/wce/internal/db"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// UUID regex pattern (RFC 4122 compliant)
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Manager handles cenv operations
type Manager struct {
	storageDir  string
	connections sync.Map // map[string]*sql.DB - cenvID -> connection pool
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

// Create creates a new cenv database file with proper permissions and initializes schema
func (m *Manager) Create(cenvID string) error {
	dbPath := m.GetDatabasePath(cenvID)

	// Check if database already exists
	if m.Exists(cenvID) {
		return fmt.Errorf("cenv %s already exists", cenvID)
	}

	// Create the database file
	connection, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	defer connection.Close()

	// Verify we can connect
	if err := connection.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Set file permissions to 600 (owner read/write only)
	if err := os.Chmod(dbPath, 0600); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Initialize schema
	if err := m.Initialize(cenvID); err != nil {
		// Clean up database file if initialization fails
		os.Remove(dbPath)
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	return nil
}

// Initialize creates the WCE system tables in a cenv database
func (m *Manager) Initialize(cenvID string) error {
	if !m.Exists(cenvID) {
		return fmt.Errorf("cenv %s does not exist", cenvID)
	}

	// Open a temporary connection for initialization
	connection, err := m.Open(cenvID)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer connection.Close()

	// Check if already initialized
	var count int
	err = connection.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='_wce_users'").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check initialization status: %w", err)
	}

	if count > 0 {
		// Already initialized
		return nil
	}

	// Execute schema in a transaction (all or nothing)
	tx, err := connection.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Will be no-op if committed

	// Execute the schema
	if _, err := tx.Exec(db.Schema); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit schema: %w", err)
	}

	return nil
}

// Open opens a connection to a cenv database
// Returns an error if the database doesn't exist
// Note: This creates a new connection. For pooled connections, use GetConnection()
func (m *Manager) Open(cenvID string) (*sql.DB, error) {
	if !m.Exists(cenvID) {
		return nil, fmt.Errorf("cenv %s does not exist", cenvID)
	}

	dbPath := m.GetDatabasePath(cenvID)

	// Open database connection
	connection, err := sql.Open("sqlite3", dbPath)
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
		if _, err := connection.Exec(pragma); err != nil {
			connection.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	return connection, nil
}

// GetConnection returns a pooled connection to a cenv database
// Connections are cached and reused. Lazy-loaded on first access.
func (m *Manager) GetConnection(cenvID string) (*sql.DB, error) {
	// Try to load existing connection from pool
	if conn, ok := m.connections.Load(cenvID); ok {
		return conn.(*sql.DB), nil
	}

	// Create new connection if not exists
	connection, err := m.Open(cenvID)
	if err != nil {
		return nil, err
	}

	// Configure connection pool settings
	connection.SetMaxOpenConns(5)                   // Max 5 concurrent connections
	connection.SetMaxIdleConns(2)                   // Keep 2 idle connections ready
	connection.SetConnMaxLifetime(30 * time.Minute) // Recycle connections after 30 min
	connection.SetConnMaxIdleTime(10 * time.Minute) // Close idle connections after 10 min

	// Store in pool
	m.connections.Store(cenvID, connection)

	return connection, nil
}

// CloseConnection closes the pooled connection for a cenv
func (m *Manager) CloseConnection(cenvID string) error {
	if conn, ok := m.connections.LoadAndDelete(cenvID); ok {
		return conn.(*sql.DB).Close()
	}
	return nil
}

// CloseAll closes all pooled connections
func (m *Manager) CloseAll() error {
	var lastErr error
	m.connections.Range(func(key, value interface{}) bool {
		if err := value.(*sql.DB).Close(); err != nil {
			lastErr = err
		}
		m.connections.Delete(key)
		return true
	})
	return lastErr
}
