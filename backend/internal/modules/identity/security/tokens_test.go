package security

import (
	"errors"
	"testing"
	"time"

	"file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/platform/config"

	"github.com/google/uuid"
)

func TestAccessTokenIssueAndParse(t *testing.T) {
	now := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	manager := NewAccessTokenManager(config.AuthConfig{
		JWTIssuer: "file-workshop", JWTAudience: "file-workshop-api",
		JWTSecret: []byte("01234567890123456789012345678901"), AccessTokenTTL: 15 * time.Minute,
	})
	userID := uuid.Must(uuid.NewV7())
	sessionID := uuid.Must(uuid.NewV7())
	raw, expiresAt, err := manager.Issue(userID, sessionID, now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if expiresAt != now.Add(15*time.Minute) {
		t.Fatalf("expiresAt = %v", expiresAt)
	}
	principal, err := manager.Parse(raw, now.Add(time.Minute))
	if err != nil || principal.UserID != userID || principal.SessionID != sessionID {
		t.Fatalf("Parse() = %#v, %v", principal, err)
	}
	if _, err := manager.Parse(raw, now.Add(16*time.Minute)); !errors.Is(err, domain.ErrAuthentication) {
		t.Fatalf("Parse(expired) error = %v, want authentication error", err)
	}
}

func TestRefreshTokensAreRandomAndStoredAsHashes(t *testing.T) {
	first, firstHash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken() error = %v", err)
	}
	second, _, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken() error = %v", err)
	}
	if first == second || len(firstHash) != 32 {
		t.Fatalf("refresh token generation did not produce distinct token and SHA-256 hash")
	}
	if actual := HashRefreshToken(first); string(actual) != string(firstHash) {
		t.Fatal("HashRefreshToken() did not reproduce stored hash")
	}
}
