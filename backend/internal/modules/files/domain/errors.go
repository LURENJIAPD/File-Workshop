package domain

import (
	"errors"
	"fmt"
)

var (
	ErrForbidden           = errors.New("file operation forbidden")
	ErrInvalidInput        = errors.New("invalid file input")
	ErrEntryNotFound       = errors.New("namespace entry not found")
	ErrSpaceNotFound       = errors.New("space not found")
	ErrConflict            = errors.New("file resource conflict")
	ErrVersionConflict     = errors.New("file resource version conflict")
	ErrIdempotencyConflict = errors.New("idempotency key request conflict")
	ErrTreeCycle           = errors.New("folder move would create a cycle")
	ErrRootOperation       = errors.New("root folder operation is not allowed")
)

type ValidationError struct {
	Field string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid file field %q", e.Field)
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidInput
}
