package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/shares/domain"

	"github.com/google/uuid"
)

type Repository interface {
	GetSourceResource(context.Context, string, uuid.UUID) (domain.SourceResource, error)
	GetShare(context.Context, uuid.UUID) (domain.Share, error)
	GetShareForUpdate(context.Context, uuid.UUID) (domain.Share, error)
	InsertShare(context.Context, domain.Share, time.Time) (domain.Share, error)
	UpdateShare(context.Context, uuid.UUID, []string, bool, *time.Time, int64, time.Time) (domain.Share, error)
	RevokeShare(context.Context, uuid.UUID, uuid.UUID, string, int64, time.Time) (domain.Share, error)
	ExpireShares(context.Context, time.Time) error
	CountCreated(context.Context, uuid.UUID) (int64, error)
	ListCreated(context.Context, uuid.UUID, int, int) ([]domain.Share, error)
	CountReceived(context.Context, uuid.UUID, []uuid.UUID, time.Time) (int64, error)
	ListReceived(context.Context, uuid.UUID, []uuid.UUID, int, int, time.Time) ([]domain.Share, error)
	ListActiveUserOrganizations(context.Context, uuid.UUID, time.Time) ([]uuid.UUID, error)
	IncrementShareVersions(context.Context, domain.Share, time.Time) error

	TryCreateIdempotency(context.Context, uuid.UUID, uuid.UUID, string, string, []byte, time.Time, time.Time) (bool, error)
	GetIdempotency(context.Context, uuid.UUID, string, string) (domain.IdempotencyRecord, error)
	CompleteIdempotency(context.Context, uuid.UUID, string, string, uuid.UUID, string, time.Time) error
	InsertEvent(context.Context, domain.Event) error
}

type Transactor interface {
	WithinTransaction(context.Context, func(Repository) error) error
}
