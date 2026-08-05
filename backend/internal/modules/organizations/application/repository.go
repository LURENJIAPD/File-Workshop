package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

type Repository interface {
	LockTreeMutation(context.Context) error
	GetOrganization(context.Context, uuid.UUID) (domain.Organization, error)
	GetOrganizationForUpdate(context.Context, uuid.UUID) (domain.Organization, error)
	ListOrganizations(context.Context, domain.OrganizationListFilter) (domain.OrganizationListResult, error)
	InsertOrganization(context.Context, domain.NewOrganization) (domain.Organization, error)
	InsertOrganizationClosure(context.Context, uuid.UUID, *uuid.UUID, time.Time) error
	InsertOrganizationSecurityVersions(context.Context, uuid.UUID, time.Time) error
	UpdateOrganization(context.Context, domain.Organization, int64, time.Time) (domain.Organization, error)
	OrganizationWouldCreateCycle(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	DeleteExternalClosureLinks(context.Context, uuid.UUID) error
	InsertMovedClosureLinks(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	UpdateMovedSubtree(context.Context, uuid.UUID, *uuid.UUID, int32, int64, time.Time) error
	IncrementSubtreeSecurityEpochs(context.Context, uuid.UUID, time.Time) error
	OrganizationDeletionBlocked(context.Context, uuid.UUID, time.Time) (bool, error)
	SetOrganizationStatus(context.Context, uuid.UUID, string, int64, time.Time) (domain.Organization, error)

	GetMembership(context.Context, uuid.UUID) (domain.Membership, error)
	GetMembershipForUpdate(context.Context, uuid.UUID, uuid.UUID) (domain.Membership, error)
	ListMemberships(context.Context, domain.MembershipListFilter) (domain.MembershipListResult, error)
	UserCanJoinOrganization(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	InsertMembership(context.Context, domain.NewMembership) (domain.Membership, error)
	UpdateMembership(context.Context, domain.Membership, int64, time.Time) (domain.Membership, error)
	DeactivateMembership(context.Context, uuid.UUID, uuid.UUID, int64, time.Time) (domain.Membership, error)
	IncrementOrganizationMembershipVersion(context.Context, uuid.UUID, time.Time) error
	IncrementUserMembershipVersion(context.Context, uuid.UUID, time.Time) error

	GetSpace(context.Context, uuid.UUID) (domain.Space, error)
	GetPersonalSpace(context.Context, uuid.UUID) (domain.Space, error)
	GetOrganizationSpace(context.Context, uuid.UUID) (domain.Space, error)
	ListSpaces(context.Context, domain.SpaceListFilter) (domain.SpaceListResult, error)
	UserExistsAndIsActive(context.Context, uuid.UUID) (bool, error)
	InsertSpace(context.Context, domain.NewSpace) (domain.Space, error)
	UpdateSpace(context.Context, domain.Space, int64, time.Time) (domain.Space, error)
	SpaceDeletionBlocked(context.Context, uuid.UUID) (bool, error)
	SetSpaceStatus(context.Context, uuid.UUID, string, int64, time.Time) (domain.Space, error)

	ReserveSpaceQuota(context.Context, uuid.UUID, int64, time.Time) (domain.Space, error)
	InsertQuotaReservation(context.Context, domain.QuotaReservation) (domain.QuotaReservation, error)
	GetQuotaReservationForUpdate(context.Context, uuid.UUID) (domain.QuotaReservation, error)
	ConsumeSpaceQuota(context.Context, uuid.UUID, int64, int64, time.Time) (bool, error)
	ReleaseSpaceQuota(context.Context, uuid.UUID, int64, time.Time) (bool, error)
	MarkReservationConsumed(context.Context, uuid.UUID, time.Time) (domain.QuotaReservation, error)
	MarkReservationReleased(context.Context, uuid.UUID, string, time.Time) (domain.QuotaReservation, error)

	GetPlan(context.Context, uuid.UUID) (domain.OrganizationChangePlan, error)
	GetPlanForUpdate(context.Context, uuid.UUID) (domain.OrganizationChangePlan, error)
	ListPlans(context.Context, domain.PlanListFilter) (domain.PlanListResult, error)
	InsertPlan(context.Context, domain.OrganizationChangePlan) (domain.OrganizationChangePlan, error)
	ListPlanOperations(context.Context, uuid.UUID) ([]domain.OrganizationChangeOperation, error)
	InsertPlanOperation(context.Context, domain.OrganizationChangeOperation) (domain.OrganizationChangeOperation, error)
	TouchDraftPlan(context.Context, uuid.UUID, time.Time) (domain.OrganizationChangePlan, error)
	SetPlanStatus(context.Context, uuid.UUID, string, *uuid.UUID, *string, int64, time.Time) (domain.OrganizationChangePlan, error)
	MarkPlanOperation(context.Context, uuid.UUID, string, *string, time.Time) (domain.OrganizationChangeOperation, error)

	TryCreateIdempotency(context.Context, uuid.UUID, uuid.UUID, string, string, []byte, time.Time, time.Time) (bool, error)
	GetIdempotencyForUpdate(context.Context, uuid.UUID, string, string) (domain.IdempotencyRecord, error)
	CompleteIdempotency(context.Context, uuid.UUID, string, string, uuid.UUID, string, time.Time) error
	InsertEvent(context.Context, domain.Event) error
}

type Transactor interface {
	WithinTransaction(context.Context, func(Repository) error) error
}
