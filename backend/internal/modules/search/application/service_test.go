package application

import (
	"context"
	"errors"
	"testing"
	"time"

	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"
	"file-workshop/backend/internal/modules/search/domain"

	"github.com/google/uuid"
)

func TestSearchFiltersInvisibleCandidatesAndMarksDegraded(t *testing.T) {
	visible := testEntry("工艺卡.docx", "工艺卡.docx")
	invisible := testEntry("工资表.xlsx", "工资表.xlsx")
	repository := fakeRepository{results: []domain.Result{{Entry: visible}, {Entry: invisible}}}
	authorizer := fakeAuthorizer{denied: map[uuid.UUID]bool{invisible.ID: true}}
	service := NewService(repository, authorizer)

	result, err := service.Search(context.Background(), testActor(), domain.Filter{Query: ptr("工艺"), Page: 1, PageSize: 50})
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
	service := NewService(fakeRepository{}, fakeAuthorizer{})

	_, err := service.Search(context.Background(), testActor(), domain.Filter{Page: 1, PageSize: 50})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestSearchRejectsIncompleteMetadataFilter(t *testing.T) {
	service := NewService(fakeRepository{}, fakeAuthorizer{})

	_, err := service.Search(context.Background(), testActor(), domain.Filter{MetadataKey: ptr("project"), Page: 1, PageSize: 50})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestSearchMarksFilterMatchedFields(t *testing.T) {
	entry := testEntry("安全规范.pdf", "安全规范.pdf")
	extension := "pdf"
	classification := "internal"
	metadataKey := "project"
	metadataValue := "a1"
	entry.ExtensionNormalized = &extension
	entry.Classification = &classification
	repository := fakeRepository{results: []domain.Result{{Entry: entry}}}
	service := NewService(repository, fakeAuthorizer{})

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

type fakeRepository struct {
	results []domain.Result
	filter  domain.Filter
}

func (r fakeRepository) SearchEntries(_ context.Context, filter domain.Filter) ([]domain.Result, error) {
	r.filter = filter
	return r.results, nil
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

func testActor() domain.Actor {
	return domain.Actor{UserID: uuid.Must(uuid.NewV7()), SessionID: uuid.Must(uuid.NewV7()), Role: "USER"}
}

func testEntry(name, normalizedName string) domain.Entry {
	now := time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)
	return domain.Entry{ID: uuid.Must(uuid.NewV7()), SpaceID: uuid.Must(uuid.NewV7()), EntryType: domain.EntryTypeDocument, Name: name, NormalizedName: normalizedName, Depth: 1, LifecycleStatus: domain.LifecycleActive, CreatedByUserID: uuid.Must(uuid.NewV7()), CreatedAt: now, UpdatedAt: now, RowVersion: 1}
}

func ptr(value string) *string {
	return &value
}
