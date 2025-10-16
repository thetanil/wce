package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/thetanil/wce/internal/auth"
	"github.com/thetanil/wce/internal/authz"
	"github.com/thetanil/wce/internal/cenv"
)

// GrantPermissionRequest represents request to grant permissions
type GrantPermissionRequest struct {
	UserID    string `json:"user_id"`
	TableName string `json:"table_name"`
	CanRead   bool   `json:"can_read"`
	CanWrite  bool   `json:"can_write"`
	CanDelete bool   `json:"can_delete"`
	CanGrant  bool   `json:"can_grant"`
}

// RevokePermissionRequest represents request to revoke permissions
type RevokePermissionRequest struct {
	UserID    string `json:"user_id"`
	TableName string `json:"table_name"`
}

// CreatePolicyRequest represents request to create row policy
type CreatePolicyRequest struct {
	TableName    string `json:"table_name"`
	UserID       string `json:"user_id"` // empty = applies to all
	PolicyType   string `json:"policy_type"` // "read", "write", "delete"
	SQLCondition string `json:"sql_condition"`
}

// requireAuth is a helper that extracts and validates authentication
// Returns (userID, role, db, error)
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request, cenvID string) (string, string, *sql.DB, error) {
	// Check if cenv exists
	if !s.cenvManager.Exists(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "cenv not found",
		})
		return "", "", nil, fmt.Errorf("cenv not found")
	}

	// Get database connection
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to connect to database",
		})
		return "", "", nil, fmt.Errorf("database connection failed: %w", err)
	}

	// Extract token
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "missing authorization header",
		})
		return "", "", nil, fmt.Errorf("missing authorization")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid authorization header format",
		})
		return "", "", nil, fmt.Errorf("invalid authorization format")
	}

	token := parts[1]

	// Validate token
	claims, err := s.jwtManager.ValidateToken(token)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid token",
		})
		return "", "", nil, fmt.Errorf("invalid token: %w", err)
	}

	// Verify token is for this cenv
	if claims.CenvID != cenvID {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "token not valid for this cenv",
		})
		return "", "", nil, fmt.Errorf("token cenv mismatch")
	}

	// Check session validity
	tokenHash := auth.GetTokenHash(token)
	valid, err := auth.IsSessionValid(db, tokenHash)
	if err != nil || !valid {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "session expired or revoked",
		})
		return "", "", nil, fmt.Errorf("invalid session")
	}

	return claims.UserID, claims.Role, db, nil
}

// handleListPermissions lists permissions (admin/owner only)
func (s *Server) handleListPermissions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cenvID := r.PathValue("cenvID")
	if !cenv.IsValidUUID(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid cenv id"})
		return
	}

	userID, role, db, err := s.requireAuth(w, r, cenvID)
	if err != nil {
		return // Response already sent
	}

	// Only owner/admin can list permissions
	if role != authz.RoleOwner && role != authz.RoleAdmin {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "only owner or admin can list permissions",
		})
		return
	}

	// Get query parameter - list for specific user or table
	targetUserID := r.URL.Query().Get("user_id")
	tableName := r.URL.Query().Get("table_name")

	var permissions []authz.Permission

	if targetUserID != "" {
		permissions, err = authz.ListUserPermissions(db, targetUserID)
	} else if tableName != "" {
		permissions, err = authz.ListTablePermissions(db, tableName)
	} else {
		// List all permissions for current user
		permissions, err = authz.ListUserPermissions(db, userID)
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to list permissions",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"permissions": permissions,
	})
}

// handleGrantPermission grants table permissions (admin/owner only)
func (s *Server) handleGrantPermission(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cenvID := r.PathValue("cenvID")
	if !cenv.IsValidUUID(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid cenv id"})
		return
	}

	_, role, db, err := s.requireAuth(w, r, cenvID)
	if err != nil {
		return // Response already sent
	}

	// Only owner/admin can grant permissions
	if role != authz.RoleOwner && role != authz.RoleAdmin {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "only owner or admin can grant permissions",
		})
		return
	}

	// Parse request
	var req GrantPermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Validate required fields
	if req.UserID == "" || req.TableName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "user_id and table_name are required",
		})
		return
	}

	// Grant permission
	err = authz.GrantPermission(db, req.UserID, req.TableName, req.CanRead, req.CanWrite, req.CanDelete, req.CanGrant)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to grant permission",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "permission granted successfully",
	})
}

// handleRevokePermission revokes table permissions (admin/owner only)
func (s *Server) handleRevokePermission(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cenvID := r.PathValue("cenvID")
	if !cenv.IsValidUUID(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid cenv id"})
		return
	}

	_, role, db, err := s.requireAuth(w, r, cenvID)
	if err != nil {
		return // Response already sent
	}

	// Only owner/admin can revoke permissions
	if role != authz.RoleOwner && role != authz.RoleAdmin {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "only owner or admin can revoke permissions",
		})
		return
	}

	// Parse request
	var req RevokePermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Validate required fields
	if req.UserID == "" || req.TableName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "user_id and table_name are required",
		})
		return
	}

	// Revoke permission
	err = authz.RevokePermission(db, req.UserID, req.TableName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to revoke permission",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "permission revoked successfully",
	})
}

// handleListPolicies lists row-level policies (admin/owner only)
func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cenvID := r.PathValue("cenvID")
	if !cenv.IsValidUUID(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid cenv id"})
		return
	}

	userID, role, db, err := s.requireAuth(w, r, cenvID)
	if err != nil {
		return // Response already sent
	}

	// Only owner/admin can list policies
	if role != authz.RoleOwner && role != authz.RoleAdmin {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "only owner or admin can list policies",
		})
		return
	}

	// Get query parameters
	tableName := r.URL.Query().Get("table_name")
	policyType := r.URL.Query().Get("policy_type")

	if tableName == "" || policyType == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "table_name and policy_type query parameters required",
		})
		return
	}

	// Get policies
	policies, err := authz.GetRowPolicies(db, userID, tableName, policyType)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to list policies",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"policies": policies,
	})
}

// handleCreatePolicy creates a row-level policy (admin/owner only)
func (s *Server) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cenvID := r.PathValue("cenvID")
	if !cenv.IsValidUUID(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid cenv id"})
		return
	}

	creatorID, role, db, err := s.requireAuth(w, r, cenvID)
	if err != nil {
		return // Response already sent
	}

	// Only owner/admin can create policies
	if role != authz.RoleOwner && role != authz.RoleAdmin {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "only owner or admin can create policies",
		})
		return
	}

	// Parse request
	var req CreatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Validate required fields
	if req.TableName == "" || req.PolicyType == "" || req.SQLCondition == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "table_name, policy_type, and sql_condition are required",
		})
		return
	}

	// Validate policy type
	validTypes := map[string]bool{"read": true, "write": true, "delete": true}
	if !validTypes[req.PolicyType] {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "policy_type must be 'read', 'write', or 'delete'",
		})
		return
	}

	// Create policy
	err = authz.CreateRowPolicy(db, req.TableName, req.UserID, req.PolicyType, req.SQLCondition, creatorID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to create policy",
		})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "policy created successfully",
	})
}
