package application

import (
	"context"
	"strings"
	"time"

	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

const operationCreateOrganization = "CREATE_ORGANIZATION"

type CreateOrganizationInput struct {
	ParentOrganizationID *uuid.UUID
	Name                 string
	Code                 *string
	TypeLabel            *string
	SortOrder            int32
	SpaceQuotaBytes      int64
	IdempotencyKey       string
	RequestID            uuid.UUID
}

type MoveOrganizationInput struct {
	NewParentOrganizationID *uuid.UUID
	RowVersion              int64
	Reason                  string
	RequestID               uuid.UUID
}

type ChangeOrganizationStatusInput struct {
	Status     string
	RowVersion int64
	Reason     string
	RequestID  uuid.UUID
}

func (s *Service) GetOrganization(ctx context.Context, actor Actor, id uuid.UUID) (domain.Organization, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Organization{}, err
	}
	return s.repository.GetOrganization(ctx, id)
}

func (s *Service) ListOrganizations(ctx context.Context, actor Actor, filter domain.OrganizationListFilter) (domain.OrganizationListResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.OrganizationListResult{}, err
	}
	page, pageSize, err := normalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return domain.OrganizationListResult{}, err
	}
	filter.Page, filter.PageSize = page, pageSize
	if filter.Status != nil {
		if err := domain.ValidateOrganizationStatus(*filter.Status); err != nil {
			return domain.OrganizationListResult{}, err
		}
	}
	return s.repository.ListOrganizations(ctx, filter)
}

func (s *Service) CreateOrganization(ctx context.Context, actor Actor, input CreateOrganizationInput) (domain.Organization, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Organization{}, err
	}
	if err := domain.ValidateOrganizationName(input.Name); err != nil {
		return domain.Organization{}, err
	}
	if input.SpaceQuotaBytes < 0 {
		return domain.Organization{}, &domain.ValidationError{Field: "spaceQuotaBytes"}
	}
	if err := validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.Organization{}, err
	}
	name := strings.TrimSpace(input.Name)
	code, normalizedCode := domain.NormalizeOptional(pointerValue(input.Code))
	typeLabel := optionalTrimmed(input.TypeLabel)
	if code != nil && len([]rune(*code)) > 128 {
		return domain.Organization{}, &domain.ValidationError{Field: "code"}
	}
	if typeLabel != nil && len([]rune(*typeLabel)) > 64 {
		return domain.Organization{}, &domain.ValidationError{Field: "typeLabel"}
	}
	organizationID, err := newUUID("organization")
	if err != nil {
		return domain.Organization{}, err
	}
	spaceID, err := newUUID("organization space")
	if err != nil {
		return domain.Organization{}, err
	}
	now := s.now().UTC()
	prepared := domain.NewOrganization{ID: organizationID, SpaceID: spaceID, ParentOrganizationID: input.ParentOrganizationID, Name: name, NormalizedName: domain.Normalize(name), Code: code, NormalizedCode: normalizedCode, TypeLabel: typeLabel, SortOrder: input.SortOrder, SpaceQuotaBytes: input.SpaceQuotaBytes, CreatedByUserID: actor.UserID, CreatedAt: now}
	hash, err := requestHash(struct {
		ParentOrganizationID *uuid.UUID
		Name                 string
		Code                 *string
		TypeLabel            *string
		SortOrder            int32
		SpaceQuotaBytes      int64
	}{prepared.ParentOrganizationID, prepared.Name, prepared.Code, prepared.TypeLabel, prepared.SortOrder, prepared.SpaceQuotaBytes})
	if err != nil {
		return domain.Organization{}, err
	}

	var result domain.Organization
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, operationCreateOrganization, input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result, err = repository.GetOrganization(ctx, *replayID)
			return err
		}
		if err := repository.LockTreeMutation(ctx); err != nil {
			return err
		}
		if prepared.ParentOrganizationID != nil {
			parent, err := repository.GetOrganizationForUpdate(ctx, *prepared.ParentOrganizationID)
			if err != nil {
				return err
			}
			if parent.Status != domain.OrganizationStatusActive {
				return domain.ErrInvalidStateTransition
			}
			prepared.Depth = parent.Depth + 1
		}
		result, err = repository.InsertOrganization(ctx, prepared)
		if err != nil {
			return err
		}
		if err := repository.InsertOrganizationClosure(ctx, prepared.ID, prepared.ParentOrganizationID, now); err != nil {
			return err
		}
		if err := repository.InsertOrganizationSecurityVersions(ctx, prepared.ID, now); err != nil {
			return err
		}
		organizationID := prepared.ID
		_, err = repository.InsertSpace(ctx, domain.NewSpace{ID: prepared.SpaceID, SpaceType: domain.SpaceTypeOrganization, Name: prepared.Name, NormalizedName: prepared.NormalizedName, OrganizationID: &organizationID, QuotaBytes: prepared.SpaceQuotaBytes, ConfigSchemaVersion: 1, ConfigJSON: []byte("{}"), CreatedByUserID: actor.UserID, CreatedAt: now})
		if err != nil {
			return err
		}
		extra := map[string]any{"spaceId": prepared.SpaceID.String()}
		if prepared.ParentOrganizationID != nil {
			extra["parentOrganizationId"] = prepared.ParentOrganizationID.String()
		}
		if err := insertOrganizationEvent(ctx, repository, result, actor.UserID, input.RequestID, "ORGANIZATION_CREATED", extra, now); err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, operationCreateOrganization, input.IdempotencyKey, result.ID, "ORGANIZATION", now)
	})
	if err != nil {
		return domain.Organization{}, err
	}
	return result, nil
}

