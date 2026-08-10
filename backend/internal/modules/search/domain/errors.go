package domain

import "errors"

var (
	ErrInvalidInput = errors.New("invalid search input")
	ErrForbidden    = errors.New("search operation forbidden")
	ErrNotFound     = errors.New("search target not found")
)

type ValidationError struct {
	Field string
}

func (e *ValidationError) Error() string {
	return "invalid search field: " + e.Field
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidInput
}
