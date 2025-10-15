package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thetanil/wce/internal/cenv"
)

// Server represents the WCE HTTP server
type Server struct {
	httpServer  *http.Server
	port        int
	cenvManager *cenv.Manager
}

// New creates a new Server instance
func New(port int, cenvManager *cenv.Manager) *Server {
	return &Server{
		port:        port,
		cenvManager: cenvManager,
	}
}

// Start starts the HTTP server with graceful shutdown support
func (s *Server) Start() error {
	// Create router
	mux := http.NewServeMux()

	// Register routes with specific patterns
	// Pattern matching priority: most specific to least specific
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("/new", s.handleNewCenv)
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

// handleNewCenv handles requests to create a new cenv
func (s *Server) handleNewCenv(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"New cenv creation - to be implemented in Phase 3"}`))
}

// handleCenvRequest handles all requests to cenv-specific endpoints
func (s *Server) handleCenvRequest(w http.ResponseWriter, r *http.Request) {
	// Extract cenv ID from path parameter
	cenvID := r.PathValue("cenvID")

	// Validate UUID format
	if !cenv.IsValidUUID(cenvID) {
		http.NotFound(w, r)
		return
	}

	// Get remaining path (if any)
	remainingPath := r.PathValue("path")
	if remainingPath == "" {
		remainingPath = "/"
	} else {
		remainingPath = "/" + remainingPath
	}

	// For now, just return a placeholder response
	// In future phases, this will:
	// 1. Load the cenv database
	// 2. Check authentication
	// 3. Route to appropriate handler
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := fmt.Sprintf(`{"cenv_id":"%s","path":"%s","message":"Cenv handler - to be implemented"}`, cenvID, remainingPath)
	w.Write([]byte(response))
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
