package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 50
	MaxPageSize     = 200

	SystemRoleAdmin = "SYSTEM_ADMIN"

	ResourceDocument        = "DOCUMENT"
	ResourceDocumentLock    = "DOCUMENT_LOCK"
	ResourceDocumentVersion = "DOCUMENT_VERSION"

	ActionReadMetadata  = "READ_METADATA"
	ActionManageVersion = "MANAGE_VERSION"
	ActionLock          = "LOCK"

	LockStatusActive   = "ACTIVE"
	LockStatusReleased = "RELEASED"
	LockStatusExpired  = "EXPIRED"
	LockStatusForced   = "FORCED"

	LockSourceWeb    = "WEB"
	LockSourceWebDAV = "WEBDAV"
	LockSourceOffice = "OFFICE"
	LockSourceAgent  = "AGENT"

	DefaultLockTTL = 15 * time.Minute
	MinLockTTL     = time.Minute
	MaxLockTTL     = 24 * time.Hour
)

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

type DocumentContext struct {
	ID               uuid.UUID
	SpaceID          uuid.UUID
	OwnerUserID      uuid.UUID
	CurrentVersionID *uuid.UUID
	Availability     string
	RowVersion       int64
}

type Version struct {
	ID                    uuid.UUID
	DocumentID            uuid.UUID
	VersionNumber         int64
	StorageObjectID       uuid.UUID
	SizeBytes             int64
	SHA256                []byte
	MIMEType              string
	ChangeNote            *string
	SourceType            string
	RestoredFromVersionID *uuid.UUID
	CreatedByUserID       uuid.UUID
	CreatedAt             time.Time
}

type VersionListResult struct {
	Items    []Version
	Page     int
	PageSize int
	Total    int64
}

type RestoreVersionInput struct {
	RowVersion     int64
	ChangeNote     *string
	IdempotencyKey string
	RequestID      uuid.UUID
}

type Lock struct {
	ID               uuid.UUID
	DocumentID       uuid.UUID
	UserID           uuid.UUID
	FencingToken     int64
	Source           string
	Status           string
	AcquiredAt       time.Time
	HeartbeatAt      time.Time
	ExpiresAt        time.Time
	ReleasedAt       *time.Time
	ReleasedByUserID *uuid.UUID
	ReleaseReason    *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	RowVersion       int64
}

type AcquiredLock struct {
	Lock      Lock
	LockToken string
}

type AcquireLockInput struct {
	Source     string
	TTLSeconds *int
	RequestID  uuid.UUID
}

type HeartbeatLockInput struct {
	LockToken  string
	RowVersion int64
	TTLSeconds *int
	RequestID  uuid.UUID
}

type ReleaseLockInput struct {
	LockToken  string
	RowVersion int64
	Reason     *string
	RequestID  uuid.UUID
}

type ForceReleaseLockInput struct {
	RowVersion int64
	Reason     string
	RequestID  uuid.UUID
}

type IdempotencyRecord struct {
	RequestHash      []byte
	Status           string
	ResultResourceID *uuid.UUID
}

type Event struct {
	ID               uuid.UUID
	AggregateType    string
	AggregateID      uuid.UUID
	AggregateVersion int64
	Type             string
	Payload          []byte
	DeduplicationKey string
	CorrelationID    uuid.UUID
	CreatedAt        time.Time
}
