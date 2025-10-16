package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/thetanil/wce/internal/cenv"
)

// TestServerIntegration tests the complete server flow
func TestServerIntegration(t *testing.T) {
	// Create temporary storage directory for test
	tmpDir, err := os.MkdirTemp("", "wce-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create cenv manager
	cenvManager := cenv.NewManager(tmpDir)

	// Create server with random port
	srv := New(0, cenvManager)

	// Start server in background
	serverErrors := make(chan error, 1)
	serverReady := make(chan string, 1)

	go func() {
		// Create listener on random port
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			serverErrors <- err
			return
		}

		// Get the actual port assigned
		port := listener.Addr().(*net.TCPAddr).Port
		serverReady <- fmt.Sprintf("http://localhost:%d", port)

		// Create router
		mux := http.NewServeMux()
		mux.HandleFunc("GET /health", srv.handleHealth)
		mux.HandleFunc("/new", srv.handleNewCenv)
		mux.HandleFunc("POST /{cenvID}/login", srv.handleLogin)

		// Document API endpoints
		// Note: Order matters - more specific routes must come first
		mux.HandleFunc("GET /{cenvID}/documents/search", srv.handleSearchDocuments)
		mux.HandleFunc("POST /{cenvID}/documents", srv.handleCreateDocument)
		mux.HandleFunc("GET /{cenvID}/documents/{docID...}", srv.handleGetDocument)
		mux.HandleFunc("PUT /{cenvID}/documents/{docID...}", srv.handleUpdateDocument)
		mux.HandleFunc("DELETE /{cenvID}/documents/{docID...}", srv.handleDeleteDocument)
		mux.HandleFunc("GET /{cenvID}/documents", srv.handleListDocuments)

		mux.HandleFunc("/{cenvID}/{path...}", srv.handleCenvRequest)

		handler := loggingMiddleware(mux)

		// Create HTTP server
		testServer := &http.Server{
			Handler: handler,
		}

		// Serve on the listener
		serverErrors <- testServer.Serve(listener)
	}()

	// Wait for server to be ready or error
	var baseURL string
	select {
	case baseURL = <-serverReady:
		// Server is ready
	case err := <-serverErrors:
		t.Fatalf("Server failed to start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Server took too long to start")
	}

	// Give server a moment to fully initialize
	time.Sleep(100 * time.Millisecond)

	// Run test cases
	t.Run("CreateNewCenv", func(t *testing.T) {
		testCreateNewCenv(t, baseURL)
	})

	t.Run("LoginAndAuthentication", func(t *testing.T) {
		testLoginAndAuthentication(t, baseURL)
	})

	t.Run("AuthenticationErrors", func(t *testing.T) {
		testAuthenticationErrors(t, baseURL)
	})

	t.Run("CompleteFlow", func(t *testing.T) {
		testCompleteFlow(t, baseURL)
	})

	t.Run("DocumentCRUD", func(t *testing.T) {
		testDocumentCRUD(t, baseURL)
	})
}

// testCreateNewCenv tests creating a new cenv
func testCreateNewCenv(t *testing.T, baseURL string) {
	reqBody := map[string]string{
		"username": "testuser",
		"password": "testpass123",
		"email":    "test@example.com",
	}

	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(baseURL+"/new", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Failed to create cenv: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}

	var result NewCenvResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.CenvID == "" {
		t.Error("Expected cenv_id to be set")
	}

	if result.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", result.Username)
	}

	if result.CenvURL == "" {
		t.Error("Expected cenv_url to be set")
	}
}

// testLoginAndAuthentication tests login flow
func testLoginAndAuthentication(t *testing.T, baseURL string) {
	// First create a cenv
	cenvID := createTestCenv(t, baseURL, "loginuser", "loginpass123")

	// Now login
	loginReq := map[string]string{
		"username": "loginuser",
		"password": "loginpass123",
	}

	body, _ := json.Marshal(loginReq)
	resp, err := http.Post(
		fmt.Sprintf("%s/%s/login", baseURL, cenvID),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var loginResult LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResult); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}

	if loginResult.Token == "" {
		t.Error("Expected token to be set")
	}

	if loginResult.Username != "loginuser" {
		t.Errorf("Expected username 'loginuser', got '%s'", loginResult.Username)
	}

	if loginResult.Role != "owner" {
		t.Errorf("Expected role 'owner', got '%s'", loginResult.Role)
	}

	if loginResult.ExpiresAt == 0 {
		t.Error("Expected expires_at to be set")
	}
}

