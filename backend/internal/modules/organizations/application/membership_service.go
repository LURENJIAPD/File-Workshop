package application

import (
	"context"
	"strings"
	"time"

	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

type AddMembershipInput struct {
	UserID         uuid.UUID
	MembershipType string
	JobTitle       *string
	EffectiveFrom  *time.Time
	EffectiveUntil *time.Time
	IdempotencyKey string
	RequestID      uuid.UUID
}

type RemoveMembershipInput struct {
	RowVersion int64
	Reason     string
	RequestID  uuid.UUID
}

func (s *Service) ListCurrentUserMemberships(ctx context.Context, actor Actor, page, pageSize int) (domain.MembershipListResult, error) {
	page, pageSize, err := normalizePage(page, pageSize)
	if err != nil {
		return domain.MembershipListResult{}, err
	}
	status := domain.MembershipStatusActive
	userID := actor.UserID
	effectiveAt := s.now().UTC()
	return s.repository.ListMemberships(ctx, domain.MembershipListFilter{UserID: &userID, Status: &status, EffectiveAt: &effectiveAt, Page: page, PageSize: pageSize})
}

func (s *Service) ListOrganizationMemberships(ctx context.Context, actor Actor, organizationID uuid.UUID, filter domain.MembershipListFilter) (domain.MembershipListResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.MembershipListResult{}, err
	}
	page, pageSize, err := normalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return domain.MembershipListResult{}, err
	}
	if filter.Status != nil {
		if err := domain.ValidateMembershipStatus(*filter.Status); err != nil {
			return domain.MembershipListResult{}, err
		}
	}
	if _, err := s.repository.GetOrganization(ctx, organizationID); err != nil {
		return domain.MembershipListResult{}, err
	}
	filter.OrganizationID = &organizationID
	filter.Page, filter.PageSize = page, pageSize
	return s.repository.ListMemberships(ctx, filter)
}

func (s *Service) AddOrganizationMembership(ctx context.Context, actor Actor, organizationID uuid.UUID, input AddMembershipInput) (domain.Membership, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Membership{}, err
	}
	if err := domain.ValidateMembershipType(input.MembershipType); err != nil {
		return domain.Membership{}, err
	}
	if err := validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.Membership{}, err
	}
	jobTitle := optionalTrimmed(input.JobTitle)
	if jobTitle != nil && len([]rune(*jobTitle)) > 128 {
		return domain.Membership{}, &domain.ValidationError{Field: "jobTitle"}
	}
	now := s.now().UTC()
	effectiveFrom := now
	if input.EffectiveFrom != nil {
		effectiveFrom = input.EffectiveFrom.UTC()
	}
	if input.EffectiveUntil != nil && !input.EffectiveUntil.After(effectiveFrom) {
		return domain.Membership{}, &domain.ValidationError{Field: "effectiveUntil"}
	}
	id, err := newUUID("organization membership")
	if err != nil {
		return domain.Membership{}, err
	}
	prepared := domain.NewMembership{ID: id, UserID: input.UserID, OrganizationID: organizationID, MembershipType: input.MembershipType, JobTitle: jobTitle, EffectiveFrom: effectiveFrom, EffectiveUntil: input.EffectiveUntil, CreatedByUserID: actor.UserID, CreatedAt: now}
	operation := "ADD_ORGANIZATION_MEMBER:" + organizationID.String()
	hash, err := requestHash(struct {
		UserID         uuid.UUID
		MembershipType string
		JobTitle       *string
		EffectiveFrom  time.Time
		EffectiveUntil *time.Time
	}{prepared.UserID, prepared.MembershipType, prepared.JobTitle, prepared.EffectiveFrom, prepared.EffectiveUntil})
	if err != nil {
		return domain.Membership{}, err
	}
	var result domain.Membership
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, operation, input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result, err = repository.GetMembership(ctx, *replayID)
			return err
		}
		allowed, err := repository.UserCanJoinOrganization(ctx, prepared.UserID, organizationID)
		if err != nil {
			return err
		}
		if !allowed {
			return domain.ErrInvalidStateTransition
		}
		result, err = repository.InsertMembership(ctx, prepared)
		if err != nil {
			return err
		}
		if err := incrementMembershipVersions(ctx, repository, result, now); err != nil {
			return err
		}
		organization, err := repository.GetOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		if err := insertOrganizationEvent(ctx, repository, organization, actor.UserID, input.RequestID, "ORGANIZATION_MEMBER_ADDED", map[string]any{"membershipId": result.ID.String(), "userId": result.UserID.String(), "membershipType": result.MembershipType}, now); err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, operation, input.IdempotencyKey, result.ID, "ORGANIZATION_MEMBERSHIP", now)
	})
	return result, err
}

