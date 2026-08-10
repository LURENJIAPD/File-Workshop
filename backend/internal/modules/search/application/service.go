package application

import (
	"context"
	"errors"
	"strings"

	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"
	"file-workshop/backend/internal/modules/search/domain"

	"github.com/google/uuid"
)

type Authorizer interface {
	EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error)
}

type Service struct {
	repository Repository
	authorizer Authorizer
}

func NewService(repository Repository, authorizer Authorizer) *Service {
	return &Service{repository: repository, authorizer: authorizer}
}

func (s *Service) Search(ctx context.Context, actor domain.Actor, filter domain.Filter) (domain.ListResult, error) {
	normalized, err := domain.NormalizeFilter(filter)
	if err != nil {
		return domain.ListResult{}, err
	}
	candidates, err := s.repository.SearchEntries(ctx, normalized)
	if err != nil {
		return domain.ListResult{}, err
	}
	visible := make([]domain.Result, 0, len(candidates))
	for _, candidate := range candidates {
		if err = s.requirePermission(ctx, actor, candidate.Entry.EntryType, candidate.Entry.ID); err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				continue
			}
			return domain.ListResult{}, err
		}
		candidate.MatchedFields = matchedFields(normalized, candidate.Entry)
		candidate.Source = domain.SourcePostgresMetadata
		visible = append(visible, candidate)
	}
	if len(visible) > normalized.PageSize {
		visible = visible[:normalized.PageSize]
	}
	return domain.ListResult{Items: visible, Page: normalized.Page, PageSize: normalized.PageSize, Total: int64(len(visible)), Degraded: true}, nil
}

func (s *Service) requirePermission(ctx context.Context, actor domain.Actor, resourceType string, resourceID uuid.UUID) error {
	result, err := s.authorizer.EvaluatePermission(ctx, permissiondomain.Actor{UserID: actor.UserID, SessionID: actor.SessionID, Role: actor.Role}, resourceType, resourceID, domain.ActionReadMetadata, nil, false)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return domain.ErrForbidden
	}
	return nil
}

func matchedFields(filter domain.Filter, entry domain.Entry) []string {
	values := map[string]struct{}{}
	query := stringValue(filter.Query)
	if query != "" {
		if strings.Contains(entry.NormalizedName, query) || strings.Contains(strings.ToLower(entry.Name), query) {
			values[domain.MatchedName] = struct{}{}
		}
		if entry.ExtensionNormalized != nil && strings.Contains(strings.ToLower(*entry.ExtensionNormalized), query) {
			values[domain.MatchedExtension] = struct{}{}
		}
		if entry.Classification != nil && strings.Contains(strings.ToLower(*entry.Classification), query) {
			values[domain.MatchedClassification] = struct{}{}
		}
	}
	if filter.Extension != nil {
		values[domain.MatchedExtension] = struct{}{}
	}
	if filter.Classification != nil {
		values[domain.MatchedClassification] = struct{}{}
	}
	if filter.MetadataKey != nil {
		values[domain.MatchedMetadata] = struct{}{}
	}
	if len(values) == 0 {
		values[domain.MatchedName] = struct{}{}
	}
	ordered := []string{domain.MatchedName, domain.MatchedExtension, domain.MatchedClassification, domain.MatchedMetadata}
	result := make([]string, 0, len(values))
	for _, value := range ordered {
		if _, ok := values[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
