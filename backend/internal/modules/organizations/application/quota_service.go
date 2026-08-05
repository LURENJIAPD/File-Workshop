package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

type ReserveQuotaInput struct {
	SpaceID       uuid.UUID
	UserID        uuid.UUID
	ReservedBytes int64
	ExpiresAt     time.Time
}

func (s *Service) ReserveQuota(ctx context.Context, input ReserveQuotaInput) (domain.QuotaReservation, error) {
	if input.ReservedBytes <= 0 {
		return domain.QuotaReservation{}, &domain.ValidationError{Field: "reservedBytes"}
	}
	now := s.now().UTC()
	if !input.ExpiresAt.After(now) {
		return domain.QuotaReservation{}, &domain.ValidationError{Field: "expiresAt"}
	}
	id, err := newUUID("quota reservation")
	if err != nil {
		return domain.QuotaReservation{}, err
	}
	var result domain.QuotaReservation
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		if _, err := repository.ReserveSpaceQuota(ctx, input.SpaceID, input.ReservedBytes, now); err != nil {
			return err
		}
		result, err = repository.InsertQuotaReservation(ctx, domain.QuotaReservation{ID: id, SpaceID: input.SpaceID, UserID: input.UserID, ReservedBytes: input.ReservedBytes, Status: domain.ReservationStatusActive, ExpiresAt: input.ExpiresAt.UTC(), CreatedAt: now, UpdatedAt: now, RowVersion: 1})
		return err
	})
	return result, err
}

func (s *Service) ConsumeQuotaReservation(ctx context.Context, id uuid.UUID, usedBytes int64) (domain.QuotaReservation, error) {
	if usedBytes < 0 {
		return domain.QuotaReservation{}, &domain.ValidationError{Field: "usedBytes"}
	}
	now := s.now().UTC()
	var result domain.QuotaReservation
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetQuotaReservationForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if current.Status == domain.ReservationStatusConsumed {
			result = current
			return nil
		}
		if current.Status != domain.ReservationStatusActive || usedBytes > current.ReservedBytes || !current.ExpiresAt.After(now) {
			return domain.ErrInvalidStateTransition
		}
		changed, err := repository.ConsumeSpaceQuota(ctx, current.SpaceID, current.ReservedBytes, usedBytes, now)
		if err != nil {
			return err
		}
		if !changed {
			return domain.ErrQuotaExceeded
		}
		result, err = repository.MarkReservationConsumed(ctx, id, now)
		return err
	})
	return result, err
}

func (s *Service) ReleaseQuotaReservation(ctx context.Context, id uuid.UUID, expired bool) (domain.QuotaReservation, error) {
	now := s.now().UTC()
	var result domain.QuotaReservation
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetQuotaReservationForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if current.Status == domain.ReservationStatusReleased || current.Status == domain.ReservationStatusExpired {
			result = current
			return nil
		}
		if current.Status != domain.ReservationStatusActive {
			return domain.ErrInvalidStateTransition
		}
		changed, err := repository.ReleaseSpaceQuota(ctx, current.SpaceID, current.ReservedBytes, now)
		if err != nil {
			return err
		}
		if !changed {
			return domain.ErrConflict
		}
		status := domain.ReservationStatusReleased
		if expired {
			status = domain.ReservationStatusExpired
		}
		result, err = repository.MarkReservationReleased(ctx, id, status, now)
		return err
	})
	return result, err
}