func (s *Service) UpdateOrganization(ctx context.Context, actor Actor, id uuid.UUID, changes domain.OrganizationChanges, requestID uuid.UUID) (domain.Organization, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Organization{}, err
	}
	if changes.RowVersion < 1 {
		return domain.Organization{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if changes.Name == nil && changes.Code == nil && changes.TypeLabel == nil && changes.SortOrder == nil {
		return domain.Organization{}, &domain.ValidationError{Field: "body"}
	}
	now := s.now().UTC()
	var result domain.Organization
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetOrganizationForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if current.RowVersion != changes.RowVersion {
			return domain.ErrVersionConflict
		}
		if changes.Name != nil {
			if err := domain.ValidateOrganizationName(*changes.Name); err != nil {
				return err
			}
			current.Name = strings.TrimSpace(*changes.Name)
			current.NormalizedName = domain.Normalize(current.Name)
		}
		if changes.Code != nil {
			current.Code, current.NormalizedCode = domain.NormalizeOptional(*changes.Code)
			if current.Code != nil && len([]rune(*current.Code)) > 128 {
				return &domain.ValidationError{Field: "code"}
			}
		}
		if changes.TypeLabel != nil {
			current.TypeLabel = optionalTrimmed(changes.TypeLabel)
			if current.TypeLabel != nil && len([]rune(*current.TypeLabel)) > 64 {
				return &domain.ValidationError{Field: "typeLabel"}
			}
		}
		if changes.SortOrder != nil {
			current.SortOrder = *changes.SortOrder
		}
		result, err = repository.UpdateOrganization(ctx, current, changes.RowVersion, now)
		if err != nil {
			return err
		}
		return insertOrganizationEvent(ctx, repository, result, actor.UserID, requestID, "ORGANIZATION_UPDATED", nil, now)
	})
	return result, err
}

func (s *Service) MoveOrganization(ctx context.Context, actor Actor, id uuid.UUID, input MoveOrganizationInput) (domain.Organization, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Organization{}, err
	}
	if input.RowVersion < 1 {
		return domain.Organization{}, &domain.ValidationError{Field: "rowVersion"}
	}
	now := s.now().UTC()
	var result domain.Organization
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		var err error
		result, err = s.moveOrganizationTx(ctx, repository, actor, id, input.NewParentOrganizationID, input.RowVersion, input.Reason, input.RequestID, now)
		return err
	})
	return result, err
}

