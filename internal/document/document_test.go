package document

import (
	"database/sql"
	"encoding/base64"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates an in-memory database with document tables
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Enable foreign keys (required for CASCADE)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	// Create minimal schema for testing
	schema := `
	CREATE TABLE _wce_users (
		user_id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL
	);

	INSERT INTO _wce_users (user_id, username) VALUES ('user-1', 'testuser');

	CREATE TABLE _wce_documents (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL,
		content_type TEXT NOT NULL,
		is_binary INTEGER DEFAULT 0,
		searchable INTEGER DEFAULT 1,
		created_at INTEGER NOT NULL,
		modified_at INTEGER NOT NULL,
		created_by TEXT NOT NULL,
		modified_by TEXT NOT NULL,
		version INTEGER DEFAULT 1,
		FOREIGN KEY (created_by) REFERENCES _wce_users(user_id),
		FOREIGN KEY (modified_by) REFERENCES _wce_users(user_id)
	);

	CREATE INDEX idx_documents_content_type ON _wce_documents(content_type);
	CREATE INDEX idx_documents_created_by ON _wce_documents(created_by);
	CREATE INDEX idx_documents_modified_at ON _wce_documents(modified_at);

	CREATE TABLE _wce_document_tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		document_id TEXT NOT NULL,
		tag TEXT NOT NULL,
		FOREIGN KEY (document_id) REFERENCES _wce_documents(id) ON DELETE CASCADE,
		UNIQUE(document_id, tag)
	);

	CREATE INDEX idx_document_tags_doc ON _wce_document_tags(document_id);
	CREATE INDEX idx_document_tags_tag ON _wce_document_tags(tag);

	CREATE VIRTUAL TABLE _wce_document_search USING fts5(
		document_id UNINDEXED,
		content
	);

	CREATE TRIGGER _wce_documents_ai AFTER INSERT ON _wce_documents BEGIN
		INSERT INTO _wce_document_search(document_id, content)
		VALUES (NEW.id, CASE WHEN NEW.searchable = 1 THEN NEW.content ELSE '' END);
	END;

	CREATE TRIGGER _wce_documents_ad AFTER DELETE ON _wce_documents BEGIN
		DELETE FROM _wce_document_search WHERE document_id = OLD.id;
	END;

	CREATE TRIGGER _wce_documents_au AFTER UPDATE ON _wce_documents BEGIN
		UPDATE _wce_document_search
		SET document_id = NEW.id,
			content = CASE WHEN NEW.searchable = 1 THEN NEW.content ELSE '' END
		WHERE document_id = OLD.id;
	END;
	`

	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	return db
}

func TestCreateDocument(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	doc, err := CreateDocument(db, "pages/home", "<h1>Hello World</h1>", "text/html", "user-1", false, true)
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}

	if doc.ID != "pages/home" {
		t.Errorf("Expected ID 'pages/home', got %s", doc.ID)
	}

	if doc.Content != "<h1>Hello World</h1>" {
		t.Errorf("Expected content '<h1>Hello World</h1>', got %s", doc.Content)
	}

	if doc.ContentType != "text/html" {
		t.Errorf("Expected content type 'text/html', got %s", doc.ContentType)
	}

	if doc.IsBinary {
		t.Error("Expected IsBinary to be false")
	}

	if !doc.Searchable {
		t.Error("Expected Searchable to be true")
	}

	if doc.Version != 1 {
		t.Errorf("Expected version 1, got %d", doc.Version)
	}

	if doc.CreatedBy != "user-1" {
		t.Errorf("Expected CreatedBy 'user-1', got %s", doc.CreatedBy)
	}
}

func TestCreateDocument_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create first document
	_, err := CreateDocument(db, "test/doc", "content", "text/plain", "user-1", false, true)
	if err != nil {
		t.Fatalf("First CreateDocument failed: %v", err)
	}

	// Try to create duplicate
	_, err = CreateDocument(db, "test/doc", "other content", "text/plain", "user-1", false, true)
	if err == nil {
		t.Error("CreateDocument should fail for duplicate ID")
	}
}

func TestCreateDocument_Binary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create base64 encoded content
	binaryData := []byte("This is binary data")
	encoded := base64.StdEncoding.EncodeToString(binaryData)

	doc, err := CreateDocument(db, "files/image.png", encoded, "image/png", "user-1", true, false)
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}

	if !doc.IsBinary {
		t.Error("Expected IsBinary to be true")
	}

	if doc.Searchable {
		t.Error("Expected Searchable to be false for binary")
	}

	// Verify content is still base64
	decoded, err := base64.StdEncoding.DecodeString(doc.Content)
	if err != nil {
		t.Errorf("Content should be valid base64: %v", err)
	}

	if string(decoded) != string(binaryData) {
		t.Error("Decoded content doesn't match original")
	}
}

func TestCreateDocument_InvalidBinary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Try to create binary doc with invalid base64
	_, err := CreateDocument(db, "files/test", "not valid base64!!!", "image/png", "user-1", true, false)
	if err == nil {
		t.Error("CreateDocument should fail for invalid base64 in binary mode")
	}
}

