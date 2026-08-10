package application

import (
	"context"
	"time"

	"file-workshop/backend/internal/modules/search/domain"

	"github.com/google/uuid"
)

type Repository interface {
	SearchEntries(context.Context, domain.Filter) ([]domain.Result, error)
	GetIndexRefreshTarget(context.Context, uuid.UUID) (domain.IndexRefreshTarget, error)
	MarkIndexRefreshPending(context.Context, uuid.UUID, time.Time) (string, error)
}
