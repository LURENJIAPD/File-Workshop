package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 50
	MaxPageSize     = 200

	ResourceFolder      = "FOLDER"
	ResourceDocument    = "DOCUMENT"
	ResourceRecycleItem = "RECYCLE_ITEM"

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

	ActionDelete       = "DELETE"
	ActionRestore      = "RESTORE"
	ActionPurge        = "PURGE"
	ActionUpload       = "UPLOAD"
	ActionCreateFolder = "CREATE_FOLDER"

	DefaultRecycleRetention = 90 * 24 * time.Hour
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