func TestGetDocument(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create document
	created, err := CreateDocument(db, "api/users", `{"users":[]}`, "application/json", "user-1", false, true)
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}

	// Get document
	doc, err := GetDocument(db, "api/users")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}

	if doc.ID != created.ID {
		t.Errorf("Expected ID %s, got %s", created.ID, doc.ID)
	}

	if doc.Content != created.Content {
		t.Errorf("Expected content %s, got %s", created.Content, doc.Content)
	}

	if doc.Version != created.Version {
		t.Errorf("Expected version %d, got %d", created.Version, doc.Version)
	}
}

func TestGetDocument_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := GetDocument(db, "nonexistent")
	if err == nil {
		t.Error("GetDocument should fail for nonexistent document")
	}
}

func TestUpdateDocument(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create document
	_, err := CreateDocument(db, "pages/about", "<h1>About</h1>", "text/html", "user-1", false, true)
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}

	// Update document
	updated, err := UpdateDocument(db, "pages/about", "<h1>About Us</h1>", "user-1")
	if err != nil {
		t.Fatalf("UpdateDocument failed: %v", err)
	}

	if updated.Content != "<h1>About Us</h1>" {
		t.Errorf("Expected updated content, got %s", updated.Content)
	}

	if updated.Version != 2 {
		t.Errorf("Expected version 2, got %d", updated.Version)
	}

	// Verify update persisted
	doc, err := GetDocument(db, "pages/about")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}

	if doc.Content != "<h1>About Us</h1>" {
		t.Error("Updated content not persisted")
	}

	if doc.Version != 2 {
		t.Error("Version not incremented")
	}
}

func TestUpdateDocument_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := UpdateDocument(db, "nonexistent", "new content", "user-1")
	if err == nil {
		t.Error("UpdateDocument should fail for nonexistent document")
	}
}

func TestDeleteDocument(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create document
	_, err := CreateDocument(db, "temp/doc", "temporary", "text/plain", "user-1", false, true)
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}

	// Delete document
	err = DeleteDocument(db, "temp/doc")
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}

	// Verify deletion
	_, err = GetDocument(db, "temp/doc")
	if err == nil {
		t.Error("Document should not exist after deletion")
	}
}

func TestDeleteDocument_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	err := DeleteDocument(db, "nonexistent")
	if err == nil {
		t.Error("DeleteDocument should fail for nonexistent document")
	}
}

func TestListDocuments(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create multiple documents
	CreateDocument(db, "pages/home", "home", "text/html", "user-1", false, true)
	CreateDocument(db, "pages/about", "about", "text/html", "user-1", false, true)
	CreateDocument(db, "api/users", "users", "application/json", "user-1", false, true)

	// List all documents
	docs, err := ListDocuments(db, "", 10, 0)
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}

	if len(docs) != 3 {
		t.Errorf("Expected 3 documents, got %d", len(docs))
	}
}

func TestListDocuments_WithPrefix(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create documents
	CreateDocument(db, "pages/home", "home", "text/html", "user-1", false, true)
	CreateDocument(db, "pages/about", "about", "text/html", "user-1", false, true)
	CreateDocument(db, "api/users", "users", "application/json", "user-1", false, true)

	// List with prefix
	docs, err := ListDocuments(db, "pages/", 10, 0)
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("Expected 2 documents with prefix 'pages/', got %d", len(docs))
	}

	for _, doc := range docs {
		if doc.ID != "pages/home" && doc.ID != "pages/about" {
			t.Errorf("Unexpected document ID: %s", doc.ID)
		}
	}
}

func TestListDocuments_Pagination(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create multiple documents
	for i := 1; i <= 5; i++ {
		id := "doc" + string(rune('0'+i))
		CreateDocument(db, id, "content", "text/plain", "user-1", false, true)
	}

	// Get first page
	page1, err := ListDocuments(db, "", 2, 0)
	if err != nil {
		t.Fatalf("ListDocuments page 1 failed: %v", err)
	}

	if len(page1) != 2 {
		t.Errorf("Expected 2 documents in page 1, got %d", len(page1))
	}

	// Get second page
	page2, err := ListDocuments(db, "", 2, 2)
	if err != nil {
		t.Fatalf("ListDocuments page 2 failed: %v", err)
	}

	if len(page2) != 2 {
		t.Errorf("Expected 2 documents in page 2, got %d", len(page2))
	}

	// Verify different documents
	if page1[0].ID == page2[0].ID {
		t.Error("Page 1 and page 2 should have different documents")
	}
}

func TestSearchDocuments(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create documents with searchable content
	CreateDocument(db, "docs/golang", "Golang is a programming language", "text/plain", "user-1", false, true)
	CreateDocument(db, "docs/python", "Python is also a programming language", "text/plain", "user-1", false, true)
	CreateDocument(db, "docs/rust", "Rust is a systems programming language", "text/plain", "user-1", false, true)

	// Search for "golang"
	results, err := SearchDocuments(db, "golang", 10)
	if err != nil {
		t.Fatalf("SearchDocuments failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'golang', got %d", len(results))
	}

	if len(results) > 0 && results[0].ID != "docs/golang" {
		t.Errorf("Expected result 'docs/golang', got %s", results[0].ID)
	}

	// Search for "programming"
	results, err = SearchDocuments(db, "programming", 10)
	if err != nil {
		t.Fatalf("SearchDocuments failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results for 'programming', got %d", len(results))
	}
}

