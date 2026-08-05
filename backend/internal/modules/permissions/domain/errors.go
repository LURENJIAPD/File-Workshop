package domain

import "errors"

var (
	ErrForbidden           = errors.New("permission denied")
	ErrNotFound            = errors.New("authorization resource not found")
	ErrDelegationNotFound  = errors.New("admin delegation not found")
	ErrGrantNotFound       = errors.New("permission grant not found")
	ErrConflict            = errors.New("authorization conflict")
	ErrVersionConflict     = errors.New("row version conflict")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrInvalidDelegation   = errors.New("invalid admin delegation")
	ErrUnsupportedAction   = errors.New("action is not supported for resource")
)

type ValidationError struct{ Field string }

func (e *ValidationError) Error() string { return "invalid field: " + e.Field }
