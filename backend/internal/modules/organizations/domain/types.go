package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	SystemRoleAdmin = "SYSTEM_ADMIN"

	OrganizationStatusActive   = "ACTIVE"
	OrganizationStatusDisabled = "DISABLED"
	OrganizationStatusArchived = "ARCHIVED"
	OrganizationStatusDeleted  = "DELETED"

	MembershipTypePrimary    = "PRIMARY"
	MembershipTypeMember     = "MEMBER"
	MembershipStatusActive   = "ACTIVE"
	MembershipStatusInactive = "INACTIVE"

	SpaceTypePersonal     = "PERSONAL"
	SpaceTypeOrganization = "ORGANIZATION"
	SpaceTypePublic       = "PUBLIC"
	SpaceStatusActive     = "ACTIVE"
	SpaceStatusFrozen     = "FROZEN"
	SpaceStatusArchived   = "ARCHIVED"
	SpaceStatusDeleted    = "DELETED"

	ReservationStatusActive   = "ACTIVE"
	ReservationStatusConsumed = "CONSUMED"
	ReservationStatusReleased = "RELEASED"
	ReservationStatusExpired  = "EXPIRED"

	PlanTypeMove            = "MOVE"
	PlanTypeMerge           = "MERGE"
	PlanTypeSplit           = "SPLIT"
	PlanTypeBulkRestructure = "BULK_RESTRUCTURE"
	PlanStatusDraft         = "DRAFT"
	PlanStatusValidated     = "VALIDATED"
	PlanStatusApproved      = "APPROVED"
	PlanStatusExecuting     = "EXECUTING"
	PlanStatusCompleted     = "COMPLETED"
	PlanStatusCancelled     = "CANCELLED"
	PlanStatusFailed        = "FAILED"

	OperationTypeMoveNode         = "MOVE_NODE"
	OperationTypeMergeNode        = "MERGE_NODE"
	OperationTypeCreateNode       = "CREATE_NODE"
	OperationTypeMoveMember       = "MOVE_MEMBER"
	OperationTypeMoveSpaceContent = "MOVE_SPACE_CONTENT"
	OperationStatusPending        = "PENDING"
	OperationStatusSuccess        = "SUCCESS"
	OperationStatusFailed         = "FAILED"
	OperationStatusSkipped        = "SKIPPED"

	PlanActionValidate = "VALIDATE"
	PlanActionApprove  = "APPROVE"
	PlanActionExecute  = "EXECUTE"
	PlanActionCancel   = "CANCEL"

	DefaultPage     = 1
	DefaultPageSize = 50
	MaximumPageSize = 200
)

type Organization struct {
	ID                   uuid.UUID
	ParentOrganizationID *uuid.UUID
	Name                 string
	NormalizedName       string
	Code                 *string
	NormalizedCode       *string
	TypeLabel            *string
	SortOrder            int32
	PathCache            *string
	Depth                int32
	TreeVersion          int64
	Status               string
	CreatedByUserID      uuid.UUID
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time
	RowVersion           int64
}

type NewOrganization struct {
	ID                   uuid.UUID
	SpaceID              uuid.UUID
	ParentOrganizationID *uuid.UUID
	Name                 string
	NormalizedName       string
	Code                 *string
	NormalizedCode       *string
	TypeLabel            *string
	SortOrder            int32
	Depth                int32
	SpaceQuotaBytes      int64
	CreatedByUserID      uuid.UUID
	CreatedAt            time.Time
}

type OrganizationChanges struct {
	Name       *string
	Code       *string
	TypeLabel  *string
	SortOrder  *int32
	RowVersion int64
}

type OrganizationListFilter struct {
	ParentOrganizationID *uuid.UUID
	Status               *string
	Page                 int
	PageSize             int
}

type OrganizationListResult struct {
	Items    []Organization
	Total    int64
	Page     int
	PageSize int
}

