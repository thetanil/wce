package document

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// Document represents a stored document
type Document struct {
	ID          string   `json:"id"`
	Content     string   `json:"content"`
	ContentType string   `json:"content_type"`
	IsBinary    bool     `json:"is_binary"`
	Searchable  bool     `json:"searchable"`
	CreatedAt   int64    `json:"created_at"`
	ModifiedAt  int64    `json:"modified_at"`
	CreatedBy   string   `json:"created_by"`
	ModifiedBy  string   `json:"modified_by"`
	Version     int      `json:"version"`
	Tags        []string `json:"tags,omitempty"`
}

// SearchResult represents a search result with ranking
type SearchResult struct {
	Document
	Rank float64 `json:"rank"`
}

// CreateDocument creates a new document in the database
func CreateDocument(db *sql.DB, id, content, contentType, userID string, isBinary, searchable bool) (*Document, error) {
	// Validate inputs
	if id == "" {
		return nil, fmt.Errorf("document id cannot be empty")
	}
	if content == "" {
		return nil, fmt.Errorf("document content cannot be empty")
	}
	if contentType == "" {
		return nil, fmt.Errorf("content type cannot be empty")
	}
	if userID == "" {
		return nil, fmt.Errorf("user id cannot be empty")
	}

	// Check if document already exists
	var exists bool
	err := db.QueryRow("SELECT 1 FROM _wce_documents WHERE id = ?", id).Scan(&exists)
	if err != sql.ErrNoRows {
		if err == nil {
			return nil, fmt.Errorf("document with id %s already exists", id)
		}
		return nil, fmt.Errorf("failed to check document existence: %w", err)
	}

	// If binary, ensure content is base64 encoded
	finalContent := content
	if isBinary {
		// Verify it's valid base64
		_, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("binary content must be base64 encoded: %w", err)
		}
		finalContent = content
	}

	now := time.Now().Unix()

	// Insert document
	_, err = db.Exec(`
		INSERT INTO _wce_documents (
			id, content, content_type, is_binary, searchable,
			created_at, modified_at, created_by, modified_by, version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
	`, id, finalContent, contentType, boolToInt(isBinary), boolToInt(searchable),
		now, now, userID, userID)

	if err != nil {
		return nil, fmt.Errorf("failed to insert document: %w", err)
	}

	return &Document{
		ID:          id,
		Content:     finalContent,
		ContentType: contentType,
		IsBinary:    isBinary,
		Searchable:  searchable,
		CreatedAt:   now,
		ModifiedAt:  now,
		CreatedBy:   userID,
		ModifiedBy:  userID,
		Version:     1,
	}, nil
}

// GetDocument retrieves a document by ID
func GetDocument(db *sql.DB, id string) (*Document, error) {
	if id == "" {
		return nil, fmt.Errorf("document id cannot be empty")
	}

	var doc Document
	var isBinaryInt, searchableInt int

	err := db.QueryRow(`
		SELECT id, content, content_type, is_binary, searchable,
		       created_at, modified_at, created_by, modified_by, version
		FROM _wce_documents
		WHERE id = ?
	`, id).Scan(
		&doc.ID, &doc.Content, &doc.ContentType, &isBinaryInt, &searchableInt,
		&doc.CreatedAt, &doc.ModifiedAt, &doc.CreatedBy, &doc.ModifiedBy, &doc.Version,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("document not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query document: %w", err)
	}

	doc.IsBinary = isBinaryInt == 1
	doc.Searchable = searchableInt == 1

	// Load tags
	tags, err := GetDocumentTags(db, id)
	if err == nil {
		doc.Tags = tags
	}

	return &doc, nil
}

// UpdateDocument updates an existing document
func UpdateDocument(db *sql.DB, id, content, userID string) (*Document, error) {
	if id == "" {
		return nil, fmt.Errorf("document id cannot be empty")
	}
	if content == "" {
		return nil, fmt.Errorf("document content cannot be empty")
	}
	if userID == "" {
		return nil, fmt.Errorf("user id cannot be empty")
	}

	// Get existing document to verify it exists and check if binary
	existing, err := GetDocument(db, id)
	if err != nil {
		return nil, err
	}

	// If binary, verify base64 encoding
	if existing.IsBinary {
		_, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("binary content must be base64 encoded: %w", err)
		}
	}

	now := time.Now().Unix()
	newVersion := existing.Version + 1

	// Update document
	result, err := db.Exec(`
		UPDATE _wce_documents
		SET content = ?, modified_at = ?, modified_by = ?, version = ?
		WHERE id = ?
	`, content, now, userID, newVersion, id)

	if err != nil {
		return nil, fmt.Errorf("failed to update document: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("document not found: %s", id)
	}

	// Return updated document
	existing.Content = content
	existing.ModifiedAt = now
	existing.ModifiedBy = userID
	existing.Version = newVersion

	return existing, nil
}

// DeleteDocument removes a document from the database
func DeleteDocument(db *sql.DB, id string) error {
	if id == "" {
		return fmt.Errorf("document id cannot be empty")
	}

	result, err := db.Exec("DELETE FROM _wce_documents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("document not found: %s", id)
	}

	return nil
}

