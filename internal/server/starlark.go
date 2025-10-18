package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/thetanil/wce/internal/auth"
	starlark_pkg "github.com/thetanil/wce/internal/starlark"
)

// Endpoint represents a Starlark endpoint
type Endpoint struct {
	ID          int64  `json:"id"`
	Path        string `json:"path"`
	Method      string `json:"method"`
	Script      string `json:"script"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   int64  `json:"created_at"`
	ModifiedAt  int64  `json:"modified_at"`
	CreatedBy   string `json:"created_by"`
	ModifiedBy  string `json:"modified_by"`
}

// handleExecuteStarlarkEndpoint executes a Starlark endpoint
func (s *Server) handleExecuteStarlarkEndpoint(w http.ResponseWriter, r *http.Request) {
	cenvID := r.PathValue("cenvID")

	// Validate cenv exists
	if !s.cenvManager.Exists(cenvID) {
		http.Error(w, "Cenv not found", http.StatusNotFound)
		return
	}

	// Get database connection
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Extract star path (everything after /star/)
	starPath := r.PathValue("starPath")
	if starPath == "" {
		starPath = "/"
	} else if starPath[0] != '/' {
		// Add leading slash if not present
		starPath = "/" + starPath
	}

	// Find matching endpoint
	endpoint, userID, err := s.findAndAuthenticateEndpoint(db, r, starPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if endpoint == nil {
		http.Error(w, "Endpoint not found", http.StatusNotFound)
		return
	}

	if !endpoint.Enabled {
		http.Error(w, "Endpoint disabled", http.StatusForbidden)
		return
	}

	// Execute the Starlark script
	execCtx := &starlark_pkg.ExecutionContext{
		DB:      db,
		UserID:  userID,
		Request: r,
		Timeout: 5 * time.Second,
	}

	result, err := starlark_pkg.Execute(r.Context(), endpoint.Script, execCtx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Script execution error: %v", err), http.StatusInternalServerError)
		return
	}

	// Set headers
	for key, value := range result.Headers {
		w.Header().Set(key, value)
	}

	// Set default Content-Type if not specified
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}

	// Write status code
	w.WriteHeader(result.StatusCode)

	// Write body
	if result.Body != nil {
		// If body is already a string, write it directly
		if bodyStr, ok := result.Body.(string); ok {
			w.Write([]byte(bodyStr))
		} else {
			// Otherwise, encode as JSON
			json.NewEncoder(w).Encode(result.Body)
		}
	}
}

// findAndAuthenticateEndpoint finds a matching endpoint and authenticates the request
func (s *Server) findAndAuthenticateEndpoint(db *sql.DB, r *http.Request, path string) (*Endpoint, string, error) {
	// Try to authenticate the request
	token := r.Header.Get("Authorization")
	var userID string

	if token != "" {
		// Remove "Bearer " prefix
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}

		// Validate token
		claims, err := s.jwtManager.ValidateToken(token)
		if err == nil {
			// Verify session is valid
			valid, _ := auth.IsSessionValid(db, auth.GetTokenHash(token))
			if valid {
				userID = claims.UserID
			}
		}
	}

	// If authentication failed, userID will be empty (anonymous access)
	// Some endpoints might allow anonymous access

	// Find matching endpoint
	// First try exact match
	var endpoint Endpoint
	err := db.QueryRow(`
		SELECT id, path, method, script, description, enabled, created_at, modified_at, created_by, modified_by
		FROM _wce_endpoints
		WHERE path = ? AND (method = ? OR method = '*') AND enabled = 1
		ORDER BY method DESC
		LIMIT 1
	`, path, r.Method).Scan(
		&endpoint.ID,
		&endpoint.Path,
		&endpoint.Method,
		&endpoint.Script,
		&endpoint.Description,
		&endpoint.Enabled,
		&endpoint.CreatedAt,
		&endpoint.ModifiedAt,
		&endpoint.CreatedBy,
		&endpoint.ModifiedBy,
	)

	if err == sql.ErrNoRows {
		return nil, userID, nil
	}

	if err != nil {
		return nil, "", fmt.Errorf("database error: %w", err)
	}

	return &endpoint, userID, nil
}

// handleListEndpoints lists all Starlark endpoints
func (s *Server) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	cenvID := r.PathValue("cenvID")

	// Validate cenv exists
	if !s.cenvManager.Exists(cenvID) {
		http.Error(w, "Cenv not found", http.StatusNotFound)
		return
	}

	// Authenticate request
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "Authorization required", http.StatusUnauthorized)
		return
	}

	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	claims, err := s.jwtManager.ValidateToken(token)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	if claims.CenvID != cenvID {
		http.Error(w, "Invalid token for this cenv", http.StatusForbidden)
		return
	}

	// Get database connection
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Check if session is valid
	valid, err := auth.IsSessionValid(db, auth.GetTokenHash(token))
	if err != nil || !valid {
		http.Error(w, "Session expired", http.StatusUnauthorized)
		return
	}

	// List endpoints
	rows, err := db.Query(`
		SELECT id, path, method, description, enabled, created_at, modified_at, created_by, modified_by
		FROM _wce_endpoints
		ORDER BY path, method
	`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	endpoints := []Endpoint{}
	for rows.Next() {
		var ep Endpoint
		err := rows.Scan(
			&ep.ID,
			&ep.Path,
			&ep.Method,
			&ep.Description,
			&ep.Enabled,
			&ep.CreatedAt,
			&ep.ModifiedAt,
			&ep.CreatedBy,
			&ep.ModifiedBy,
		)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		// Don't include script content in list
		endpoints = append(endpoints, ep)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(endpoints)
}

// handleGetEndpoint gets a specific endpoint
func (s *Server) handleGetEndpoint(w http.ResponseWriter, r *http.Request) {
	cenvID := r.PathValue("cenvID")
	endpointID := r.PathValue("endpointID")

	// Validate cenv exists
	if !s.cenvManager.Exists(cenvID) {
		http.Error(w, "Cenv not found", http.StatusNotFound)
		return
	}

	// Authenticate request
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "Authorization required", http.StatusUnauthorized)
		return
	}

	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	claims, err := s.jwtManager.ValidateToken(token)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	if claims.CenvID != cenvID {
		http.Error(w, "Invalid token for this cenv", http.StatusForbidden)
		return
	}

	// Get database connection
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Check if session is valid
	valid, err := auth.IsSessionValid(db, auth.GetTokenHash(token))
	if err != nil || !valid {
		http.Error(w, "Session expired", http.StatusUnauthorized)
		return
	}

	// Get endpoint
	var ep Endpoint
	err = db.QueryRow(`
		SELECT id, path, method, script, description, enabled, created_at, modified_at, created_by, modified_by
		FROM _wce_endpoints
		WHERE id = ?
	`, endpointID).Scan(
		&ep.ID,
		&ep.Path,
		&ep.Method,
		&ep.Script,
		&ep.Description,
		&ep.Enabled,
		&ep.CreatedAt,
		&ep.ModifiedAt,
		&ep.CreatedBy,
		&ep.ModifiedBy,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Endpoint not found", http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ep)
}

// handleCreateEndpoint creates or updates a Starlark endpoint
func (s *Server) handleCreateEndpoint(w http.ResponseWriter, r *http.Request) {
	cenvID := r.PathValue("cenvID")

	// Validate cenv exists
	if !s.cenvManager.Exists(cenvID) {
		http.Error(w, "Cenv not found", http.StatusNotFound)
		return
	}

	// Authenticate request
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "Authorization required", http.StatusUnauthorized)
		return
	}

	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	claims, err := s.jwtManager.ValidateToken(token)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	if claims.CenvID != cenvID {
		http.Error(w, "Invalid token for this cenv", http.StatusForbidden)
		return
	}

	userID := claims.UserID
	role := claims.Role

	// Get database connection
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Check if session is valid
	valid, err := auth.IsSessionValid(db, auth.GetTokenHash(token))
	if err != nil || !valid {
		http.Error(w, "Session expired", http.StatusUnauthorized)
		return
	}

	// Only admin and owner can create endpoints
	if role != "admin" && role != "owner" {
		http.Error(w, "Only admin or owner can create endpoints", http.StatusForbidden)
		return
	}

	// Parse request body
	var req struct {
		Path        string `json:"path"`
		Method      string `json:"method"`
		Script      string `json:"script"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"` // pointer to distinguish between false and not provided
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Path == "" || req.Method == "" || req.Script == "" {
		http.Error(w, "path, method, and script are required", http.StatusBadRequest)
		return
	}

	// Default enabled to true if not specified
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Insert or update endpoint
	now := time.Now().Unix()
	_, err = db.Exec(`
		INSERT INTO _wce_endpoints (path, method, script, description, enabled, created_at, modified_at, created_by, modified_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path, method) DO UPDATE SET
			script = excluded.script,
			description = excluded.description,
			enabled = excluded.enabled,
			modified_at = excluded.modified_at,
			modified_by = excluded.modified_by
	`, req.Path, req.Method, req.Script, req.Description, enabled, now, now, userID, userID)

	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Endpoint created/updated successfully",
		"path":    req.Path,
		"method":  req.Method,
	})
}

