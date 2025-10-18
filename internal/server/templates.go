// Package server provides HTTP handlers for template rendering
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/thetanil/wce/internal/auth"
	"github.com/thetanil/wce/internal/template"
)

// handleRenderPage renders a Jinja template and returns HTML
// Route: GET /{cenvID}/pages/{path...}
func (s *Server) handleRenderPage(w http.ResponseWriter, r *http.Request) {
	cenvID := r.PathValue("cenvID")
	path := r.PathValue("path")

	// Validate cenv exists
	if !s.cenvManager.Exists(cenvID) {
		http.Error(w, "Cenv not found", http.StatusNotFound)
		return
	}

	// Get database connection
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Map URL path to template document ID
	// e.g., /pages/home -> templates/pages/home.html
	templateID := "templates/pages/" + path + ".html"
	if path == "" {
		templateID = "templates/pages/index.html"
	}

	// Prepare rendering context
	variables := make(map[string]interface{})

	// Add request information
	variables["request"] = map[string]interface{}{
		"path":   r.URL.Path,
		"method": r.Method,
		"query":  queryParamsToMap(r),
	}

	// Add user info if authenticated (optional for pages)
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		claims, err := s.extractAndValidateClaims(r, cenvID)
		if err == nil {
			// User is authenticated
			variables["user"] = map[string]interface{}{
				"id":       claims.UserID,
				"username": claims.Username,
				"role":     claims.Role,
			}
		}
	}

	// Create render context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Render template
	html, err := template.RenderTemplateFromDB(ctx, db, templateID, variables)
	if err != nil {
		// Check if it's a "template not found" error
		if err.Error() == fmt.Sprintf("template not found: %s", templateID) {
			http.Error(w, "Page not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Template render error: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Return rendered HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// handlePreviewTemplate renders a template without saving it
// Route: POST /{cenvID}/templates/preview
func (s *Server) handlePreviewTemplate(w http.ResponseWriter, r *http.Request) {
	cenvID := r.PathValue("cenvID")

	// Validate cenv exists
	if !s.cenvManager.Exists(cenvID) {
		http.Error(w, "Cenv not found", http.StatusNotFound)
		return
	}

	// Require authentication
	claims, err := s.extractAndValidateClaims(r, cenvID)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var req struct {
		Template string                 `json:"template"` // Template source
		Context  map[string]interface{} `json:"context"`  // Optional context data
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Template == "" {
		http.Error(w, "Template source is required", http.StatusBadRequest)
		return
	}

	// Prepare context
	if req.Context == nil {
		req.Context = make(map[string]interface{})
	}

	// Add default context variables
	req.Context["user"] = map[string]interface{}{
		"id":       claims.UserID,
		"username": claims.Username,
		"role":     claims.Role,
	}

	// Get database connection for template loader
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Create render context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	renderCtx := &template.RenderContext{
		Variables: req.Context,
		Loader:    template.DocumentLoader(db),
	}

	// Render template
	html, err := template.RenderTemplate(ctx, req.Template, renderCtx)
	if err != nil {
		// Return error in JSON format for easier debugging
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Template render error",
			"message": err.Error(),
		})
		return
	}

	// Return rendered HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// handleListTemplates lists all templates in the cenv
// Route: GET /{cenvID}/templates
func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	cenvID := r.PathValue("cenvID")

	// Validate cenv exists
	if !s.cenvManager.Exists(cenvID) {
		http.Error(w, "Cenv not found", http.StatusNotFound)
		return
	}

	// Require authentication
	_, err := s.extractAndValidateClaims(r, cenvID)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get database connection
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Query templates
	// Templates are documents with content_type containing 'jinja' or starting with 'templates/'
	rows, err := db.Query(`
		SELECT id, content_type, created_at, modified_at, created_by, modified_by
		FROM _wce_documents
		WHERE id LIKE 'templates/%' OR content_type LIKE '%jinja%'
		ORDER BY id
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database query error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type TemplateInfo struct {
		ID          string `json:"id"`
		ContentType string `json:"content_type"`
		CreatedAt   int64  `json:"created_at"`
		ModifiedAt  int64  `json:"modified_at"`
		CreatedBy   string `json:"created_by"`
		ModifiedBy  string `json:"modified_by"`
	}

	templates := []TemplateInfo{}
	for rows.Next() {
		var tmpl TemplateInfo
		if err := rows.Scan(&tmpl.ID, &tmpl.ContentType, &tmpl.CreatedAt, &tmpl.ModifiedAt, &tmpl.CreatedBy, &tmpl.ModifiedBy); err != nil {
			http.Error(w, fmt.Sprintf("Scan error: %v", err), http.StatusInternalServerError)
			return
		}
		templates = append(templates, tmpl)
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"templates": templates,
		"count":     len(templates),
	})
}

// queryParamsToMap converts URL query parameters to a map
func queryParamsToMap(r *http.Request) map[string]interface{} {
	result := make(map[string]interface{})
	for key, values := range r.URL.Query() {
		if len(values) == 1 {
			result[key] = values[0]
		} else {
			result[key] = values
		}
	}
	return result
}

// extractAndValidateClaims extracts and validates JWT claims from request
// This is a helper function that duplicates some logic from handleCenvRequest
// but is useful for endpoints that optionally require authentication
func (s *Server) extractAndValidateClaims(r *http.Request, cenvID string) (*auth.Claims, error) {
	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("missing Authorization header")
	}

	// Expect "Bearer <token>"
	var token string
	if _, err := fmt.Sscanf(authHeader, "Bearer %s", &token); err != nil {
		return nil, fmt.Errorf("invalid Authorization header format")
	}

	// Validate token
	claims, err := s.jwtManager.ValidateToken(token)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Verify cenv_id matches
	if claims.CenvID != cenvID {
		return nil, fmt.Errorf("token cenv_id mismatch")
	}

	// Verify session is still valid
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	tokenHash := auth.GetTokenHash(token)
	valid, err := auth.IsSessionValid(db, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("session check error: %w", err)
	}
	if !valid {
		return nil, fmt.Errorf("session expired or revoked")
	}

	return claims, nil
}
