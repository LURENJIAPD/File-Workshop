package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrIdentityNotFound   = errors.New("identity not found")
	ErrAuthentication     = errors.New("authentication required")
	ErrTokenReused        = errors.New("refresh token reused")
	ErrAccountUnavailable = errors.New("account unavailable")
)

type AccountLockedError struct {
	RetryAfter time.Duration
}

func (e *AccountLockedError) Error() string {
	return "account is temporarily locked"
}

type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return "login rate limit exceeded"
}