// handleDeleteEndpoint deletes a Starlark endpoint
func (s *Server) handleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	cenvID := r.PathValue("cenvID")
	endpointID := r.PathValue("endpointID")

	// Validate cenv exists
	if !s.cenvManager.Exists(cenvID) {
		http.Error(w, "Cenv not found", http.StatusNotFound)
		return
	}

	// Authenticate and authorize (admin/owner only)
	db, role, err := s.authenticateAndAuthorize(r, cenvID, []string{"admin", "owner"})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if role == "" {
		http.Error(w, "Only admin or owner can delete endpoints", http.StatusForbidden)
		return
	}

	// Delete endpoint
	result, err := db.Exec(`DELETE FROM _wce_endpoints WHERE id = ?`, endpointID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Endpoint not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Endpoint deleted successfully",
	})
}

// authenticateAndAuthorize is a helper to authenticate and check role
func (s *Server) authenticateAndAuthorize(r *http.Request, cenvID string, allowedRoles []string) (*sql.DB, string, error) {
	token := r.Header.Get("Authorization")
	if token == "" {
		return nil, "", fmt.Errorf("authorization required")
	}

	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	claims, err := s.jwtManager.ValidateToken(token)
	if err != nil {
		return nil, "", fmt.Errorf("invalid token")
	}

	if claims.CenvID != cenvID {
		return nil, "", fmt.Errorf("invalid token for this cenv")
	}

	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		return nil, "", fmt.Errorf("database error")
	}

	valid, err := auth.IsSessionValid(db, auth.GetTokenHash(token))
	if err != nil || !valid {
		return nil, "", fmt.Errorf("session expired")
	}

	role := claims.Role

	// Check if role is allowed
	if len(allowedRoles) > 0 {
		allowed := false
		for _, r := range allowedRoles {
			if role == r {
				allowed = true
				break
			}
		}
		if !allowed {
			return db, "", nil // Return empty role to indicate not authorized
		}
	}

	return db, role, nil
}
