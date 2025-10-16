package authz

import (
	"strings"
	"testing"
)

func TestParseQuery_Select(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		wantType  QueryType
		wantTable string
	}{
		{
			name:      "simple select",
			sql:       "SELECT * FROM users",
			wantType:  QueryTypeSelect,
			wantTable: "users",
		},
		{
			name:      "select with where",
			sql:       "SELECT id, name FROM customers WHERE active = 1",
			wantType:  QueryTypeSelect,
			wantTable: "customers",
		},
		{
			name:      "select lowercase",
			sql:       "select * from products",
			wantType:  QueryTypeSelect,
			wantTable: "products",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseQuery(tt.sql)
			if err != nil {
				t.Fatalf("ParseQuery failed: %v", err)
			}

			if parsed.Type != tt.wantType {
				t.Errorf("Expected type %v, got %v", tt.wantType, parsed.Type)
			}

			if parsed.TableName != tt.wantTable {
				t.Errorf("Expected table %s, got %s", tt.wantTable, parsed.TableName)
			}
		})
	}
}

func TestParseQuery_Insert(t *testing.T) {
	sql := "INSERT INTO products (name, price) VALUES (?, ?)"

	parsed, err := ParseQuery(sql)
	if err != nil {
		t.Fatalf("ParseQuery failed: %v", err)
	}

	if parsed.Type != QueryTypeInsert {
		t.Errorf("Expected type INSERT, got %v", parsed.Type)
	}

	if parsed.TableName != "products" {
		t.Errorf("Expected table products, got %s", parsed.TableName)
	}
}

func TestParseQuery_Update(t *testing.T) {
	sql := "UPDATE users SET last_login = ? WHERE id = ?"

	parsed, err := ParseQuery(sql)
	if err != nil {
		t.Fatalf("ParseQuery failed: %v", err)
	}

	if parsed.Type != QueryTypeUpdate {
		t.Errorf("Expected type UPDATE, got %v", parsed.Type)
	}

	if parsed.TableName != "users" {
		t.Errorf("Expected table users, got %s", parsed.TableName)
	}
}

func TestParseQuery_Delete(t *testing.T) {
	sql := "DELETE FROM sessions WHERE expired = 1"

	parsed, err := ParseQuery(sql)
	if err != nil {
		t.Fatalf("ParseQuery failed: %v", err)
	}

	if parsed.Type != QueryTypeDelete {
		t.Errorf("Expected type DELETE, got %v", parsed.Type)
	}

	if parsed.TableName != "sessions" {
		t.Errorf("Expected table sessions, got %s", parsed.TableName)
	}
}

func TestValidateQuery(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID := "test-user"

	// Grant read permission
	err := GrantPermission(db, userID, "test_data", true, false, false, false)
	if err != nil {
		t.Fatalf("GrantPermission failed: %v", err)
	}

	// Valid SELECT query
	err = ValidateQuery(db, userID, RoleEditor, "SELECT * FROM test_data")
	if err != nil {
		t.Errorf("ValidateQuery should allow SELECT with read permission: %v", err)
	}

	// Invalid INSERT query (no write permission)
	err = ValidateQuery(db, userID, RoleEditor, "INSERT INTO test_data (content) VALUES ('test')")
	if err == nil {
		t.Error("ValidateQuery should deny INSERT without write permission")
	}
}

func TestApplyRowPolicies_NoWhere(t *testing.T) {
	sql := "SELECT * FROM test_data"
	userID := "user-123"

	policies := []RowPolicy{
		{
			SQLCondition: "owner_id = $user_id",
		},
	}

	rewritten, err := ApplyRowPolicies(sql, userID, policies)
	if err != nil {
		t.Fatalf("ApplyRowPolicies failed: %v", err)
	}

	expected := "SELECT * FROM test_data WHERE (owner_id = 'user-123')"
	if rewritten != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, rewritten)
	}
}

func TestApplyRowPolicies_WithWhere(t *testing.T) {
	sql := "SELECT * FROM test_data WHERE active = 1"
	userID := "user-123"

	policies := []RowPolicy{
		{
			SQLCondition: "owner_id = $user_id",
		},
	}

	rewritten, err := ApplyRowPolicies(sql, userID, policies)
	if err != nil {
		t.Fatalf("ApplyRowPolicies failed: %v", err)
	}

	// Should add policy condition before existing WHERE
	if !strings.Contains(rewritten, "owner_id = 'user-123'") {
		t.Errorf("Rewritten query should contain policy condition")
	}

	if !strings.Contains(rewritten, "active = 1") {
		t.Errorf("Rewritten query should preserve original WHERE clause")
	}
}

func TestApplyRowPolicies_MultiplePolicies(t *testing.T) {
	sql := "SELECT * FROM test_data"
	userID := "user-123"

	policies := []RowPolicy{
		{
			SQLCondition: "owner_id = $user_id",
		},
		{
			SQLCondition: "enabled = 1",
		},
	}

	rewritten, err := ApplyRowPolicies(sql, userID, policies)
	if err != nil {
		t.Fatalf("ApplyRowPolicies failed: %v", err)
	}

	// Should combine policies with AND
	if !strings.Contains(rewritten, "owner_id = 'user-123'") {
		t.Errorf("Rewritten query should contain first policy")
	}

	if !strings.Contains(rewritten, "enabled = 1") {
		t.Errorf("Rewritten query should contain second policy")
	}

	if !strings.Contains(rewritten, " AND ") {
		t.Errorf("Policies should be combined with AND")
	}
}

