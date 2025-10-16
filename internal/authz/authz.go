package authz

import (
	"database/sql"
	"fmt"
	"strings"
)

// Role constants (matches auth package roles)
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// Permission represents a user's permission on a table
type Permission struct {
	ID        int
	TableName string
	UserID    string
	CanRead   bool
	CanWrite  bool
	CanDelete bool
	CanGrant  bool
}

// RowPolicy represents a row-level security policy
type RowPolicy struct {
	ID           int
	TableName    string
	UserID       string // empty = applies to all users
	PolicyType   string // "read", "write", "delete"
	SQLCondition string // e.g., "owner_id = $user_id"
	CreatedAt    int64
	CreatedBy    string
}

// CanRead checks if a user can read from a table
func CanRead(db *sql.DB, userID, role, tableName string) (bool, error) {
	// Owner and Admin have implicit full access
	if role == RoleOwner || role == RoleAdmin {
		return true, nil
	}

	// System tables are read-only for admins, hidden for others
	if strings.HasPrefix(tableName, "_wce_") {
		return role == RoleAdmin, nil
	}

	// Check explicit permissions
	query := `
		SELECT can_read FROM _wce_table_permissions
		WHERE user_id = ? AND table_name = ?
	`

	var canRead bool
	err := db.QueryRow(query, userID, tableName).Scan(&canRead)
	if err == sql.ErrNoRows {
		// No explicit permission = denied
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check read permission: %w", err)
	}

	return canRead, nil
}

// CanWrite checks if a user can write to a table
func CanWrite(db *sql.DB, userID, role, tableName string) (bool, error) {
	// Owner and Admin have implicit full access
	if role == RoleOwner || role == RoleAdmin {
		return true, nil
	}

	// System tables are protected
	if strings.HasPrefix(tableName, "_wce_") {
		return false, nil
	}

	// Check explicit permissions
	query := `
		SELECT can_write FROM _wce_table_permissions
		WHERE user_id = ? AND table_name = ?
	`

	var canWrite bool
	err := db.QueryRow(query, userID, tableName).Scan(&canWrite)
	if err == sql.ErrNoRows {
		// No explicit permission = denied
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check write permission: %w", err)
	}

	return canWrite, nil
}

// CanDelete checks if a user can delete from a table
func CanDelete(db *sql.DB, userID, role, tableName string) (bool, error) {
	// Owner and Admin have implicit full access
	if role == RoleOwner || role == RoleAdmin {
		return true, nil
	}

	// System tables are protected
	if strings.HasPrefix(tableName, "_wce_") {
		return false, nil
	}

	// Check explicit permissions
	query := `
		SELECT can_delete FROM _wce_table_permissions
		WHERE user_id = ? AND table_name = ?
	`

	var canDelete bool
	err := db.QueryRow(query, userID, tableName).Scan(&canDelete)
	if err == sql.ErrNoRows {
		// No explicit permission = denied
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check delete permission: %w", err)
	}

	return canDelete, nil
}

// CanGrant checks if a user can grant permissions on a table
func CanGrant(db *sql.DB, userID, role, tableName string) (bool, error) {
	// Only Owner and Admin can grant permissions
	if role == RoleOwner || role == RoleAdmin {
		return true, nil
	}

	// Check explicit grant permission
	query := `
		SELECT can_grant FROM _wce_table_permissions
		WHERE user_id = ? AND table_name = ?
	`

	var canGrant bool
	err := db.QueryRow(query, userID, tableName).Scan(&canGrant)
	if err == sql.ErrNoRows {
		// No explicit permission = denied
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check grant permission: %w", err)
	}

	return canGrant, nil
}

// GetTablePermission retrieves a user's permission for a table
func GetTablePermission(db *sql.DB, userID, tableName string) (*Permission, error) {
	query := `
		SELECT id, table_name, user_id, can_read, can_write, can_delete, can_grant
		FROM _wce_table_permissions
		WHERE user_id = ? AND table_name = ?
	`

	perm := &Permission{}
	err := db.QueryRow(query, userID, tableName).Scan(
		&perm.ID,
		&perm.TableName,
		&perm.UserID,
		&perm.CanRead,
		&perm.CanWrite,
		&perm.CanDelete,
		&perm.CanGrant,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No permission found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get table permission: %w", err)
	}

	return perm, nil
}

