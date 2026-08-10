package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"file-workshop/backend/internal/modules/background/domain"

	"github.com/google/uuid"
)

type OperationsRepository interface {
	ListOutboxEvents(ctx context.Context, filter domain.OutboxListFilter) (domain.OutboxListResult, error)
	RetryOutboxEvent(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.OutboxEvent, error)
	ListBackgroundJobs(ctx context.Context, filter domain.JobListFilter) (domain.JobListResult, error)
	RetryBackgroundJob(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error)
}

type Service struct {
	repository OperationsRepository
	jobWriter  JobRepository
	now        func() time.Time
}

func NewService(repository OperationsRepository, jobWriter JobRepository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, jobWriter: jobWriter, now: now}
}

func (s *Service) ListOutboxEvents(ctx context.Context, actor domain.Actor, filter domain.OutboxListFilter) (domain.OutboxListResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.OutboxListResult{}, err
	}
	page, pageSize, err := domain.NormalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return domain.OutboxListResult{}, err
	}
	if filter.Status != nil {
		value := strings.TrimSpace(*filter.Status)
		if err := domain.ValidateOutboxStatus(value); err != nil {
			return domain.OutboxListResult{}, err
		}
		filter.Status = &value
	}
	filter.EventType = trimmedOptional(filter.EventType)
	filter.Page, filter.PageSize = page, pageSize
	return s.repository.ListOutboxEvents(ctx, filter)
}

func (s *Service) RetryOutboxEvent(ctx context.Context, actor domain.Actor, id uuid.UUID, rowVersion int64, reason string) (domain.OutboxEvent, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.OutboxEvent{}, err
	}
	reason = strings.TrimSpace(reason)
	if rowVersion < 1 || reason == "" || len(reason) > 256 {
		return domain.OutboxEvent{}, domain.ErrInvalidInput
	}
	return s.repository.RetryOutboxEvent(ctx, id, rowVersion, "manual retry: "+reason, s.now().UTC())
}

func (s *Service) ListBackgroundJobs(ctx context.Context, actor domain.Actor, filter domain.JobListFilter) (domain.JobListResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.JobListResult{}, err
	}
	page, pageSize, err := domain.NormalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return domain.JobListResult{}, err
	}
	if filter.Status != nil {
		value := strings.TrimSpace(*filter.Status)
		if err := domain.ValidateJobStatus(value); err != nil {
			return domain.JobListResult{}, err
		}
		filter.Status = &value
	}
	filter.JobType = trimmedOptional(filter.JobType)
	filter.Page, filter.PageSize = page, pageSize
	return s.repository.ListBackgroundJobs(ctx, filter)
}

func (s *Service) RetryBackgroundJob(ctx context.Context, actor domain.Actor, id uuid.UUID, rowVersion int64, reason string) (domain.BackgroundJob, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.BackgroundJob{}, err
	}
	reason = strings.TrimSpace(reason)
	if rowVersion < 1 || reason == "" || len(reason) > 256 {
		return domain.BackgroundJob{}, domain.ErrInvalidInput
	}
	return s.repository.RetryBackgroundJob(ctx, id, rowVersion, "manual retry: "+reason, s.now().UTC())
}

func (s *Service) EnqueueJob(ctx context.Context, command EnqueueJobCommand) (domain.BackgroundJob, error) {
	if s.jobWriter == nil {
		return domain.BackgroundJob{}, domain.ErrInvalidInput
	}
	jobType := strings.TrimSpace(command.JobType)
	deduplicationKey := strings.TrimSpace(command.DeduplicationKey)
	if jobType == "" || len(jobType) > 128 || deduplicationKey == "" || len(deduplicationKey) > 256 || command.PayloadSchemaVersion < 1 || command.MaxAttempts < 1 {
		return domain.BackgroundJob{}, domain.ErrInvalidInput
	}
	payload := command.PayloadJSON
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		return domain.BackgroundJob{}, domain.ErrInvalidInput
	}
	id := uuid.Must(uuid.NewV7())
	if command.AvailableAt.IsZero() {
		command.AvailableAt = s.now().UTC()
	}
	return s.jobWriter.EnqueueJob(ctx, domain.EnqueueJobInput{ID: id, JobType: jobType, TargetDocumentID: command.TargetDocumentID, TargetDocumentVersionID: command.TargetDocumentVersionID, TargetStorageObjectID: command.TargetStorageObjectID, PayloadSchemaVersion: command.PayloadSchemaVersion, PayloadJSON: payload, DeduplicationKey: deduplicationKey, Priority: command.Priority, MaxAttempts: command.MaxAttempts, AvailableAt: command.AvailableAt.UTC()}, s.now().UTC())
}

func requireAdmin(actor domain.Actor) error {
	if actor.Role != domain.SystemRoleAdmin {
		return domain.ErrForbidden
	}
	return nil
}

func trimmedOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
