package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/permissions/application"
	"file-workshop/backend/internal/modules/permissions/domain"
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
		return fmt.Errorf("begin permission transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	repository := &PostgreSQL{pool: r.pool, queries: r.queries.WithTx(tx)}
	if err = operation(repository); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return mapDatabaseError(err)
	}
	return nil
}

func (r *PostgreSQL) GetAdminDelegation(ctx context.Context, id uuid.UUID) (domain.AdminDelegation, error) {
	row, err := r.queries.GetAdminDelegationWithCapabilities(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminDelegation{}, domain.ErrDelegationNotFound
	}
	if err != nil {
		return domain.AdminDelegation{}, mapDatabaseError(err)
	}
	return delegation(row.AdminDelegationID, row.UserID, row.OrganizationID, row.Scope, row.CanDelegate, row.ParentAdminDelegationID, row.GrantedByUserID, row.ValidFrom, row.ValidUntil, row.Status, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokeReason, row.RowVersion, row.Capabilities), nil
}

func (r *PostgreSQL) GetAdminDelegationForUpdate(ctx context.Context, id uuid.UUID) (domain.AdminDelegation, error) {
	row, err := r.queries.GetAdminDelegationWithCapabilitiesForUpdate(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminDelegation{}, domain.ErrDelegationNotFound
	}
	if err != nil {
		return domain.AdminDelegation{}, mapDatabaseError(err)
	}
	return delegation(row.AdminDelegationID, row.UserID, row.OrganizationID, row.Scope, row.CanDelegate, row.ParentAdminDelegationID, row.GrantedByUserID, row.ValidFrom, row.ValidUntil, row.Status, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokeReason, row.RowVersion, row.Capabilities), nil
}

