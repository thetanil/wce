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
