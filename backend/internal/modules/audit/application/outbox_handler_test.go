package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"file-workshop/backend/internal/modules/audit/domain"
	backgrounddomain "file-workshop/backend/internal/modules/background/domain"

	"github.com/google/uuid"
)

func TestHandleOutboxEventMapsHighRiskUserEvent(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{}
	service := NewService(repository, func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC) })
	actorID := uuid.New()
	requestID := uuid.New()
	aggregateID := uuid.New()
	payload, _ := json.Marshal(map[string]any{"actorUserId": actorID.String(), "reason": "角色调整"})
	event := backgrounddomain.OutboxEvent{ID: uuid.New(), AggregateType: "USER", AggregateID: aggregateID, AggregateVersion: 3, EventType: "USER_ROLE_CHANGED", EventSchemaVersion: 1, PayloadJSON: payload, CorrelationID: &requestID, DeduplicationKey: "dedup", CreatedAt: time.Date(2026, 8, 10, 9, 30, 0, 0, time.UTC)}

	if err := service.HandleOutboxEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleOutboxEvent() error = %v", err)
	}
	if len(repository.events) != 1 {
		t.Fatalf("inserted events = %d, want 1", len(repository.events))
	}
	inserted := repository.events[0]
	if inserted.EventType != "USER_ROLE_CHANGED" || inserted.RiskLevel != domain.RiskHigh {
		t.Fatalf("unexpected event type/risk: %s/%s", inserted.EventType, inserted.RiskLevel)
	}
	if inserted.ActorType != domain.ActorTypeUser || inserted.ActorID == nil || *inserted.ActorID != actorID {
		t.Fatalf("actor not mapped from payload: %#v", inserted)
	}
	if inserted.RequestID != requestID || inserted.CorrelationID == nil || *inserted.CorrelationID != requestID {
		t.Fatalf("request/correlation not mapped: request=%s correlation=%v", inserted.RequestID, inserted.CorrelationID)
	}
	if inserted.ResourceType == nil || *inserted.ResourceType != "USER" || inserted.ResourceID == nil || *inserted.ResourceID != aggregateID {
		t.Fatalf("resource not mapped: %#v", inserted)
	}
	if inserted.Reason == nil || *inserted.Reason != "角色调整" {
		t.Fatalf("reason not mapped: %#v", inserted.Reason)
	}
}

func TestHandleOutboxEventFallsBackToOutboxIDWhenCorrelationMissing(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{}
	service := NewService(repository, nil)
	outboxID := uuid.New()
	event := backgrounddomain.OutboxEvent{ID: outboxID, AggregateType: "FOLDER", AggregateID: uuid.New(), EventType: "FOLDER_CREATED", EventSchemaVersion: 1, PayloadJSON: json.RawMessage(`{}`), CreatedAt: time.Date(2026, 8, 10, 9, 30, 0, 0, time.UTC)}

	if err := service.HandleOutboxEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleOutboxEvent() error = %v", err)
	}
	inserted := repository.events[0]
	if inserted.RequestID != outboxID {
		t.Fatalf("RequestID = %s, want outbox id %s", inserted.RequestID, outboxID)
	}
	metadata := map[string]any{}
	if err := json.Unmarshal(inserted.MetadataJSON, &metadata); err != nil {
		t.Fatalf("metadata unmarshal: %v", err)
	}
	if metadata["requestIdFallback"] != true {
		t.Fatalf("requestIdFallback metadata = %#v, want true", metadata["requestIdFallback"])
	}
}

func TestGetSummaryRequiresAdmin(t *testing.T) {
	t.Parallel()
	service := NewService(&fakeRepository{}, nil)
	_, err := service.GetSummary(context.Background(), domain.Actor{UserID: uuid.New(), Role: "USER"}, domain.SummaryFilter{DateFrom: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), DateTo: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)})
	if err != domain.ErrForbidden {
		t.Fatalf("GetSummary() error = %v, want forbidden", err)
	}
}

func TestGetSummaryNormalizesDateRange(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{summary: domain.Summary{TotalEvents: 3, RiskLevelCounts: []domain.CountByValue{{Value: domain.RiskHigh, Count: 1}}}}
	service := NewService(repository, nil)
	from := time.Date(2026, 8, 10, 12, 30, 0, 0, time.FixedZone("test", 8*60*60))
	to := time.Date(2026, 8, 11, 23, 59, 0, 0, time.FixedZone("test", 8*60*60))

	result, err := service.GetSummary(context.Background(), domain.Actor{UserID: uuid.New(), Role: domain.SystemRoleAdmin}, domain.SummaryFilter{DateFrom: from, DateTo: to})
	if err != nil {
		t.Fatalf("GetSummary() error = %v", err)
	}
	if result.TotalEvents != 3 || len(result.RiskLevelCounts) != 1 {
		t.Fatalf("unexpected summary: %#v", result)
	}
	if repository.summaryFilter.DateFrom.Hour() != 0 || repository.summaryFilter.DateTo.Hour() != 0 {
		t.Fatalf("date range not normalized: %#v", repository.summaryFilter)
	}
}

func TestGetSummaryRejectsInvertedRange(t *testing.T) {
	t.Parallel()
	service := NewService(&fakeRepository{}, nil)
	_, err := service.GetSummary(context.Background(), domain.Actor{UserID: uuid.New(), Role: domain.SystemRoleAdmin}, domain.SummaryFilter{DateFrom: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC), DateTo: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)})
	if err == nil {
		t.Fatal("GetSummary() error = nil, want validation error")
	}
}

type fakeRepository struct {
	events        []domain.Event
	summary       domain.Summary
	summaryFilter domain.SummaryFilter
}

func (f *fakeRepository) ListEvents(context.Context, domain.EventListFilter) (domain.EventListResult, error) {
	return domain.EventListResult{}, nil
}

func (f *fakeRepository) GetEvent(context.Context, uuid.UUID, time.Time) (domain.Event, error) {
	return domain.Event{}, nil
}

func (f *fakeRepository) GetSummary(_ context.Context, filter domain.SummaryFilter) (domain.Summary, error) {
	f.summaryFilter = filter
	f.summary.DateFrom = filter.DateFrom
	f.summary.DateTo = filter.DateTo
	return f.summary, nil
}

func (f *fakeRepository) ListChainHeads(context.Context, domain.IntegrityFilter) (domain.IntegrityResult, error) {
	return domain.IntegrityResult{}, nil
}

func (f *fakeRepository) VerifyChain(context.Context, string, time.Time, time.Time) (domain.VerificationResult, error) {
	return domain.VerificationResult{}, nil
}

func (f *fakeRepository) InsertEvent(_ context.Context, event domain.Event) error {
	f.events = append(f.events, event)
	return nil
}
