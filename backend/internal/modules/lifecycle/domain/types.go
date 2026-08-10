package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 50
	MaxPageSize     = 200

	ResourceFolder           = "FOLDER"
	ResourceDocument         = "DOCUMENT"
	ResourceRecycleItem      = "RECYCLE_ITEM"
	ResourcePreservationHold = "PRESERVATION_HOLD"

	EntryTypeFolder   = "FOLDER"
	EntryTypeDocument = "DOCUMENT"

	LifecycleActive   = "ACTIVE"
	LifecycleTrashed  = "TRASHED"
	LifecycleArchived = "ARCHIVED"
	LifecyclePurging  = "PURGING"
	LifecyclePurged   = "PURGED"

	RecycleStatusActive   = "ACTIVE"
	RecycleStatusRestored = "RESTORED"
	RecycleStatusPurging  = "PURGING"
	RecycleStatusPurged   = "PURGED"

	PreservationHoldStatusActive   = "ACTIVE"
	PreservationHoldStatusReleased = "RELEASED"

	ActionDelete       = "DELETE"
	ActionRestore      = "RESTORE"
	ActionPurge        = "PURGE"
	ActionUpload       = "UPLOAD"
	ActionCreateFolder = "CREATE_FOLDER"

	DefaultRecycleRetention = 90 * 24 * time.Hour

	SystemRoleAdmin = "SYSTEM_ADMIN"

	LifecyclePurgeJobType       = "LIFECYCLE_PURGE"
	DefaultExpiredScanBatchSize = 100
	MaxExpiredScanBatchSize     = 500
)

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

type Entry struct {
	ID              uuid.UUID
	SpaceID         uuid.UUID
	ParentFolderID  *uuid.UUID
	EntryType       string
	Name            string
	NormalizedName  string
	PathCache       *string
	Depth           int32
	LifecycleStatus string
	CreatedByUserID uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
	RowVersion      int64
	IsRoot          bool
}

type RecycleItem struct {
	ID                     uuid.UUID
	EntryID                uuid.UUID
	EntryType              string
	OriginalSpaceID        uuid.UUID
	OriginalParentFolderID *uuid.UUID
	OriginalName           string
	CurrentName            string
	LifecycleStatus        string
	DeletedByUserID        uuid.UUID
	DeletedAt              time.Time
	ExpiresAt              time.Time
	Status                 string
	RestoredToFolderID     *uuid.UUID
	RestoredAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
	RowVersion             int64
}

type TrashInput struct {
	RowVersion     int64
	Reason         *string
	IdempotencyKey string
	RequestID      uuid.UUID
}

type RestoreInput struct {
	TargetParentFolderID *uuid.UUID
	Name                 *string
	RowVersion           int64
	RequestID            uuid.UUID
}

type PurgeInput struct {
	Reason     string
	RowVersion int64
	RequestID  uuid.UUID
}

type ListFilter struct {
	SpaceID  *uuid.UUID
	Page     int
	PageSize int
}

type ListResult struct {
	Items    []RecycleItem
	Page     int
	PageSize int
	Total    int64
}

type ExpiredScanInput struct {
	BatchSize int
	RequestID uuid.UUID
}

type ExpiredScanResult struct {
	Scanned                 int
	Enqueued                int
	SkippedPreservationHold int
	JobType                 string
}

type DocumentRef struct {
	ID               uuid.UUID
	SpaceID          uuid.UUID
	Name             string
	LifecycleStatus  string
	CurrentVersionID *uuid.UUID
}

type PreservationHold struct {
	ID                uuid.UUID
	DocumentID        uuid.UUID
	DocumentVersionID *uuid.UUID
	CaseReference     string
	Reason            string
	Status            string
	PlacedByUserID    uuid.UUID
	PlacedAt          time.Time
	ReleasedByUserID  *uuid.UUID
	ReleasedAt        *time.Time
	ReleaseReason     *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	RowVersion        int64
}

type PlacePreservationHoldInput struct {
	DocumentVersionID *uuid.UUID
	CaseReference     string
	Reason            string
	IdempotencyKey    string
	RequestID         uuid.UUID
}

type ReleasePreservationHoldInput struct {
	Reason     string
	RowVersion int64
	RequestID  uuid.UUID
}

type PreservationHoldListFilter struct {
	DocumentID    *uuid.UUID
	Status        *string
	CaseReference *string
	Page          int
	PageSize      int
}

type PreservationHoldListResult struct {
	Items    []PreservationHold
	Page     int
	PageSize int
	Total    int64
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
