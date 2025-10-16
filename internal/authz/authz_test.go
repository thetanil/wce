package authz

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates an in-memory database with WCE schema
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create minimal schema for testing
	schema := `
	CREATE TABLE _wce_users (
		user_id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'viewer',
		enabled INTEGER DEFAULT 1
	);

	CREATE TABLE _wce_table_permissions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		table_name TEXT NOT NULL,
		user_id TEXT NOT NULL,
		can_read INTEGER DEFAULT 0,
		can_write INTEGER DEFAULT 0,
		can_delete INTEGER DEFAULT 0,
		can_grant INTEGER DEFAULT 0,
		UNIQUE(table_name, user_id)
	);

	CREATE TABLE _wce_row_policies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		table_name TEXT NOT NULL,
		user_id TEXT,
		policy_type TEXT NOT NULL,
		sql_condition TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		created_by TEXT
	);

	CREATE TABLE test_data (
		id INTEGER PRIMARY KEY,
		owner_id TEXT,
		content TEXT
	);
	`

	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	return db
}

func TestCanRead_Owner(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Owner should have implicit read access
	canRead, err := CanRead(db, "owner-id", RoleOwner, "test_data")
	if err != nil {
		t.Fatalf("CanRead failed: %v", err)
	}

	if !canRead {
		t.Error("Owner should have implicit read access")
	}
}

func TestCanRead_Admin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Admin should have implicit read access
	canRead, err := CanRead(db, "admin-id", RoleAdmin, "test_data")
	if err != nil {
		t.Fatalf("CanRead failed: %v", err)
	}

	if !canRead {
		t.Error("Admin should have implicit read access")
	}
}

func TestCanRead_WithPermission(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID := "editor-id"

	// Grant read permission
	err := GrantPermission(db, userID, "test_data", true, false, false, false)
	if err != nil {
		t.Fatalf("GrantPermission failed: %v", err)
	}

	// Check read permission
	canRead, err := CanRead(db, userID, RoleEditor, "test_data")
	if err != nil {
		t.Fatalf("CanRead failed: %v", err)
	}

	if !canRead {
		t.Error("Editor should have read access after grant")
	}
}

func TestCanRead_WithoutPermission(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID := "viewer-id"

	// Check read permission without grant
	canRead, err := CanRead(db, userID, RoleViewer, "test_data")
	if err != nil {
		t.Fatalf("CanRead failed: %v", err)
	}

	if canRead {
		t.Error("Viewer should not have read access without permission")
	}
}

func TestCanWrite_Owner(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	canWrite, err := CanWrite(db, "owner-id", RoleOwner, "test_data")
	if err != nil {
		t.Fatalf("CanWrite failed: %v", err)
	}

	if !canWrite {
		t.Error("Owner should have implicit write access")
	}
}

func TestCanWrite_SystemTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Even editors cannot write to system tables
	canWrite, err := CanWrite(db, "editor-id", RoleEditor, "_wce_users")
	if err != nil {
		t.Fatalf("CanWrite failed: %v", err)
	}

	if canWrite {
		t.Error("Editor should not have write access to system tables")
	}
}

func TestCanDelete_WithPermission(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID := "editor-id"

	// Grant delete permission
	err := GrantPermission(db, userID, "test_data", false, false, true, false)
	if err != nil {
		t.Fatalf("GrantPermission failed: %v", err)
	}

	// Check delete permission
	canDelete, err := CanDelete(db, userID, RoleEditor, "test_data")
	if err != nil {
		t.Fatalf("CanDelete failed: %v", err)
	}

	if !canDelete {
		t.Error("Editor should have delete access after grant")
	}
}

func TestCanGrant_OnlyOwnerAdmin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Owner can grant
	canGrant, err := CanGrant(db, "owner-id", RoleOwner, "test_data")
	if err != nil {
		t.Fatalf("CanGrant failed: %v", err)
	}
	if !canGrant {
		t.Error("Owner should be able to grant")
	}

	// Admin can grant
	canGrant, err = CanGrant(db, "admin-id", RoleAdmin, "test_data")
	if err != nil {
		t.Fatalf("CanGrant failed: %v", err)
	}
	if !canGrant {
		t.Error("Admin should be able to grant")
	}

	// Editor cannot grant without explicit permission
	canGrant, err = CanGrant(db, "editor-id", RoleEditor, "test_data")
	if err != nil {
		t.Fatalf("CanGrant failed: %v", err)
	}
	if canGrant {
		t.Error("Editor should not be able to grant without permission")
	}
}

