package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"file-workshop/backend/internal/modules/background/domain"

	"github.com/google/uuid"
)

type OperationsRepository interface {
	CountOutboxEventsByStatus(ctx context.Context) ([]domain.OutboxStatusCount, error)
	CountOutboxFailuresByErrorCode(ctx context.Context) ([]domain.FailureSummaryItem, error)
	ListOutboxEvents(ctx context.Context, filter domain.OutboxListFilter) (domain.OutboxListResult, error)
	RetryOutboxEvent(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.OutboxEvent, error)
	CountBackgroundJobsByStatus(ctx context.Context) ([]domain.OutboxStatusCount, error)
	GetQueueLagSummary(ctx context.Context, now time.Time) (domain.QueueLagSummary, error)
	CountBackgroundJobFailuresByErrorCode(ctx context.Context) ([]domain.FailureSummaryItem, error)
	ListBackgroundJobs(ctx context.Context, filter domain.JobListFilter) (domain.JobListResult, error)
	RetryBackgroundJob(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error)
	CancelBackgroundJob(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error)
	DeadLetterBackgroundJob(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error)
	SkipBackgroundJob(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error)
	RecoverExpiredLeases(ctx context.Context, batchSize int, reason string, now time.Time) (domain.LeaseRecoveryResult, error)
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

func (s *Service) GetSummary(ctx context.Context, actor domain.Actor) (domain.AdministrationSummary, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.AdministrationSummary{}, err
	}
	outboxCounts, err := s.repository.CountOutboxEventsByStatus(ctx)
	if err != nil {
		return domain.AdministrationSummary{}, err
	}
	jobCounts, err := s.repository.CountBackgroundJobsByStatus(ctx)
	if err != nil {
		return domain.AdministrationSummary{}, err
	}
	return domain.AdministrationSummary{OutboxEvents: outboxCounts, BackgroundJobs: jobCounts}, nil
}

func (s *Service) GetFailureSummary(ctx context.Context, actor domain.Actor) (domain.FailureSummary, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.FailureSummary{}, err
	}
	outboxFailures, err := s.repository.CountOutboxFailuresByErrorCode(ctx)
	if err != nil {
		return domain.FailureSummary{}, err
	}
	jobFailures, err := s.repository.CountBackgroundJobFailuresByErrorCode(ctx)
	if err != nil {
		return domain.FailureSummary{}, err
	}
	return domain.FailureSummary{OutboxEvents: outboxFailures, BackgroundJobs: jobFailures}, nil
}

func (s *Service) GetQueueLagSummary(ctx context.Context, actor domain.Actor) (domain.QueueLagSummary, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.QueueLagSummary{}, err
	}
	return s.repository.GetQueueLagSummary(ctx, s.now().UTC())
}

func (s *Service) GetHealthSummary(ctx context.Context, actor domain.Actor) (domain.HealthSummary, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.HealthSummary{}, err
	}
	outboxCounts, err := s.repository.CountOutboxEventsByStatus(ctx)
	if err != nil {
		return domain.HealthSummary{}, err
	}
	jobCounts, err := s.repository.CountBackgroundJobsByStatus(ctx)
	if err != nil {
		return domain.HealthSummary{}, err
	}
	lag, err := s.repository.GetQueueLagSummary(ctx, s.now().UTC())
	if err != nil {
		return domain.HealthSummary{}, err
	}
	signals := make([]domain.HealthSignal, 0, 8)
	signals = appendHealthSignal(signals, countStatus(outboxCounts, domain.OutboxStatusDead), "OUTBOX_DEAD_EVENTS", domain.HealthSignalSourceOutboxEvents, domain.HealthSignalSeverityCritical, nil, "dead Outbox events require manual review")
	signals = appendHealthSignal(signals, countStatus(jobCounts, domain.JobStatusDead), "BACKGROUND_JOB_DEAD", domain.HealthSignalSourceBackgroundJobs, domain.HealthSignalSeverityCritical, nil, "dead background jobs require manual review")
	signals = appendHealthSignal(signals, lag.OutboxEvents.ExpiredProcessingCount, "OUTBOX_EXPIRED_PROCESSING", domain.HealthSignalSourceOutboxEvents, domain.HealthSignalSeverityCritical, lag.OutboxEvents.OldestDueAt, "expired Outbox processing leases can be recovered")
	signals = appendHealthSignal(signals, lag.BackgroundJobs.ExpiredProcessingCount, "BACKGROUND_JOB_EXPIRED_PROCESSING", domain.HealthSignalSourceBackgroundJobs, domain.HealthSignalSeverityCritical, lag.BackgroundJobs.OldestDueAt, "expired background job processing leases can be recovered")
	signals = appendHealthSignal(signals, lag.OutboxEvents.DueFailedCount, "OUTBOX_DUE_FAILED", domain.HealthSignalSourceOutboxEvents, domain.HealthSignalSeverityWarning, lag.OutboxEvents.OldestDueAt, "failed Outbox events are due for retry")
	signals = appendHealthSignal(signals, lag.BackgroundJobs.DueFailedCount, "BACKGROUND_JOB_DUE_FAILED", domain.HealthSignalSourceBackgroundJobs, domain.HealthSignalSeverityWarning, lag.BackgroundJobs.OldestDueAt, "failed background jobs are due for retry")
	signals = appendHealthSignal(signals, lag.OutboxEvents.DuePendingCount, "OUTBOX_DUE_PENDING", domain.HealthSignalSourceOutboxEvents, domain.HealthSignalSeverityInfo, lag.OutboxEvents.OldestDueAt, "pending Outbox events are ready to be processed")
	signals = appendHealthSignal(signals, lag.BackgroundJobs.DuePendingCount, "BACKGROUND_JOB_DUE_PENDING", domain.HealthSignalSourceBackgroundJobs, domain.HealthSignalSeverityInfo, lag.BackgroundJobs.OldestDueAt, "pending background jobs are ready to be processed")
	status := domain.HealthStatusOK
	for _, signal := range signals {
		if signal.Severity == domain.HealthSignalSeverityWarning || signal.Severity == domain.HealthSignalSeverityCritical {
			status = domain.HealthStatusAttentionRequired
			break
		}
	}
	return domain.HealthSummary{Status: status, Signals: signals}, nil
}

