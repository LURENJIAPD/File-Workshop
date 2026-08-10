package application

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"file-workshop/backend/internal/modules/background/domain"
)

type OutboxRepository interface {
	ClaimOutboxEventsByType(ctx context.Context, eventType, workerID string, batchSize int32, leaseUntil time.Time, now time.Time) ([]domain.OutboxEvent, error)
	MarkOutboxEventPublished(ctx context.Context, event domain.OutboxEvent, workerID string, now time.Time) (bool, error)
	MarkOutboxEventFailed(ctx context.Context, event domain.OutboxEvent, workerID string, code, summary string, nextRetryAt time.Time, now time.Time) (bool, error)
	MarkOutboxEventDead(ctx context.Context, event domain.OutboxEvent, workerID string, code, summary string, now time.Time) (bool, error)
	RenewOutboxEventLease(ctx context.Context, event domain.OutboxEvent, workerID string, leaseUntil time.Time, now time.Time) (bool, error)
	CountOutboxEventsByStatus(ctx context.Context) ([]domain.OutboxStatusCount, error)
}

type OutboxHandler interface {
	EventTypes() []string
	HandleOutboxEvent(context.Context, domain.OutboxEvent) error
}

type OutboxHandlerFunc struct {
	Types []string
	Fn    func(context.Context, domain.OutboxEvent) error
}

func (h OutboxHandlerFunc) EventTypes() []string { return append([]string(nil), h.Types...) }

func (h OutboxHandlerFunc) HandleOutboxEvent(ctx context.Context, event domain.OutboxEvent) error {
	if h.Fn == nil {
		return domain.PermanentError("HANDLER_NOT_CONFIGURED", "outbox handler is not configured")
	}
	return h.Fn(ctx, event)
}

type RunnerConfig struct {
	WorkerID            string
	Concurrency         int
	BatchSize           int32
	PollInterval        time.Duration
	LeaseDuration       time.Duration
	HandlerTimeout      time.Duration
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
}

type Runner struct {
	repository OutboxRepository
	handlers   map[string]OutboxHandler
	eventTypes []string
	config     RunnerConfig
	logger     *slog.Logger
	now        func() time.Time
}