func (s *Service) moveOrganizationTx(ctx context.Context, repository Repository, actor Actor, id uuid.UUID, newParentID *uuid.UUID, expectedVersion int64, reason string, requestID uuid.UUID, now time.Time) (domain.Organization, error) {
	if err := repository.LockTreeMutation(ctx); err != nil {
		return domain.Organization{}, err
	}
	current, err := repository.GetOrganizationForUpdate(ctx, id)
	if err != nil {
		return domain.Organization{}, err
	}
	if current.RowVersion != expectedVersion {
		return domain.Organization{}, domain.ErrVersionConflict
	}
	if current.Status == domain.OrganizationStatusDeleted || current.Status == domain.OrganizationStatusArchived {
		return domain.Organization{}, domain.ErrInvalidStateTransition
	}
	if sameUUID(current.ParentOrganizationID, newParentID) {
		return current, nil
	}
	newDepth := int32(0)
	if newParentID != nil {
		if *newParentID == id {
			return domain.Organization{}, domain.ErrTreeCycle
		}
		parent, err := repository.GetOrganizationForUpdate(ctx, *newParentID)
		if err != nil {
			return domain.Organization{}, err
		}
		if parent.Status != domain.OrganizationStatusActive {
			return domain.Organization{}, domain.ErrInvalidStateTransition
		}
		cycle, err := repository.OrganizationWouldCreateCycle(ctx, id, parent.ID)
		if err != nil {
			return domain.Organization{}, err
		}
		if cycle {
			return domain.Organization{}, domain.ErrTreeCycle
		}
		newDepth = parent.Depth + 1
	}
	if err := repository.IncrementSubtreeSecurityEpochs(ctx, id, now); err != nil {
		return domain.Organization{}, err
	}
	if err := repository.DeleteExternalClosureLinks(ctx, id); err != nil {
		return domain.Organization{}, err
	}
	if newParentID != nil {
		if err := repository.InsertMovedClosureLinks(ctx, id, *newParentID, now); err != nil {
			return domain.Organization{}, err
		}
	}
	if err := repository.UpdateMovedSubtree(ctx, id, newParentID, newDepth-current.Depth, expectedVersion, now); err != nil {
		return domain.Organization{}, err
	}
	if err := repository.IncrementSubtreeSecurityEpochs(ctx, id, now); err != nil {
		return domain.Organization{}, err
	}
	result, err := repository.GetOrganization(ctx, id)
	if err != nil {
		return domain.Organization{}, err
	}
	extra := map[string]any{"previousParentOrganizationId": current.ParentOrganizationID, "reason": strings.TrimSpace(reason)}
	if newParentID != nil {
		extra["newParentOrganizationId"] = newParentID.String()
	}
	if err := insertOrganizationEvent(ctx, repository, result, actor.UserID, requestID, "ORGANIZATION_MOVED", extra, now); err != nil {
		return domain.Organization{}, err
	}
	return result, nil
}

func (s *Service) ChangeOrganizationStatus(ctx context.Context, actor Actor, id uuid.UUID, input ChangeOrganizationStatusInput) (domain.Organization, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Organization{}, err
	}
	if input.RowVersion < 1 || strings.TrimSpace(input.Reason) == "" {
		return domain.Organization{}, &domain.ValidationError{Field: "rowVersion/reason"}
	}
	if err := domain.ValidateOrganizationStatus(input.Status); err != nil {
		return domain.Organization{}, err
	}
	now := s.now().UTC()
	var result domain.Organization
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetOrganizationForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if current.RowVersion != input.RowVersion {
			return domain.ErrVersionConflict
		}
		if current.Status == domain.OrganizationStatusDeleted {
			return domain.ErrInvalidStateTransition
		}
		if current.Status == input.Status {
			result = current
			return nil
		}
		if input.Status == domain.OrganizationStatusDeleted {
			blocked, err := repository.OrganizationDeletionBlocked(ctx, id, now)
			if err != nil {
				return err
			}
			if blocked {
				return domain.ErrDeletionBlocked
			}
		}
		result, err = repository.SetOrganizationStatus(ctx, id, input.Status, input.RowVersion, now)
		if err != nil {
			return err
		}
		if err := repository.IncrementSubtreeSecurityEpochs(ctx, id, now); err != nil {
			return err
		}
		eventType := "ORGANIZATION_UPDATED"
		if input.Status == domain.OrganizationStatusArchived {
			eventType = "ORGANIZATION_ARCHIVED"
		}
		return insertOrganizationEvent(ctx, repository, result, actor.UserID, input.RequestID, eventType, map[string]any{"previousStatus": current.Status, "reason": strings.TrimSpace(input.Reason)}, now)
	})
	return result, err
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
