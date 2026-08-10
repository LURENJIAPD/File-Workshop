package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 50
	MaxPageSize     = 200

	SystemRoleAdmin = "SYSTEM_ADMIN"

	EntryTypeFolder   = "FOLDER"
	EntryTypeDocument = "DOCUMENT"
	EntryTypeShared   = "SHARED_ENTRY"

	LifecycleActive   = "ACTIVE"
	LifecycleTrashed  = "TRASHED"
	LifecycleArchived = "ARCHIVED"
	LifecyclePurging  = "PURGING"
	LifecyclePurged   = "PURGED"

	InheritanceInherit = "INHERIT"
	InheritanceBreak   = "BREAK"

	AvailabilityAvailable   = "AVAILABLE"
	AvailabilityPendingScan = "PENDING_SCAN"
	AvailabilityQuarantined = "QUARANTINED"
	AvailabilityBlocked     = "BLOCKED"
)

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

type SpaceDirectoryInfo struct {
	ID             uuid.UUID
	SpaceType      string
	OwnerUserID    *uuid.UUID
	OrganizationID *uuid.UUID
	RootFolderID   *uuid.UUID
	Status         string
	RowVersion     int64
}

type NamespaceEntry struct {
	ID                    uuid.UUID
	SpaceID               uuid.UUID
	ParentFolderID        *uuid.UUID
	EntryType             string
	Name                  string
	NormalizedName        string
	PathCache             *string
	Depth                 int32
	LifecycleStatus       string
	CreatedByUserID       uuid.UUID
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time
	RowVersion            int64
	IsRoot                bool
	InheritanceMode       *string
	ACLVersion            *int64
	OwnerUserID           *uuid.UUID
	CurrentVersionID      *uuid.UUID
	AvailabilityStatus    *string
	ExtensionNormalized   *string
	Classification        *string
	MetadataSchemaVersion *int32
	MetadataJSON          json.RawMessage
}

type EntryListFilter struct {
	SpaceID         uuid.UUID
	ParentFolderID  *uuid.UUID
	EntryType       *string
	LifecycleStatus *string
	Page            int
	PageSize        int
}

type EntryListResult struct {
	Items          []NamespaceEntry
	SpaceID        uuid.UUID
	ParentFolderID *uuid.UUID
	RootFolderID   *uuid.UUID
	Page           int
	PageSize       int
	Total          int64
}

type NewNamespaceEntry struct {
	ID              uuid.UUID
	SpaceID         uuid.UUID
	ParentFolderID  *uuid.UUID
	EntryType       string
	Name            string
	NormalizedName  string
	PathCache       string
	Depth           int32
	CreatedByUserID uuid.UUID
	CreatedAt       time.Time
}

type NewDocument struct {
	ID                  uuid.UUID
	OwnerUserID         uuid.UUID
	AvailabilityStatus  string
	ExtensionNormalized *string
	Classification      *string
	MetadataJSON        json.RawMessage
	CreatedAt           time.Time
}

type CreateFolderInput struct {
	ParentFolderID *uuid.UUID
	Name           string
	IdempotencyKey string
	RequestID      uuid.UUID
}

type CreateDocumentInput struct {
	ParentFolderID *uuid.UUID
	Name           string
	Classification *string
	MetadataJSON   json.RawMessage
	IdempotencyKey string
	RequestID      uuid.UUID
}

type RenameEntryInput struct {
	Name       string
	RowVersion int64
	RequestID  uuid.UUID
}

type MoveEntryInput struct {
	TargetParentFolderID *uuid.UUID
	RowVersion           int64
	RequestID            uuid.UUID
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

type IdempotencyRecord struct {
	RequestHash      []byte
	Status           string
	ResultResourceID *uuid.UUID
}