func (r *PostgreSQL) ListAdminDelegations(ctx context.Context, filter domain.AdminDelegationListFilter) (domain.AdminDelegationListResult, error) {
	params := &dbgen.CountVisibleAdminDelegationsParams{ViewerIsAdmin: filter.ViewerIsAdmin, ViewerUserID: pgUUID(filter.ViewerUserID), OrganizationID: optionalUUID(filter.OrganizationID), Status: optionalText(filter.Status)}
	total, err := r.queries.CountVisibleAdminDelegations(ctx, params)
	if err != nil {
		return domain.AdminDelegationListResult{}, mapDatabaseError(err)
	}
	rows, err := r.queries.ListVisibleAdminDelegations(ctx, &dbgen.ListVisibleAdminDelegationsParams{ViewerIsAdmin: params.ViewerIsAdmin, ViewerUserID: params.ViewerUserID, OrganizationID: params.OrganizationID, Status: params.Status, PageOffset: pageOffset(filter.Page, filter.PageSize), PageSize: int32(filter.PageSize)})
	if err != nil {
		return domain.AdminDelegationListResult{}, mapDatabaseError(err)
	}
	items := make([]domain.AdminDelegation, 0, len(rows))
	for _, row := range rows {
		items = append(items, delegation(row.AdminDelegationID, row.UserID, row.OrganizationID, row.Scope, row.CanDelegate, row.ParentAdminDelegationID, row.GrantedByUserID, row.ValidFrom, row.ValidUntil, row.Status, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokeReason, row.RowVersion, row.Capabilities))
	}
	return domain.AdminDelegationListResult{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *PostgreSQL) ListOrganizationAdministrators(ctx context.Context, organizationID uuid.UUID, page, pageSize int, at time.Time) (domain.AdminDelegationListResult, error) {
	total, err := r.queries.CountOrganizationAdministrators(ctx, &dbgen.CountOrganizationAdministratorsParams{OrganizationID: pgUUID(organizationID), EffectiveAt: timestamp(at)})
	if err != nil {
		return domain.AdminDelegationListResult{}, mapDatabaseError(err)
	}
	rows, err := r.queries.ListOrganizationAdministrators(ctx, &dbgen.ListOrganizationAdministratorsParams{OrganizationID: pgUUID(organizationID), PageOffset: pageOffset(page, pageSize), PageSize: int32(pageSize), EffectiveAt: timestamp(at)})
	if err != nil {
		return domain.AdminDelegationListResult{}, mapDatabaseError(err)
	}
	items := make([]domain.AdminDelegation, 0, len(rows))
	for _, row := range rows {
		items = append(items, delegation(row.AdminDelegationID, row.UserID, row.OrganizationID, row.Scope, row.CanDelegate, row.ParentAdminDelegationID, row.GrantedByUserID, row.ValidFrom, row.ValidUntil, row.Status, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokeReason, row.RowVersion, row.Capabilities))
	}
	return domain.AdminDelegationListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r *PostgreSQL) OrganizationExists(ctx context.Context, organizationID uuid.UUID) (bool, error) {
	value, err := r.queries.PermissionOrganizationExists(ctx, pgUUID(organizationID))
	return value, mapDatabaseError(err)
}

func (r *PostgreSQL) InsertAdminDelegation(ctx context.Context, value domain.NewAdminDelegation) (domain.AdminDelegation, error) {
	row, err := r.queries.InsertAdminDelegation(ctx, &dbgen.InsertAdminDelegationParams{AdminDelegationID: pgUUID(value.ID), UserID: pgUUID(value.UserID), OrganizationID: pgUUID(value.OrganizationID), Scope: value.Scope, CanDelegate: value.CanDelegate, ParentAdminDelegationID: optionalUUID(value.ParentDelegationID), GrantedByUserID: pgUUID(value.GrantedByUserID), ValidFrom: timestamp(value.ValidFrom), ValidUntil: optionalTime(value.ValidUntil), CreatedAt: timestamp(value.CreatedAt)})
	if err != nil {
		return domain.AdminDelegation{}, mapDatabaseError(err)
	}
	for _, capability := range value.Capabilities {
		if err = r.queries.InsertAdminDelegationCapability(ctx, &dbgen.InsertAdminDelegationCapabilityParams{AdminDelegationID: pgUUID(value.ID), Capability: capability, CreatedAt: timestamp(value.CreatedAt)}); err != nil {
			return domain.AdminDelegation{}, mapDatabaseError(err)
		}
	}
	return delegation(row.AdminDelegationID, row.UserID, row.OrganizationID, row.Scope, row.CanDelegate, row.ParentAdminDelegationID, row.GrantedByUserID, row.ValidFrom, row.ValidUntil, row.Status, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokeReason, row.RowVersion, value.Capabilities), nil
}

func (r *PostgreSQL) RevokeAdminDelegation(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, at time.Time) (domain.AdminDelegation, error) {
	row, err := r.queries.RevokeAdminDelegation(ctx, &dbgen.RevokeAdminDelegationParams{AdminDelegationID: pgUUID(id), RowVersion: rowVersion, RevokedAt: timestamp(at), RevokeReason: pgtype.Text{String: reason, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminDelegation{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.AdminDelegation{}, mapDatabaseError(err)
	}
	capabilities, err := r.queries.GetAdminDelegationWithCapabilities(ctx, pgUUID(id))
	if err != nil {
		return domain.AdminDelegation{}, mapDatabaseError(err)
	}
	return delegation(row.AdminDelegationID, row.UserID, row.OrganizationID, row.Scope, row.CanDelegate, row.ParentAdminDelegationID, row.GrantedByUserID, row.ValidFrom, row.ValidUntil, row.Status, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokeReason, row.RowVersion, capabilities.Capabilities), nil
}

func (r *PostgreSQL) InvalidateDescendantDelegations(ctx context.Context, id uuid.UUID, at time.Time) ([]uuid.UUID, error) {
	rows, err := r.queries.InvalidateDescendantAdminDelegations(ctx, &dbgen.InvalidateDescendantAdminDelegationsParams{ParentAdminDelegationID: pgUUID(id), UpdatedAt: timestamp(at)})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	values := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		values = append(values, uuidValue(row))
	}
	return values, nil
}

func (r *PostgreSQL) FindEffectiveAdminDelegation(ctx context.Context, userID, organizationID uuid.UUID, capability string, at time.Time) (*domain.AdminDelegation, error) {
	row, err := r.queries.FindEffectiveAdminDelegation(ctx, &dbgen.FindEffectiveAdminDelegationParams{Capability: capability, UserID: pgUUID(userID), EffectiveAt: timestamp(at), OrganizationID: pgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	value := delegation(row.AdminDelegationID, row.UserID, row.OrganizationID, row.Scope, row.CanDelegate, row.ParentAdminDelegationID, row.GrantedByUserID, row.ValidFrom, row.ValidUntil, row.Status, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokeReason, row.RowVersion, row.Capabilities)
	return &value, nil
}

func (r *PostgreSQL) AdminDelegationIsEffective(ctx context.Context, id uuid.UUID, at time.Time) (bool, error) {
	value, err := r.queries.AdminDelegationIsEffective(ctx, &dbgen.AdminDelegationIsEffectiveParams{AdminDelegationID: pgUUID(id), EffectiveAt: timestamp(at)})
	return value, mapDatabaseError(err)
}

func (r *PostgreSQL) IncrementDelegationSecurityVersions(ctx context.Context, userID, organizationID uuid.UUID, at time.Time) error {
	if err := r.queries.IncrementPrincipalDelegationVersion(ctx, &dbgen.IncrementPrincipalDelegationVersionParams{UserID: pgUUID(userID), UpdatedAt: timestamp(at)}); err != nil {
		return mapDatabaseError(err)
	}
	return mapDatabaseError(r.queries.IncrementOrganizationDelegationVersion(ctx, &dbgen.IncrementOrganizationDelegationVersionParams{DescendantOrganizationID: pgUUID(organizationID), UpdatedAt: timestamp(at)}))
}

func (r *PostgreSQL) GetResource(ctx context.Context, resourceType string, id uuid.UUID) (domain.Resource, error) {
	result := domain.Resource{Type: resourceType, ID: id, InheritanceMode: domain.InheritanceInherit}
	switch resourceType {
	case domain.ResourceSpace:
		row, err := r.queries.GetSpaceAuthorizationResource(ctx, pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Resource{}, domain.ErrNotFound
		}
		if err != nil {
			return domain.Resource{}, mapDatabaseError(err)
		}
		result.SpaceID = uuidValue(row.SpaceID)
		result.SpaceType = row.SpaceType
		result.SpaceOwnerUserID = uuidPointer(row.OwnerUserID)
		result.OrganizationID = uuidPointer(row.OrganizationID)
		result.ACLVersion = row.AclVersion
		result.RowVersion = row.RowVersion
	case domain.ResourceFolder:
		row, err := r.queries.GetFolderAuthorizationResource(ctx, pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Resource{}, domain.ErrNotFound
		}
		if err != nil {
			return domain.Resource{}, mapDatabaseError(err)
		}
		result.SpaceID = uuidValue(row.SpaceID)
		result.SpaceType = row.SpaceType
		result.SpaceOwnerUserID = uuidPointer(row.OwnerUserID)
		result.OrganizationID = uuidPointer(row.OrganizationID)
		result.InheritanceMode = row.InheritanceMode
		result.ACLVersion = row.AclVersion
		result.RowVersion = row.RowVersion
	case domain.ResourceDocument:
		row, err := r.queries.GetDocumentAuthorizationResource(ctx, pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Resource{}, domain.ErrNotFound
		}
		if err != nil {
			return domain.Resource{}, mapDatabaseError(err)
		}
		result.SpaceID = uuidValue(row.SpaceID)
		result.SpaceType = row.SpaceType
		result.SpaceOwnerUserID = uuidPointer(row.OwnerUserID)
		result.OrganizationID = uuidPointer(row.OrganizationID)
		result.InheritanceMode = row.InheritanceMode
		result.ACLVersion = row.AclVersion
		result.RowVersion = row.RowVersion
	default:
		return domain.Resource{}, &domain.ValidationError{Field: "resourceType"}
	}
	if resourceType != domain.ResourceSpace {
		rows, err := r.queries.ListFolderAuthorizationAncestors(ctx, pgUUID(id))
		if err != nil {
			return domain.Resource{}, mapDatabaseError(err)
		}
		for _, row := range rows {
			result.FolderAncestors = append(result.FolderAncestors, domain.FolderAncestor{ID: uuidValue(row.FolderID), Distance: int(row.Distance), InheritanceMode: row.InheritanceMode})
		}
	}
	return result, nil
}

func (r *PostgreSQL) GetPermissionGrant(ctx context.Context, id uuid.UUID) (domain.PermissionGrant, error) {
	row, err := r.queries.GetPermissionGrantWithActions(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PermissionGrant{}, domain.ErrGrantNotFound
	}
	if err != nil {
		return domain.PermissionGrant{}, mapDatabaseError(err)
	}
	return permissionGrant(row.PermissionGrantID, row.SubjectUserID, row.SubjectOrganizationID, row.SpaceID, row.FolderID, row.DocumentID, row.InheritToDescendants, row.GrantSource, row.ValidFrom, row.ValidUntil, row.Status, row.GrantedByUserID, row.GrantReason, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokedByUserID, row.RevokeReason, row.RowVersion, row.Actions), nil
}

func (r *PostgreSQL) GetPermissionGrantForUpdate(ctx context.Context, id uuid.UUID) (domain.PermissionGrant, error) {
	row, err := r.queries.GetPermissionGrantWithActionsForUpdate(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PermissionGrant{}, domain.ErrGrantNotFound
	}
	if err != nil {
		return domain.PermissionGrant{}, mapDatabaseError(err)
	}
	return permissionGrant(row.PermissionGrantID, row.SubjectUserID, row.SubjectOrganizationID, row.SpaceID, row.FolderID, row.DocumentID, row.InheritToDescendants, row.GrantSource, row.ValidFrom, row.ValidUntil, row.Status, row.GrantedByUserID, row.GrantReason, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokedByUserID, row.RevokeReason, row.RowVersion, row.Actions), nil
}

func (r *PostgreSQL) ListDirectPermissionGrants(ctx context.Context, resourceType string, resourceID uuid.UUID, page, pageSize int) (domain.PermissionGrantListResult, error) {
	total, err := r.queries.CountDirectPermissionGrants(ctx, &dbgen.CountDirectPermissionGrantsParams{ResourceType: resourceType, ResourceID: pgUUID(resourceID)})
	if err != nil {
		return domain.PermissionGrantListResult{}, mapDatabaseError(err)
	}
	rows, err := r.queries.ListDirectPermissionGrants(ctx, &dbgen.ListDirectPermissionGrantsParams{ResourceType: resourceType, ResourceID: pgUUID(resourceID), PageOffset: pageOffset(page, pageSize), PageSize: int32(pageSize)})
	if err != nil {
		return domain.PermissionGrantListResult{}, mapDatabaseError(err)
	}
	items := make([]domain.PermissionGrant, 0, len(rows))
	for _, row := range rows {
		items = append(items, permissionGrant(row.PermissionGrantID, row.SubjectUserID, row.SubjectOrganizationID, row.SpaceID, row.FolderID, row.DocumentID, row.InheritToDescendants, row.GrantSource, row.ValidFrom, row.ValidUntil, row.Status, row.GrantedByUserID, row.GrantReason, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokedByUserID, row.RevokeReason, row.RowVersion, row.Actions))
	}
	return domain.PermissionGrantListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r *PostgreSQL) ListCandidatePermissionGrants(ctx context.Context, userID uuid.UUID, organizationIDs []uuid.UUID, resource domain.Resource, at time.Time) ([]domain.PermissionGrant, error) {
	organizations := make([]pgtype.UUID, 0, len(organizationIDs))
	for _, id := range organizationIDs {
		organizations = append(organizations, pgUUID(id))
	}
	folders := make([]pgtype.UUID, 0, len(resource.FolderAncestors)+1)
	if resource.Type == domain.ResourceFolder {
		folders = append(folders, pgUUID(resource.ID))
	}
	for _, ancestor := range resource.FolderAncestors {
		folders = append(folders, pgUUID(ancestor.ID))
	}
	var documentID pgtype.UUID
	if resource.Type == domain.ResourceDocument {
		documentID = pgUUID(resource.ID)
	}
	rows, err := r.queries.ListCandidatePermissionGrants(ctx, &dbgen.ListCandidatePermissionGrantsParams{EffectiveAt: timestamp(at), UserID: pgUUID(userID), OrganizationIds: organizations, SpaceID: pgUUID(resource.SpaceID), FolderIds: folders, DocumentID: documentID})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	items := make([]domain.PermissionGrant, 0, len(rows))
	for _, row := range rows {
		items = append(items, permissionGrant(row.PermissionGrantID, row.SubjectUserID, row.SubjectOrganizationID, row.SpaceID, row.FolderID, row.DocumentID, row.InheritToDescendants, row.GrantSource, row.ValidFrom, row.ValidUntil, row.Status, row.GrantedByUserID, row.GrantReason, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokedByUserID, row.RevokeReason, row.RowVersion, row.Actions))
	}
	return items, nil
}

func (r *PostgreSQL) ListActiveUserOrganizations(ctx context.Context, userID uuid.UUID, at time.Time) ([]uuid.UUID, error) {
	rows, err := r.queries.ListActivePermissionUserOrganizations(ctx, &dbgen.ListActivePermissionUserOrganizationsParams{UserID: pgUUID(userID), EffectiveFrom: timestamp(at)})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	items := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		items = append(items, uuidValue(row))
	}
	return items, nil
}

func (r *PostgreSQL) InsertPermissionGrant(ctx context.Context, value domain.NewPermissionGrant) (domain.PermissionGrant, error) {
	row, err := r.queries.InsertPermissionGrant(ctx, &dbgen.InsertPermissionGrantParams{PermissionGrantID: pgUUID(value.ID), SubjectUserID: optionalUUID(value.SubjectUserID), SubjectOrganizationID: optionalUUID(value.SubjectOrganizationID), SpaceID: optionalUUID(value.SpaceID), FolderID: optionalUUID(value.FolderID), DocumentID: optionalUUID(value.DocumentID), InheritToDescendants: value.InheritToDescendants, GrantSource: value.GrantSource, ValidFrom: timestamp(value.ValidFrom), ValidUntil: optionalTime(value.ValidUntil), GrantedByUserID: pgUUID(value.GrantedByUserID), GrantReason: optionalText(value.GrantReason), CreatedAt: timestamp(value.CreatedAt)})
	if err != nil {
		return domain.PermissionGrant{}, mapDatabaseError(err)
	}
	for _, action := range value.Actions {
		if err = r.queries.InsertPermissionGrantAction(ctx, &dbgen.InsertPermissionGrantActionParams{PermissionGrantID: pgUUID(value.ID), Action: action, CreatedAt: timestamp(value.CreatedAt)}); err != nil {
			return domain.PermissionGrant{}, mapDatabaseError(err)
		}
	}
	return permissionGrant(row.PermissionGrantID, row.SubjectUserID, row.SubjectOrganizationID, row.SpaceID, row.FolderID, row.DocumentID, row.InheritToDescendants, row.GrantSource, row.ValidFrom, row.ValidUntil, row.Status, row.GrantedByUserID, row.GrantReason, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokedByUserID, row.RevokeReason, row.RowVersion, value.Actions), nil
}

func (r *PostgreSQL) UpdatePermissionGrant(ctx context.Context, id uuid.UUID, actions []string, inherit bool, validUntil *time.Time, reason *string, rowVersion int64, at time.Time) (domain.PermissionGrant, error) {
	if err := r.queries.DeletePermissionGrantActions(ctx, pgUUID(id)); err != nil {
		return domain.PermissionGrant{}, mapDatabaseError(err)
	}
	row, err := r.queries.UpdatePermissionGrant(ctx, &dbgen.UpdatePermissionGrantParams{PermissionGrantID: pgUUID(id), InheritToDescendants: inherit, ValidUntil: optionalTime(validUntil), GrantReason: optionalText(reason), UpdatedAt: timestamp(at), RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PermissionGrant{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.PermissionGrant{}, mapDatabaseError(err)
	}
	for _, action := range actions {
		if err = r.queries.InsertPermissionGrantAction(ctx, &dbgen.InsertPermissionGrantActionParams{PermissionGrantID: pgUUID(id), Action: action, CreatedAt: timestamp(at)}); err != nil {
			return domain.PermissionGrant{}, mapDatabaseError(err)
		}
	}
	return permissionGrant(row.PermissionGrantID, row.SubjectUserID, row.SubjectOrganizationID, row.SpaceID, row.FolderID, row.DocumentID, row.InheritToDescendants, row.GrantSource, row.ValidFrom, row.ValidUntil, row.Status, row.GrantedByUserID, row.GrantReason, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokedByUserID, row.RevokeReason, row.RowVersion, actions), nil
}

func (r *PostgreSQL) RevokePermissionGrant(ctx context.Context, id, actorID uuid.UUID, reason string, rowVersion int64, at time.Time) (domain.PermissionGrant, error) {
	row, err := r.queries.RevokePermissionGrant(ctx, &dbgen.RevokePermissionGrantParams{PermissionGrantID: pgUUID(id), RevokedByUserID: pgUUID(actorID), RevokeReason: pgtype.Text{String: reason, Valid: true}, RevokedAt: timestamp(at), RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PermissionGrant{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.PermissionGrant{}, mapDatabaseError(err)
	}
	existing, err := r.queries.GetPermissionGrantWithActions(ctx, pgUUID(id))
	if err != nil {
		return domain.PermissionGrant{}, mapDatabaseError(err)
	}
	return permissionGrant(row.PermissionGrantID, row.SubjectUserID, row.SubjectOrganizationID, row.SpaceID, row.FolderID, row.DocumentID, row.InheritToDescendants, row.GrantSource, row.ValidFrom, row.ValidUntil, row.Status, row.GrantedByUserID, row.GrantReason, row.CreatedAt, row.UpdatedAt, row.RevokedAt, row.RevokedByUserID, row.RevokeReason, row.RowVersion, existing.Actions), nil
}

func (r *PostgreSQL) ChangeInheritance(ctx context.Context, resourceType string, id uuid.UUID, mode string, rowVersion int64, at time.Time) (domain.InheritanceResult, error) {
	result := domain.InheritanceResult{ResourceType: resourceType, ResourceID: id}
	if resourceType == domain.ResourceFolder {
		row, err := r.queries.ChangeFolderInheritance(ctx, &dbgen.ChangeFolderInheritanceParams{FolderID: pgUUID(id), InheritanceMode: mode, UpdatedAt: timestamp(at), RowVersion: rowVersion})
		if errors.Is(err, pgx.ErrNoRows) {
			return result, domain.ErrVersionConflict
		}
		if err != nil {
			return result, mapDatabaseError(err)
		}
		result.Mode = row.InheritanceMode
		result.ACLVersion = row.AclVersion
		result.RowVersion = row.RowVersion
		return result, nil
	}
	row, err := r.queries.ChangeDocumentInheritance(ctx, &dbgen.ChangeDocumentInheritanceParams{DocumentID: pgUUID(id), InheritanceMode: mode, UpdatedAt: timestamp(at), RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return result, domain.ErrVersionConflict
	}
	if err != nil {
		return result, mapDatabaseError(err)
	}
	result.Mode = row.InheritanceMode
	result.ACLVersion = row.AclVersion
	result.RowVersion = row.RowVersion
	return result, nil
}

func (r *PostgreSQL) IncrementGrantSecurityVersions(ctx context.Context, grant domain.PermissionGrant, resource domain.Resource, at time.Time) error {
	switch resource.Type {
	case domain.ResourceSpace:
		if err := r.queries.IncrementSpaceACLVersion(ctx, &dbgen.IncrementSpaceACLVersionParams{SpaceID: pgUUID(resource.ID), UpdatedAt: timestamp(at)}); err != nil {
			return mapDatabaseError(err)
		}
	case domain.ResourceFolder:
		if err := r.queries.IncrementFolderACLVersion(ctx, &dbgen.IncrementFolderACLVersionParams{FolderID: pgUUID(resource.ID), UpdatedAt: timestamp(at)}); err != nil {
			return mapDatabaseError(err)
		}
		if err := r.queries.IncrementSpaceACLVersion(ctx, &dbgen.IncrementSpaceACLVersionParams{SpaceID: pgUUID(resource.SpaceID), UpdatedAt: timestamp(at)}); err != nil {
			return mapDatabaseError(err)
		}
	case domain.ResourceDocument:
		if err := r.queries.IncrementDocumentACLVersion(ctx, &dbgen.IncrementDocumentACLVersionParams{DocumentID: pgUUID(resource.ID), UpdatedAt: timestamp(at)}); err != nil {
			return mapDatabaseError(err)
		}
		if err := r.queries.IncrementSpaceACLVersion(ctx, &dbgen.IncrementSpaceACLVersionParams{SpaceID: pgUUID(resource.SpaceID), UpdatedAt: timestamp(at)}); err != nil {
			return mapDatabaseError(err)
		}
	}
	if grant.SubjectUserID != nil {
		return mapDatabaseError(r.queries.IncrementPrincipalGrantVersion(ctx, &dbgen.IncrementPrincipalGrantVersionParams{UserID: pgUUID(*grant.SubjectUserID), UpdatedAt: timestamp(at)}))
	}
	if grant.SubjectOrganizationID != nil {
		return mapDatabaseError(r.queries.IncrementOrganizationGrantVersion(ctx, &dbgen.IncrementOrganizationGrantVersionParams{DescendantOrganizationID: pgUUID(*grant.SubjectOrganizationID), UpdatedAt: timestamp(at)}))
	}
	return nil
}

func (r *PostgreSQL) GetAuthorizationVersion(ctx context.Context, userID, spaceID uuid.UUID, at time.Time) (string, error) {
	value, err := r.queries.GetPermissionAuthorizationVersion(ctx, &dbgen.GetPermissionAuthorizationVersionParams{UserID: pgUUID(userID), EffectiveAt: timestamp(at), SpaceID: pgUUID(spaceID)})
	if err != nil {
		return "", mapDatabaseError(err)
	}
	return value, nil
}

func (r *PostgreSQL) TryCreateIdempotency(ctx context.Context, id, actorID uuid.UUID, operation, key string, hash []byte, expiresAt, at time.Time) (bool, error) {
	count, err := r.queries.TryCreatePermissionIdempotency(ctx, &dbgen.TryCreatePermissionIdempotencyParams{IdempotencyRecordID: pgUUID(id), UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, RequestHash: hash, ExpiresAt: timestamp(expiresAt), CreatedAt: timestamp(at)})
	return count == 1, mapDatabaseError(err)
}
func (r *PostgreSQL) GetIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string) (application.IdempotencyRecord, error) {
	row, err := r.queries.GetPermissionIdempotency(ctx, &dbgen.GetPermissionIdempotencyParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key})
	if err != nil {
		return application.IdempotencyRecord{}, mapDatabaseError(err)
	}
	return application.IdempotencyRecord{RequestHash: row.RequestHash, Status: row.Status, ResultResourceID: uuidPointer(row.ResultResourceID)}, nil
}
func (r *PostgreSQL) CompleteIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string, resultID uuid.UUID, resultType string, at time.Time) error {
	return mapDatabaseError(r.queries.CompletePermissionIdempotency(ctx, &dbgen.CompletePermissionIdempotencyParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, ResultResourceID: pgUUID(resultID), ResultResourceType: pgtype.Text{String: resultType, Valid: true}, CompletedAt: timestamp(at)}))
}
func (r *PostgreSQL) InsertEvent(ctx context.Context, event domain.Event) error {
	return mapDatabaseError(r.queries.InsertPermissionOutboxEvent(ctx, &dbgen.InsertPermissionOutboxEventParams{OutboxEventID: pgUUID(event.ID), AggregateType: event.AggregateType, AggregateID: pgUUID(event.AggregateID), AggregateVersion: event.AggregateVersion, EventType: event.Type, PayloadJson: event.Payload, DeduplicationKey: event.DeduplicationKey, CorrelationID: pgUUID(event.CorrelationID), AvailableAt: timestamp(event.CreatedAt)}))
}

func delegation(id, userID, organizationID pgtype.UUID, scope string, canDelegate bool, parentID, grantedBy pgtype.UUID, validFrom, validUntil pgtype.Timestamptz, status string, createdAt, updatedAt, revokedAt pgtype.Timestamptz, revokeReason pgtype.Text, rowVersion int64, capabilities []string) domain.AdminDelegation {
	return domain.AdminDelegation{ID: uuidValue(id), UserID: uuidValue(userID), OrganizationID: uuidValue(organizationID), Scope: scope, CanDelegate: canDelegate, ParentDelegationID: uuidPointer(parentID), Capabilities: capabilities, GrantedByUserID: uuidValue(grantedBy), ValidFrom: timeValue(validFrom), ValidUntil: timePointer(validUntil), Status: status, CreatedAt: timeValue(createdAt), UpdatedAt: timeValue(updatedAt), RevokedAt: timePointer(revokedAt), RevokeReason: textPointer(revokeReason), RowVersion: rowVersion}
}
func permissionGrant(id, subjectUserID, subjectOrganizationID, spaceID, folderID, documentID pgtype.UUID, inherit bool, source string, validFrom, validUntil pgtype.Timestamptz, status string, grantedBy pgtype.UUID, grantReason pgtype.Text, createdAt, updatedAt, revokedAt pgtype.Timestamptz, revokedBy pgtype.UUID, revokeReason pgtype.Text, rowVersion int64, actions []string) domain.PermissionGrant {
	return domain.PermissionGrant{ID: uuidValue(id), SubjectUserID: uuidPointer(subjectUserID), SubjectOrganizationID: uuidPointer(subjectOrganizationID), SpaceID: uuidPointer(spaceID), FolderID: uuidPointer(folderID), DocumentID: uuidPointer(documentID), Actions: actions, InheritToDescendants: inherit, GrantSource: source, ValidFrom: timeValue(validFrom), ValidUntil: timePointer(validUntil), Status: status, GrantedByUserID: uuidValue(grantedBy), GrantReason: textPointer(grantReason), CreatedAt: timeValue(createdAt), UpdatedAt: timeValue(updatedAt), RevokedAt: timePointer(revokedAt), RevokedByUserID: uuidPointer(revokedBy), RevokeReason: textPointer(revokeReason), RowVersion: rowVersion}
}
func pgUUID(value uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: value, Valid: true} }
func optionalUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgUUID(*value)
}
func uuidValue(value pgtype.UUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return uuid.UUID(value.Bytes)
}
func uuidPointer(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	result := uuid.UUID(value.Bytes)
	return &result
}
func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func optionalTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
}
func timeValue(value pgtype.Timestamptz) time.Time { return value.Time }
func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
func pageOffset(page, pageSize int) int64 { return int64(page-1) * int64(pageSize) }
func mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return domain.ErrNotFound
		case "23505":
			return domain.ErrConflict
		case "23514":
			return domain.ErrInvalidDelegation
		}
	}
	return err
}
