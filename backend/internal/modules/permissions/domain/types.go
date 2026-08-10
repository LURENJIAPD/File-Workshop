package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	SystemRoleAdmin = "SYSTEM_ADMIN"

	ResourceSpace    = "SPACE"
	ResourceFolder   = "FOLDER"
	ResourceDocument = "DOCUMENT"

	SubjectUser         = "USER"
	SubjectOrganization = "ORGANIZATION"

	StatusActive      = "ACTIVE"
	StatusRevoked     = "REVOKED"
	StatusExpired     = "EXPIRED"
	StatusInvalidated = "INVALIDATED"

	ScopeSelf    = "SELF"
	ScopeSubtree = "SUBTREE"

	InheritanceInherit = "INHERIT"
	InheritanceBreak   = "BREAK"

	CapabilityManageContent    = "MANAGE_SPACE_CONTENT"
	CapabilityManagePermission = "MANAGE_SPACE_PERMISSION"
	CapabilityManageMembers    = "MANAGE_SPACE_MEMBERS"
	CapabilityManageRecycleBin = "MANAGE_SPACE_RECYCLE_BIN"
	CapabilityForceUnlock      = "FORCE_UNLOCK"
	CapabilityViewAudit        = "VIEW_SPACE_AUDIT"
	CapabilityDelegateAdmin    = "DELEGATE_ADMIN"
)

var Capabilities = map[string]struct{}{
	CapabilityManageContent: {}, CapabilityManagePermission: {}, CapabilityManageMembers: {},
	CapabilityManageRecycleBin: {}, CapabilityForceUnlock: {}, CapabilityViewAudit: {}, CapabilityDelegateAdmin: {},
}

var Actions = map[string]struct{}{
	"LIST": {}, "READ_METADATA": {}, "PREVIEW": {}, "DOWNLOAD": {}, "UPLOAD": {},
	"CREATE_FOLDER": {}, "WRITE_CONTENT": {}, "RENAME": {}, "MOVE": {}, "DELETE": {},
	"RESTORE": {}, "PURGE": {}, "SHARE": {}, "LOCK": {}, "MANAGE_VERSION": {}, "MANAGE_PERMISSION": {},
}

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

type AdminDelegation struct {
	ID                 uuid.UUID
	UserID             uuid.UUID
	OrganizationID     uuid.UUID
	Scope              string
	CanDelegate        bool
	ParentDelegationID *uuid.UUID
	Capabilities       []string
	GrantedByUserID    uuid.UUID
	ValidFrom          time.Time
	ValidUntil         *time.Time
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	RevokedAt          *time.Time
	RevokeReason       *string
	RowVersion         int64
}

type NewAdminDelegation struct {
	ID                 uuid.UUID
	UserID             uuid.UUID
	OrganizationID     uuid.UUID
	Scope              string
	CanDelegate        bool
	ParentDelegationID *uuid.UUID
	Capabilities       []string
	GrantedByUserID    uuid.UUID
	ValidFrom          time.Time
	ValidUntil         *time.Time
	CreatedAt          time.Time
}

type AdminDelegationListFilter struct {
	ViewerUserID   uuid.UUID
	ViewerIsAdmin  bool
	OrganizationID *uuid.UUID
	Status         *string
	EffectiveAt    *time.Time
	OnlyEffective  bool
	Page           int
	PageSize       int
}

type AdminDelegationListResult struct {
	Items    []AdminDelegation
	Total    int64
	Page     int
	PageSize int
}

type PermissionGrant struct {
	ID                    uuid.UUID
	SubjectUserID         *uuid.UUID
	SubjectOrganizationID *uuid.UUID
	SpaceID               *uuid.UUID
	FolderID              *uuid.UUID
	DocumentID            *uuid.UUID
	Actions               []string
	InheritToDescendants  bool
	GrantSource           string
	ValidFrom             time.Time
	ValidUntil            *time.Time
	Status                string
	GrantedByUserID       uuid.UUID
	GrantReason           *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	RevokedAt             *time.Time
	RevokedByUserID       *uuid.UUID
	RevokeReason          *string
	RowVersion            int64
}

type ShareGrant struct {
	ID      uuid.UUID
	Actions []string
}

func (g PermissionGrant) Subject() (string, uuid.UUID) {
	if g.SubjectUserID != nil {
		return SubjectUser, *g.SubjectUserID
	}
	return SubjectOrganization, *g.SubjectOrganizationID
}

func (g PermissionGrant) Resource() (string, uuid.UUID) {
	if g.SpaceID != nil {
		return ResourceSpace, *g.SpaceID
	}
	if g.FolderID != nil {
		return ResourceFolder, *g.FolderID
	}
	return ResourceDocument, *g.DocumentID
}

type NewPermissionGrant struct {
	ID                    uuid.UUID
	SubjectUserID         *uuid.UUID
	SubjectOrganizationID *uuid.UUID
	SpaceID               *uuid.UUID
	FolderID              *uuid.UUID
	DocumentID            *uuid.UUID
	Actions               []string
	InheritToDescendants  bool
	GrantSource           string
	ValidFrom             time.Time
	ValidUntil            *time.Time
	GrantedByUserID       uuid.UUID
	GrantReason           *string
	CreatedAt             time.Time
}

type PermissionGrantListResult struct {
	Items    []PermissionGrant
	Total    int64
	Page     int
	PageSize int
}

type FolderAncestor struct {
	ID              uuid.UUID
	Distance        int
	InheritanceMode string
}

type Resource struct {
	Type             string
	ID               uuid.UUID
	SpaceID          uuid.UUID
	SpaceType        string
	SpaceOwnerUserID *uuid.UUID
	OrganizationID   *uuid.UUID
	InheritanceMode  string
	ACLVersion       int64
	RowVersion       int64
	FolderAncestors  []FolderAncestor
}

type PermissionEvaluation struct {
	ResourceType             string
	ResourceID               uuid.UUID
	Action                   string
	Allowed                  bool
	Source                   string
	MatchedGrantIDs          []uuid.UUID
	PrivilegedAccessRequired bool
}

type InheritanceResult struct {
	ResourceType string
	ResourceID   uuid.UUID
	Mode         string
	ACLVersion   int64
	RowVersion   int64
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