func TestSearchDocuments_NonSearchable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create non-searchable document
	CreateDocument(db, "secret/data", "secret content", "text/plain", "user-1", false, false)

	// Search should not find it
	results, err := SearchDocuments(db, "secret", 10)
	if err != nil {
		t.Fatalf("SearchDocuments failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Non-searchable document should not appear in search results")
	}
}

func TestAddDocumentTag(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create document
	CreateDocument(db, "posts/first", "My first post", "text/plain", "user-1", false, true)

	// Add tag
	err := AddDocumentTag(db, "posts/first", "blog")
	if err != nil {
		t.Fatalf("AddDocumentTag failed: %v", err)
	}

	// Verify tag
	tags, err := GetDocumentTags(db, "posts/first")
	if err != nil {
		t.Fatalf("GetDocumentTags failed: %v", err)
	}

	if len(tags) != 1 {
		t.Errorf("Expected 1 tag, got %d", len(tags))
	}

	if len(tags) > 0 && tags[0] != "blog" {
		t.Errorf("Expected tag 'blog', got %s", tags[0])
	}
}

func TestAddDocumentTag_Multiple(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	CreateDocument(db, "posts/tech", "Tech post", "text/plain", "user-1", false, true)

	// Add multiple tags
	AddDocumentTag(db, "posts/tech", "blog")
	AddDocumentTag(db, "posts/tech", "technology")
	AddDocumentTag(db, "posts/tech", "golang")

	tags, err := GetDocumentTags(db, "posts/tech")
	if err != nil {
		t.Fatalf("GetDocumentTags failed: %v", err)
	}

	if len(tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(tags))
	}
}

func TestAddDocumentTag_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	CreateDocument(db, "posts/dup", "content", "text/plain", "user-1", false, true)

	// Add same tag twice
	AddDocumentTag(db, "posts/dup", "test")
	AddDocumentTag(db, "posts/dup", "test")

	tags, err := GetDocumentTags(db, "posts/dup")
	if err != nil {
		t.Fatalf("GetDocumentTags failed: %v", err)
	}

	// Should only have one tag (UNIQUE constraint)
	if len(tags) != 1 {
		t.Errorf("Expected 1 tag (duplicate ignored), got %d", len(tags))
	}
}

func TestRemoveDocumentTag(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	CreateDocument(db, "posts/remove", "content", "text/plain", "user-1", false, true)
	AddDocumentTag(db, "posts/remove", "tag1")
	AddDocumentTag(db, "posts/remove", "tag2")

	// Remove one tag
	err := RemoveDocumentTag(db, "posts/remove", "tag1")
	if err != nil {
		t.Fatalf("RemoveDocumentTag failed: %v", err)
	}

	tags, err := GetDocumentTags(db, "posts/remove")
	if err != nil {
		t.Fatalf("GetDocumentTags failed: %v", err)
	}

	if len(tags) != 1 {
		t.Errorf("Expected 1 tag after removal, got %d", len(tags))
	}

	if len(tags) > 0 && tags[0] != "tag2" {
		t.Errorf("Expected remaining tag 'tag2', got %s", tags[0])
	}
}

func TestListDocumentsByTag(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create documents with tags
	CreateDocument(db, "posts/post1", "content1", "text/plain", "user-1", false, true)
	CreateDocument(db, "posts/post2", "content2", "text/plain", "user-1", false, true)
	CreateDocument(db, "posts/post3", "content3", "text/plain", "user-1", false, true)

	AddDocumentTag(db, "posts/post1", "golang")
	AddDocumentTag(db, "posts/post2", "golang")
	AddDocumentTag(db, "posts/post3", "python")

	// List documents with "golang" tag
	docs, err := ListDocumentsByTag(db, "golang", 10, 0)
	if err != nil {
		t.Fatalf("ListDocumentsByTag failed: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("Expected 2 documents with tag 'golang', got %d", len(docs))
	}
}

func TestDeleteDocument_CascadeTags(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create document with tags
	CreateDocument(db, "posts/cascade", "content", "text/plain", "user-1", false, true)
	AddDocumentTag(db, "posts/cascade", "tag1")
	AddDocumentTag(db, "posts/cascade", "tag2")

	// Delete document
	err := DeleteDocument(db, "posts/cascade")
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}

	// Verify tags are also deleted (CASCADE)
	tags, err := GetDocumentTags(db, "posts/cascade")
	if err != nil {
		t.Fatalf("GetDocumentTags failed: %v", err)
	}

	if len(tags) != 0 {
		t.Errorf("Expected 0 tags after document deletion, got %d", len(tags))
	}
}