func TestGrantPermission(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID := "test-user"
	tableName := "test_data"

	// Grant permissions
	err := GrantPermission(db, userID, tableName, true, true, false, false)
	if err != nil {
		t.Fatalf("GrantPermission failed: %v", err)
	}

	// Verify permissions were granted
	perm, err := GetTablePermission(db, userID, tableName)
	if err != nil {
		t.Fatalf("GetTablePermission failed: %v", err)
	}

	if perm == nil {
		t.Fatal("Permission should exist")
	}

	if !perm.CanRead {
		t.Error("CanRead should be true")
	}

	if !perm.CanWrite {
		t.Error("CanWrite should be true")
	}

	if perm.CanDelete {
		t.Error("CanDelete should be false")
	}
}

func TestGrantPermission_Update(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID := "test-user"
	tableName := "test_data"

	// Grant initial permissions
	err := GrantPermission(db, userID, tableName, true, false, false, false)
	if err != nil {
		t.Fatalf("Initial GrantPermission failed: %v", err)
	}

	// Update permissions
	err = GrantPermission(db, userID, tableName, true, true, true, false)
	if err != nil {
		t.Fatalf("Update GrantPermission failed: %v", err)
	}

	// Verify permissions were updated
	perm, err := GetTablePermission(db, userID, tableName)
	if err != nil {
		t.Fatalf("GetTablePermission failed: %v", err)
	}

	if !perm.CanWrite {
		t.Error("CanWrite should be updated to true")
	}

	if !perm.CanDelete {
		t.Error("CanDelete should be updated to true")
	}
}

func TestRevokePermission(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID := "test-user"
	tableName := "test_data"

	// Grant permissions
	err := GrantPermission(db, userID, tableName, true, true, false, false)
	if err != nil {
		t.Fatalf("GrantPermission failed: %v", err)
	}

	// Revoke permissions
	err = RevokePermission(db, userID, tableName)
	if err != nil {
		t.Fatalf("RevokePermission failed: %v", err)
	}

	// Verify permissions were revoked
	perm, err := GetTablePermission(db, userID, tableName)
	if err != nil {
		t.Fatalf("GetTablePermission failed: %v", err)
	}

	if perm != nil {
		t.Error("Permission should be nil after revocation")
	}
}

func TestListUserPermissions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID := "test-user"

	// Grant permissions on multiple tables
	GrantPermission(db, userID, "table1", true, false, false, false)
	GrantPermission(db, userID, "table2", true, true, false, false)
	GrantPermission(db, userID, "table3", true, true, true, false)

	// List permissions
	perms, err := ListUserPermissions(db, userID)
	if err != nil {
		t.Fatalf("ListUserPermissions failed: %v", err)
	}

	if len(perms) != 3 {
		t.Errorf("Expected 3 permissions, got %d", len(perms))
	}

	// Verify they're sorted by table name
	if perms[0].TableName != "table1" {
		t.Errorf("Expected first table to be table1, got %s", perms[0].TableName)
	}
}

func TestListTablePermissions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tableName := "test_data"

	// Grant permissions to multiple users
	GrantPermission(db, "user1", tableName, true, false, false, false)
	GrantPermission(db, "user2", tableName, true, true, false, false)

	// List permissions
	perms, err := ListTablePermissions(db, tableName)
	if err != nil {
		t.Fatalf("ListTablePermissions failed: %v", err)
	}

	if len(perms) != 2 {
		t.Errorf("Expected 2 permissions, got %d", len(perms))
	}
}

func TestGetRowPolicies(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tableName := "test_data"
	userID := "test-user"

	// Create row policy
	err := CreateRowPolicy(db, tableName, userID, "read", "owner_id = $user_id", "admin")
	if err != nil {
		t.Fatalf("CreateRowPolicy failed: %v", err)
	}

	// Get policies
	policies, err := GetRowPolicies(db, userID, tableName, "read")
	if err != nil {
		t.Fatalf("GetRowPolicies failed: %v", err)
	}

	if len(policies) != 1 {
		t.Errorf("Expected 1 policy, got %d", len(policies))
	}

	if policies[0].SQLCondition != "owner_id = $user_id" {
		t.Errorf("Expected condition 'owner_id = $user_id', got '%s'", policies[0].SQLCondition)
	}
}

func TestCreateRowPolicy_GlobalPolicy(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create global policy (userID = "")
	err := CreateRowPolicy(db, "test_data", "", "read", "enabled = 1", "admin")
	if err != nil {
		t.Fatalf("CreateRowPolicy failed: %v", err)
	}

	// Global policies should apply to any user
	policies, err := GetRowPolicies(db, "any-user", "test_data", "read")
	if err != nil {
		t.Fatalf("GetRowPolicies failed: %v", err)
	}

	if len(policies) != 1 {
		t.Errorf("Expected 1 global policy, got %d", len(policies))
	}
}
