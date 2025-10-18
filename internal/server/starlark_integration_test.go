package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thetanil/wce/internal/cenv"
	_ "github.com/mattn/go-sqlite3"
)

// TestStarlarkIntegration tests the full Starlark endpoint flow
func TestStarlarkIntegration(t *testing.T) {
	// Create a temporary cenv manager for testing
	tempDir := t.TempDir()
	manager := cenv.NewManager(tempDir)

	// Create a test server
	srv := New(5309, manager)

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("POST /new", srv.handleNewCenv)
	mux.HandleFunc("POST /{cenvID}/login", srv.handleLogin)
	mux.HandleFunc("POST /{cenvID}/admin/endpoints", srv.handleCreateEndpoint)
	mux.HandleFunc("GET /{cenvID}/admin/endpoints", srv.handleListEndpoints)
	mux.HandleFunc("GET /{cenvID}/admin/endpoints/{endpointID}", srv.handleGetEndpoint)
	mux.HandleFunc("DELETE /{cenvID}/admin/endpoints/{endpointID}", srv.handleDeleteEndpoint)
	mux.HandleFunc("/{cenvID}/star/{starPath...}", srv.handleExecuteStarlarkEndpoint)

	// Test: Create a new cenv
	t.Run("CreateCenv", func(t *testing.T) {
		body := map[string]string{
			"username": "admin",
			"password": "adminpass123",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/new", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp["cenv_id"] == nil {
			t.Fatal("Expected cenv_id in response")
		}
	})

	// Get the cenv ID from the first test
	cenvID := ""
	{
		body := map[string]string{
			"username": "admin",
			"password": "adminpass123",
		}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/new", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)
		cenvID = resp["cenv_id"].(string)
	}

	// Login to get token
	var token string
	t.Run("Login", func(t *testing.T) {
		body := map[string]string{
			"username": "admin",
			"password": "adminpass123",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/"+cenvID+"/login", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		token = resp["token"].(string)
		if token == "" {
			t.Fatal("Expected token in response")
		}
	})

	// Create a simple Starlark endpoint
	t.Run("CreateSimpleEndpoint", func(t *testing.T) {
		endpoint := map[string]interface{}{
			"path":        "/hello",
			"method":      "GET",
			"script":      "def handle_request(req):\n    return response({\"message\": \"Hello from Starlark!\"})",
			"description": "Simple hello endpoint",
		}
		bodyBytes, _ := json.Marshal(endpoint)

		req := httptest.NewRequest("POST", "/"+cenvID+"/admin/endpoints", bytes.NewReader(bodyBytes))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Execute the Starlark endpoint
	t.Run("ExecuteSimpleEndpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/"+cenvID+"/star/hello", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp["message"] != "Hello from Starlark!" {
			t.Errorf("Expected message 'Hello from Starlark!', got %v", resp["message"])
		}
	})

	// Create endpoint with database access
	t.Run("CreateDatabaseEndpoint", func(t *testing.T) {
		// First create a test table
		db, err := manager.GetConnection(cenvID)
		if err != nil {
			t.Fatalf("Failed to get database: %v", err)
		}
		_, err = db.Exec("CREATE TABLE test_data (id INTEGER PRIMARY KEY, value TEXT)")
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}
		_, err = db.Exec("INSERT INTO test_data (value) VALUES ('test1'), ('test2')")
		if err != nil {
			t.Fatalf("Failed to insert data: %v", err)
		}

		endpoint := map[string]interface{}{
			"path":   "/data",
			"method": "GET",
			"script": `def handle_request(req):
    rows = db.query("SELECT id, value FROM test_data", [])
    return response({"data": rows})`,
			"description": "Database query endpoint",
		}
		bodyBytes, _ := json.Marshal(endpoint)

		req := httptest.NewRequest("POST", "/"+cenvID+"/admin/endpoints", bytes.NewReader(bodyBytes))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Execute the database endpoint
	t.Run("ExecuteDatabaseEndpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/"+cenvID+"/star/data", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		data, ok := resp["data"].([]interface{})
		if !ok {
			t.Fatalf("Expected data to be array, got %T", resp["data"])
		}

		if len(data) != 2 {
			t.Errorf("Expected 2 rows, got %d", len(data))
		}
	})

	// List all endpoints
	t.Run("ListEndpoints", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/"+cenvID+"/admin/endpoints", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var endpoints []Endpoint
		if err := json.NewDecoder(w.Body).Decode(&endpoints); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(endpoints) != 2 {
			t.Errorf("Expected 2 endpoints, got %d", len(endpoints))
		}

		// Check that script is not included in list
		for _, ep := range endpoints {
			if ep.Script != "" {
				t.Error("Script should not be included in list response")
			}
		}
	})

	// Get a specific endpoint
	t.Run("GetEndpoint", func(t *testing.T) {
		// First list to get an ID
		req := httptest.NewRequest("GET", "/"+cenvID+"/admin/endpoints", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		var endpoints []Endpoint
		json.NewDecoder(w.Body).Decode(&endpoints)
		if len(endpoints) == 0 {
			t.Fatal("No endpoints found")
		}

		endpointID := endpoints[0].ID

		// Get the specific endpoint
		req = httptest.NewRequest("GET", "/"+cenvID+"/admin/endpoints/"+fmt.Sprint(endpointID), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var endpoint Endpoint
		if err := json.NewDecoder(w.Body).Decode(&endpoint); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Script should be included in get response
		if endpoint.Script == "" {
			t.Error("Script should be included in get response")
		}
	})

	// Test endpoint with custom status code
	t.Run("CustomStatusCode", func(t *testing.T) {
		endpoint := map[string]interface{}{
			"path":   "/notfound",
			"method": "GET",
			"script": `def handle_request(req):
    return response({"error": "Not found"}, status=404)`,
		}
		bodyBytes, _ := json.Marshal(endpoint)

		req := httptest.NewRequest("POST", "/"+cenvID+"/admin/endpoints", bytes.NewReader(bodyBytes))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d: %s", w.Code, w.Body.String())
		}

		// Execute the endpoint
		req = httptest.NewRequest("GET", "/"+cenvID+"/star/notfound", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
		}
	})

	// Test accessing request data
	t.Run("AccessRequestData", func(t *testing.T) {
		endpoint := map[string]interface{}{
			"path":   "/echo",
			"method": "GET",
			"script": `def handle_request(req):
    return response({
        "method": req.method,
        "path": req.path,
        "user_id": req.user["id"]
    })`,
		}
		bodyBytes, _ := json.Marshal(endpoint)

		req := httptest.NewRequest("POST", "/"+cenvID+"/admin/endpoints", bytes.NewReader(bodyBytes))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// Execute
		req = httptest.NewRequest("GET", "/"+cenvID+"/star/echo", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)

		if resp["method"] != "GET" {
			t.Errorf("Expected method GET, got %v", resp["method"])
		}
	})

	// Test delete endpoint
	t.Run("DeleteEndpoint", func(t *testing.T) {
		// Get an endpoint ID
		req := httptest.NewRequest("GET", "/"+cenvID+"/admin/endpoints", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		var endpoints []Endpoint
		json.NewDecoder(w.Body).Decode(&endpoints)
		if len(endpoints) == 0 {
			t.Fatal("No endpoints found")
		}

		endpointID := endpoints[0].ID

		// Delete it
		req = httptest.NewRequest("DELETE", "/"+cenvID+"/admin/endpoints/"+fmt.Sprint(endpointID), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		// Verify it's deleted
		req = httptest.NewRequest("GET", "/"+cenvID+"/admin/endpoints", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		var endpointsAfter []Endpoint
		json.NewDecoder(w.Body).Decode(&endpointsAfter)

		if len(endpointsAfter) >= len(endpoints) {
			t.Error("Endpoint was not deleted")
		}
	})
}
