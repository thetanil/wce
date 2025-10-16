package auth

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateToken(t *testing.T) {
	jwtManager := NewJWTManager("test-secret-key")

	token, err := jwtManager.GenerateToken("user-123", "testuser", "cenv-456", RoleAdmin, "session-789", 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Token should have 3 parts separated by dots
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("Expected token to have 3 parts, got %d", len(parts))
	}
}

func TestValidateToken(t *testing.T) {
	jwtManager := NewJWTManager("test-secret-key")

	// Generate a token
	userID := "user-123"
	username := "testuser"
	cenvID := "cenv-456"
	role := RoleAdmin
	sessionID := "session-789"

	token, err := jwtManager.GenerateToken(userID, username, cenvID, role, sessionID, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Validate the token
	claims, err := jwtManager.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	// Check claims
	if claims.UserID != userID {
		t.Errorf("Expected UserID %s, got %s", userID, claims.UserID)
	}
	if claims.Username != username {
		t.Errorf("Expected Username %s, got %s", username, claims.Username)
	}
	if claims.CenvID != cenvID {
		t.Errorf("Expected CenvID %s, got %s", cenvID, claims.CenvID)
	}
	if claims.Role != role {
		t.Errorf("Expected Role %s, got %s", role, claims.Role)
	}
	if claims.SessionID != sessionID {
		t.Errorf("Expected SessionID %s, got %s", sessionID, claims.SessionID)
	}
}

func TestValidateToken_Invalid(t *testing.T) {
	jwtManager := NewJWTManager("test-secret-key")

	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"invalid format", "not.a.valid.token"},
		{"tampered token", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoidGFtcGVyZWQifQ.invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := jwtManager.ValidateToken(tt.token)
			if err == nil {
				t.Error("Expected validation to fail, but it succeeded")
			}
		})
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	jwtManager1 := NewJWTManager("secret-1")
	jwtManager2 := NewJWTManager("secret-2")

	// Generate token with first manager
	token, err := jwtManager1.GenerateToken("user-123", "testuser", "cenv-456", RoleAdmin, "session-789", 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Try to validate with second manager (different secret)
	_, err = jwtManager2.ValidateToken(token)
	if err == nil {
		t.Error("Expected validation to fail with wrong secret, but it succeeded")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	jwtManager := NewJWTManager("test-secret-key")

	// Generate token that expires immediately
	token, err := jwtManager.GenerateToken("user-123", "testuser", "cenv-456", RoleAdmin, "session-789", -1*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Try to validate expired token
	_, err = jwtManager.ValidateToken(token)
	if err == nil {
		t.Error("Expected validation to fail for expired token, but it succeeded")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("Expected error to mention expiration, got: %v", err)
	}
}

func TestGetTokenHash(t *testing.T) {
	token := "test.token.string"
	hash1 := GetTokenHash(token)
	hash2 := GetTokenHash(token)

	// Same token should produce same hash
	if hash1 != hash2 {
		t.Error("Same token produced different hashes")
	}

	// Different token should produce different hash
	hash3 := GetTokenHash("different.token.string")
	if hash1 == hash3 {
		t.Error("Different tokens produced same hash")
	}
}
