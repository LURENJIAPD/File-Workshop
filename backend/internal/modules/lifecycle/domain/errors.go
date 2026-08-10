package domain

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid lifecycle input")
	ErrForbidden           = errors.New("lifecycle operation forbidden")
	ErrNotFound            = errors.New("recycle item or entry not found")
	ErrConflict            = errors.New("lifecycle state conflict")
	ErrVersionConflict     = errors.New("row version conflict")
	ErrIdempotencyConflict = errors.New("idempotency key request conflict")
	ErrRootOperation       = errors.New("root folder cannot enter recycle bin")
	ErrNameConflict        = errors.New("restore target name conflict")
	ErrLegalHoldActive     = errors.New("active legal hold prevents purge")
)

type ValidationError struct {
	Field string
}

func (e *ValidationError) Error() string {
	return "invalid field: " + e.Field
}
