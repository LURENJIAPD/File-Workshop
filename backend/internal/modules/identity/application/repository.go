package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/identity/domain"

	"github.com/google/uuid"
)

type Repository interface {
	FindPasswordIdentity(context.Context, string, time.Time) (domain.PasswordIdentity, error)
	GetRecentFailureState(context.Context, string, time.Time) (domain.FailureState, error)
	InsertLoginAttempt(context.Context, domain.LoginAttempt) error
	CreateSession(context.Context, domain.NewSession) (domain.Session, error)
	CreateRefreshToken(context.Context, domain.NewRefreshToken) error
	TouchLoginIdentity(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	GetRefreshTokenForUpdate(context.Context, []byte) (domain.RefreshSession, error)
	MarkRefreshTokenUsed(context.Context, uuid.UUID, time.Time) (bool, error)
	MarkRefreshTokenReused(context.Context, uuid.UUID, time.Time) error
	RevokeSession(context.Context, uuid.UUID, time.Time, string) error
	RevokeActiveRefreshTokens(context.Context, uuid.UUID, time.Time) error
	GetCurrentSession(context.Context, uuid.UUID) (domain.Session, domain.User, error)
	FindSessionIDByRefreshHash(context.Context, []byte) (uuid.UUID, error)
	TouchSession(context.Context, uuid.UUID, time.Time) error
}

type Transactor interface {
	WithinTransaction(context.Context, func(Repository) error) error
}

type AccessTokens interface {
	Issue(uuid.UUID, uuid.UUID, time.Time) (string, time.Time, error)
	Parse(string, time.Time) (domain.AccessPrincipal, error)
}

type LoginLimiter interface {
	Allow(string, time.Time) (bool, time.Duration)
}
