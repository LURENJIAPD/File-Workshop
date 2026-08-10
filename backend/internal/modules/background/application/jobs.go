package application

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"file-workshop/backend/internal/modules/background/domain"

	"github.com/google/uuid"
)

type JobRepository interface {
	EnqueueJob(ctx context.Context, input domain.EnqueueJobInput, now time.Time) (domain.BackgroundJob, error)
	ClaimBackgroundJobsByType(ctx context.Context, jobType, workerID string, batchSize int32, leaseUntil time.Time, now time.Time) ([]domain.BackgroundJob, error)
	MarkBackgroundJobSuccess(ctx context.Context, job domain.BackgroundJob, workerID string, now time.Time) (bool, error)
	MarkBackgroundJobFailed(ctx context.Context, job domain.BackgroundJob, workerID string, code, summary string, nextRetryAt time.Time, now time.Time) (bool, error)
	MarkBackgroundJobDead(ctx context.Context, job domain.BackgroundJob, workerID string, code, summary string, now time.Time) (bool, error)
	RenewBackgroundJobLease(ctx context.Context, job domain.BackgroundJob, workerID string, leaseUntil time.Time, now time.Time) (bool, error)
	CountBackgroundJobsByStatus(ctx context.Context) ([]domain.OutboxStatusCount, error)
}

type JobHandler interface {
	JobTypes() []string
	HandleBackgroundJob(context.Context, domain.BackgroundJob) error
}

type JobHandlerFunc struct {
	Types []string
	Fn    func(context.Context, domain.BackgroundJob) error
}

func (h JobHandlerFunc) JobTypes() []string { return append([]string(nil), h.Types...) }

func (h JobHandlerFunc) HandleBackgroundJob(ctx context.Context, job domain.BackgroundJob) error {
	if h.Fn == nil {
		return domain.PermanentError("HANDLER_NOT_CONFIGURED", "background job handler is not configured")
	}
	return h.Fn(ctx, job)
}

type JobRunner struct {
	repository JobRepository
	handlers   map[string]JobHandler
	jobTypes   []string
	config     RunnerConfig
	logger     *slog.Logger
	now        func() time.Time
}

func NewJobRunner(repository JobRepository, handlers []JobHandler, config RunnerConfig, logger *slog.Logger, now func() time.Time) (*JobRunner, error) {
	normalized := normalizeConfig(config)
	if repository == nil {
		return nil, errors.New("background job repository is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	handlerMap := map[string]JobHandler{}
	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		for _, jobType := range handler.JobTypes() {
			jobType = strings.TrimSpace(jobType)
			if jobType != "" {
				handlerMap[jobType] = handler
			}
		}
	}
	jobTypes := make([]string, 0, len(handlerMap))
	for jobType := range handlerMap {
		jobTypes = append(jobTypes, jobType)
	}
	sort.Strings(jobTypes)
	return &JobRunner{repository: repository, handlers: handlerMap, jobTypes: jobTypes, config: normalized, logger: logger, now: now}, nil
}

func (r *JobRunner) Run(ctx context.Context) error {
	if len(r.jobTypes) == 0 {
		r.logger.WarnContext(ctx, "background job worker started without registered handlers", "workerId", r.config.WorkerID)
		<-ctx.Done()
		return nil
	}
	var waitGroup sync.WaitGroup
	for index := 0; index < r.config.Concurrency; index++ {
		waitGroup.Add(1)
		go func(workerIndex int) {
			defer waitGroup.Done()
			r.loop(ctx, workerIndex)
		}(index)
	}
	<-ctx.Done()
	waitGroup.Wait()
	return nil
}

func (r *JobRunner) RunOnce(ctx context.Context) (int, error) {
	if len(r.jobTypes) == 0 {
		return 0, domain.ErrNoHandlers
	}
	total := 0
	for _, jobType := range r.jobTypes {
		claimed, err := r.claimAndProcess(ctx, jobType)
		if err != nil {
			return total, err
		}
		total += claimed
	}
	return total, nil
}

func (r *JobRunner) loop(ctx context.Context, workerIndex int) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		processed := 0
		for _, jobType := range r.jobTypes {
			claimed, err := r.claimAndProcess(ctx, jobType)
			if err != nil {
				r.logger.ErrorContext(ctx, "background job claim or processing failed", "workerId", r.config.WorkerID, "workerIndex", workerIndex, "jobType", jobType, "error", err)
				continue
			}
			processed += claimed
		}
		delay := r.config.PollInterval
		if processed > 0 {
			delay = 0
		}
		timer.Reset(delay)
	}
}

