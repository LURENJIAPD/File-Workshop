package domain

import "errors"

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrForbidden           = errors.New("user operation forbidden")
	ErrConflict            = errors.New("user data conflict")
	ErrVersionConflict     = errors.New("user row version conflict")
	ErrInvalidState        = errors.New("invalid user state transition")
	ErrIdempotencyConflict = errors.New("idempotency key request conflict")
	ErrLastSystemAdmin     = errors.New("last active system administrator cannot be removed")
	ErrPasswordCredential  = errors.New("active password credential not found")
	ErrInvalidUserInput    = errors.New("invalid user input")
)

type ValidationError struct {
	Field string
}

func (e *ValidationError) Error() string { return "invalid user field: " + e.Field }

func (e *ValidationError) Unwrap() error { return ErrInvalidUserInput }
