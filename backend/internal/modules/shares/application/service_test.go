package application

import (
	"bytes"
	"context"
	"testing"
	"time"

	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"
	"file-workshop/backend/internal/modules/shares/domain"

	"github.com/google/uuid"
)

func TestCreateLinkShareReturnsTokenAndStoresHash(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository, repository, allowAuthorizer{}, fixedNow)
	result, err := service.CreateShare(context.Background(), fakeActor(), domain.CreateInput{SourceType: domain.ResourceDocument, SourceID: fakeDocumentID, TargetKind: domain.TargetLink, Actions: []string{domain.ActionReadMetadata, domain.ActionDownload}, IdempotencyKey: "share-key-1", RequestID: fakeRequestID})
	if err != nil {
		t.Fatalf("create share failed: %v", err)
	}
	if result.ShareToken == nil || *result.ShareToken == "" {
		t.Fatalf("link share should return a one-time share token")
	}
	if len(result.Share.TokenHash) != 32 || bytes.Contains(result.Share.TokenHash, []byte(*result.ShareToken)) {
		t.Fatalf("repository should receive sha256 token hash, not raw token")
	}
	if !repository.versionIncremented {
		t.Fatalf("share version should be incremented")
	}
}

func TestCreateShareRejectsUnsupportedSpaceTarget(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository, repository, allowAuthorizer{}, fixedNow)
	_, err := service.CreateShare(context.Background(), fakeActor(), domain.CreateInput{SourceType: domain.ResourceDocument, SourceID: fakeDocumentID, TargetKind: domain.TargetSpace, Actions: []string{domain.ActionReadMetadata}, IdempotencyKey: "share-key-2", RequestID: fakeRequestID})
	if err != domain.ErrTargetUnsupported {
		t.Fatalf("expected ErrTargetUnsupported, got %v", err)
	}
}

func TestCreateShareDoesNotExpandCreatorPermissions(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository, repository, denyDownloadAuthorizer{}, fixedNow)
	_, err := service.CreateShare(context.Background(), fakeActor(), domain.CreateInput{SourceType: domain.ResourceDocument, SourceID: fakeDocumentID, TargetKind: domain.TargetUser, TargetUserID: &fakeRecipientID, Actions: []string{domain.ActionReadMetadata, domain.ActionDownload}, IdempotencyKey: "share-key-3", RequestID: fakeRequestID})
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestOpenLinkShareRequiresCorrectToken(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository, repository, allowAuthorizer{}, fixedNow)
	result, err := service.CreateShare(context.Background(), fakeActor(), domain.CreateInput{SourceType: domain.ResourceDocument, SourceID: fakeDocumentID, TargetKind: domain.TargetLink, Actions: []string{domain.ActionReadMetadata}, IdempotencyKey: "share-key-4", RequestID: fakeRequestID})
	if err != nil {
		t.Fatalf("create share failed: %v", err)
	}
	if _, err = service.OpenShare(context.Background(), domain.Actor{UserID: fakeRecipientID, SessionID: fakeSessionID}, result.Share.ID, domain.OpenInput{RequestID: fakeRequestID}); err != domain.ErrShareTokenRequired {
		t.Fatalf("expected token required, got %v", err)
	}
	if _, err = service.OpenShare(context.Background(), domain.Actor{UserID: fakeRecipientID, SessionID: fakeSessionID}, result.Share.ID, domain.OpenInput{ShareToken: result.ShareToken, RequestID: fakeRequestID}); err != nil {
		t.Fatalf("open share with token failed: %v", err)
	}
}

var (
	fakeDocumentID  = uuid.MustParse("0198b100-0000-7000-8000-000000000001")
	fakeRequestID   = uuid.MustParse("0198b100-0000-7000-8000-000000000002")
	fakeActorID     = uuid.MustParse("0198b100-0000-7000-8000-000000000003")
	fakeSessionID   = uuid.MustParse("0198b100-0000-7000-8000-000000000004")
	fakeRecipientID = uuid.MustParse("0198b100-0000-7000-8000-000000000005")
)

func fixedNow() time.Time {
	return time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
}

func fakeActor() domain.Actor {
	return domain.Actor{UserID: fakeActorID, SessionID: fakeSessionID}
}

type allowAuthorizer struct{}

