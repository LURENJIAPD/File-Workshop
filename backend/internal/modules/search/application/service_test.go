package application

import (
	"context"
	"errors"
	"testing"
	"time"

	backgroundapplication "file-workshop/backend/internal/modules/background/application"
	backgrounddomain "file-workshop/backend/internal/modules/background/domain"
	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"
	"file-workshop/backend/internal/modules/search/domain"

	"github.com/google/uuid"
)

func TestSearchFiltersInvisibleCandidatesAndMarksDegraded(t *testing.T) {
	visible := testEntry("process-card.docx", "process-card.docx")
	invisible := testEntry("salary.xlsx", "salary.xlsx")
	repository := fakeRepository{results: []domain.Result{{Entry: visible}, {Entry: invisible}}}
	authorizer := fakeAuthorizer{denied: map[uuid.UUID]bool{invisible.ID: true}}
	service := NewService(repository, authorizer, nil, time.Now)

	result, err := service.Search(context.Background(), testActor(), domain.Filter{Query: ptr("process"), Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Entry.ID != visible.ID {
		t.Fatalf("unexpected search result: %+v", result.Items)
	}
	if !result.Degraded || result.Total != 1 {
		t.Fatalf("unexpected degraded/total: degraded=%v total=%d", result.Degraded, result.Total)
	}
	if len(result.Items[0].MatchedFields) != 1 || result.Items[0].MatchedFields[0] != domain.MatchedName {
		t.Fatalf("unexpected matched fields: %+v", result.Items[0].MatchedFields)
	}
}

func TestSearchRequiresAtLeastOneCondition(t *testing.T) {
	service := NewService(fakeRepository{}, fakeAuthorizer{}, nil, time.Now)

	_, err := service.Search(context.Background(), testActor(), domain.Filter{Page: 1, PageSize: 50})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestSearchRejectsIncompleteMetadataFilter(t *testing.T) {
	service := NewService(fakeRepository{}, fakeAuthorizer{}, nil, time.Now)

	_, err := service.Search(context.Background(), testActor(), domain.Filter{MetadataKey: ptr("project"), Page: 1, PageSize: 50})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestSearchMarksFilterMatchedFields(t *testing.T) {
	entry := testEntry("safety-policy.pdf", "safety-policy.pdf")
	extension := "pdf"
	classification := "internal"
	metadataKey := "project"
	metadataValue := "a1"
	entry.ExtensionNormalized = &extension
	entry.Classification = &classification
	repository := fakeRepository{results: []domain.Result{{Entry: entry}}}
	service := NewService(repository, fakeAuthorizer{}, nil, time.Now)

	result, err := service.Search(context.Background(), testActor(), domain.Filter{Extension: ptr("PDF"), Classification: ptr("Internal"), MetadataKey: &metadataKey, MetadataValue: &metadataValue, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	got := result.Items[0].MatchedFields
	want := []string{domain.MatchedExtension, domain.MatchedClassification, domain.MatchedMetadata}
	if len(got) != len(want) {
		t.Fatalf("matched fields length = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matched field[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestEnqueueIndexRefreshJobsMarksPendingAndEnqueuesJobs(t *testing.T) {
	now := time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC)
	documentID := uuid.Must(uuid.NewV7())
	versionID := uuid.Must(uuid.NewV7())
	repository := fakeRepository{targets: map[uuid.UUID]domain.IndexRefreshTarget{
		documentID: {DocumentID: documentID, CurrentVersionID: &versionID, ACLVersion: 7, SpaceSecurityEpoch: 9},
	}}
	enqueuer := &fakeJobEnqueuer{}
	service := NewService(repository, fakeAuthorizer{}, enqueuer, func() time.Time { return now })

	result, err := service.EnqueueIndexRefreshJobs(context.Background(), adminActor(), []uuid.UUID{documentID}, "manual refresh")
	if err != nil {
		t.Fatalf("EnqueueIndexRefreshJobs returned error: %v", err)
	}
	if result.Succeeded != 1 || result.Failed != 0 || len(result.Items) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Items[0].IndexStatus == nil || *result.Items[0].IndexStatus != domain.IndexStatusPending {
		t.Fatalf("index status=%v, want PENDING", result.Items[0].IndexStatus)
	}
	if len(enqueuer.commands) != 1 {
		t.Fatalf("commands=%d, want 1", len(enqueuer.commands))
	}
	command := enqueuer.commands[0]
	if command.JobType != domain.IndexJobType || command.TargetDocumentID == nil || *command.TargetDocumentID != documentID || command.TargetDocumentVersionID == nil || *command.TargetDocumentVersionID != versionID {
		t.Fatalf("unexpected command: %#v", command)
	}
	if command.DeduplicationKey != "index:"+documentID.String() || !command.AvailableAt.Equal(now) {
		t.Fatalf("unexpected dedupe/availableAt: %s %s", command.DeduplicationKey, command.AvailableAt)
	}
}

func TestEnqueueIndexRefreshJobsReturnsItemFailureForMissingDocument(t *testing.T) {
	missingID := uuid.Must(uuid.NewV7())
	service := NewService(fakeRepository{targets: map[uuid.UUID]domain.IndexRefreshTarget{}}, fakeAuthorizer{}, &fakeJobEnqueuer{}, time.Now)

	result, err := service.EnqueueIndexRefreshJobs(context.Background(), adminActor(), []uuid.UUID{missingID}, "manual refresh")
	if err != nil {
		t.Fatalf("EnqueueIndexRefreshJobs returned error: %v", err)
	}
	if result.Succeeded != 0 || result.Failed != 1 || result.Items[0].ErrorCode == nil || *result.Items[0].ErrorCode != "SEARCH_INDEX_TARGET_NOT_FOUND" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestEnqueueIndexRefreshJobsRequiresAdmin(t *testing.T) {
	service := NewService(fakeRepository{}, fakeAuthorizer{}, &fakeJobEnqueuer{}, time.Now)

	_, err := service.EnqueueIndexRefreshJobs(context.Background(), testActor(), []uuid.UUID{uuid.Must(uuid.NewV7())}, "manual refresh")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v, want ErrForbidden", err)
	}
}

func TestEnqueueIndexRefreshJobsRejectsDuplicateDocuments(t *testing.T) {
	service := NewService(fakeRepository{}, fakeAuthorizer{}, &fakeJobEnqueuer{}, time.Now)
	documentID := uuid.Must(uuid.NewV7())

	_, err := service.EnqueueIndexRefreshJobs(context.Background(), adminActor(), []uuid.UUID{documentID, documentID}, "manual refresh")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error=%v, want ErrInvalidInput", err)
	}
}

type fakeRepository struct {
	results []domain.Result
	targets map[uuid.UUID]domain.IndexRefreshTarget
}

func (r fakeRepository) SearchEntries(context.Context, domain.Filter) ([]domain.Result, error) {
	return r.results, nil
}

func (r fakeRepository) GetIndexRefreshTarget(_ context.Context, documentID uuid.UUID) (domain.IndexRefreshTarget, error) {
	target, ok := r.targets[documentID]
	if !ok {
		return domain.IndexRefreshTarget{}, domain.ErrNotFound
	}
	return target, nil
}

func (r fakeRepository) MarkIndexRefreshPending(context.Context, uuid.UUID, time.Time) (string, error) {
	return domain.IndexStatusPending, nil
}

type fakeAuthorizer struct {
	denied map[uuid.UUID]bool
}

func (a fakeAuthorizer) EvaluatePermission(_ context.Context, _ permissiondomain.Actor, _ string, resourceID uuid.UUID, action string, _ *string, _ bool) (permissiondomain.PermissionEvaluation, error) {
	if action != domain.ActionReadMetadata {
		return permissiondomain.PermissionEvaluation{}, errors.New("unexpected action")
	}
	if a.denied[resourceID] {
		return permissiondomain.PermissionEvaluation{Allowed: false}, nil
	}
	return permissiondomain.PermissionEvaluation{Allowed: true}, nil
}

type fakeJobEnqueuer struct {
	commands []backgroundapplication.EnqueueJobCommand
}

func (e *fakeJobEnqueuer) EnqueueJob(_ context.Context, command backgroundapplication.EnqueueJobCommand) (backgrounddomain.BackgroundJob, error) {
	e.commands = append(e.commands, command)
	return backgrounddomain.BackgroundJob{ID: uuid.Must(uuid.NewV7()), JobType: command.JobType, TargetDocumentID: command.TargetDocumentID, TargetDocumentVersionID: command.TargetDocumentVersionID, Status: backgrounddomain.JobStatusPending, RowVersion: 1}, nil
}

func testActor() domain.Actor {
	return domain.Actor{UserID: uuid.Must(uuid.NewV7()), SessionID: uuid.Must(uuid.NewV7()), Role: "USER"}
}

func adminActor() domain.Actor {
	return domain.Actor{UserID: uuid.Must(uuid.NewV7()), SessionID: uuid.Must(uuid.NewV7()), Role: domain.SystemRoleAdmin}
}

func testEntry(name, normalizedName string) domain.Entry {
	now := time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)
	return domain.Entry{ID: uuid.Must(uuid.NewV7()), SpaceID: uuid.Must(uuid.NewV7()), EntryType: domain.EntryTypeDocument, Name: name, NormalizedName: normalizedName, Depth: 1, LifecycleStatus: domain.LifecycleActive, CreatedByUserID: uuid.Must(uuid.NewV7()), CreatedAt: now, UpdatedAt: now, RowVersion: 1}
}

func ptr(value string) *string {
	return &value
}
