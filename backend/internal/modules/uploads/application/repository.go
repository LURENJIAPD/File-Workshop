package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/uploads/domain"

	"github.com/google/uuid"
)

type Repository interface {
	GetFolderContext(context.Context, uuid.UUID, uuid.UUID) (domain.FolderContext, error)
	GetDocumentContext(context.Context, uuid.UUID, uuid.UUID) (domain.DocumentContext, error)
	GetSession(context.Context, uuid.UUID) (domain.Session, error)
	GetSessionForUpdate(context.Context, uuid.UUID) (domain.Session, error)
	TryCreateIdempotency(context.Context, uuid.UUID, uuid.UUID, string, string, []byte, time.Time, time.Time) (bool, error)
	GetIdempotency(context.Context, uuid.UUID, string, string) (domain.IdempotencyRecord, error)
	CompleteIdempotency(context.Context, uuid.UUID, string, string, uuid.UUID, string, time.Time) error
	ReserveQuota(context.Context, uuid.UUID, int64, time.Time) error
	InsertQuotaReservation(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, time.Time, time.Time) error
	ReleaseQuotaReservation(context.Context, uuid.UUID, uuid.UUID, int64, time.Time) error
	InsertSession(context.Context, domain.NewSession) (domain.Session, error)
	MarkUploading(context.Context, uuid.UUID, time.Time) (domain.Session, error)
	AbortSession(context.Context, uuid.UUID, int64, string, time.Time) (domain.Session, error)
	InsertEvent(context.Context, domain.Event) error
}

type Transactor interface {
	WithinTransaction(context.Context, func(Repository) error) error
}
