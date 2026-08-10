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

	ResourceFolder   = "FOLDER"
	ResourceDocument = "DOCUMENT"

	EntryTypeFolder   = "FOLDER"
	EntryTypeDocument = "DOCUMENT"

	LifecycleActive = "ACTIVE"

	ActionReadMetadata = "READ_METADATA"

	MatchedName           = "NAME"
	MatchedExtension      = "EXTENSION"
	MatchedClassification = "CLASSIFICATION"
	MatchedMetadata       = "METADATA"

	SourcePostgresMetadata = "POSTGRES_METADATA"
)

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

type Entry struct {
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

type Filter struct {
	Query           *string
	SpaceID         *uuid.UUID
	EntryType       *string
	Extension       *string
	Classification  *string
	CreatedByUserID *uuid.UUID
	UpdatedFrom     *time.Time
	UpdatedTo       *time.Time
	MetadataKey     *string
	MetadataValue   *string
	Page            int
	PageSize        int
}

type Result struct {
	Entry         Entry
	MatchedFields []string
	IndexStatus   *string
	Source        string
}

type ListResult struct {
	Items    []Result
	Page     int
	PageSize int
	Total    int64
	Degraded bool
}
