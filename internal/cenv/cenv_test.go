package cenv

import (
	"os"
	"sync"
	"testing"
)

func TestCreateDatabase(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	manager := NewManager(tempDir)
	cenvID := "123e4567-e89b-12d3-a456-426614174000"

	// Test creating database
	if err := manager.Create(cenvID); err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Verify database file exists
	if !manager.Exists(cenvID) {
		t.Error("Database file should exist after creation")
	}

	// Verify file permissions
	dbPath := manager.GetDatabasePath(cenvID)
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Failed to stat database file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Expected permissions 0600, got %o", perm)
	}

	// Test that creating again fails
	if err := manager.Create(cenvID); err == nil {
		t.Error("Creating duplicate database should fail")
	}
}

func TestSchemaInitialization(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	manager := NewManager(tempDir)
	cenvID := "223e4567-e89b-12d3-a456-426614174000"

	// Create database (should auto-initialize schema)
	if err := manager.Create(cenvID); err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Open connection to verify schema
	db, err := manager.Open(cenvID)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Check all required tables exist
	requiredTables := []string{
		"_wce_users",
		"_wce_sessions",
		"_wce_table_permissions",
		"_wce_row_policies",
		"_wce_config",
		"_wce_audit_log",
	}

	for _, tableName := range requiredTables {
		var count int
		query := "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?"
		err := db.QueryRow(query, tableName).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query for table %s: %v", tableName, err)
		}
		if count != 1 {
			t.Errorf("Table %s should exist, found %d", tableName, count)
		}
	}

	// Verify default config values exist
	var sessionTimeout string
	err = db.QueryRow("SELECT value FROM _wce_config WHERE key='session_timeout_hours'").Scan(&sessionTimeout)
	if err != nil {
		t.Fatalf("Failed to query default config: %v", err)
	}
	if sessionTimeout != "24" {
		t.Errorf("Expected session_timeout_hours=24, got %s", sessionTimeout)
	}

	// Verify indexes were created
	var indexCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'").Scan(&indexCount)
	if err != nil {
		t.Fatalf("Failed to count indexes: %v", err)
	}
	if indexCount == 0 {
		t.Error("Expected indexes to be created")
	}
}

func TestOpenDatabase(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	manager := NewManager(tempDir)
	cenvID := "123e4567-e89b-12d3-a456-426614174000"

	// Opening non-existent database should fail
	_, err := manager.Open(cenvID)
	if err == nil {
		t.Error("Opening non-existent database should fail")
	}

	// Create database
	if err := manager.Create(cenvID); err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Open should succeed
	db, err := manager.Open(cenvID)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Verify connection works
	if err := db.Ping(); err != nil {
		t.Errorf("Database ping failed: %v", err)
	}

	// Verify pragmas are set by checking journal mode
	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("Failed to query journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("Expected journal_mode=wal, got %s", journalMode)
	}
}

func TestGetDatabasePath(t *testing.T) {
	manager := NewManager("/test/dir")
	cenvID := "123e4567-e89b-12d3-a456-426614174000"

	expected := "/test/dir/123e4567-e89b-12d3-a456-426614174000.db"
	actual := manager.GetDatabasePath(cenvID)

	if actual != expected {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		uuid  string
		valid bool
	}{
		{"123e4567-e89b-12d3-a456-426614174000", true},
		{"123E4567-E89B-12D3-A456-426614174000", true}, // uppercase
		{"invalid-uuid", false},
		{"123e4567-e89b-12d3-a456", false},
		{"", false},
		{"not-a-uuid-at-all", false},
	}

	for _, test := range tests {
		result := IsValidUUID(test.uuid)
		if result != test.valid {
			t.Errorf("IsValidUUID(%s) = %v, want %v", test.uuid, result, test.valid)
		}
	}
}

func TestParsePath(t *testing.T) {
	tests := []struct {
		path          string
		wantCenvID    string
		wantRemaining string
		wantOK        bool
	}{
		{
			"/123e4567-e89b-12d3-a456-426614174000/",
			"123e4567-e89b-12d3-a456-426614174000",
			"/",
			true,
		},
		{
			"/123e4567-e89b-12d3-a456-426614174000/page/home",
			"123e4567-e89b-12d3-a456-426614174000",
			"/page/home",
			true,
		},
		{
			"/invalid-uuid/page",
			"",
			"",
			false,
		},
		{
			"/",
			"",
			"",
			false,
		},
	}

	for _, test := range tests {
		gotCenvID, gotRemaining, gotOK := ParsePath(test.path)
		if gotOK != test.wantOK {
			t.Errorf("ParsePath(%s) ok = %v, want %v", test.path, gotOK, test.wantOK)
		}
		if gotOK && (gotCenvID != test.wantCenvID || gotRemaining != test.wantRemaining) {
			t.Errorf("ParsePath(%s) = (%s, %s), want (%s, %s)",
				test.path, gotCenvID, gotRemaining, test.wantCenvID, test.wantRemaining)
		}
	}
}

func TestConnectionPooling(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	manager := NewManager(tempDir)
	cenvID := "323e4567-e89b-12d3-a456-426614174000"

	// Create database
	if err := manager.Create(cenvID); err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Get connection - should be created
	conn1, err := manager.GetConnection(cenvID)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	// Get connection again - should return same connection
	conn2, err := manager.GetConnection(cenvID)
	if err != nil {
		t.Fatalf("Failed to get connection second time: %v", err)
	}

	if conn1 != conn2 {
		t.Error("Expected same connection to be returned from pool")
	}

	// Test concurrent access
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := manager.GetConnection(cenvID)
			if err != nil {
				errors <- err
				return
			}
			// Try to query the database
			var count int
			err = conn.QueryRow("SELECT COUNT(*) FROM _wce_users").Scan(&count)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}

	// Test CloseConnection
	if err := manager.CloseConnection(cenvID); err != nil {
		t.Errorf("Failed to close connection: %v", err)
	}

	// After closing, should get new connection
	conn3, err := manager.GetConnection(cenvID)
	if err != nil {
		t.Fatalf("Failed to get connection after close: %v", err)
	}

	if conn3 == conn1 {
		t.Error("Expected new connection after closing")
	}

	// Test CloseAll
	if err := manager.CloseAll(); err != nil {
		t.Errorf("Failed to close all connections: %v", err)
	}
}
