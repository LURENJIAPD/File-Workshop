package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/platform/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type AccessTokenManager struct {
	issuer   string
	audience string
	secret   []byte
	ttl      time.Duration
}

type accessClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func NewAccessTokenManager(cfg config.AuthConfig) *AccessTokenManager {
	return &AccessTokenManager{
		issuer:   cfg.JWTIssuer,
		audience: cfg.JWTAudience,
		secret:   append([]byte(nil), cfg.JWTSecret...),
		ttl:      cfg.AccessTokenTTL,
	}
}

func (m *AccessTokenManager) Issue(userID, sessionID uuid.UUID, now time.Time) (string, time.Time, error) {
	expiresAt := now.Add(m.ttl)
	claims := accessClaims{
		SessionID: sessionID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{m.audience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.Must(uuid.NewV7()).String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

func (m *AccessTokenManager) Parse(raw string, now time.Time) (domain.AccessPrincipal, error) {
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(
		raw,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return m.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.audience),
		jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil || !token.Valid {
		return domain.AccessPrincipal{}, domain.ErrAuthentication
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return domain.AccessPrincipal{}, domain.ErrAuthentication
	}
	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return domain.AccessPrincipal{}, domain.ErrAuthentication
	}
	return domain.AccessPrincipal{UserID: userID, SessionID: sessionID}, nil
}

func NewRefreshToken() (string, []byte, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", nil, fmt.Errorf("generate refresh token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(secret)
	hash := sha256.Sum256([]byte(raw))
	return raw, hash[:], nil
}

func HashRefreshToken(raw string) []byte {
	hash := sha256.Sum256([]byte(raw))
	return hash[:]
}