func (allowAuthorizer) EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error) {
	return permissiondomain.PermissionEvaluation{Allowed: true}, nil
}

type denyDownloadAuthorizer struct{}

func (denyDownloadAuthorizer) EvaluatePermission(_ context.Context, _ permissiondomain.Actor, _ string, _ uuid.UUID, action string, _ *string, _ bool) (permissiondomain.PermissionEvaluation, error) {
	return permissiondomain.PermissionEvaluation{Allowed: action != domain.ActionDownload}, nil
}

type fakeRepository struct {
	shares             map[uuid.UUID]domain.Share
	idempotency        map[string]domain.IdempotencyRecord
	versionIncremented bool
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{shares: map[uuid.UUID]domain.Share{}, idempotency: map[string]domain.IdempotencyRecord{}}
}

func (r *fakeRepository) WithinTransaction(ctx context.Context, operation func(Repository) error) error {
	return operation(r)
}

func (r *fakeRepository) GetSourceResource(context.Context, string, uuid.UUID) (domain.SourceResource, error) {
	return domain.SourceResource{ID: fakeDocumentID, Type: domain.ResourceDocument}, nil
}

func (r *fakeRepository) GetShare(_ context.Context, shareID uuid.UUID) (domain.Share, error) {
	share, ok := r.shares[shareID]
	if !ok {
		return domain.Share{}, domain.ErrNotFound
	}
	return share, nil
}

func (r *fakeRepository) GetShareForUpdate(ctx context.Context, shareID uuid.UUID) (domain.Share, error) {
	return r.GetShare(ctx, shareID)
}

func (r *fakeRepository) InsertShare(_ context.Context, share domain.Share, _ time.Time) (domain.Share, error) {
	share.RowVersion = 1
	r.shares[share.ID] = share
	return share, nil
}

func (r *fakeRepository) UpdateShare(context.Context, uuid.UUID, []string, bool, *time.Time, int64, time.Time) (domain.Share, error) {
	return domain.Share{}, nil
}

func (r *fakeRepository) RevokeShare(context.Context, uuid.UUID, uuid.UUID, string, int64, time.Time) (domain.Share, error) {
	return domain.Share{}, nil
}

func (r *fakeRepository) ExpireShares(context.Context, time.Time) error { return nil }

func (r *fakeRepository) CountCreated(context.Context, uuid.UUID) (int64, error) { return 0, nil }

func (r *fakeRepository) ListCreated(context.Context, uuid.UUID, int, int) ([]domain.Share, error) {
	return nil, nil
}

func (r *fakeRepository) CountReceived(context.Context, uuid.UUID, []uuid.UUID, time.Time) (int64, error) {
	return 0, nil
}

func (r *fakeRepository) ListReceived(context.Context, uuid.UUID, []uuid.UUID, int, int, time.Time) ([]domain.Share, error) {
	return nil, nil
}

func (r *fakeRepository) ListActiveUserOrganizations(context.Context, uuid.UUID, time.Time) ([]uuid.UUID, error) {
	return nil, nil
}

func (r *fakeRepository) IncrementShareVersions(context.Context, domain.Share, time.Time) error {
	r.versionIncremented = true
	return nil
}

func (r *fakeRepository) TryCreateIdempotency(_ context.Context, recordID, actorID uuid.UUID, operation, key string, hash []byte, expiresAt, now time.Time) (bool, error) {
	if _, ok := r.idempotency[key]; ok {
		return false, nil
	}
	r.idempotency[key] = domain.IdempotencyRecord{RequestHash: append([]byte(nil), hash...), Status: "IN_PROGRESS"}
	return true, nil
}

func (r *fakeRepository) GetIdempotency(_ context.Context, actorID uuid.UUID, operation, key string) (domain.IdempotencyRecord, error) {
	record, ok := r.idempotency[key]
	if !ok {
		return domain.IdempotencyRecord{}, domain.ErrConflict
	}
	return record, nil
}

func (r *fakeRepository) CompleteIdempotency(_ context.Context, actorID uuid.UUID, operation, key string, resourceID uuid.UUID, resourceType string, now time.Time) error {
	record := r.idempotency[key]
	record.Status = "COMPLETED"
	record.ResultResourceID = &resourceID
	r.idempotency[key] = record
	return nil
}

func (r *fakeRepository) InsertEvent(context.Context, domain.Event) error { return nil }