func NewRunner(repository OutboxRepository, handlers []OutboxHandler, config RunnerConfig, logger *slog.Logger, now func() time.Time) (*Runner, error) {
	normalized := normalizeConfig(config)
	if repository == nil {
		return nil, errors.New("outbox repository is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	handlerMap := map[string]OutboxHandler{}
	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		for _, eventType := range handler.EventTypes() {
			eventType = strings.TrimSpace(eventType)
			if eventType == "" {
				continue
			}
			handlerMap[eventType] = handler
		}
	}
	eventTypes := make([]string, 0, len(handlerMap))
	for eventType := range handlerMap {
		eventTypes = append(eventTypes, eventType)
	}
	sort.Strings(eventTypes)
	return &Runner{repository: repository, handlers: handlerMap, eventTypes: eventTypes, config: normalized, logger: logger, now: now}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	if len(r.eventTypes) == 0 {
		r.logger.WarnContext(ctx, "outbox worker started without registered handlers", "workerId", r.config.WorkerID)
		<-ctx.Done()
		return ctx.Err()
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

func (r *Runner) RunOnce(ctx context.Context) (int, error) {
	if len(r.eventTypes) == 0 {
		return 0, domain.ErrNoHandlers
	}
	total := 0
	for _, eventType := range r.eventTypes {
		claimed, err := r.claimAndProcess(ctx, eventType)
		if err != nil {
			return total, err
		}
		total += claimed
	}
	return total, nil
}

func (r *Runner) loop(ctx context.Context, workerIndex int) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		processed := 0
		for _, eventType := range r.eventTypes {
			claimed, err := r.claimAndProcess(ctx, eventType)
			if err != nil {
				r.logger.ErrorContext(ctx, "outbox claim or processing failed", "workerId", r.config.WorkerID, "workerIndex", workerIndex, "eventType", eventType, "error", err)
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

func (r *Runner) claimAndProcess(ctx context.Context, eventType string) (int, error) {
	now := r.now().UTC()
	events, err := r.repository.ClaimOutboxEventsByType(ctx, eventType, r.config.WorkerID, r.config.BatchSize, now.Add(r.config.LeaseDuration), now)
	if err != nil {
		return 0, err
	}
	handler := r.handlers[eventType]
	for _, event := range events {
		r.process(ctx, handler, event)
	}
	return len(events), nil
}

func (r *Runner) process(ctx context.Context, handler OutboxHandler, event domain.OutboxEvent) {
	if handler == nil {
		_ = r.markDead(ctx, event, "HANDLER_NOT_REGISTERED", "no handler registered for event type")
		return
	}
	processContext, cancel := context.WithTimeout(ctx, r.config.HandlerTimeout)
	defer cancel()
	err := handler.HandleOutboxEvent(processContext, event)
	now := r.now().UTC()
	if err == nil {
		ok, markErr := r.repository.MarkOutboxEventPublished(ctx, event, r.config.WorkerID, now)
		if markErr != nil {
			r.logger.ErrorContext(ctx, "mark outbox event published failed", "eventId", event.ID, "eventType", event.EventType, "error", markErr)
			return
		}
		if !ok {
			r.logger.WarnContext(ctx, "outbox event was not marked published because lease changed", "eventId", event.ID, "eventType", event.EventType)
		}
		return
	}
	code, summary, retryable := domain.ClassifyError(err)
	if !retryable || event.AttemptCount >= event.MaxAttempts {
		_ = r.markDead(ctx, event, code, summary)
		return
	}
	nextRetryAt := now.Add(r.retryDelay(event.AttemptCount))
	ok, markErr := r.repository.MarkOutboxEventFailed(ctx, event, r.config.WorkerID, code, summary, nextRetryAt, now)
	if markErr != nil {
		r.logger.ErrorContext(ctx, "mark outbox event failed failed", "eventId", event.ID, "eventType", event.EventType, "error", markErr)
		return
	}
	if !ok {
		r.logger.WarnContext(ctx, "outbox event was not marked failed because lease changed or attempts were exhausted", "eventId", event.ID, "eventType", event.EventType)
	}
}

func (r *Runner) markDead(ctx context.Context, event domain.OutboxEvent, code, summary string) bool {
	ok, err := r.repository.MarkOutboxEventDead(ctx, event, r.config.WorkerID, code, summary, r.now().UTC())
	if err != nil {
		r.logger.ErrorContext(ctx, "mark outbox event dead failed", "eventId", event.ID, "eventType", event.EventType, "error", err)
		return false
	}
	if !ok {
		r.logger.WarnContext(ctx, "outbox event was not marked dead because lease changed", "eventId", event.ID, "eventType", event.EventType)
	}
	return ok
}

func (r *Runner) retryDelay(attemptCount int32) time.Duration {
	exponent := int(attemptCount - 1)
	if exponent < 0 {
		exponent = 0
	}
	multiplier := math.Pow(2, float64(exponent))
	delay := time.Duration(float64(r.config.RetryInitialBackoff) * multiplier)
	if delay > r.config.RetryMaxBackoff {
		return r.config.RetryMaxBackoff
	}
	return delay
}

func normalizeConfig(config RunnerConfig) RunnerConfig {
	if strings.TrimSpace(config.WorkerID) == "" {
		config.WorkerID = "file-workshop-worker"
	}
	if config.Concurrency < 1 {
		config.Concurrency = 1
	}
	if config.BatchSize < 1 {
		config.BatchSize = 10
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = 30 * time.Second
	}
	if config.HandlerTimeout <= 0 {
		config.HandlerTimeout = 20 * time.Second
	}
	if config.RetryInitialBackoff <= 0 {
		config.RetryInitialBackoff = time.Second
	}
	if config.RetryMaxBackoff <= 0 {
		config.RetryMaxBackoff = time.Minute
	}
	if config.RetryMaxBackoff < config.RetryInitialBackoff {
		config.RetryMaxBackoff = config.RetryInitialBackoff
	}
	return config
}
