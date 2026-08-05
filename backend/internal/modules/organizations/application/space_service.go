package application

import (
	"context"
	"encoding/json"
	"strings"

	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

const (
	operationProvisionPersonalSpace = "PROVISION_PERSONAL_SPACE"
	operationCreatePublicSpace      = "CREATE_PUBLIC_SPACE"
)

type CreateSpaceInput struct {
	Name           string
	QuotaBytes     int64
	ConfigJSON     json.RawMessage
	IdempotencyKey string
	RequestID      uuid.UUID
}

type ChangeSpaceStatusInput struct {
	Status     string
	RowVersion int64
	Reason     string
	RequestID  uuid.UUID
}

func (s *Service) GetCurrentUserPersonalSpace(ctx context.Context, actor Actor) (domain.Space, error) {
	return s.repository.GetPersonalSpace(ctx, actor.UserID)
}

func (s *Service) GetSpace(ctx context.Context, actor Actor, id uuid.UUID) (domain.Space, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Space{}, err
	}
	return s.repository.GetSpace(ctx, id)
}

func (s *Service) ListSpaces(ctx context.Context, actor Actor, filter domain.SpaceListFilter) (domain.SpaceListResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.SpaceListResult{}, err
	}
	page, pageSize, err := normalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return domain.SpaceListResult{}, err
	}
	if filter.SpaceType != nil {
		if err := domain.ValidateSpaceType(*filter.SpaceType); err != nil {
			return domain.SpaceListResult{}, err
		}
	}
	if filter.Status != nil {
		if err := domain.ValidateSpaceStatus(*filter.Status); err != nil {
			return domain.SpaceListResult{}, err
		}
	}
	filter.Page, filter.PageSize = page, pageSize
	return s.repository.ListSpaces(ctx, filter)
}

func (s *Service) ProvisionPersonalSpace(ctx context.Context, actor Actor, userID uuid.UUID, input CreateSpaceInput) (domain.Space, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Space{}, err
	}
	return s.createStandaloneSpace(ctx, actor, domain.SpaceTypePersonal, &userID, input)
}

func (s *Service) CreatePublicSpace(ctx context.Context, actor Actor, input CreateSpaceInput) (domain.Space, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Space{}, err
	}
	return s.createStandaloneSpace(ctx, actor, domain.SpaceTypePublic, nil, input)
}

func (s *Service) createStandaloneSpace(ctx context.Context, actor Actor, spaceType string, ownerUserID *uuid.UUID, input CreateSpaceInput) (domain.Space, error) {
	if err := domain.ValidateOrganizationName(input.Name); err != nil {
		return domain.Space{}, err
	}
	if input.QuotaBytes < 0 {
		return domain.Space{}, &domain.ValidationError{Field: "quotaBytes"}
	}
	if err := validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.Space{}, err
	}
	config := input.ConfigJSON
	if len(config) == 0 {
		config = json.RawMessage("{}")
	}
	if err := domain.ValidateJSONObject(config); err != nil {
		return domain.Space{}, err
	}
	id, err := newUUID("space")
	if err != nil {
		return domain.Space{}, err
	}
	now := s.now().UTC()
	name := strings.TrimSpace(input.Name)
	prepared := domain.NewSpace{ID: id, SpaceType: spaceType, Name: name, NormalizedName: domain.Normalize(name), OwnerUserID: ownerUserID, QuotaBytes: input.QuotaBytes, ConfigSchemaVersion: 1, ConfigJSON: config, CreatedByUserID: actor.UserID, CreatedAt: now}
	operation := operationCreatePublicSpace
	resourceType := "PUBLIC_SPACE"
	if spaceType == domain.SpaceTypePersonal {
		operation = operationProvisionPersonalSpace + ":" + ownerUserID.String()
		resourceType = "PERSONAL_SPACE"
	}
	hash, err := requestHash(struct {
		SpaceType   string
		Name        string
		OwnerUserID *uuid.UUID
		QuotaBytes  int64
		ConfigJSON  json.RawMessage
	}{prepared.SpaceType, prepared.Name, prepared.OwnerUserID, prepared.QuotaBytes, prepared.ConfigJSON})
	if err != nil {
		return domain.Space{}, err
	}
	var result domain.Space
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, operation, input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result, err = repository.GetSpace(ctx, *replayID)
			return err
		}
		if ownerUserID != nil {
			active, err := repository.UserExistsAndIsActive(ctx, *ownerUserID)
			if err != nil {
				return err
			}
			if !active {
				return domain.ErrInvalidStateTransition
			}
		}
		result, err = repository.InsertSpace(ctx, prepared)
		if err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, operation, input.IdempotencyKey, result.ID, resourceType, now)
	})
	return result, err
}

