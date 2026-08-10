package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	backgroundapplication "file-workshop/backend/internal/modules/background/application"
	backgrounddomain "file-workshop/backend/internal/modules/background/domain"
	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"
	"file-workshop/backend/internal/modules/search/domain"

	"github.com/google/uuid"
)

type Authorizer interface {
	EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error)
}

type JobEnqueuer interface {
	EnqueueJob(context.Context, backgroundapplication.EnqueueJobCommand) (backgrounddomain.BackgroundJob, error)
}

type Service struct {
	repository  Repository
	authorizer  Authorizer
	jobEnqueuer JobEnqueuer
	now         func() time.Time
}

func NewService(repository Repository, authorizer Authorizer, jobEnqueuer JobEnqueuer, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, authorizer: authorizer, jobEnqueuer: jobEnqueuer, now: now}
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

func (s *Service) EnqueueIndexRefreshJobs(ctx context.Context, actor domain.Actor, documentIDs []uuid.UUID, reason string) (domain.IndexRefreshResult, error) {
	if actor.Role != domain.SystemRoleAdmin {
		return domain.IndexRefreshResult{}, domain.ErrForbidden
	}
	documentIDs, reason, err := domain.NormalizeIndexRefreshInput(documentIDs, reason)
	if err != nil {
		return domain.IndexRefreshResult{}, err
	}
	if s.jobEnqueuer == nil {
		return domain.IndexRefreshResult{}, domain.ErrInvalidInput
	}
	result := domain.IndexRefreshResult{Items: make([]domain.IndexRefreshItemResult, 0, len(documentIDs))}
	now := s.now().UTC()
	for _, documentID := range documentIDs {
		target, err := s.repository.GetIndexRefreshTarget(ctx, documentID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				result.Failed++
				code, message := "SEARCH_INDEX_TARGET_NOT_FOUND", "document does not exist or is not active"
				result.Items = append(result.Items, domain.IndexRefreshItemResult{DocumentID: documentID, Success: false, ErrorCode: &code, ErrorMessage: &message})
				continue
			}
			return domain.IndexRefreshResult{}, err
		}
		status, err := s.repository.MarkIndexRefreshPending(ctx, documentID, now)
		if err != nil {
			return domain.IndexRefreshResult{}, err
		}
		job, err := s.enqueueIndexJob(ctx, actor, target, reason, now)
		if err != nil {
			return domain.IndexRefreshResult{}, err
		}
		result.Succeeded++
		jobID := job.ID
		result.Items = append(result.Items, domain.IndexRefreshItemResult{DocumentID: documentID, Success: true, IndexStatus: &status, BackgroundJobID: &jobID})
	}
	return result, nil
}

func (s *Service) enqueueIndexJob(ctx context.Context, actor domain.Actor, target domain.IndexRefreshTarget, reason string, now time.Time) (backgrounddomain.BackgroundJob, error) {
	payload, err := json.Marshal(map[string]any{
		"documentId":         target.DocumentID.String(),
		"currentVersionId":   optionalUUIDString(target.CurrentVersionID),
		"aclVersion":         target.ACLVersion,
		"spaceSecurityEpoch": target.SpaceSecurityEpoch,
		"requestedByUserId":  actor.UserID.String(),
		"reason":             reason,
	})
	if err != nil {
		return backgrounddomain.BackgroundJob{}, err
	}
	return s.jobEnqueuer.EnqueueJob(ctx, backgroundapplication.EnqueueJobCommand{
		JobType:                 domain.IndexJobType,
		TargetDocumentID:        &target.DocumentID,
		TargetDocumentVersionID: target.CurrentVersionID,
		PayloadSchemaVersion:    1,
		PayloadJSON:             payload,
		DeduplicationKey:        fmt.Sprintf("index:%s", target.DocumentID),
		Priority:                0,
		MaxAttempts:             5,
		AvailableAt:             now,
	})
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

func optionalUUIDString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}