func (s *Service) RecoverExpiredLeases(ctx context.Context, actor domain.Actor, batchSize int, reason string) (domain.LeaseRecoveryResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.LeaseRecoveryResult{}, err
	}
	if batchSize == 0 {
		batchSize = domain.DefaultLeaseRecoveryBatchSize
	}
	reason = strings.TrimSpace(reason)
	if batchSize < 1 || batchSize > domain.MaxLeaseRecoveryBatchSize || reason == "" || len(reason) > 256 {
		return domain.LeaseRecoveryResult{}, domain.ErrInvalidInput
	}
	return s.repository.RecoverExpiredLeases(ctx, batchSize, "lease expired: "+reason, s.now().UTC())
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

func (s *Service) BatchRetryOutboxEvents(ctx context.Context, actor domain.Actor, items []domain.BatchOutboxEventItem, reason string) (domain.BatchOutboxEventOperationResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.BatchOutboxEventOperationResult{}, err
	}
	reason, err := validateOutboxBatchInput(items, reason)
	if err != nil {
		return domain.BatchOutboxEventOperationResult{}, err
	}
	now := s.now().UTC()
	result := domain.BatchOutboxEventOperationResult{Items: make([]domain.BatchOutboxEventOperationResultItem, 0, len(items))}
	for _, item := range items {
		event, err := s.repository.RetryOutboxEvent(ctx, item.ID, item.RowVersion, "manual batch retry: "+reason, now)
		if err == nil {
			result.Succeeded++
			eventCopy := event
			result.Items = append(result.Items, domain.BatchOutboxEventOperationResultItem{ID: item.ID, Success: true, Event: &eventCopy})
			continue
		}
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrConflict) {
			result.Failed++
			code, message := batchOutboxError(err)
			result.Items = append(result.Items, domain.BatchOutboxEventOperationResultItem{ID: item.ID, Success: false, ErrorCode: &code, ErrorMessage: &message})
			continue
		}
		return domain.BatchOutboxEventOperationResult{}, err
	}
	return result, nil
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
	reason, err := validateOperationReason(rowVersion, reason)
	if err != nil {
		return domain.BackgroundJob{}, domain.ErrInvalidInput
	}
	return s.repository.RetryBackgroundJob(ctx, id, rowVersion, "manual retry: "+reason, s.now().UTC())
}

func (s *Service) CancelBackgroundJob(ctx context.Context, actor domain.Actor, id uuid.UUID, rowVersion int64, reason string) (domain.BackgroundJob, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.BackgroundJob{}, err
	}
	reason, err := validateOperationReason(rowVersion, reason)
	if err != nil {
		return domain.BackgroundJob{}, domain.ErrInvalidInput
	}
	return s.repository.CancelBackgroundJob(ctx, id, rowVersion, "manual cancel: "+reason, s.now().UTC())
}

func (s *Service) BatchRetryBackgroundJobs(ctx context.Context, actor domain.Actor, items []domain.BatchJobItem, reason string) (domain.BatchJobOperationResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.BatchJobOperationResult{}, err
	}
	reason, err := validateBatchInput(items, reason)
	if err != nil {
		return domain.BatchJobOperationResult{}, err
	}
	now := s.now().UTC()
	return s.batchOperateJobs(ctx, items, func(ctx context.Context, item domain.BatchJobItem) (domain.BackgroundJob, error) {
		return s.repository.RetryBackgroundJob(ctx, item.ID, item.RowVersion, "manual batch retry: "+reason, now)
	})
}

func (s *Service) BatchCancelBackgroundJobs(ctx context.Context, actor domain.Actor, items []domain.BatchJobItem, reason string) (domain.BatchJobOperationResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.BatchJobOperationResult{}, err
	}
	reason, err := validateBatchInput(items, reason)
	if err != nil {
		return domain.BatchJobOperationResult{}, err
	}
	now := s.now().UTC()
	return s.batchOperateJobs(ctx, items, func(ctx context.Context, item domain.BatchJobItem) (domain.BackgroundJob, error) {
		return s.repository.CancelBackgroundJob(ctx, item.ID, item.RowVersion, "manual batch cancel: "+reason, now)
	})
}

