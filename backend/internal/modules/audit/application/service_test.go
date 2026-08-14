package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"file-workshop/backend/internal/modules/audit/domain"

	"github.com/google/uuid"
)

func TestServiceGetsSummaryWithTopCounts(t *testing.T) {
	dateFrom := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	dateTo := time.Date(2026, 8, 14, 23, 0, 0, 0, time.UTC)
	repository := &fakeSummaryRepository{
		summary: domain.Summary{
			TotalEvents:        5,
			EventTypeCounts:    []domain.CountByValue{{Value: "DOCUMENT_CREATED", Count: 3}},
			ResourceTypeCounts: []domain.CountByValue{{Value: "DOCUMENT", Count: 4}},
			FailureCodeCounts:  []domain.CountByValue{{Value: "AUTH_FORBIDDEN", Count: 2}},
		},
	}
	service := NewService(repository, time.Now)

	summary, err := service.GetSummary(context.Background(), adminActor(), domain.SummaryFilter{DateFrom: dateFrom, DateTo: dateTo})
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if !repository.summaryFilter.DateFrom.Equal(time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("dateFrom=%s", repository.summaryFilter.DateFrom)
	}
	if summary.EventTypeCounts[0].Value != "DOCUMENT_CREATED" || summary.FailureCodeCounts[0].Count != 2 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestServiceGetSummaryRequiresAdmin(t *testing.T) {
	service := NewService(&fakeSummaryRepository{}, time.Now)

	_, err := service.GetSummary(context.Background(), domain.Actor{UserID: uuid.Must(uuid.NewV7()), Role: "USER"}, domain.SummaryFilter{DateFrom: time.Now(), DateTo: time.Now()})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v, want ErrForbidden", err)
	}
}

func adminActor() domain.Actor {
	return domain.Actor{UserID: uuid.Must(uuid.NewV7()), SessionID: uuid.Must(uuid.NewV7()), Role: domain.SystemRoleAdmin}
}

type fakeSummaryRepository struct {
	summary       domain.Summary
	summaryFilter domain.SummaryFilter
}

func (r *fakeSummaryRepository) ListEvents(context.Context, domain.EventListFilter) (domain.EventListResult, error) {
	return domain.EventListResult{}, nil
}

func (r *fakeSummaryRepository) GetEvent(context.Context, uuid.UUID, time.Time) (domain.Event, error) {
	return domain.Event{}, nil
}

func (r *fakeSummaryRepository) GetSummary(_ context.Context, filter domain.SummaryFilter) (domain.Summary, error) {
	r.summaryFilter = filter
	r.summary.DateFrom = filter.DateFrom
	r.summary.DateTo = filter.DateTo
	return r.summary, nil
}

func (r *fakeSummaryRepository) ListChainHeads(context.Context, domain.IntegrityFilter) (domain.IntegrityResult, error) {
	return domain.IntegrityResult{}, nil
}

func (r *fakeSummaryRepository) VerifyChain(context.Context, string, time.Time, time.Time) (domain.VerificationResult, error) {
	return domain.VerificationResult{}, nil
}

func (r *fakeSummaryRepository) InsertEvent(context.Context, domain.Event) error {
	return nil
}