func (r *JobRunner) claimAndProcess(ctx context.Context, jobType string) (int, error) {
	now := r.now().UTC()
	jobs, err := r.repository.ClaimBackgroundJobsByType(ctx, jobType, r.config.WorkerID, r.config.BatchSize, now.Add(r.config.LeaseDuration), now)
	if err != nil {
		return 0, err
	}
	handler := r.handlers[jobType]
	for _, job := range jobs {
		r.process(ctx, handler, job)
	}
	return len(jobs), nil
}

func (r *JobRunner) process(ctx context.Context, handler JobHandler, job domain.BackgroundJob) {
	if handler == nil {
		_ = r.markDead(ctx, job, "HANDLER_NOT_REGISTERED", "no handler registered for job type")
		return
	}
	processContext, cancel := context.WithTimeout(ctx, r.config.HandlerTimeout)
	defer cancel()
	err := handler.HandleBackgroundJob(processContext, job)
	now := r.now().UTC()
	if err == nil {
		ok, markErr := r.repository.MarkBackgroundJobSuccess(ctx, job, r.config.WorkerID, now)
		if markErr != nil {
			r.logger.ErrorContext(ctx, "mark background job success failed", "jobId", job.ID, "jobType", job.JobType, "error", markErr)
			return
		}
		if !ok {
			r.logger.WarnContext(ctx, "background job was not marked success because lease changed", "jobId", job.ID, "jobType", job.JobType)
		}
		return
	}
	code, summary, retryable := domain.ClassifyError(err)
	if !retryable || job.AttemptCount >= job.MaxAttempts {
		_ = r.markDead(ctx, job, code, summary)
		return
	}
	nextRetryAt := now.Add(r.retryDelay(job.AttemptCount))
	ok, markErr := r.repository.MarkBackgroundJobFailed(ctx, job, r.config.WorkerID, code, summary, nextRetryAt, now)
	if markErr != nil {
		r.logger.ErrorContext(ctx, "mark background job failed failed", "jobId", job.ID, "jobType", job.JobType, "error", markErr)
		return
	}
	if !ok {
		r.logger.WarnContext(ctx, "background job was not marked failed because lease changed or attempts were exhausted", "jobId", job.ID, "jobType", job.JobType)
	}
}

func (r *JobRunner) markDead(ctx context.Context, job domain.BackgroundJob, code, summary string) bool {
	ok, err := r.repository.MarkBackgroundJobDead(ctx, job, r.config.WorkerID, code, summary, r.now().UTC())
	if err != nil {
		r.logger.ErrorContext(ctx, "mark background job dead failed", "jobId", job.ID, "jobType", job.JobType, "error", err)
		return false
	}
	if !ok {
		r.logger.WarnContext(ctx, "background job was not marked dead because lease changed", "jobId", job.ID, "jobType", job.JobType)
	}
	return ok
}

func (r *JobRunner) retryDelay(attemptCount int32) time.Duration {
	exponent := int(attemptCount - 1)
	if exponent < 0 {
		exponent = 0
	}
	delay := time.Duration(float64(r.config.RetryInitialBackoff) * math.Pow(2, float64(exponent)))
	if delay > r.config.RetryMaxBackoff {
		return r.config.RetryMaxBackoff
	}
	return delay
}

type EnqueueJobCommand struct {
	JobType                 string
	TargetDocumentID        *uuid.UUID
	TargetDocumentVersionID *uuid.UUID
	TargetStorageObjectID   *uuid.UUID
	PayloadSchemaVersion    int32
	PayloadJSON             json.RawMessage
	DeduplicationKey        string
	Priority                int32
	MaxAttempts             int32
	AvailableAt             time.Time
}
