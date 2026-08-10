package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	IntentCreate     = "CREATE"
	IntentNewVersion = "NEW_VERSION"

	StatusInitiated  = "INITIATED"
	StatusUploading  = "UPLOADING"
	StatusCompleting = "COMPLETING"
	StatusCompleted  = "COMPLETED"
	StatusAborted    = "ABORTED"
	StatusExpired    = "EXPIRED"
	StatusFailed     = "FAILED"

	FailureUserAborted = "USER_ABORTED"

	ResourceFolder        = "FOLDER"
	ResourceDocument      = "DOCUMENT"
	ResourceUploadSession = "UPLOAD_SESSION"

	ActionUpload = "UPLOAD"

	DefaultSessionTTL = 24 * time.Hour
	MinPartSizeBytes  = 5 * 1024 * 1024
	MaxPartCount      = 10000
)

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

type CreateSessionInput struct {
	SpaceID                  uuid.UUID
	FolderID                 uuid.UUID
	TargetDocumentID         *uuid.UUID
	UploadIntent             string
	FileName                 string
	DeclaredSizeBytes        int64
	DeclaredSHA256           []byte
	DeclaredMIMEType         *string
	PartSizeBytes            int64
	ExpectedCurrentVersionID *uuid.UUID
	ExpectedLockFencingToken *int64
	LockTokenHash            []byte
	IdempotencyKey           string
	RequestID                uuid.UUID
}

type NewSession struct {
	ID                       uuid.UUID
	UserID                   uuid.UUID
	SpaceID                  uuid.UUID
	FolderID                 uuid.UUID
	QuotaReservationID       uuid.UUID
	TargetDocumentID         *uuid.UUID
	UploadIntent             string
	FileName                 string
	NormalizedName           string
	DeclaredSizeBytes        int64
	DeclaredSHA256           []byte
	DeclaredMIMEType         *string
	ProviderUploadID         string
	TemporaryObjectKey       string
	PartSizeBytes            int64
	ExpectedPartCount        int32
	ExpectedCurrentVersionID *uuid.UUID
	ExpectedLockFencingToken *int64
	LockTokenHash            []byte
	ExpiresAt                time.Time
	CreatedAt                time.Time
}

type Session struct {
	ID                       uuid.UUID
	UserID                   uuid.UUID
	SpaceID                  uuid.UUID
	FolderID                 uuid.UUID
	QuotaReservationID       uuid.UUID
	TargetDocumentID         *uuid.UUID
	UploadIntent             string
	FileName                 string
	NormalizedName           string
	DeclaredSizeBytes        int64
	DeclaredSHA256           []byte
	DeclaredMIMEType         *string
	ProviderUploadID         *string
	TemporaryObjectKey       string
	PartSizeBytes            int64
	ExpectedPartCount        int32
	ExpectedCurrentVersionID *uuid.UUID
	ExpectedLockFencingToken *int64
	LockTokenHash            []byte
	Status                   string
	ExpiresAt                time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
	CompletedAt              *time.Time
	FailureCode              *string
	ResultDocumentID         *uuid.UUID
	ResultVersionID          *uuid.UUID
	RowVersion               int64
}

type FolderContext struct {
	ID      uuid.UUID
	SpaceID uuid.UUID
}

type DocumentContext struct {
	ID               uuid.UUID
	SpaceID          uuid.UUID
	CurrentVersionID *uuid.UUID
	Availability     string
	RowVersion       int64
}

type IdempotencyRecord struct {
	RequestHash      []byte
	Status           string
	ResultResourceID *uuid.UUID
}

type PresignedPart struct {
	SessionID  uuid.UUID
	PartNumber int32
	Method     string
	URL        string
	Headers    map[string]string
	ExpiresAt  time.Time
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
