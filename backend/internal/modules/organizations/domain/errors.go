package domain

import (
	"errors"
	"fmt"
)

var (
	ErrForbidden              = errors.New("organization operation forbidden")
	ErrInvalidInput           = errors.New("invalid organization input")
	ErrOrganizationNotFound   = errors.New("organization not found")
	ErrMembershipNotFound     = errors.New("organization membership not found")
	ErrSpaceNotFound          = errors.New("space not found")
	ErrPlanNotFound           = errors.New("organization change plan not found")
	ErrReservationNotFound    = errors.New("quota reservation not found")
	ErrConflict               = errors.New("organization resource conflict")
	ErrVersionConflict        = errors.New("organization resource version conflict")
	ErrIdempotencyConflict    = errors.New("idempotency key request conflict")
	ErrTreeCycle              = errors.New("organization tree cycle")
	ErrDeletionBlocked        = errors.New("organization or space deletion blocked")
	ErrQuotaExceeded          = errors.New("space quota exceeded")
	ErrUnsupportedOperation   = errors.New("organization change operation is not executable yet")
	ErrInvalidStateTransition = errors.New("invalid organization state transition")
)

type ValidationError struct {
	Field string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid organization field %q", e.Field)
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidInput
}
