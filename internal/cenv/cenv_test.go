package cenv

import (
	"os"
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
