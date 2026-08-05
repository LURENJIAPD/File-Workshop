package application

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MFAAdapter is the boundary for a concrete TOTP or WebAuthn implementation.
// No HTTP route is exposed until a secret-reference provider and the matching
// frontend ceremony are available.
type MFAAdapter interface {
	MethodType() string
	Begin(context.Context, MFAChallengeRequest) (MFAChallenge, error)
	Verify(context.Context, MFAVerificationRequest) error
}

type MFAChallengeRequest struct {
	UserID   uuid.UUID
	MethodID uuid.UUID
}

type MFAChallenge struct {
	OpaquePayload []byte
	ExpiresAt     time.Time
}

type MFAVerificationRequest struct {
	UserID         uuid.UUID
	MethodID       uuid.UUID
	OpaqueResponse []byte
}

// TOTPSecretVerifier keeps secret_ref resolution inside a Vault/KMS adapter;
// identity application code never receives or persists the raw TOTP secret.
type TOTPSecretVerifier interface {
	VerifyReference(context.Context, string, string, time.Time) (bool, error)
}

// ExternalIdentityAdapter is the boundary for LDAP, OIDC and AD providers.
// A successful result is mapped back to an existing authoritative users row.
type ExternalIdentityAdapter interface {
	Provider() string
	Authenticate(context.Context, ExternalAuthenticationRequest) (ExternalAuthenticationResult, error)
}

type ExternalAuthenticationRequest struct {
	Identifier string
	Secret     string
}

type ExternalAuthenticationResult struct {
	UserID uuid.UUID
}
