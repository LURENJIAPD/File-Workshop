package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/organizations/application"
	"file-workshop/backend/internal/modules/organizations/domain"
	"file-workshop/backend/internal/platform/database/dbgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgreSQL struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewPostgreSQL(pool *pgxpool.Pool) *PostgreSQL {
	return &PostgreSQL{pool: pool, queries: dbgen.New(pool)}
}

func (r *PostgreSQL) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin organization transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	transactionRepository := &PostgreSQL{pool: r.pool, queries: r.queries.WithTx(tx)}
	if err := operation(transactionRepository); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit organization transaction: %w", mapDatabaseError(err))
	}
	return nil
}

func (r *PostgreSQL) LockTreeMutation(ctx context.Context) error {
	return mapDatabaseError(r.queries.LockOrganizationTreeMutation(ctx))
}

func (r *PostgreSQL) GetOrganization(ctx context.Context, id uuid.UUID) (domain.Organization, error) {
	row, err := r.queries.GetOrganization(ctx, pgUUID(id))
	if err != nil {
		return domain.Organization{}, mapNotFound(err, domain.ErrOrganizationNotFound)
	}
	return organization(row)
}

func (r *PostgreSQL) GetOrganizationForUpdate(ctx context.Context, id uuid.UUID) (domain.Organization, error) {
	row, err := r.queries.GetOrganizationForUpdate(ctx, pgUUID(id))
	if err != nil {
		return domain.Organization{}, mapNotFound(err, domain.ErrOrganizationNotFound)
	}
	return organization(row)
}

