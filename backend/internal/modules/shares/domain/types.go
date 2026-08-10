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

	ResourceShare    = "SHARE"
	ResourceDocument = "DOCUMENT"
	ResourceFolder   = "FOLDER"

	TargetUser         = "USER"
	TargetOrganization = "ORGANIZATION"
	TargetSpace        = "SPACE"
	TargetLink         = "LINK"

	StatusActive            = "ACTIVE"
	StatusExpired           = "EXPIRED"
	StatusRevoked           = "REVOKED"
	StatusSuspended         = "SUSPENDED"
	StatusSourceUnavailable = "SOURCE_UNAVAILABLE"

	ActionReadMetadata = "READ_METADATA"
	ActionPreview      = "PREVIEW"
	ActionDownload     = "DOWNLOAD"
	ActionWriteContent = "WRITE_CONTENT"
	ActionShare        = "SHARE"
	ActionManagePerm   = "MANAGE_PERMISSION"
)

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

type SourceResource struct {
	ID               uuid.UUID
	Type             string
	SpaceID          uuid.UUID
	Name             string
	SpaceType        string
	SpaceOwnerUserID *uuid.UUID
	OrganizationID   *uuid.UUID
	OwnerUserID      *uuid.UUID
	InheritanceMode  string
	ACLVersion       int64
	RowVersion       int64
}

type Share struct {
	ID                   uuid.UUID
	SourceDocumentID     *uuid.UUID
	SourceFolderID       *uuid.UUID
	CreatorUserID        uuid.UUID
	TargetKind           string
	TargetUserID         *uuid.UUID
	TargetOrganizationID *uuid.UUID
	TargetSpaceID        *uuid.UUID
	TokenHash            []byte
	AllowReshare         bool
	Actions              []string
	ValidFrom            time.Time
	ValidUntil           *time.Time
	Status               string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	RevokedAt            *time.Time
	RevokedByUserID      *uuid.UUID
	RevokeReason         *string
	RowVersion           int64
}

func (s Share) Source() (string, uuid.UUID) {
	if s.SourceDocumentID != nil {
		return ResourceDocument, *s.SourceDocumentID
	}
	return ResourceFolder, *s.SourceFolderID
}

type CreateInput struct {
	SourceType           string
	SourceID             uuid.UUID
	TargetKind           string
	TargetUserID         *uuid.UUID
	TargetOrganizationID *uuid.UUID
	AllowReshare         bool
	Actions              []string
	ValidUntil           *time.Time
	Note                 *string
	IdempotencyKey       string
	RequestID            uuid.UUID
}

type UpdateInput struct {
	Actions      *[]string
	ValidUntil   *time.Time
	AllowReshare *bool
	RowVersion   int64
	RequestID    uuid.UUID
}

type RevokeInput struct {
	Reason     string
	RowVersion int64
	RequestID  uuid.UUID
}

type OpenInput struct {
	ShareToken *string
	RequestID  uuid.UUID
}

type CreateResult struct {
	Share      Share
	ShareToken *string
}

type ListResult struct {
	Items    []Share
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
