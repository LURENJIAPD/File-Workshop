package domain

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid share input")
	ErrForbidden           = errors.New("share operation forbidden")
	ErrNotFound            = errors.New("share not found")
	ErrConflict            = errors.New("share state conflict")
	ErrVersionConflict     = errors.New("share row version conflict")
	ErrIdempotencyConflict = errors.New("idempotency key request conflict")
	ErrTargetUnsupported   = errors.New("share target unsupported in current cycle")
	ErrShareTokenRequired  = errors.New("share token required")
	ErrShareTokenInvalid   = errors.New("share token invalid")
)

type ValidationError struct {
	Field string
}

func (e *ValidationError) Error() string {
	return "invalid field: " + e.Field
}
