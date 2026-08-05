package domain

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

const (
	UserStatusActive    = "ACTIVE"
	SessionStatusActive = "ACTIVE"
	RefreshStatusActive = "ACTIVE"
	RefreshStatusUsed   = "USED"
	MFAMethodTOTP       = "TOTP"
	MFAMethodWebAuthn   = "WEBAUTHN"
)

type User struct {
	ID          uuid.UUID
	Username    string
	DisplayName string
	SystemRole  string
	Status      string
	Locale      string
	Timezone    string
}

type PasswordIdentity struct {
	User         User
	CredentialID uuid.UUID
	SecretHash   string
}

type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	DeviceID   *string
	Status     string
	ExpiresAt  time.Time
	LastSeenAt *time.Time
	CreatedAt  time.Time
}

type RefreshSession struct {
	RefreshTokenID        uuid.UUID
	Session               Session
	FamilyID              uuid.UUID
	RotationNumber        int32
	RefreshTokenStatus    string
	RefreshTokenExpiresAt time.Time
	User                  User
}

type FailureState struct {
	Count         int64
	LastFailureAt *time.Time
}

type RequestMetadata struct {
	DeviceID  *string
	IPAddress *netip.Addr
	UserAgent string
	RequestID uuid.UUID
}

type LoginAttempt struct {
	ID                 uuid.UUID
	UsernameNormalized string
	UserID             *uuid.UUID
	Result             string
	FailureCode        *string
	Metadata           RequestMetadata
	CreatedAt          time.Time
}

type NewSession struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Metadata  RequestMetadata
	ExpiresAt time.Time
	CreatedAt time.Time
}

type NewRefreshToken struct {
	ID             uuid.UUID
	SessionID      uuid.UUID
	FamilyID       uuid.UUID
	ParentID       *uuid.UUID
	RotationNumber int32
	Hash           []byte
	IssuedAt       time.Time
	ExpiresAt      time.Time
}

type AccessPrincipal struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
}
