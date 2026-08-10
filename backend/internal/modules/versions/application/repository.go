package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/versions/domain"

	"github.com/google/uuid"
)

type Repository interface {
	GetDocumentContext(context.Context, uuid.UUID) (domain.DocumentContext, error)
	CountVersions(context.Context, uuid.UUID) (int64, error)
	ListVersions(context.Context, uuid.UUID, int, int) ([]domain.Version, error)
	GetVersion(context.Context, uuid.UUID, uuid.UUID) (domain.Version, error)
	GetDocumentForUpdate(context.Context, uuid.UUID) (domain.DocumentContext, error)
	InsertRestoredVersion(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, *string, time.Time) (domain.Version, error)
	SetCurrentVersion(context.Context, uuid.UUID, uuid.UUID, int64, time.Time) error
	ExpireLocks(context.Context, uuid.UUID, time.Time) error
	EnsureLockCounter(context.Context, uuid.UUID, time.Time) error
	IncrementLockCounter(context.Context, uuid.UUID, time.Time) (int64, error)
	InsertLock(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, []byte, int64, string, time.Time, time.Time) (domain.Lock, error)
	GetActiveLock(context.Context, uuid.UUID) (*domain.Lock, error)
	GetActiveLockForUpdate(context.Context, uuid.UUID) (*domain.Lock, error)
	HeartbeatLock(context.Context, uuid.UUID, []byte, int64, uuid.UUID, time.Time, time.Time) (domain.Lock, error)
	ReleaseLock(context.Context, uuid.UUID, []byte, int64, uuid.UUID, time.Time, *string) (domain.Lock, error)
	ForceReleaseLock(context.Context, uuid.UUID, int64, uuid.UUID, time.Time, string) (domain.Lock, error)
	TryCreateIdempotency(context.Context, uuid.UUID, uuid.UUID, string, string, []byte, time.Time, time.Time) (bool, error)
	GetIdempotency(context.Context, uuid.UUID, string, string) (domain.IdempotencyRecord, error)
	CompleteIdempotency(context.Context, uuid.UUID, string, string, uuid.UUID, string, time.Time) error
	InsertEvent(context.Context, domain.Event) error
}

type Transactor interface {
	WithinTransaction(context.Context, func(Repository) error) error
}