func (s *Service) UpdateOrganizationMembership(ctx context.Context, actor Actor, organizationID, membershipID uuid.UUID, changes domain.MembershipChanges, requestID uuid.UUID) (domain.Membership, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Membership{}, err
	}
	if changes.RowVersion < 1 {
		return domain.Membership{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if changes.MembershipType == nil && changes.JobTitle == nil && changes.Status == nil && changes.EffectiveUntil == nil {
		return domain.Membership{}, &domain.ValidationError{Field: "body"}
	}
	now := s.now().UTC()
	var result domain.Membership
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetMembershipForUpdate(ctx, organizationID, membershipID)
		if err != nil {
			return err
		}
		if current.RowVersion != changes.RowVersion {
			return domain.ErrVersionConflict
		}
		previousStatus := current.Status
		if changes.MembershipType != nil {
			if err := domain.ValidateMembershipType(*changes.MembershipType); err != nil {
				return err
			}
			current.MembershipType = *changes.MembershipType
		}
		if changes.JobTitle != nil {
			current.JobTitle = optionalTrimmed(changes.JobTitle)
			if current.JobTitle != nil && len([]rune(*current.JobTitle)) > 128 {
				return &domain.ValidationError{Field: "jobTitle"}
			}
		}
		if changes.Status != nil {
			if err := domain.ValidateMembershipStatus(*changes.Status); err != nil {
				return err
			}
			current.Status = *changes.Status
			if current.Status == domain.MembershipStatusActive {
				current.EffectiveUntil = nil
			} else if current.EffectiveUntil == nil {
				current.EffectiveUntil = &now
			}
		}
		if changes.EffectiveUntil != nil {
			until := changes.EffectiveUntil.UTC()
			if !until.After(current.EffectiveFrom) {
				return &domain.ValidationError{Field: "effectiveUntil"}
			}
			current.EffectiveUntil = &until
		}
		if current.Status == domain.MembershipStatusActive {
			allowed, err := repository.UserCanJoinOrganization(ctx, current.UserID, organizationID)
			if err != nil {
				return err
			}
			if !allowed {
				return domain.ErrInvalidStateTransition
			}
		}
		result, err = repository.UpdateMembership(ctx, current, changes.RowVersion, now)
		if err != nil {
			return err
		}
		if err := incrementMembershipVersions(ctx, repository, result, now); err != nil {
			return err
		}
		organization, err := repository.GetOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		eventType := "ORGANIZATION_UPDATED"
		if previousStatus != result.Status {
			if result.Status == domain.MembershipStatusActive {
				eventType = "ORGANIZATION_MEMBER_ADDED"
			} else {
				eventType = "ORGANIZATION_MEMBER_REMOVED"
			}
		}
		return insertOrganizationEvent(ctx, repository, organization, actor.UserID, requestID, eventType, map[string]any{"membershipId": result.ID.String(), "userId": result.UserID.String()}, now)
	})
	return result, err
}

func (s *Service) RemoveOrganizationMembership(ctx context.Context, actor Actor, organizationID, membershipID uuid.UUID, input RemoveMembershipInput) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if input.RowVersion < 1 || strings.TrimSpace(input.Reason) == "" {
		return &domain.ValidationError{Field: "rowVersion/reason"}
	}
	now := s.now().UTC()
	return s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetMembershipForUpdate(ctx, organizationID, membershipID)
		if err != nil {
			return err
		}
		if current.RowVersion != input.RowVersion {
			return domain.ErrVersionConflict
		}
		if current.Status == domain.MembershipStatusInactive {
			return nil
		}
		result, err := repository.DeactivateMembership(ctx, organizationID, membershipID, input.RowVersion, now)
		if err != nil {
			return err
		}
		if err := incrementMembershipVersions(ctx, repository, result, now); err != nil {
			return err
		}
		organization, err := repository.GetOrganization(ctx, organizationID)
		if err != nil {
			return err
		}
		return insertOrganizationEvent(ctx, repository, organization, actor.UserID, input.RequestID, "ORGANIZATION_MEMBER_REMOVED", map[string]any{"membershipId": result.ID.String(), "userId": result.UserID.String(), "reason": strings.TrimSpace(input.Reason)}, now)
	})
}

func incrementMembershipVersions(ctx context.Context, repository Repository, membership domain.Membership, now time.Time) error {
	if err := repository.IncrementOrganizationMembershipVersion(ctx, membership.OrganizationID, now); err != nil {
		return err
	}
	return repository.IncrementUserMembershipVersion(ctx, membership.UserID, now)
}
