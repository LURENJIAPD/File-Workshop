package domain

import (
	"errors"
	"fmt"
)

var (
	ErrForbidden           = errors.New("upload operation forbidden")
	ErrInvalidInput        = errors.New("invalid upload input")
	ErrNotFound            = errors.New("upload resource not found")
	ErrConflict            = errors.New("upload resource conflict")
	ErrVersionConflict     = errors.New("upload resource version conflict")
	ErrIdempotencyConflict = errors.New("idempotency key request conflict")
	ErrStorageUnavailable  = errors.New("object storage unavailable")
	ErrQuotaExceeded       = errors.New("space quota exceeded")
)

type ValidationError struct {
	Field string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid upload field %q", e.Field)
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidInput
}