func (r *PostgreSQL) ListOrganizations(ctx context.Context, filter domain.OrganizationListFilter) (domain.OrganizationListResult, error) {
	params := &dbgen.ListOrganizationsParams{ParentOrganizationID: optionalUUID(filter.ParentOrganizationID), Status: optionalFilter(filter.Status), PageOffset: pageOffset(filter.Page, filter.PageSize), PageSize: int32(filter.PageSize)}
	rows, err := r.queries.ListOrganizations(ctx, params)
	if err != nil {
		return domain.OrganizationListResult{}, mapDatabaseError(err)
	}
	items := make([]domain.Organization, 0, len(rows))
	for _, row := range rows {
		item, err := organization(row)
		if err != nil {
			return domain.OrganizationListResult{}, err
		}
		items = append(items, item)
	}
	total, err := r.queries.CountOrganizations(ctx, &dbgen.CountOrganizationsParams{ParentOrganizationID: optionalUUID(filter.ParentOrganizationID), Status: optionalFilter(filter.Status)})
	if err != nil {
		return domain.OrganizationListResult{}, mapDatabaseError(err)
	}
	return domain.OrganizationListResult{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *PostgreSQL) InsertOrganization(ctx context.Context, input domain.NewOrganization) (domain.Organization, error) {
	row, err := r.queries.InsertOrganization(ctx, &dbgen.InsertOrganizationParams{OrganizationID: pgUUID(input.ID), ParentOrganizationID: optionalUUID(input.ParentOrganizationID), Name: input.Name, NormalizedName: input.NormalizedName, Code: optionalText(input.Code), NormalizedCode: optionalText(input.NormalizedCode), TypeLabel: optionalText(input.TypeLabel), SortOrder: input.SortOrder, Depth: input.Depth, CreatedByUserID: pgUUID(input.CreatedByUserID), CreatedAt: timestamptz(input.CreatedAt)})
	if err != nil {
		return domain.Organization{}, mapDatabaseError(err)
	}
	return organization(row)
}

func (r *PostgreSQL) InsertOrganizationClosure(ctx context.Context, id uuid.UUID, parentID *uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.InsertOrganizationClosure(ctx, &dbgen.InsertOrganizationClosureParams{OrganizationID: pgUUID(id), ParentOrganizationID: optionalUUID(parentID), CreatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) InsertOrganizationSecurityVersions(ctx context.Context, id uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.InsertOrganizationSecurityVersions(ctx, &dbgen.InsertOrganizationSecurityVersionsParams{OrganizationID: pgUUID(id), UpdatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) UpdateOrganization(ctx context.Context, value domain.Organization, expectedVersion int64, now time.Time) (domain.Organization, error) {
	row, err := r.queries.UpdateOrganization(ctx, &dbgen.UpdateOrganizationParams{OrganizationID: pgUUID(value.ID), Name: value.Name, NormalizedName: value.NormalizedName, Code: optionalText(value.Code), NormalizedCode: optionalText(value.NormalizedCode), TypeLabel: optionalText(value.TypeLabel), SortOrder: value.SortOrder, UpdatedAt: timestamptz(now), RowVersion: expectedVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Organization{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Organization{}, mapDatabaseError(err)
	}
	return organization(row)
}

func (r *PostgreSQL) OrganizationWouldCreateCycle(ctx context.Context, id, parentID uuid.UUID) (bool, error) {
	value, err := r.queries.OrganizationWouldCreateCycle(ctx, &dbgen.OrganizationWouldCreateCycleParams{AncestorOrganizationID: pgUUID(id), DescendantOrganizationID: pgUUID(parentID)})
	return value, mapDatabaseError(err)
}

func (r *PostgreSQL) DeleteExternalClosureLinks(ctx context.Context, id uuid.UUID) error {
	return mapDatabaseError(r.queries.DeleteOrganizationExternalClosureLinks(ctx, pgUUID(id)))
}

func (r *PostgreSQL) InsertMovedClosureLinks(ctx context.Context, id, parentID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.InsertMovedOrganizationClosureLinks(ctx, &dbgen.InsertMovedOrganizationClosureLinksParams{AncestorOrganizationID: pgUUID(id), DescendantOrganizationID: pgUUID(parentID), CreatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) UpdateMovedSubtree(ctx context.Context, id uuid.UUID, parentID *uuid.UUID, depthDelta int32, expectedVersion int64, now time.Time) error {
	return mapDatabaseError(r.queries.UpdateMovedOrganizationSubtree(ctx, &dbgen.UpdateMovedOrganizationSubtreeParams{DepthDelta: depthDelta, OrganizationID: pgUUID(id), NewParentOrganizationID: optionalUUID(parentID), UpdatedAt: timestamptz(now), ExpectedRowVersion: expectedVersion}))
}

func (r *PostgreSQL) IncrementSubtreeSecurityEpochs(ctx context.Context, id uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.IncrementOrganizationSubtreeSecurityEpochs(ctx, &dbgen.IncrementOrganizationSubtreeSecurityEpochsParams{DescendantOrganizationID: pgUUID(id), UpdatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) OrganizationDeletionBlocked(ctx context.Context, id uuid.UUID, now time.Time) (bool, error) {
	value, err := r.queries.OrganizationDeletionBlocked(ctx, &dbgen.OrganizationDeletionBlockedParams{ParentOrganizationID: pgUUID(id), ValidUntil: timestamptz(now)})
	return value, mapDatabaseError(err)
}

func (r *PostgreSQL) SetOrganizationStatus(ctx context.Context, id uuid.UUID, status string, expectedVersion int64, now time.Time) (domain.Organization, error) {
	row, err := r.queries.SetOrganizationStatus(ctx, &dbgen.SetOrganizationStatusParams{OrganizationID: pgUUID(id), Status: status, UpdatedAt: timestamptz(now), RowVersion: expectedVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Organization{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Organization{}, mapDatabaseError(err)
	}
	return organization(row)
}

func (r *PostgreSQL) GetMembership(ctx context.Context, id uuid.UUID) (domain.Membership, error) {
	row, err := r.queries.GetMembership(ctx, pgUUID(id))
	if err != nil {
		return domain.Membership{}, mapNotFound(err, domain.ErrMembershipNotFound)
	}
	return membership(row)
}

func (r *PostgreSQL) GetMembershipForUpdate(ctx context.Context, organizationID, id uuid.UUID) (domain.Membership, error) {
	row, err := r.queries.GetMembershipForUpdate(ctx, &dbgen.GetMembershipForUpdateParams{UserOrganizationID: pgUUID(id), OrganizationID: pgUUID(organizationID)})
	if err != nil {
		return domain.Membership{}, mapNotFound(err, domain.ErrMembershipNotFound)
	}
	return membership(row)
}

func (r *PostgreSQL) ListMemberships(ctx context.Context, filter domain.MembershipListFilter) (domain.MembershipListResult, error) {
	params := &dbgen.ListMembershipsParams{OrganizationID: optionalUUID(filter.OrganizationID), UserID: optionalUUID(filter.UserID), Status: optionalFilter(filter.Status), EffectiveAt: optionalTimeValue(filter.EffectiveAt), PageOffset: pageOffset(filter.Page, filter.PageSize), PageSize: int32(filter.PageSize)}
	rows, err := r.queries.ListMemberships(ctx, params)
	if err != nil {
		return domain.MembershipListResult{}, mapDatabaseError(err)
	}
	items := make([]domain.Membership, 0, len(rows))
	for _, row := range rows {
		item, err := membership(row)
		if err != nil {
			return domain.MembershipListResult{}, err
		}
		items = append(items, item)
	}
	total, err := r.queries.CountMemberships(ctx, &dbgen.CountMembershipsParams{OrganizationID: optionalUUID(filter.OrganizationID), UserID: optionalUUID(filter.UserID), Status: optionalFilter(filter.Status), EffectiveAt: optionalTimeValue(filter.EffectiveAt)})
	if err != nil {
		return domain.MembershipListResult{}, mapDatabaseError(err)
	}
	return domain.MembershipListResult{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *PostgreSQL) UserCanJoinOrganization(ctx context.Context, userID, organizationID uuid.UUID) (bool, error) {
	value, err := r.queries.UserCanJoinOrganization(ctx, &dbgen.UserCanJoinOrganizationParams{UserID: pgUUID(userID), OrganizationID: pgUUID(organizationID)})
	return value.Valid && value.Bool, mapDatabaseError(err)
}

func (r *PostgreSQL) InsertMembership(ctx context.Context, input domain.NewMembership) (domain.Membership, error) {
	row, err := r.queries.InsertMembership(ctx, &dbgen.InsertMembershipParams{UserOrganizationID: pgUUID(input.ID), UserID: pgUUID(input.UserID), OrganizationID: pgUUID(input.OrganizationID), MembershipType: input.MembershipType, JobTitle: optionalText(input.JobTitle), EffectiveFrom: timestamptz(input.EffectiveFrom), EffectiveUntil: optionalTimeValue(input.EffectiveUntil), CreatedByUserID: pgUUID(input.CreatedByUserID), CreatedAt: timestamptz(input.CreatedAt)})
	if err != nil {
		return domain.Membership{}, mapDatabaseError(err)
	}
	return membership(row)
}

func (r *PostgreSQL) UpdateMembership(ctx context.Context, value domain.Membership, expectedVersion int64, now time.Time) (domain.Membership, error) {
	row, err := r.queries.UpdateMembership(ctx, &dbgen.UpdateMembershipParams{UserOrganizationID: pgUUID(value.ID), OrganizationID: pgUUID(value.OrganizationID), MembershipType: value.MembershipType, JobTitle: optionalText(value.JobTitle), Status: value.Status, EffectiveUntil: optionalTimeValue(value.EffectiveUntil), UpdatedAt: timestamptz(now), RowVersion: expectedVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Membership{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Membership{}, mapDatabaseError(err)
	}
	return membership(row)
}

func (r *PostgreSQL) DeactivateMembership(ctx context.Context, organizationID, id uuid.UUID, expectedVersion int64, now time.Time) (domain.Membership, error) {
	row, err := r.queries.DeactivateMembership(ctx, &dbgen.DeactivateMembershipParams{UserOrganizationID: pgUUID(id), OrganizationID: pgUUID(organizationID), UpdatedAt: timestamptz(now), RowVersion: expectedVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Membership{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Membership{}, mapDatabaseError(err)
	}
	return membership(row)
}

func (r *PostgreSQL) IncrementOrganizationMembershipVersion(ctx context.Context, organizationID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.IncrementOrganizationMembershipVersion(ctx, &dbgen.IncrementOrganizationMembershipVersionParams{OrganizationID: pgUUID(organizationID), UpdatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) IncrementUserMembershipVersion(ctx context.Context, userID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.IncrementUserOrganizationMembershipVersion(ctx, &dbgen.IncrementUserOrganizationMembershipVersionParams{UserID: pgUUID(userID), UpdatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) GetSpace(ctx context.Context, id uuid.UUID) (domain.Space, error) {
	row, err := r.queries.GetSpace(ctx, pgUUID(id))
	if err != nil {
		return domain.Space{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	return space(row)
}

func (r *PostgreSQL) GetPersonalSpace(ctx context.Context, userID uuid.UUID) (domain.Space, error) {
	row, err := r.queries.GetPersonalSpaceByUser(ctx, pgUUID(userID))
	if err != nil {
		return domain.Space{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	return space(row)
}

func (r *PostgreSQL) GetOrganizationSpace(ctx context.Context, organizationID uuid.UUID) (domain.Space, error) {
	row, err := r.queries.GetOrganizationSpace(ctx, pgUUID(organizationID))
	if err != nil {
		return domain.Space{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	return space(row)
}

func (r *PostgreSQL) ListSpaces(ctx context.Context, filter domain.SpaceListFilter) (domain.SpaceListResult, error) {
	params := &dbgen.ListSpacesParams{SpaceType: optionalFilter(filter.SpaceType), Status: optionalFilter(filter.Status), OrganizationID: optionalUUID(filter.OrganizationID), OwnerUserID: optionalUUID(filter.OwnerUserID), PageOffset: pageOffset(filter.Page, filter.PageSize), PageSize: int32(filter.PageSize)}
	rows, err := r.queries.ListSpaces(ctx, params)
	if err != nil {
		return domain.SpaceListResult{}, mapDatabaseError(err)
	}
	items := make([]domain.Space, 0, len(rows))
	for _, row := range rows {
		item, err := space(row)
		if err != nil {
			return domain.SpaceListResult{}, err
		}
		items = append(items, item)
	}
	total, err := r.queries.CountSpaces(ctx, &dbgen.CountSpacesParams{SpaceType: optionalFilter(filter.SpaceType), Status: optionalFilter(filter.Status), OrganizationID: optionalUUID(filter.OrganizationID), OwnerUserID: optionalUUID(filter.OwnerUserID)})
	if err != nil {
		return domain.SpaceListResult{}, mapDatabaseError(err)
	}
	return domain.SpaceListResult{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *PostgreSQL) UserExistsAndIsActive(ctx context.Context, userID uuid.UUID) (bool, error) {
	value, err := r.queries.UserExistsAndIsActive(ctx, pgUUID(userID))
	return value, mapDatabaseError(err)
}

func (r *PostgreSQL) InsertSpace(ctx context.Context, input domain.NewSpace) (domain.Space, error) {
	row, err := r.queries.InsertSpace(ctx, &dbgen.InsertSpaceParams{SpaceID: pgUUID(input.ID), SpaceType: input.SpaceType, Name: input.Name, NormalizedName: input.NormalizedName, OwnerUserID: optionalUUID(input.OwnerUserID), OrganizationID: optionalUUID(input.OrganizationID), QuotaBytes: input.QuotaBytes, ConfigSchemaVersion: input.ConfigSchemaVersion, ConfigJson: input.ConfigJSON, CreatedByUserID: pgUUID(input.CreatedByUserID), CreatedAt: timestamptz(input.CreatedAt)})
	if err != nil {
		return domain.Space{}, mapDatabaseError(err)
	}
	return space(row)
}

func (r *PostgreSQL) UpdateSpace(ctx context.Context, value domain.Space, expectedVersion int64, now time.Time) (domain.Space, error) {
	row, err := r.queries.UpdateSpace(ctx, &dbgen.UpdateSpaceParams{SpaceID: pgUUID(value.ID), Name: value.Name, NormalizedName: value.NormalizedName, QuotaBytes: value.QuotaBytes, ConfigSchemaVersion: value.ConfigSchemaVersion, ConfigJson: value.ConfigJSON, UpdatedAt: timestamptz(now), RowVersion: expectedVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Space{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Space{}, mapDatabaseError(err)
	}
	return space(row)
}

func (r *PostgreSQL) SpaceDeletionBlocked(ctx context.Context, id uuid.UUID) (bool, error) {
	value, err := r.queries.SpaceDeletionBlocked(ctx, pgUUID(id))
	return value, mapDatabaseError(err)
}

func (r *PostgreSQL) SetSpaceStatus(ctx context.Context, id uuid.UUID, status string, expectedVersion int64, now time.Time) (domain.Space, error) {
	row, err := r.queries.SetSpaceStatus(ctx, &dbgen.SetSpaceStatusParams{SpaceID: pgUUID(id), Status: status, UpdatedAt: timestamptz(now), RowVersion: expectedVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Space{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Space{}, mapDatabaseError(err)
	}
	return space(row)
}

func (r *PostgreSQL) ReserveSpaceQuota(ctx context.Context, id uuid.UUID, bytes int64, now time.Time) (domain.Space, error) {
	row, err := r.queries.ReserveSpaceQuota(ctx, &dbgen.ReserveSpaceQuotaParams{SpaceID: pgUUID(id), ReservedBytes: bytes, UpdatedAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Space{}, domain.ErrQuotaExceeded
	}
	if err != nil {
		return domain.Space{}, mapDatabaseError(err)
	}
	return space(row)
}

func (r *PostgreSQL) InsertQuotaReservation(ctx context.Context, input domain.QuotaReservation) (domain.QuotaReservation, error) {
	row, err := r.queries.InsertQuotaReservation(ctx, &dbgen.InsertQuotaReservationParams{QuotaReservationID: pgUUID(input.ID), SpaceID: pgUUID(input.SpaceID), UserID: pgUUID(input.UserID), ReservedBytes: input.ReservedBytes, ExpiresAt: timestamptz(input.ExpiresAt), CreatedAt: timestamptz(input.CreatedAt)})
	if err != nil {
		return domain.QuotaReservation{}, mapDatabaseError(err)
	}
	return reservation(row)
}

func (r *PostgreSQL) GetQuotaReservationForUpdate(ctx context.Context, id uuid.UUID) (domain.QuotaReservation, error) {
	row, err := r.queries.GetQuotaReservationForUpdate(ctx, pgUUID(id))
	if err != nil {
		return domain.QuotaReservation{}, mapNotFound(err, domain.ErrReservationNotFound)
	}
	return reservation(row)
}

func (r *PostgreSQL) ConsumeSpaceQuota(ctx context.Context, spaceID uuid.UUID, reservedBytes, usedBytes int64, now time.Time) (bool, error) {
	rows, err := r.queries.ConsumeSpaceQuotaReservation(ctx, &dbgen.ConsumeSpaceQuotaReservationParams{SpaceID: pgUUID(spaceID), ReservedBytes: reservedBytes, UsedBytes: usedBytes, UpdatedAt: timestamptz(now)})
	return rows == 1, mapDatabaseError(err)
}

func (r *PostgreSQL) ReleaseSpaceQuota(ctx context.Context, spaceID uuid.UUID, reservedBytes int64, now time.Time) (bool, error) {
	rows, err := r.queries.ReleaseSpaceQuotaReservation(ctx, &dbgen.ReleaseSpaceQuotaReservationParams{SpaceID: pgUUID(spaceID), ReservedBytes: reservedBytes, UpdatedAt: timestamptz(now)})
	return rows == 1, mapDatabaseError(err)
}

func (r *PostgreSQL) MarkReservationConsumed(ctx context.Context, id uuid.UUID, now time.Time) (domain.QuotaReservation, error) {
	row, err := r.queries.MarkQuotaReservationConsumed(ctx, &dbgen.MarkQuotaReservationConsumedParams{QuotaReservationID: pgUUID(id), ConsumedAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.QuotaReservation{}, domain.ErrConflict
	}
	if err != nil {
		return domain.QuotaReservation{}, mapDatabaseError(err)
	}
	return reservation(row)
}

func (r *PostgreSQL) MarkReservationReleased(ctx context.Context, id uuid.UUID, status string, now time.Time) (domain.QuotaReservation, error) {
	row, err := r.queries.MarkQuotaReservationReleased(ctx, &dbgen.MarkQuotaReservationReleasedParams{QuotaReservationID: pgUUID(id), Status: status, ReleasedAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.QuotaReservation{}, domain.ErrConflict
	}
	if err != nil {
		return domain.QuotaReservation{}, mapDatabaseError(err)
	}
	return reservation(row)
}

func (r *PostgreSQL) GetPlan(ctx context.Context, id uuid.UUID) (domain.OrganizationChangePlan, error) {
	row, err := r.queries.GetOrganizationChangePlan(ctx, pgUUID(id))
	if err != nil {
		return domain.OrganizationChangePlan{}, mapNotFound(err, domain.ErrPlanNotFound)
	}
	return plan(row)
}

func (r *PostgreSQL) GetPlanForUpdate(ctx context.Context, id uuid.UUID) (domain.OrganizationChangePlan, error) {
	row, err := r.queries.GetOrganizationChangePlanForUpdate(ctx, pgUUID(id))
	if err != nil {
		return domain.OrganizationChangePlan{}, mapNotFound(err, domain.ErrPlanNotFound)
	}
	return plan(row)
}

func (r *PostgreSQL) ListPlans(ctx context.Context, filter domain.PlanListFilter) (domain.PlanListResult, error) {
	rows, err := r.queries.ListOrganizationChangePlans(ctx, &dbgen.ListOrganizationChangePlansParams{Status: optionalFilter(filter.Status), PageOffset: pageOffset(filter.Page, filter.PageSize), PageSize: int32(filter.PageSize)})
	if err != nil {
		return domain.PlanListResult{}, mapDatabaseError(err)
	}
	items := make([]domain.OrganizationChangePlan, 0, len(rows))
	for _, row := range rows {
		item, err := plan(row)
		if err != nil {
			return domain.PlanListResult{}, err
		}
		items = append(items, item)
	}
	total, err := r.queries.CountOrganizationChangePlans(ctx, optionalFilter(filter.Status))
	if err != nil {
		return domain.PlanListResult{}, mapDatabaseError(err)
	}
	return domain.PlanListResult{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *PostgreSQL) InsertPlan(ctx context.Context, input domain.OrganizationChangePlan) (domain.OrganizationChangePlan, error) {
	row, err := r.queries.InsertOrganizationChangePlan(ctx, &dbgen.InsertOrganizationChangePlanParams{OrganizationChangePlanID: pgUUID(input.ID), PlanType: input.PlanType, Name: input.Name, ExpectedTreeVersion: input.ExpectedTreeVersion, CreatedByUserID: pgUUID(input.CreatedByUserID), CreatedAt: timestamptz(input.CreatedAt)})
	if err != nil {
		return domain.OrganizationChangePlan{}, mapDatabaseError(err)
	}
	return plan(row)
}

func (r *PostgreSQL) ListPlanOperations(ctx context.Context, id uuid.UUID) ([]domain.OrganizationChangeOperation, error) {
	rows, err := r.queries.ListOrganizationChangeOperations(ctx, pgUUID(id))
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	items := make([]domain.OrganizationChangeOperation, 0, len(rows))
	for _, row := range rows {
		item, err := planOperation(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *PostgreSQL) InsertPlanOperation(ctx context.Context, input domain.OrganizationChangeOperation) (domain.OrganizationChangeOperation, error) {
	row, err := r.queries.InsertOrganizationChangeOperation(ctx, &dbgen.InsertOrganizationChangeOperationParams{OrganizationChangeOperationID: pgUUID(input.ID), OrganizationChangePlanID: pgUUID(input.PlanID), SequenceNumber: input.SequenceNumber, OperationType: input.OperationType, SourceOrganizationID: optionalUUID(input.SourceOrganizationID), TargetOrganizationID: optionalUUID(input.TargetOrganizationID), OperationSchemaVersion: input.OperationSchemaVersion, OperationJson: input.OperationJSON, CreatedAt: timestamptz(input.CreatedAt)})
	if err != nil {
		return domain.OrganizationChangeOperation{}, mapDatabaseError(err)
	}
	return planOperation(row)
}

func (r *PostgreSQL) TouchDraftPlan(ctx context.Context, id uuid.UUID, now time.Time) (domain.OrganizationChangePlan, error) {
	row, err := r.queries.TouchDraftOrganizationChangePlan(ctx, &dbgen.TouchDraftOrganizationChangePlanParams{OrganizationChangePlanID: pgUUID(id), UpdatedAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OrganizationChangePlan{}, domain.ErrInvalidStateTransition
	}
	if err != nil {
		return domain.OrganizationChangePlan{}, mapDatabaseError(err)
	}
	return plan(row)
}

func (r *PostgreSQL) SetPlanStatus(ctx context.Context, id uuid.UUID, status string, approvedBy *uuid.UUID, failureCode *string, expectedVersion int64, now time.Time) (domain.OrganizationChangePlan, error) {
	row, err := r.queries.SetOrganizationChangePlanStatus(ctx, &dbgen.SetOrganizationChangePlanStatusParams{OrganizationChangePlanID: pgUUID(id), Status: status, ApprovedByUserID: optionalUUID(approvedBy), UpdatedAt: timestamptz(now), FailureCode: optionalText(failureCode), RowVersion: expectedVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OrganizationChangePlan{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.OrganizationChangePlan{}, mapDatabaseError(err)
	}
	return plan(row)
}

func (r *PostgreSQL) MarkPlanOperation(ctx context.Context, id uuid.UUID, status string, failureCode *string, now time.Time) (domain.OrganizationChangeOperation, error) {
	row, err := r.queries.MarkOrganizationChangeOperation(ctx, &dbgen.MarkOrganizationChangeOperationParams{OrganizationChangeOperationID: pgUUID(id), Status: status, CompletedAt: timestamptz(now), FailureCode: optionalText(failureCode)})
	if err != nil {
		return domain.OrganizationChangeOperation{}, mapDatabaseError(err)
	}
	return planOperation(row)
}

func (r *PostgreSQL) TryCreateIdempotency(ctx context.Context, recordID, actorID uuid.UUID, operation, key string, hash []byte, expiresAt, now time.Time) (bool, error) {
	rows, err := r.queries.TryCreateOrganizationIdempotencyRecord(ctx, &dbgen.TryCreateOrganizationIdempotencyRecordParams{IdempotencyRecordID: pgUUID(recordID), UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, RequestHash: hash, ExpiresAt: timestamptz(expiresAt), CreatedAt: timestamptz(now)})
	return rows == 1, mapDatabaseError(err)
}

func (r *PostgreSQL) GetIdempotencyForUpdate(ctx context.Context, actorID uuid.UUID, operation, key string) (domain.IdempotencyRecord, error) {
	row, err := r.queries.GetOrganizationIdempotencyRecordForUpdate(ctx, &dbgen.GetOrganizationIdempotencyRecordForUpdateParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key})
	if err != nil {
		return domain.IdempotencyRecord{}, mapDatabaseError(err)
	}
	return domain.IdempotencyRecord{RequestHash: row.RequestHash, Status: row.Status, ResultResourceID: optionalGoogleUUID(row.ResultResourceID)}, nil
}

func (r *PostgreSQL) CompleteIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string, resourceID uuid.UUID, resourceType string, now time.Time) error {
	return mapDatabaseError(r.queries.CompleteOrganizationIdempotencyRecord(ctx, &dbgen.CompleteOrganizationIdempotencyRecordParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, ResultResourceID: pgUUID(resourceID), ResultResourceType: pgtype.Text{String: resourceType, Valid: true}, CompletedAt: timestamptz(now)}))
}

func (r *PostgreSQL) InsertEvent(ctx context.Context, event domain.Event) error {
	return mapDatabaseError(r.queries.InsertOrganizationOutboxEvent(ctx, &dbgen.InsertOrganizationOutboxEventParams{OutboxEventID: pgUUID(event.ID), AggregateType: event.AggregateType, AggregateID: pgUUID(event.AggregateID), AggregateVersion: event.AggregateVersion, EventType: event.Type, PayloadJson: event.Payload, DeduplicationKey: event.DeduplicationKey, CorrelationID: pgUUID(event.CorrelationID), AvailableAt: timestamptz(event.CreatedAt)}))
}

func organization(row *dbgen.Organization) (domain.Organization, error) {
	id, err := googleUUID(row.OrganizationID)
	if err != nil {
		return domain.Organization{}, err
	}
	creator, err := googleUUID(row.CreatedByUserID)
	if err != nil {
		return domain.Organization{}, err
	}
	return domain.Organization{ID: id, ParentOrganizationID: optionalGoogleUUID(row.ParentOrganizationID), Name: row.Name, NormalizedName: row.NormalizedName, Code: optionalString(row.Code), NormalizedCode: optionalString(row.NormalizedCode), TypeLabel: optionalString(row.TypeLabel), SortOrder: row.SortOrder, PathCache: optionalString(row.PathCache), Depth: row.Depth, TreeVersion: row.TreeVersion, Status: row.Status, CreatedByUserID: creator, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), DeletedAt: optionalTime(row.DeletedAt), RowVersion: row.RowVersion}, nil
}

func membership(row *dbgen.UserOrganization) (domain.Membership, error) {
	id, err := googleUUID(row.UserOrganizationID)
	if err != nil {
		return domain.Membership{}, err
	}
	userID, err := googleUUID(row.UserID)
	if err != nil {
		return domain.Membership{}, err
	}
	organizationID, err := googleUUID(row.OrganizationID)
	if err != nil {
		return domain.Membership{}, err
	}
	creator, err := googleUUID(row.CreatedByUserID)
	if err != nil {
		return domain.Membership{}, err
	}
	return domain.Membership{ID: id, UserID: userID, OrganizationID: organizationID, MembershipType: row.MembershipType, JobTitle: optionalString(row.JobTitle), Status: row.Status, EffectiveFrom: row.EffectiveFrom.Time.UTC(), EffectiveUntil: optionalTime(row.EffectiveUntil), CreatedByUserID: creator, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), RowVersion: row.RowVersion}, nil
}

func space(row *dbgen.Space) (domain.Space, error) {
	id, err := googleUUID(row.SpaceID)
	if err != nil {
		return domain.Space{}, err
	}
	creator, err := googleUUID(row.CreatedByUserID)
	if err != nil {
		return domain.Space{}, err
	}
	return domain.Space{ID: id, SpaceType: row.SpaceType, Name: row.Name, NormalizedName: row.NormalizedName, OwnerUserID: optionalGoogleUUID(row.OwnerUserID), OrganizationID: optionalGoogleUUID(row.OrganizationID), RootFolderID: optionalGoogleUUID(row.RootFolderID), QuotaBytes: row.QuotaBytes, UsedBytes: row.UsedBytes, ReservedBytes: row.ReservedBytes, ACLVersion: row.AclVersion, SecurityEpoch: row.SecurityEpoch, ConfigSchemaVersion: row.ConfigSchemaVersion, ConfigJSON: append([]byte(nil), row.ConfigJson...), Status: row.Status, CreatedByUserID: creator, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), DeletedAt: optionalTime(row.DeletedAt), RowVersion: row.RowVersion}, nil
}

func reservation(row *dbgen.QuotaReservation) (domain.QuotaReservation, error) {
	id, err := googleUUID(row.QuotaReservationID)
	if err != nil {
		return domain.QuotaReservation{}, err
	}
	spaceID, err := googleUUID(row.SpaceID)
	if err != nil {
		return domain.QuotaReservation{}, err
	}
	userID, err := googleUUID(row.UserID)
	if err != nil {
		return domain.QuotaReservation{}, err
	}
	return domain.QuotaReservation{ID: id, SpaceID: spaceID, UserID: userID, ReservedBytes: row.ReservedBytes, Status: row.Status, ExpiresAt: row.ExpiresAt.Time.UTC(), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), ConsumedAt: optionalTime(row.ConsumedAt), ReleasedAt: optionalTime(row.ReleasedAt), RowVersion: row.RowVersion}, nil
}

func plan(row *dbgen.OrganizationChangePlan) (domain.OrganizationChangePlan, error) {
	id, err := googleUUID(row.OrganizationChangePlanID)
	if err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	creator, err := googleUUID(row.CreatedByUserID)
	if err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	return domain.OrganizationChangePlan{ID: id, PlanType: row.PlanType, Name: row.Name, Status: row.Status, ExpectedTreeVersion: row.ExpectedTreeVersion, CreatedByUserID: creator, ApprovedByUserID: optionalGoogleUUID(row.ApprovedByUserID), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), ValidatedAt: optionalTime(row.ValidatedAt), ApprovedAt: optionalTime(row.ApprovedAt), StartedAt: optionalTime(row.StartedAt), CompletedAt: optionalTime(row.CompletedAt), FailureCode: optionalString(row.FailureCode), RowVersion: row.RowVersion, Operations: []domain.OrganizationChangeOperation{}}, nil
}

func planOperation(row *dbgen.OrganizationChangeOperation) (domain.OrganizationChangeOperation, error) {
	id, err := googleUUID(row.OrganizationChangeOperationID)
	if err != nil {
		return domain.OrganizationChangeOperation{}, err
	}
	planID, err := googleUUID(row.OrganizationChangePlanID)
	if err != nil {
		return domain.OrganizationChangeOperation{}, err
	}
	return domain.OrganizationChangeOperation{ID: id, PlanID: planID, SequenceNumber: row.SequenceNumber, OperationType: row.OperationType, SourceOrganizationID: optionalGoogleUUID(row.SourceOrganizationID), TargetOrganizationID: optionalGoogleUUID(row.TargetOrganizationID), OperationSchemaVersion: row.OperationSchemaVersion, OperationJSON: append([]byte(nil), row.OperationJson...), Status: row.Status, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), CompletedAt: optionalTime(row.CompletedAt), FailureCode: optionalString(row.FailureCode), RowVersion: row.RowVersion}, nil
}

func mapNotFound(err, target error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return target
	}
	return mapDatabaseError(err)
}

func mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%w: unique value", domain.ErrConflict)
		case "23514":
			return fmt.Errorf("%w: database constraint", domain.ErrConflict)
		case "23503":
			return fmt.Errorf("%w: referenced resource", domain.ErrConflict)
		}
	}
	return err
}

func pageOffset(page, pageSize int) int64 { return int64(page-1) * int64(pageSize) }
func pgUUID(value uuid.UUID) pgtype.UUID  { return pgtype.UUID{Bytes: value, Valid: true} }
func optionalUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgUUID(*value)
}
func googleUUID(value pgtype.UUID) (uuid.UUID, error) {
	if !value.Valid {
		return uuid.Nil, fmt.Errorf("database UUID is null")
	}
	return uuid.UUID(value.Bytes), nil
}
func optionalGoogleUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	result := uuid.UUID(value.Bytes)
	return &result
}
func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
func optionalFilter(value *string) pgtype.Text { return optionalText(value) }
func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
func optionalTimeValue(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*value)
}
func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
