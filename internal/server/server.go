package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/thetanil/wce/internal/auth"
	"github.com/thetanil/wce/internal/cenv"
)

// Server represents the WCE HTTP server
type Server struct {
	httpServer  *http.Server
	port        int
	cenvManager *cenv.Manager
	jwtManager  *auth.JWTManager
	jwtSecret   string
}

// New creates a new Server instance
func New(port int, cenvManager *cenv.Manager) *Server {
	// Generate a random JWT secret if not provided
	// In production, this should be loaded from environment or config
	jwtSecret := generateRandomSecret()

	return &Server{
		port:        port,
		cenvManager: cenvManager,
		jwtManager:  auth.NewJWTManager(jwtSecret),
		jwtSecret:   jwtSecret,
	}
}

// generateRandomSecret generates a random secret for JWT signing
func generateRandomSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return fmt.Sprintf("%x", bytes)
}

// Start starts the HTTP server with graceful shutdown support
func (s *Server) Start() error {
	// Create router
	mux := http.NewServeMux()

	// Register routes with specific patterns
	// Pattern matching priority: most specific to least specific
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("/new", s.handleNewCenv)
	mux.HandleFunc("POST /{cenvID}/login", s.handleLogin)

	// Permission management endpoints (admin only)
	mux.HandleFunc("GET /{cenvID}/admin/permissions", s.handleListPermissions)
	mux.HandleFunc("POST /{cenvID}/admin/permissions", s.handleGrantPermission)
	mux.HandleFunc("DELETE /{cenvID}/admin/permissions", s.handleRevokePermission)
	mux.HandleFunc("GET /{cenvID}/admin/policies", s.handleListPolicies)
	mux.HandleFunc("POST /{cenvID}/admin/policies", s.handleCreatePolicy)

	// Document API endpoints
	// Note: Order matters - more specific routes must come first
	// The {docID...} pattern captures paths with slashes (e.g., "pages/home", "api/users/list")
	mux.HandleFunc("GET /{cenvID}/documents/search", s.handleSearchDocuments)
	mux.HandleFunc("POST /{cenvID}/documents", s.handleCreateDocument)
	mux.HandleFunc("GET /{cenvID}/documents/{docID...}", s.handleGetDocument)
	mux.HandleFunc("PUT /{cenvID}/documents/{docID...}", s.handleUpdateDocument)
	mux.HandleFunc("DELETE /{cenvID}/documents/{docID...}", s.handleDeleteDocument)
	mux.HandleFunc("GET /{cenvID}/documents", s.handleListDocuments)

	// Match both /{cenvID}/ and /{cenvID}/path/to/resource
	mux.HandleFunc("/{cenvID}/{path...}", s.handleCenvRequest)

	// Wrap with logging middleware
	handler := loggingMiddleware(mux)

	// Configure HTTP server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Channel to listen for errors coming from the listener
	serverErrors := make(chan error, 1)

	// Start the server
	go func() {
		log.Printf("Starting WCE server on http://localhost:%d", s.port)
		serverErrors <- s.httpServer.ListenAndServe()
	}()

	// Channel to listen for interrupt signal to terminate
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until we receive a signal or an error
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		log.Printf("Received signal %v, starting graceful shutdown", sig)

		// Give outstanding requests 30 seconds to complete
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := s.httpServer.Shutdown(ctx); err != nil {
			// Force close if graceful shutdown fails
			s.httpServer.Close()
			return fmt.Errorf("could not gracefully shutdown server: %w", err)
		}

		log.Println("Server stopped gracefully")
	}

	return nil
}

// handleHealth handles the health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"wce"}`))
}

// NewCenvRequest represents the request body for creating a new cenv
type NewCenvRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
}

// NewCenvResponse represents the response for a new cenv creation
type NewCenvResponse struct {
	CenvID   string `json:"cenv_id"`
	CenvURL  string `json:"cenv_url"`
	Username string `json:"username"`
	Message  string `json:"message"`
}

// handleNewCenv handles requests to create a new cenv
func (s *Server) handleNewCenv(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Only accept POST requests
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "method not allowed, use POST",
		})
		return
	}

	// Parse request body
	var req NewCenvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Validate inputs
	if req.Username == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "username is required",
		})
		return
	}

	if req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "password is required",
		})
		return
	}

	if len(req.Password) < 8 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "password must be at least 8 characters",
		})
		return
	}

	// Generate a new cenv ID
	cenvID, err := auth.GenerateUUID()
	if err != nil {
		log.Printf("Failed to generate cenv ID: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to generate cenv ID",
		})
		return
	}

	// Create the cenv database
	if err := s.cenvManager.Create(cenvID); err != nil {
		log.Printf("Failed to create cenv %s: %v", cenvID, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to create cenv",
		})
		return
	}

	// Get database connection
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		log.Printf("Failed to connect to cenv %s: %v", cenvID, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to connect to cenv database",
		})
		return
	}

	// Create the owner user
	user, err := auth.CreateUser(db, req.Username, req.Password, auth.RoleOwner, req.Email, "")
	if err != nil {
		log.Printf("Failed to create owner user for cenv %s: %v", cenvID, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to create user",
		})
		return
	}

	log.Printf("Created new cenv %s with owner %s (%s)", cenvID, user.Username, user.UserID)

	// Build the cenv URL
	host := r.Host
	if host == "" {
		host = "localhost:5309"
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	cenvURL := fmt.Sprintf("%s://%s/%s/", scheme, host, cenvID)

	// Return success response
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(NewCenvResponse{
		CenvID:   cenvID,
		CenvURL:  cenvURL,
		Username: user.Username,
		Message:  "Cenv created successfully. Please login to access your environment.",
	})
}

