package application

import (
	"context"

	"file-workshop/backend/internal/modules/search/domain"
)

type Repository interface {
	SearchEntries(context.Context, domain.Filter) ([]domain.Result, error)
}