// GrantPermission grants table permissions to a user
func GrantPermission(db *sql.DB, userID, tableName string, canRead, canWrite, canDelete, canGrant bool) error {
	query := `
		INSERT INTO _wce_table_permissions (table_name, user_id, can_read, can_write, can_delete, can_grant)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(table_name, user_id) DO UPDATE SET
			can_read = excluded.can_read,
			can_write = excluded.can_write,
			can_delete = excluded.can_delete,
			can_grant = excluded.can_grant
	`

	_, err := db.Exec(query, tableName, userID, canRead, canWrite, canDelete, canGrant)
	if err != nil {
		return fmt.Errorf("failed to grant permission: %w", err)
	}

	return nil
}

// RevokePermission removes all permissions for a user on a table
func RevokePermission(db *sql.DB, userID, tableName string) error {
	query := `DELETE FROM _wce_table_permissions WHERE user_id = ? AND table_name = ?`

	result, err := db.Exec(query, userID, tableName)
	if err != nil {
		return fmt.Errorf("failed to revoke permission: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("permission not found")
	}

	return nil
}

// ListUserPermissions lists all permissions for a user
func ListUserPermissions(db *sql.DB, userID string) ([]Permission, error) {
	query := `
		SELECT id, table_name, user_id, can_read, can_write, can_delete, can_grant
		FROM _wce_table_permissions
		WHERE user_id = ?
		ORDER BY table_name
	`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list permissions: %w", err)
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		var perm Permission
		err := rows.Scan(
			&perm.ID,
			&perm.TableName,
			&perm.UserID,
			&perm.CanRead,
			&perm.CanWrite,
			&perm.CanDelete,
			&perm.CanGrant,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		permissions = append(permissions, perm)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating permissions: %w", err)
	}

	return permissions, nil
}

// ListTablePermissions lists all permissions for a table
func ListTablePermissions(db *sql.DB, tableName string) ([]Permission, error) {
	query := `
		SELECT id, table_name, user_id, can_read, can_write, can_delete, can_grant
		FROM _wce_table_permissions
		WHERE table_name = ?
		ORDER BY user_id
	`

	rows, err := db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to list table permissions: %w", err)
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		var perm Permission
		err := rows.Scan(
			&perm.ID,
			&perm.TableName,
			&perm.UserID,
			&perm.CanRead,
			&perm.CanWrite,
			&perm.CanDelete,
			&perm.CanGrant,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		permissions = append(permissions, perm)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating permissions: %w", err)
	}

	return permissions, nil
}

// GetRowPolicies retrieves row-level policies for a table and user
func GetRowPolicies(db *sql.DB, userID, tableName, policyType string) ([]RowPolicy, error) {
	query := `
		SELECT id, table_name, user_id, policy_type, sql_condition, created_at, created_by
		FROM _wce_row_policies
		WHERE table_name = ? AND policy_type = ? AND (user_id = ? OR user_id IS NULL)
		ORDER BY id
	`

	rows, err := db.Query(query, tableName, policyType, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get row policies: %w", err)
	}
	defer rows.Close()

	var policies []RowPolicy
	for rows.Next() {
		var policy RowPolicy
		var userIDNull sql.NullString
		var createdByNull sql.NullString

		err := rows.Scan(
			&policy.ID,
			&policy.TableName,
			&userIDNull,
			&policy.PolicyType,
			&policy.SQLCondition,
			&policy.CreatedAt,
			&createdByNull,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan policy: %w", err)
		}

		if userIDNull.Valid {
			policy.UserID = userIDNull.String
		}
		if createdByNull.Valid {
			policy.CreatedBy = createdByNull.String
		}

		policies = append(policies, policy)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating policies: %w", err)
	}

	return policies, nil
}

// CreateRowPolicy creates a new row-level security policy
func CreateRowPolicy(db *sql.DB, tableName, userID, policyType, sqlCondition, createdBy string) error {
	query := `
		INSERT INTO _wce_row_policies (table_name, user_id, policy_type, sql_condition, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	var userIDParam interface{}
	if userID == "" {
		userIDParam = nil
	} else {
		userIDParam = userID
	}

	_, err := db.Exec(query, tableName, userIDParam, policyType, sqlCondition, 0, createdBy) // timestamp set by trigger or app
	if err != nil {
		return fmt.Errorf("failed to create row policy: %w", err)
	}

	return nil
}
