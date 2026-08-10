package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/files/domain"

	"github.com/google/uuid"
)

type Repository interface {
	GetSpaceDirectoryInfo(context.Context, uuid.UUID) (domain.SpaceDirectoryInfo, error)
	GetSpaceDirectoryInfoForUpdate(context.Context, uuid.UUID) (domain.SpaceDirectoryInfo, error)
	GetEntry(context.Context, uuid.UUID) (domain.NamespaceEntry, error)
	GetEntryForUpdate(context.Context, uuid.UUID) (domain.NamespaceEntry, error)
	ListEntries(context.Context, domain.EntryListFilter) (domain.EntryListResult, error)
	InsertNamespaceEntry(context.Context, domain.NewNamespaceEntry) (domain.NamespaceEntry, error)
	InsertFolder(context.Context, uuid.UUID, time.Time) error
	InsertDocument(context.Context, domain.NewDocument) (domain.NamespaceEntry, error)
	UpdateSpaceRootFolder(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	RenameEntry(context.Context, uuid.UUID, string, string, *string, int64, time.Time) (domain.NamespaceEntry, error)
	MoveEntry(context.Context, uuid.UUID, uuid.UUID, string, int32, int64, time.Time) (domain.NamespaceEntry, error)
	UpdateDescendantPaths(context.Context, uuid.UUID, string, int32, time.Time) error
	FolderIsDescendantOf(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	TouchSpaceSecurityEpoch(context.Context, uuid.UUID, time.Time) error
	TryCreateIdempotency(context.Context, uuid.UUID, uuid.UUID, string, string, []byte, time.Time, time.Time) (bool, error)
	GetIdempotency(context.Context, uuid.UUID, string, string) (domain.IdempotencyRecord, error)
	CompleteIdempotency(context.Context, uuid.UUID, string, string, uuid.UUID, string, time.Time) error
	InsertEvent(context.Context, domain.Event) error
}

type Transactor interface {
	WithinTransaction(context.Context, func(Repository) error) error
}
