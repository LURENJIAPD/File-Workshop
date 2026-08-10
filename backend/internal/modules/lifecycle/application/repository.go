package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/lifecycle/domain"

	"github.com/google/uuid"
)

type Repository interface {
	GetEntryForUpdate(context.Context, uuid.UUID) (domain.Entry, error)
	GetFolderForUpdate(context.Context, uuid.UUID) (domain.Entry, error)
	NameExists(context.Context, uuid.UUID, *uuid.UUID, string, uuid.UUID) (bool, error)
	TrashEntrySubtree(context.Context, uuid.UUID, time.Time) (int64, error)
	MoveRestoreRoot(context.Context, uuid.UUID, *uuid.UUID, string, string, *string, int32, time.Time) (domain.Entry, error)
	RestoreEntrySubtree(context.Context, uuid.UUID, time.Time) (int64, error)
	UpdateDescendantPaths(context.Context, uuid.UUID, string, int32, time.Time) error
	MarkEntrySubtreePurging(context.Context, uuid.UUID, time.Time) (int64, error)
	MarkSharesSourceUnavailable(context.Context, uuid.UUID, time.Time) error
	TouchSpaceSecurityEpoch(context.Context, uuid.UUID, time.Time) error
	ActiveLegalHoldExists(context.Context, uuid.UUID) (bool, error)

	InsertRecycleItem(context.Context, uuid.UUID, domain.Entry, uuid.UUID, time.Time, time.Time) (domain.RecycleItem, error)
	GetRecycleItem(context.Context, uuid.UUID) (domain.RecycleItem, error)
	GetRecycleItemForUpdate(context.Context, uuid.UUID) (domain.RecycleItem, error)
	CountRecycleItems(context.Context, *uuid.UUID) (int64, error)
	ListRecycleItems(context.Context, *uuid.UUID, int, int) ([]domain.RecycleItem, error)
	ListExpiredActiveRecycleItems(context.Context, time.Time, int) ([]domain.RecycleItem, error)
	RestoreRecycleItem(context.Context, uuid.UUID, uuid.UUID, int64, time.Time) (domain.RecycleItem, error)
	MarkRecycleItemPurging(context.Context, uuid.UUID, int64, time.Time) (domain.RecycleItem, error)

	TryCreateIdempotency(context.Context, uuid.UUID, uuid.UUID, string, string, []byte, time.Time, time.Time) (bool, error)
	GetIdempotency(context.Context, uuid.UUID, string, string) (domain.IdempotencyRecord, error)
	CompleteIdempotency(context.Context, uuid.UUID, string, string, uuid.UUID, string, time.Time) error
	InsertEvent(context.Context, domain.Event) error
}

type Transactor interface {
	WithinTransaction(context.Context, func(Repository) error) error
}
