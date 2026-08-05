package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/users/domain"

	"github.com/google/uuid"
)

type Repository interface {
	GetUser(context.Context, uuid.UUID) (domain.User, error)
	GetUserForUpdate(context.Context, uuid.UUID) (domain.User, error)
	ListUsers(context.Context, domain.ListFilter) (domain.ListResult, error)
	InsertUser(context.Context, domain.NewUser) (domain.User, error)
	InsertPasswordCredential(context.Context, domain.NewUser) error
	InsertSecurityVersions(context.Context, uuid.UUID, time.Time) error
	UpdateUser(context.Context, domain.User, int64, time.Time) (domain.User, error)
	SetUserStatus(context.Context, uuid.UUID, string, *time.Time, int64, time.Time) (domain.User, error)
	TouchUserForPasswordReset(context.Context, uuid.UUID, int64, time.Time) (int64, error)
	UpdatePasswordCredential(context.Context, uuid.UUID, string, time.Time) (bool, error)
	CountActiveSystemAdminsForUpdate(context.Context) (int, error)
	LockSystemAdminMutation(context.Context) error
	IncrementGlobalAuthorizationVersion(context.Context, uuid.UUID, time.Time) error
	RevokeUserSessions(context.Context, uuid.UUID, time.Time, string) error
	RevokeUserRefreshTokens(context.Context, uuid.UUID, time.Time) error
	RevokeUserCredentials(context.Context, uuid.UUID, time.Time, string) error
	ListUserSessions(context.Context, uuid.UUID, int, int) (domain.SessionListResult, error)
	OwnedSessionExists(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	RevokeOwnedSessionTokens(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	RevokeOwnedSession(context.Context, uuid.UUID, uuid.UUID, time.Time) (bool, error)
	TryCreateIdempotency(context.Context, uuid.UUID, uuid.UUID, string, []byte, time.Time, time.Time) (bool, error)
	GetCreateIdempotencyForUpdate(context.Context, uuid.UUID, string) (domain.IdempotencyRecord, error)
	CompleteCreateIdempotency(context.Context, uuid.UUID, string, uuid.UUID, time.Time) error
	InsertEvent(context.Context, domain.Event) error
}

type Transactor interface {
	WithinTransaction(context.Context, func(Repository) error) error
}
