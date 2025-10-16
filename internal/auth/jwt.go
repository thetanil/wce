package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JWT claims structure
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	CenvID   string `json:"cenv_id"`
	Role     string `json:"role"`
	IssuedAt int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	SessionID string `json:"jti"` // JWT ID for session tracking
}

// JWTManager handles JWT token operations
type JWTManager struct {
	secret []byte
}

// NewJWTManager creates a new JWT manager with the given secret
func NewJWTManager(secret string) *JWTManager {
	return &JWTManager{
		secret: []byte(secret),
	}
}

// GenerateToken creates a new JWT token for the given claims
func (j *JWTManager) GenerateToken(userID, username, cenvID, role, sessionID string, expiresIn time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		Username:  username,
		CenvID:    cenvID,
		Role:      role,
		SessionID: sessionID,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(expiresIn).Unix(),
	}

	// Create header
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	// Encode header
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header: %w", err)
	}
	headerEncoded := base64.RawURLEncoding.EncodeToString(headerJSON)

	// Encode payload
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadJSON)

	// Create signature
	message := headerEncoded + "." + payloadEncoded
	signature := j.sign(message)
	signatureEncoded := base64.RawURLEncoding.EncodeToString(signature)

	// Combine all parts
	token := message + "." + signatureEncoded

	return token, nil
}

// ValidateToken validates a JWT token and returns the claims
func (j *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	// Split token into parts
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	headerEncoded := parts[0]
	payloadEncoded := parts[1]
	signatureEncoded := parts[2]

	// Verify signature
	message := headerEncoded + "." + payloadEncoded
	expectedSignature := j.sign(message)
	expectedSignatureEncoded := base64.RawURLEncoding.EncodeToString(expectedSignature)

	if !hmac.Equal([]byte(signatureEncoded), []byte(expectedSignatureEncoded)) {
		return nil, fmt.Errorf("invalid token signature")
	}

	// Decode payload
	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadEncoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Unmarshal claims
	var claims Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal claims: %w", err)
	}

	// Check expiration
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token has expired")
	}

	return &claims, nil
}

// sign creates HMAC-SHA256 signature for the given message
func (j *JWTManager) sign(message string) []byte {
	h := hmac.New(sha256.New, j.secret)
	h.Write([]byte(message))
	return h.Sum(nil)
}

// GetTokenHash returns SHA256 hash of the token for storage/revocation
func GetTokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.URLEncoding.EncodeToString(hash[:])
}