func TestApplyRowPolicies_WithOrderBy(t *testing.T) {
	sql := "SELECT * FROM test_data ORDER BY created_at DESC"
	userID := "user-123"

	policies := []RowPolicy{
		{
			SQLCondition: "owner_id = $user_id",
		},
	}

	rewritten, err := ApplyRowPolicies(sql, userID, policies)
	if err != nil {
		t.Fatalf("ApplyRowPolicies failed: %v", err)
	}

	// WHERE should be inserted before ORDER BY
	whereIdx := strings.Index(strings.ToUpper(rewritten), " WHERE ")
	orderIdx := strings.Index(strings.ToUpper(rewritten), " ORDER BY ")

	if whereIdx == -1 {
		t.Error("Rewritten query should have WHERE clause")
	}

	if orderIdx == -1 {
		t.Error("Rewritten query should preserve ORDER BY")
	}

	if whereIdx > orderIdx {
		t.Error("WHERE clause should come before ORDER BY")
	}
}

func TestApplyRowPolicies_Update(t *testing.T) {
	sql := "UPDATE test_data SET content = 'new' WHERE id = 1"
	userID := "user-123"

	policies := []RowPolicy{
		{
			SQLCondition: "owner_id = $user_id",
		},
	}

	rewritten, err := ApplyRowPolicies(sql, userID, policies)
	if err != nil {
		t.Fatalf("ApplyRowPolicies failed: %v", err)
	}

	// Policy should be applied to UPDATE queries
	if !strings.Contains(rewritten, "owner_id = 'user-123'") {
		t.Errorf("Rewritten UPDATE should contain policy condition")
	}
}

func TestApplyRowPolicies_Delete(t *testing.T) {
	sql := "DELETE FROM test_data WHERE expired = 1"
	userID := "user-123"

	policies := []RowPolicy{
		{
			SQLCondition: "owner_id = $user_id",
		},
	}

	rewritten, err := ApplyRowPolicies(sql, userID, policies)
	if err != nil {
		t.Fatalf("ApplyRowPolicies failed: %v", err)
	}

	// Policy should be applied to DELETE queries
	if !strings.Contains(rewritten, "owner_id = 'user-123'") {
		t.Errorf("Rewritten DELETE should contain policy condition")
	}
}

func TestApplyRowPolicies_Insert(t *testing.T) {
	sql := "INSERT INTO test_data (content) VALUES ('test')"
	userID := "user-123"

	policies := []RowPolicy{
		{
			SQLCondition: "owner_id = $user_id",
		},
	}

	// Policies should NOT be applied to INSERT
	rewritten, err := ApplyRowPolicies(sql, userID, policies)
	if err != nil {
		t.Fatalf("ApplyRowPolicies failed: %v", err)
	}

	if rewritten != sql {
		t.Error("INSERT queries should not be rewritten")
	}
}

func TestApplyRowPolicies_NoPolicies(t *testing.T) {
	sql := "SELECT * FROM test_data"
	userID := "user-123"

	rewritten, err := ApplyRowPolicies(sql, userID, nil)
	if err != nil {
		t.Fatalf("ApplyRowPolicies failed: %v", err)
	}

	if rewritten != sql {
		t.Error("Query should not be rewritten when no policies")
	}
}

func TestValidateAndRewriteQuery(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID := "test-user"
	tableName := "test_data"

	// Grant read permission
	err := GrantPermission(db, userID, tableName, true, false, false, false)
	if err != nil {
		t.Fatalf("GrantPermission failed: %v", err)
	}

	// Create row policy
	err = CreateRowPolicy(db, tableName, userID, "read", "owner_id = $user_id", "admin")
	if err != nil {
		t.Fatalf("CreateRowPolicy failed: %v", err)
	}

	// Validate and rewrite
	sql := "SELECT * FROM test_data"
	rewritten, err := ValidateAndRewriteQuery(db, userID, RoleEditor, sql)
	if err != nil {
		t.Fatalf("ValidateAndRewriteQuery failed: %v", err)
	}

	// Should contain policy condition
	if !strings.Contains(rewritten, "owner_id") {
		t.Errorf("Rewritten query should contain policy condition")
	}
}

func TestValidateAndRewriteQuery_PermissionDenied(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID := "test-user"

	// No permission granted
	sql := "SELECT * FROM test_data"
	_, err := ValidateAndRewriteQuery(db, userID, RoleViewer, sql)
	if err == nil {
		t.Error("ValidateAndRewriteQuery should deny query without permission")
	}

	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("Error should mention permission denied, got: %v", err)
	}
}

func TestValidateAndRewriteQuery_OwnerBypassesPolicies(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create row policy (should be bypassed by owner)
	err := CreateRowPolicy(db, "test_data", "owner-id", "read", "owner_id = $user_id", "admin")
	if err != nil {
		t.Fatalf("CreateRowPolicy failed: %v", err)
	}

	// Owner query should not be rewritten
	sql := "SELECT * FROM test_data"
	rewritten, err := ValidateAndRewriteQuery(db, "owner-id", RoleOwner, sql)
	if err != nil {
		t.Fatalf("ValidateAndRewriteQuery failed: %v", err)
	}

	if rewritten != sql {
		t.Error("Owner queries should not be rewritten with policies")
	}
}