func (s *Service) UpdateSpace(ctx context.Context, actor Actor, id uuid.UUID, changes domain.SpaceChanges, requestID uuid.UUID) (domain.Space, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Space{}, err
	}
	if changes.RowVersion < 1 {
		return domain.Space{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if changes.Name == nil && changes.QuotaBytes == nil && changes.ConfigSchemaVersion == nil && len(changes.ConfigJSON) == 0 {
		return domain.Space{}, &domain.ValidationError{Field: "body"}
	}
	now := s.now().UTC()
	var result domain.Space
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetSpace(ctx, id)
		if err != nil {
			return err
		}
		if current.RowVersion != changes.RowVersion {
			return domain.ErrVersionConflict
		}
		if current.Status == domain.SpaceStatusDeleted {
			return domain.ErrInvalidStateTransition
		}
		if changes.Name != nil {
			if err := domain.ValidateOrganizationName(*changes.Name); err != nil {
				return err
			}
			current.Name = strings.TrimSpace(*changes.Name)
			current.NormalizedName = domain.Normalize(current.Name)
		}
		if changes.QuotaBytes != nil {
			if *changes.QuotaBytes < current.UsedBytes+current.ReservedBytes {
				return domain.ErrQuotaExceeded
			}
			current.QuotaBytes = *changes.QuotaBytes
		}
		if changes.ConfigSchemaVersion != nil {
			if *changes.ConfigSchemaVersion < 1 {
				return &domain.ValidationError{Field: "configSchemaVersion"}
			}
			current.ConfigSchemaVersion = *changes.ConfigSchemaVersion
		}
		if len(changes.ConfigJSON) > 0 {
			if err := domain.ValidateJSONObject(changes.ConfigJSON); err != nil {
				return err
			}
			current.ConfigJSON = changes.ConfigJSON
		}
		result, err = repository.UpdateSpace(ctx, current, changes.RowVersion, now)
		if err != nil {
			return err
		}
		if result.OrganizationID != nil {
			organization, err := repository.GetOrganization(ctx, *result.OrganizationID)
			if err != nil {
				return err
			}
			return insertOrganizationEvent(ctx, repository, organization, actor.UserID, requestID, "ORGANIZATION_UPDATED", map[string]any{"spaceId": result.ID.String()}, now)
		}
		return nil
	})
	return result, err
}

func (s *Service) ChangeSpaceStatus(ctx context.Context, actor Actor, id uuid.UUID, input ChangeSpaceStatusInput) (domain.Space, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Space{}, err
	}
	if input.RowVersion < 1 || strings.TrimSpace(input.Reason) == "" {
		return domain.Space{}, &domain.ValidationError{Field: "rowVersion/reason"}
	}
	if err := domain.ValidateSpaceStatus(input.Status); err != nil {
		return domain.Space{}, err
	}
	now := s.now().UTC()
	var result domain.Space
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetSpace(ctx, id)
		if err != nil {
			return err
		}
		if current.RowVersion != input.RowVersion {
			return domain.ErrVersionConflict
		}
		if current.Status == domain.SpaceStatusDeleted {
			return domain.ErrInvalidStateTransition
		}
		if current.Status == input.Status {
			result = current
			return nil
		}
		if input.Status == domain.SpaceStatusDeleted {
			if current.SpaceType == domain.SpaceTypePersonal {
				return domain.ErrDeletionBlocked
			}
			blocked, err := repository.SpaceDeletionBlocked(ctx, id)
			if err != nil {
				return err
			}
			if blocked {
				return domain.ErrDeletionBlocked
			}
			if current.OrganizationID != nil {
				organization, err := repository.GetOrganization(ctx, *current.OrganizationID)
				if err != nil {
					return err
				}
				if organization.Status == domain.OrganizationStatusActive {
					return domain.ErrDeletionBlocked
				}
			}
		}
		result, err = repository.SetSpaceStatus(ctx, id, input.Status, input.RowVersion, now)
		if err != nil {
			return err
		}
		if result.OrganizationID != nil {
			organization, err := repository.GetOrganization(ctx, *result.OrganizationID)
			if err != nil {
				return err
			}
			return insertOrganizationEvent(ctx, repository, organization, actor.UserID, input.RequestID, "ORGANIZATION_UPDATED", map[string]any{"spaceId": result.ID.String(), "spaceStatus": result.Status, "reason": strings.TrimSpace(input.Reason)}, now)
		}
		return nil
	})
	return result, err
}
