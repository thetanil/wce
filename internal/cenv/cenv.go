package cenv

import (
	"fmt"
	"regexp"
	"strings"
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
