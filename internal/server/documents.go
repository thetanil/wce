package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/thetanil/wce/internal/authz"
	"github.com/thetanil/wce/internal/cenv"
	"github.com/thetanil/wce/internal/document"
)

// handleCreateDocument creates a new document
func (s *Server) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
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

	// Check write permission to _wce_documents table
	canWrite, err := authz.CanWrite(db, userID, role, "_wce_documents")
	if err != nil || !canWrite {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "permission denied: cannot write documents",
		})
		return
	}

	// Parse request
	var req struct {
		ID          string `json:"id"`
		Content     string `json:"content"`
		ContentType string `json:"content_type"`
		IsBinary    bool   `json:"is_binary"`
		Searchable  bool   `json:"searchable"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Create document
	doc, err := document.CreateDocument(db, req.ID, req.Content, req.ContentType, userID, req.IsBinary, req.Searchable)
	if err != nil {
		// Check if error is due to duplicate
		if strings.Contains(err.Error(), "already exists") {
			w.WriteHeader(http.StatusConflict)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(doc)
}

// handleGetDocument retrieves a document
func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cenvID := r.PathValue("cenvID")
	docID := r.PathValue("docID")

	if !cenv.IsValidUUID(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid cenv id"})
		return
	}

	userID, role, db, err := s.requireAuth(w, r, cenvID)
	if err != nil {
		return // Response already sent
	}

	// Check read permission
	canRead, err := authz.CanRead(db, userID, role, "_wce_documents")
	if err != nil || !canRead {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "permission denied: cannot read documents",
		})
		return
	}

	// Get document
	doc, err := document.GetDocument(db, docID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Check if content type should be returned as raw
	acceptHeader := r.Header.Get("Accept")
	if acceptHeader == doc.ContentType || acceptHeader == "*/*" {
		// Return raw content with proper content type
		w.Header().Set("Content-Type", doc.ContentType)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(doc.Content))
		return
	}

	// Return as JSON
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(doc)
}

// handleUpdateDocument updates an existing document
func (s *Server) handleUpdateDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cenvID := r.PathValue("cenvID")
	docID := r.PathValue("docID")

	if !cenv.IsValidUUID(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid cenv id"})
		return
	}

	userID, role, db, err := s.requireAuth(w, r, cenvID)
	if err != nil {
		return // Response already sent
	}

	// Check write permission
	canWrite, err := authz.CanWrite(db, userID, role, "_wce_documents")
	if err != nil || !canWrite {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "permission denied: cannot write documents",
		})
		return
	}

	// Parse request
	var req struct {
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Update document
	doc, err := document.UpdateDocument(db, docID, req.Content, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(doc)
}

// handleDeleteDocument deletes a document
func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cenvID := r.PathValue("cenvID")
	docID := r.PathValue("docID")

	if !cenv.IsValidUUID(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid cenv id"})
		return
	}

	userID, role, db, err := s.requireAuth(w, r, cenvID)
	if err != nil {
		return // Response already sent
	}

	// Check delete permission
	canDelete, err := authz.CanDelete(db, userID, role, "_wce_documents")
	if err != nil || !canDelete {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "permission denied: cannot delete documents",
		})
		return
	}

	// Delete document
	err = document.DeleteDocument(db, docID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "document deleted successfully",
	})
}

// handleListDocuments lists documents with optional filtering
func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
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

	// Check read permission
	canRead, err := authz.CanRead(db, userID, role, "_wce_documents")
	if err != nil || !canRead {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "permission denied: cannot read documents",
		})
		return
	}

	// Parse query parameters
	prefix := r.URL.Query().Get("prefix")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	// List documents
	docs, err := document.ListDocuments(db, prefix, limit, offset)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"documents": docs,
		"count":     len(docs),
	})
}

// handleSearchDocuments searches documents using FTS5
func (s *Server) handleSearchDocuments(w http.ResponseWriter, r *http.Request) {
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

	// Check read permission
	canRead, err := authz.CanRead(db, userID, role, "_wce_documents")
	if err != nil || !canRead {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "permission denied: cannot read documents",
		})
		return
	}

	// Get search query
	query := r.URL.Query().Get("q")
	if query == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "query parameter 'q' is required",
		})
		return
	}

	// Parse limit
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	// Search documents
	results, err := document.SearchDocuments(db, query, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
		"count":   len(results),
	})
}