// ListDocuments lists documents with optional prefix filter and pagination
func ListDocuments(db *sql.DB, prefix string, limit, offset int) ([]Document, error) {
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 1000 {
		limit = 1000 // Max limit
	}
	if offset < 0 {
		offset = 0
	}

	var rows *sql.Rows
	var err error

	if prefix != "" {
		// List documents with prefix
		rows, err = db.Query(`
			SELECT id, content, content_type, is_binary, searchable,
			       created_at, modified_at, created_by, modified_by, version
			FROM _wce_documents
			WHERE id LIKE ? || '%'
			ORDER BY id
			LIMIT ? OFFSET ?
		`, prefix, limit, offset)
	} else {
		// List all documents
		rows, err = db.Query(`
			SELECT id, content, content_type, is_binary, searchable,
			       created_at, modified_at, created_by, modified_by, version
			FROM _wce_documents
			ORDER BY id
			LIMIT ? OFFSET ?
		`, limit, offset)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query documents: %w", err)
	}
	defer rows.Close()

	var documents []Document
	for rows.Next() {
		var doc Document
		var isBinaryInt, searchableInt int

		err := rows.Scan(
			&doc.ID, &doc.Content, &doc.ContentType, &isBinaryInt, &searchableInt,
			&doc.CreatedAt, &doc.ModifiedAt, &doc.CreatedBy, &doc.ModifiedBy, &doc.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan document: %w", err)
		}

		doc.IsBinary = isBinaryInt == 1
		doc.Searchable = searchableInt == 1

		documents = append(documents, doc)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating documents: %w", err)
	}

	return documents, nil
}

// SearchDocuments performs full-text search on documents
func SearchDocuments(db *sql.DB, query string, limit int) ([]SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit for search
	}

	// Use FTS5 for full-text search
	rows, err := db.Query(`
		SELECT d.id, d.content, d.content_type, d.is_binary, d.searchable,
		       d.created_at, d.modified_at, d.created_by, d.modified_by, d.version,
		       s.rank
		FROM _wce_document_search s
		JOIN _wce_documents d ON d.id = s.document_id
		WHERE _wce_document_search MATCH ?
		ORDER BY s.rank
		LIMIT ?
	`, query, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to search documents: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		var isBinaryInt, searchableInt int

		err := rows.Scan(
			&result.ID, &result.Content, &result.ContentType, &isBinaryInt, &searchableInt,
			&result.CreatedAt, &result.ModifiedAt, &result.CreatedBy, &result.ModifiedBy,
			&result.Version, &result.Rank,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}

		result.IsBinary = isBinaryInt == 1
		result.Searchable = searchableInt == 1

		results = append(results, result)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return results, nil
}

// AddDocumentTag adds a tag to a document
func AddDocumentTag(db *sql.DB, documentID, tag string) error {
	if documentID == "" {
		return fmt.Errorf("document id cannot be empty")
	}
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}

	// Normalize tag (lowercase, trim)
	tag = strings.ToLower(strings.TrimSpace(tag))

	_, err := db.Exec(`
		INSERT OR IGNORE INTO _wce_document_tags (document_id, tag)
		VALUES (?, ?)
	`, documentID, tag)

	if err != nil {
		return fmt.Errorf("failed to add tag: %w", err)
	}

	return nil
}

// RemoveDocumentTag removes a tag from a document
func RemoveDocumentTag(db *sql.DB, documentID, tag string) error {
	if documentID == "" {
		return fmt.Errorf("document id cannot be empty")
	}
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}

	tag = strings.ToLower(strings.TrimSpace(tag))

	_, err := db.Exec(`
		DELETE FROM _wce_document_tags
		WHERE document_id = ? AND tag = ?
	`, documentID, tag)

	if err != nil {
		return fmt.Errorf("failed to remove tag: %w", err)
	}

	return nil
}

// GetDocumentTags retrieves all tags for a document
func GetDocumentTags(db *sql.DB, documentID string) ([]string, error) {
	if documentID == "" {
		return nil, fmt.Errorf("document id cannot be empty")
	}

	rows, err := db.Query(`
		SELECT tag FROM _wce_document_tags
		WHERE document_id = ?
		ORDER BY tag
	`, documentID)

	if err != nil {
		return nil, fmt.Errorf("failed to query tags: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		tags = append(tags, tag)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tags: %w", err)
	}

	return tags, nil
}

// ListDocumentsByTag lists all documents with a specific tag
func ListDocumentsByTag(db *sql.DB, tag string, limit, offset int) ([]Document, error) {
	if tag == "" {
		return nil, fmt.Errorf("tag cannot be empty")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	tag = strings.ToLower(strings.TrimSpace(tag))

	rows, err := db.Query(`
		SELECT d.id, d.content, d.content_type, d.is_binary, d.searchable,
		       d.created_at, d.modified_at, d.created_by, d.modified_by, d.version
		FROM _wce_documents d
		JOIN _wce_document_tags t ON d.id = t.document_id
		WHERE t.tag = ?
		ORDER BY d.id
		LIMIT ? OFFSET ?
	`, tag, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("failed to query documents by tag: %w", err)
	}
	defer rows.Close()

	var documents []Document
	for rows.Next() {
		var doc Document
		var isBinaryInt, searchableInt int

		err := rows.Scan(
			&doc.ID, &doc.Content, &doc.ContentType, &isBinaryInt, &searchableInt,
			&doc.CreatedAt, &doc.ModifiedAt, &doc.CreatedBy, &doc.ModifiedBy, &doc.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan document: %w", err)
		}

		doc.IsBinary = isBinaryInt == 1
		doc.Searchable = searchableInt == 1

		documents = append(documents, doc)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating documents: %w", err)
	}

	return documents, nil
}

// boolToInt converts bool to integer for SQLite
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