// testAuthenticationErrors tests various authentication error cases
func testAuthenticationErrors(t *testing.T, baseURL string) {
	// Create a cenv for testing
	cenvID := createTestCenv(t, baseURL, "erroruser", "errorpass123")

	t.Run("LoginWithWrongPassword", func(t *testing.T) {
		loginReq := map[string]string{
			"username": "erroruser",
			"password": "wrongpassword",
		}

		body, _ := json.Marshal(loginReq)
		resp, err := http.Post(
			fmt.Sprintf("%s/%s/login", baseURL, cenvID),
			"application/json",
			bytes.NewBuffer(body),
		)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("LoginWithNonexistentUser", func(t *testing.T) {
		loginReq := map[string]string{
			"username": "nonexistent",
			"password": "somepassword",
		}

		body, _ := json.Marshal(loginReq)
		resp, err := http.Post(
			fmt.Sprintf("%s/%s/login", baseURL, cenvID),
			"application/json",
			bytes.NewBuffer(body),
		)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("AccessWithoutToken", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("%s/%s/dashboard", baseURL, cenvID))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("AccessWithInvalidToken", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/%s/dashboard", baseURL, cenvID), nil)
		req.Header.Set("Authorization", "Bearer invalid-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("CreateCenvWithShortPassword", func(t *testing.T) {
		reqBody := map[string]string{
			"username": "shortpass",
			"password": "short",
		}

		body, _ := json.Marshal(reqBody)
		resp, err := http.Post(baseURL+"/new", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})
}

// testCompleteFlow tests the complete authentication flow
func testCompleteFlow(t *testing.T, baseURL string) {
	// Step 1: Create cenv
	reqBody := map[string]string{
		"username": "flowuser",
		"password": "flowpass123",
		"email":    "flow@example.com",
	}

	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(baseURL+"/new", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Failed to create cenv: %v", err)
	}

	var createResult NewCenvResponse
	json.NewDecoder(resp.Body).Decode(&createResult)
	resp.Body.Close()

	cenvID := createResult.CenvID
	t.Logf("Created cenv: %s", cenvID)

	// Step 2: Login
	loginReq := map[string]string{
		"username": "flowuser",
		"password": "flowpass123",
	}

	body, _ = json.Marshal(loginReq)
	resp, err = http.Post(
		fmt.Sprintf("%s/%s/login", baseURL, cenvID),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	var loginResult LoginResponse
	json.NewDecoder(resp.Body).Decode(&loginResult)
	resp.Body.Close()

	token := loginResult.Token
	t.Logf("Got token: %s...", token[:20])

	// Step 3: Make authenticated request
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/%s/dashboard", baseURL, cenvID), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make authenticated request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// Verify response contains expected fields
	if result["cenv_id"] != cenvID {
		t.Errorf("Expected cenv_id %s, got %v", cenvID, result["cenv_id"])
	}

	if result["username"] != "flowuser" {
		t.Errorf("Expected username 'flowuser', got %v", result["username"])
	}

	if result["role"] != "owner" {
		t.Errorf("Expected role 'owner', got %v", result["role"])
	}

	// Step 4: Test different paths with authentication
	paths := []string{"/", "/dashboard", "/settings", "/api/data"}
	for _, path := range paths {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/%s%s", baseURL, cenvID, path), nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to request %s: %v", path, err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 for %s, got %d", path, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Step 5: Verify token from this cenv doesn't work on another cenv
	otherCenvID := createTestCenv(t, baseURL, "otheruser", "otherpass123")

	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/%s/dashboard", baseURL, otherCenvID), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make cross-cenv request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for cross-cenv request, got %d", resp.StatusCode)
	}
}

// createTestCenv is a helper that creates a cenv and returns its ID
func createTestCenv(t *testing.T, baseURL, username, password string) string {
	reqBody := map[string]string{
		"username": username,
		"password": password,
	}

	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(baseURL+"/new", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Failed to create test cenv: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create cenv: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result NewCenvResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode create response: %v", err)
	}

	return result.CenvID
}

// testDocumentCRUD tests complete document CRUD operations via HTTP
func testDocumentCRUD(t *testing.T, baseURL string) {
	// Step 1: Create cenv and login
	cenvID := createTestCenv(t, baseURL, "docuser", "docpass123")

	loginReq := map[string]string{
		"username": "docuser",
		"password": "docpass123",
	}

	body, _ := json.Marshal(loginReq)
	resp, err := http.Post(
		fmt.Sprintf("%s/%s/login", baseURL, cenvID),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	var loginResult LoginResponse
	json.NewDecoder(resp.Body).Decode(&loginResult)
	resp.Body.Close()

	token := loginResult.Token
	t.Logf("Logged in with token: %s...", token[:20])

	// Step 2: Create a document
	createDoc := map[string]interface{}{
		"id":           "test/hello",
		"content":      "Hello World! This is a test document.",
		"content_type": "text/plain",
		"is_binary":    false,
		"searchable":   true,
	}

	body, _ = json.Marshal(createDoc)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/%s/documents", baseURL, cenvID), bytes.NewBuffer(body))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create document: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 201, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var createdDoc map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdDoc)

	if createdDoc["id"] != "test/hello" {
		t.Errorf("Expected document id 'test/hello', got %v", createdDoc["id"])
	}

	if createdDoc["content"] != "Hello World! This is a test document." {
		t.Errorf("Expected content to match, got %v", createdDoc["content"])
	}

	if createdDoc["version"] != float64(1) {
		t.Errorf("Expected version 1, got %v", createdDoc["version"])
	}

	t.Log("Document created successfully")

	// Step 3: Get the document
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/%s/documents/test/hello", baseURL, cenvID), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Accept", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var fetchedDoc map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &fetchedDoc); err != nil {
		t.Fatalf("Failed to decode GET response: %v. Body: %s", err, string(bodyBytes))
	}

	if fetchedDoc["id"] != "test/hello" {
		t.Errorf("Expected document id 'test/hello', got %v. Full response: %+v", fetchedDoc["id"], fetchedDoc)
	}

	if fetchedDoc["content"] != "Hello World! This is a test document." {
		t.Errorf("Expected content to match, got %v", fetchedDoc["content"])
	}

	t.Log("Document retrieved successfully")

	// Step 4: Update the document
	updateDoc := map[string]interface{}{
		"content":      "Updated content for the test document!",
		"content_type": "text/plain",
		"is_binary":    false,
		"searchable":   true,
	}

	body, _ = json.Marshal(updateDoc)
	req, _ = http.NewRequest("PUT", fmt.Sprintf("%s/%s/documents/test/hello", baseURL, cenvID), bytes.NewBuffer(body))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to update document: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var updatedDoc map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&updatedDoc)

	if updatedDoc["content"] != "Updated content for the test document!" {
		t.Errorf("Expected updated content, got %v", updatedDoc["content"])
	}

	if updatedDoc["version"] != float64(2) {
		t.Errorf("Expected version 2 after update, got %v", updatedDoc["version"])
	}

	t.Log("Document updated successfully")

	// Step 5: List documents
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/%s/documents", baseURL, cenvID), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to list documents: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var listResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listResult)

	documents, ok := listResult["documents"].([]interface{})
	if !ok {
		t.Fatalf("Expected documents array in response")
	}

	if len(documents) < 1 {
		t.Error("Expected at least 1 document in list")
	}

	// Find our document in the list
	found := false
	for _, doc := range documents {
		docMap := doc.(map[string]interface{})
		if docMap["id"] == "test/hello" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find 'test/hello' in document list")
	}

	t.Log("Documents listed successfully")

	// Step 6: Search documents (FTS5)
	createSearchDoc := map[string]interface{}{
		"id":           "test/searchable",
		"content":      "Unique keyword: xylophone banana elephant",
		"content_type": "text/plain",
		"is_binary":    false,
		"searchable":   true,
	}

	body, _ = json.Marshal(createSearchDoc)
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/%s/documents", baseURL, cenvID), bytes.NewBuffer(body))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create searchable document: %v", err)
	}
	resp.Body.Close()

	// Now search for it
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/%s/documents/search?q=xylophone", baseURL, cenvID), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to search documents: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200 for search, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var searchResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&searchResult)

	searchDocs, ok := searchResult["results"].([]interface{})
	if !ok {
		t.Fatalf("Expected results array in search response")
	}

	if len(searchDocs) < 1 {
		t.Error("Expected at least 1 document in search results")
	}

	// Verify the searchable document is in results
	foundSearch := false
	for _, doc := range searchDocs {
		docMap := doc.(map[string]interface{})
		if docMap["id"] == "test/searchable" {
			foundSearch = true
			break
		}
	}

	if !foundSearch {
		t.Error("Expected to find 'test/searchable' in search results for 'xylophone'")
	}

	t.Log("Document search (FTS5) successful")

	// Step 7: Delete the document
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("%s/%s/documents/test/hello", baseURL, cenvID), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to delete document: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200/204, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	t.Log("Document deleted successfully")

	// Step 8: Verify document is gone
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/%s/documents/test/hello", baseURL, cenvID), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Accept", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to verify deletion: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404 after deletion, got %d", resp.StatusCode)
	}

	t.Log("Verified document was deleted")
}
