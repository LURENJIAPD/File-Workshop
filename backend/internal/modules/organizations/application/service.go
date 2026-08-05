package application

import (
	"time"

	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

const idempotencyTTL = 24 * time.Hour

type Service struct {
	repository Repository
	transactor Transactor
	now        func() time.Time
}

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

func NewService(repository Repository, transactor Transactor, now func() time.Time) *Service {
	return &Service{repository: repository, transactor: transactor, now: now}
}

func requireAdmin(actor Actor) error {
	if actor.Role != domain.SystemRoleAdmin {
		return domain.ErrForbidden
	}
	return nil
}

func normalizePage(page, pageSize int) (int, int, error) {
	return domain.NormalizePage(page, pageSize)
}

func newUUID(label string) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, &uuidError{label: label, cause: err}
	}
	return id, nil
}

type uuidError struct {
	label string
	cause error
}

func (e *uuidError) Error() string { return "generate " + e.label + " ID: " + e.cause.Error() }
func (e *uuidError) Unwrap() error { return e.cause }

func optionalTrimmed(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed, _ := domain.NormalizeOptional(*value)
	return trimmed
}

func sameUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
