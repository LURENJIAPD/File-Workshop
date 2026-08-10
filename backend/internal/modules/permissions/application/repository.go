package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/permissions/domain"

	"github.com/google/uuid"
)

type Repository interface {
	GetAdminDelegation(context.Context, uuid.UUID) (domain.AdminDelegation, error)
	GetAdminDelegationForUpdate(context.Context, uuid.UUID) (domain.AdminDelegation, error)
	ListAdminDelegations(context.Context, domain.AdminDelegationListFilter) (domain.AdminDelegationListResult, error)
	ListOrganizationAdministrators(context.Context, uuid.UUID, int, int, time.Time) (domain.AdminDelegationListResult, error)
	OrganizationExists(context.Context, uuid.UUID) (bool, error)
	InsertAdminDelegation(context.Context, domain.NewAdminDelegation) (domain.AdminDelegation, error)
	RevokeAdminDelegation(context.Context, uuid.UUID, int64, string, time.Time) (domain.AdminDelegation, error)
	InvalidateDescendantDelegations(context.Context, uuid.UUID, time.Time) ([]uuid.UUID, error)
	FindEffectiveAdminDelegation(context.Context, uuid.UUID, uuid.UUID, string, time.Time) (*domain.AdminDelegation, error)
	AdminDelegationIsEffective(context.Context, uuid.UUID, time.Time) (bool, error)
	IncrementDelegationSecurityVersions(context.Context, uuid.UUID, uuid.UUID, time.Time) error

	GetResource(context.Context, string, uuid.UUID) (domain.Resource, error)
	GetPermissionGrant(context.Context, uuid.UUID) (domain.PermissionGrant, error)
	GetPermissionGrantForUpdate(context.Context, uuid.UUID) (domain.PermissionGrant, error)
	ListDirectPermissionGrants(context.Context, string, uuid.UUID, int, int) (domain.PermissionGrantListResult, error)
	ListCandidatePermissionGrants(context.Context, uuid.UUID, []uuid.UUID, domain.Resource, time.Time) ([]domain.PermissionGrant, error)
	ListCandidateShareGrants(context.Context, uuid.UUID, []uuid.UUID, domain.Resource, time.Time) ([]domain.ShareGrant, error)
	ListActiveUserOrganizations(context.Context, uuid.UUID, time.Time) ([]uuid.UUID, error)
	InsertPermissionGrant(context.Context, domain.NewPermissionGrant) (domain.PermissionGrant, error)
	UpdatePermissionGrant(context.Context, uuid.UUID, []string, bool, *time.Time, *string, int64, time.Time) (domain.PermissionGrant, error)
	RevokePermissionGrant(context.Context, uuid.UUID, uuid.UUID, string, int64, time.Time) (domain.PermissionGrant, error)
	ChangeInheritance(context.Context, string, uuid.UUID, string, int64, time.Time) (domain.InheritanceResult, error)
	IncrementGrantSecurityVersions(context.Context, domain.PermissionGrant, domain.Resource, time.Time) error
	GetAuthorizationVersion(context.Context, uuid.UUID, uuid.UUID, time.Time) (string, error)

	TryCreateIdempotency(context.Context, uuid.UUID, uuid.UUID, string, string, []byte, time.Time, time.Time) (bool, error)
	GetIdempotency(context.Context, uuid.UUID, string, string) (IdempotencyRecord, error)
	CompleteIdempotency(context.Context, uuid.UUID, string, string, uuid.UUID, string, time.Time) error
	InsertEvent(context.Context, domain.Event) error
}

type Transactor interface {
	WithinTransaction(context.Context, func(Repository) error) error
}

type IdempotencyRecord struct {
	RequestHash      []byte
	Status           string
	ResultResourceID *uuid.UUID
}

type DecisionCache interface {
	Get(context.Context, string) (domain.PermissionEvaluation, bool)
	Set(context.Context, string, domain.PermissionEvaluation)
}