func (s *Service) BatchMarkBackgroundJobsDead(ctx context.Context, actor domain.Actor, items []domain.BatchJobItem, reason string) (domain.BatchJobOperationResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.BatchJobOperationResult{}, err
	}
	reason, err := validateBatchInput(items, reason)
	if err != nil {
		return domain.BatchJobOperationResult{}, err
	}
	now := s.now().UTC()
	return s.batchOperateJobs(ctx, items, func(ctx context.Context, item domain.BatchJobItem) (domain.BackgroundJob, error) {
		return s.repository.DeadLetterBackgroundJob(ctx, item.ID, item.RowVersion, "manual batch dead letter: "+reason, now)
	})
}

func (s *Service) BatchSkipBackgroundJobs(ctx context.Context, actor domain.Actor, items []domain.BatchJobItem, reason string) (domain.BatchJobOperationResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.BatchJobOperationResult{}, err
	}
	reason, err := validateBatchInput(items, reason)
	if err != nil {
		return domain.BatchJobOperationResult{}, err
	}
	now := s.now().UTC()
	return s.batchOperateJobs(ctx, items, func(ctx context.Context, item domain.BatchJobItem) (domain.BackgroundJob, error) {
		return s.repository.SkipBackgroundJob(ctx, item.ID, item.RowVersion, "manual batch skip: "+reason, now)
	})
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

func validateOperationReason(rowVersion int64, reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if rowVersion < 1 || reason == "" || len(reason) > 256 {
		return "", domain.ErrInvalidInput
	}
	return reason, nil
}

func validateBatchInput(items []domain.BatchJobItem, reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 256 || len(items) < 1 || len(items) > domain.MaxBatchSize {
		return "", domain.ErrInvalidInput
	}
	seen := make(map[uuid.UUID]struct{}, len(items))
	for _, item := range items {
		if item.ID == uuid.Nil || item.RowVersion < 1 {
			return "", domain.ErrInvalidInput
		}
		if _, ok := seen[item.ID]; ok {
			return "", domain.ErrInvalidInput
		}
		seen[item.ID] = struct{}{}
	}
	return reason, nil
}

func validateOutboxBatchInput(items []domain.BatchOutboxEventItem, reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 256 || len(items) < 1 || len(items) > domain.MaxBatchSize {
		return "", domain.ErrInvalidInput
	}
	seen := make(map[uuid.UUID]struct{}, len(items))
	for _, item := range items {
		if item.ID == uuid.Nil || item.RowVersion < 1 {
			return "", domain.ErrInvalidInput
		}
		if _, ok := seen[item.ID]; ok {
			return "", domain.ErrInvalidInput
		}
		seen[item.ID] = struct{}{}
	}
	return reason, nil
}

func (s *Service) batchOperateJobs(ctx context.Context, items []domain.BatchJobItem, operate func(context.Context, domain.BatchJobItem) (domain.BackgroundJob, error)) (domain.BatchJobOperationResult, error) {
	result := domain.BatchJobOperationResult{Items: make([]domain.BatchJobOperationResultItem, 0, len(items))}
	for _, item := range items {
		job, err := operate(ctx, item)
		if err == nil {
			result.Succeeded++
			jobCopy := job
			result.Items = append(result.Items, domain.BatchJobOperationResultItem{ID: item.ID, Success: true, Job: &jobCopy})
			continue
		}
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrConflict) {
			result.Failed++
			code, message := batchError(err)
			result.Items = append(result.Items, domain.BatchJobOperationResultItem{ID: item.ID, Success: false, ErrorCode: &code, ErrorMessage: &message})
			continue
		}
		return domain.BatchJobOperationResult{}, err
	}
	return result, nil
}

func batchError(err error) (string, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return "BACKGROUND_ITEM_NOT_FOUND", "background job not found"
	case errors.Is(err, domain.ErrConflict):
		return "BACKGROUND_STATE_CONFLICT", "background job state or rowVersion does not allow this operation"
	default:
		return "BACKGROUND_OPERATION_FAILED", "background job operation failed"
	}
}

func batchOutboxError(err error) (string, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return "BACKGROUND_ITEM_NOT_FOUND", "outbox event not found"
	case errors.Is(err, domain.ErrConflict):
		return "BACKGROUND_STATE_CONFLICT", "outbox event state or rowVersion does not allow this operation"
	default:
		return "BACKGROUND_OPERATION_FAILED", "outbox event operation failed"
	}
}

func appendHealthSignal(signals []domain.HealthSignal, count int64, code, source, severity string, oldestAt *time.Time, message string) []domain.HealthSignal {
	if count <= 0 {
		return signals
	}
	return append(signals, domain.HealthSignal{Code: code, Source: source, Severity: severity, Count: count, OldestAt: oldestAt, Message: message})
}

func countStatus(items []domain.OutboxStatusCount, status string) int64 {
	for _, item := range items {
		if item.Status == status {
			return item.Count
		}
	}
	return 0
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
