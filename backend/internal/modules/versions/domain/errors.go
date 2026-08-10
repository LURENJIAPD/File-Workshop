package domain

import (
	"errors"
	"fmt"
)

var (
	ErrForbidden           = errors.New("version operation forbidden")
	ErrInvalidInput        = errors.New("invalid version input")
	ErrNotFound            = errors.New("version resource not found")
	ErrConflict            = errors.New("version resource conflict")
	ErrVersionConflict     = errors.New("version resource version conflict")
	ErrIdempotencyConflict = errors.New("idempotency key request conflict")
	ErrLocked              = errors.New("document is locked")
)

type ValidationError struct {
	Field string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid version field %q", e.Field)
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidInput
}