// LoginRequest represents the request body for login
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents the response for a successful login
type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
}

// handleLogin handles user login requests
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract cenv ID from path
	cenvID := r.PathValue("cenvID")

	// Validate cenv ID
	if !cenv.IsValidUUID(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "cenv not found",
		})
		return
	}

	// Check if cenv exists
	if !s.cenvManager.Exists(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "cenv not found",
		})
		return
	}

	// Parse request body
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Validate inputs
	if req.Username == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "username and password are required",
		})
		return
	}

	// Get database connection
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		log.Printf("Failed to connect to cenv %s: %v", cenvID, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to connect to database",
		})
		return
	}

	// Get user by username
	user, err := auth.GetUserByUsername(db, req.Username)
	if err != nil {
		// Don't reveal whether user exists or not
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid credentials",
		})
		return
	}

	// Check if user is enabled
	if !user.Enabled {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "account disabled",
		})
		return
	}

	// Verify password
	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid credentials",
		})
		return
	}

	// Generate session ID
	sessionID, err := auth.GenerateSessionID()
	if err != nil {
		log.Printf("Failed to generate session ID: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to create session",
		})
		return
	}

	// Generate JWT token
	expiresIn := auth.DefaultSessionTimeout
	token, err := s.jwtManager.GenerateToken(user.UserID, user.Username, cenvID, user.Role, sessionID, expiresIn)
	if err != nil {
		log.Printf("Failed to generate token: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to generate token",
		})
		return
	}

	// Get token hash for storage
	tokenHash := auth.GetTokenHash(token)

	// Get client info
	ipAddress := r.RemoteAddr
	userAgent := r.UserAgent()

	// Create session record
	_, err = auth.CreateSession(db, user.UserID, tokenHash, ipAddress, userAgent, expiresIn)
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to create session",
		})
		return
	}

	// Update last login timestamp
	if err := auth.UpdateLastLogin(db, user.UserID); err != nil {
		log.Printf("Failed to update last login for user %s: %v", user.UserID, err)
		// Don't fail the request for this
	}

	log.Printf("User %s logged in to cenv %s", user.Username, cenvID)

	// Return success response with token
	expiresAt := time.Now().Add(expiresIn).Unix()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		UserID:    user.UserID,
		Username:  user.Username,
		Role:      user.Role,
	})
}

// handleCenvRequest handles all requests to cenv-specific endpoints
func (s *Server) handleCenvRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract cenv ID from path parameter
	cenvID := r.PathValue("cenvID")

	// Validate UUID format
	if !cenv.IsValidUUID(cenvID) {
		http.NotFound(w, r)
		return
	}

	// Check if cenv exists
	if !s.cenvManager.Exists(cenvID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "cenv not found",
		})
		return
	}

	// Get database connection
	db, err := s.cenvManager.GetConnection(cenvID)
	if err != nil {
		log.Printf("Failed to connect to cenv %s: %v", cenvID, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to connect to database",
		})
		return
	}

	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "missing authorization header",
		})
		return
	}

	// Check for Bearer token format
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid authorization header format",
		})
		return
	}

	token := parts[1]

	// Validate token
	claims, err := s.jwtManager.ValidateToken(token)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid token",
		})
		return
	}

	// Verify token is for this cenv
	if claims.CenvID != cenvID {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "token not valid for this cenv",
		})
		return
	}

	// Check if session is still valid (not revoked)
	tokenHash := auth.GetTokenHash(token)
	valid, err := auth.IsSessionValid(db, tokenHash)
	if err != nil {
		log.Printf("Failed to validate session: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to validate session",
		})
		return
	}

	if !valid {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "session expired or revoked",
		})
		return
	}

	// Get remaining path (if any)
	remainingPath := r.PathValue("path")
	if remainingPath == "" {
		remainingPath = "/"
	} else {
		remainingPath = "/" + remainingPath
	}

	// Request is authenticated - for now return a placeholder
	// In future phases, this will route to appropriate handlers
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cenv_id":  cenvID,
		"path":     remainingPath,
		"user_id":  claims.UserID,
		"username": claims.Username,
		"role":     claims.Role,
		"message":  "Authenticated request - handler to be implemented",
	})
}

// loggingMiddleware logs all HTTP requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Call the next handler
		next.ServeHTTP(wrapped, r)

		// Log the request
		log.Printf(
			"%s %s %d %s",
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			time.Since(start),
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