type Membership struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	OrganizationID  uuid.UUID
	MembershipType  string
	JobTitle        *string
	Status          string
	EffectiveFrom   time.Time
	EffectiveUntil  *time.Time
	CreatedByUserID uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	RowVersion      int64
}

type NewMembership struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	OrganizationID  uuid.UUID
	MembershipType  string
	JobTitle        *string
	EffectiveFrom   time.Time
	EffectiveUntil  *time.Time
	CreatedByUserID uuid.UUID
	CreatedAt       time.Time
}

type MembershipChanges struct {
	MembershipType *string
	JobTitle       *string
	Status         *string
	EffectiveUntil *time.Time
	RowVersion     int64
}

type MembershipListFilter struct {
	OrganizationID *uuid.UUID
	UserID         *uuid.UUID
	Status         *string
	EffectiveAt    *time.Time
	Page           int
	PageSize       int
}

type MembershipListResult struct {
	Items    []Membership
	Total    int64
	Page     int
	PageSize int
}

type Space struct {
	ID                  uuid.UUID
	SpaceType           string
	Name                string
	NormalizedName      string
	OwnerUserID         *uuid.UUID
	OrganizationID      *uuid.UUID
	RootFolderID        *uuid.UUID
	QuotaBytes          int64
	UsedBytes           int64
	ReservedBytes       int64
	ACLVersion          int64
	SecurityEpoch       int64
	ConfigSchemaVersion int32
	ConfigJSON          json.RawMessage
	Status              string
	CreatedByUserID     uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
	RowVersion          int64
}

type NewSpace struct {
	ID                  uuid.UUID
	SpaceType           string
	Name                string
	NormalizedName      string
	OwnerUserID         *uuid.UUID
	OrganizationID      *uuid.UUID
	QuotaBytes          int64
	ConfigSchemaVersion int32
	ConfigJSON          json.RawMessage
	CreatedByUserID     uuid.UUID
	CreatedAt           time.Time
}

type SpaceChanges struct {
	Name                *string
	QuotaBytes          *int64
	ConfigSchemaVersion *int32
	ConfigJSON          json.RawMessage
	RowVersion          int64
}

type SpaceListFilter struct {
	SpaceType      *string
	Status         *string
	OrganizationID *uuid.UUID
	OwnerUserID    *uuid.UUID
	Page           int
	PageSize       int
}

type SpaceListResult struct {
	Items    []Space
	Total    int64
	Page     int
	PageSize int
}

type QuotaReservation struct {
	ID            uuid.UUID
	SpaceID       uuid.UUID
	UserID        uuid.UUID
	ReservedBytes int64
	Status        string
	ExpiresAt     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ConsumedAt    *time.Time
	ReleasedAt    *time.Time
	RowVersion    int64
}

type OrganizationChangePlan struct {
	ID                  uuid.UUID
	PlanType            string
	Name                string
	Status              string
	ExpectedTreeVersion int64
	CreatedByUserID     uuid.UUID
	ApprovedByUserID    *uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ValidatedAt         *time.Time
	ApprovedAt          *time.Time
	StartedAt           *time.Time
	CompletedAt         *time.Time
	FailureCode         *string
	RowVersion          int64
	Operations          []OrganizationChangeOperation
}

type OrganizationChangeOperation struct {
	ID                     uuid.UUID
	PlanID                 uuid.UUID
	SequenceNumber         int32
	OperationType          string
	SourceOrganizationID   *uuid.UUID
	TargetOrganizationID   *uuid.UUID
	OperationSchemaVersion int32
	OperationJSON          json.RawMessage
	Status                 string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	CompletedAt            *time.Time
	FailureCode            *string
	RowVersion             int64
}

type PlanListFilter struct {
	Status   *string
	Page     int
	PageSize int
}

type PlanListResult struct {
	Items    []OrganizationChangePlan
	Total    int64
	Page     int
	PageSize int
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
