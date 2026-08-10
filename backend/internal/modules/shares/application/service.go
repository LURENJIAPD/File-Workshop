package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"
	"file-workshop/backend/internal/modules/shares/domain"

	"github.com/google/uuid"
)

const (
	createShareOperation = "CREATE_SHARE"
	idempotencyTTL       = 24 * time.Hour
)

type Authorizer interface {
	EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error)
}

type Service struct {
	repository Repository
	transactor Transactor
	authorizer Authorizer
	now        func() time.Time
}

func NewService(repository Repository, transactor Transactor, authorizer Authorizer, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, transactor: transactor, authorizer: authorizer, now: now}
}

func (s *Service) CreateShare(ctx context.Context, actor domain.Actor, input domain.CreateInput) (domain.CreateResult, error) {
	now := s.now().UTC()
	if err := domain.ValidateSourceType(input.SourceType); err != nil {
		return domain.CreateResult{}, err
	}
	if err := domain.ValidateTarget(input); err != nil {
		return domain.CreateResult{}, err
	}
	actions, err := domain.NormalizeActions(input.Actions)
	if err != nil {
		return domain.CreateResult{}, err
	}
	if err = domain.ValidatePeriod(input.ValidUntil, now); err != nil {
		return domain.CreateResult{}, err
	}
	if err = validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.CreateResult{}, err
	}
	source, err := s.repository.GetSourceResource(ctx, input.SourceType, input.SourceID)
	if err != nil {
		return domain.CreateResult{}, err
	}
	if err = s.requirePermission(ctx, actor, source, domain.ActionShare); err != nil {
		return domain.CreateResult{}, err
	}
	for _, action := range actions {
		if err = s.requirePermission(ctx, actor, source, action); err != nil {
			return domain.CreateResult{}, err
		}
	}
	requestHash, err := requestHash(struct {
		SourceType string
		SourceID   uuid.UUID
		TargetKind string
		TargetUser *uuid.UUID
		TargetOrg  *uuid.UUID
		Actions    []string
		ValidUntil *time.Time
		Reshare    bool
	}{input.SourceType, input.SourceID, input.TargetKind, input.TargetUserID, input.TargetOrganizationID, actions, input.ValidUntil, input.AllowReshare})
	if err != nil {
		return domain.CreateResult{}, err
	}
	var result domain.CreateResult
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, createShareOperation, input.IdempotencyKey, requestHash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result.Share, err = repository.GetShare(ctx, *replayID)
			return err
		}
		shareID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		var tokenHash []byte
		if input.TargetKind == domain.TargetLink {
			token, hash, err := newShareToken()
			if err != nil {
				return err
			}
			tokenHash = hash
			result.ShareToken = &token
		}
		share := domain.Share{ID: shareID, CreatorUserID: actor.UserID, TargetKind: input.TargetKind, TargetUserID: input.TargetUserID, TargetOrganizationID: input.TargetOrganizationID, AllowReshare: input.AllowReshare, Actions: actions, ValidFrom: now, ValidUntil: input.ValidUntil, Status: domain.StatusActive, CreatedAt: now, UpdatedAt: now, TokenHash: tokenHash}
		if input.SourceType == domain.ResourceDocument {
			share.SourceDocumentID = &input.SourceID
		} else {
			share.SourceFolderID = &input.SourceID
		}
		result.Share, err = repository.InsertShare(ctx, share, now)
		if err != nil {
			return err
		}
		if err = repository.IncrementShareVersions(ctx, result.Share, now); err != nil {
			return err
		}
		if err = insertEvent(ctx, repository, "SHARE_CREATED", result.Share, input.RequestID, now); err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, createShareOperation, input.IdempotencyKey, result.Share.ID, domain.ResourceShare, now)
	})
	return result, err
}

func (s *Service) ListCreated(ctx context.Context, actor domain.Actor, page, pageSize int) (domain.ListResult, error) {
	page, pageSize, err := domain.NormalizePage(page, pageSize)
	if err != nil {
		return domain.ListResult{}, err
	}
	now := s.now().UTC()
	_ = s.repository.ExpireShares(ctx, now)
	total, err := s.repository.CountCreated(ctx, actor.UserID)
	if err != nil {
		return domain.ListResult{}, err
	}
	items, err := s.repository.ListCreated(ctx, actor.UserID, page, pageSize)
	if err != nil {
		return domain.ListResult{}, err
	}
	return domain.ListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) ListReceived(ctx context.Context, actor domain.Actor, page, pageSize int) (domain.ListResult, error) {
	page, pageSize, err := domain.NormalizePage(page, pageSize)
	if err != nil {
		return domain.ListResult{}, err
	}
	now := s.now().UTC()
	_ = s.repository.ExpireShares(ctx, now)
	organizations, err := s.repository.ListActiveUserOrganizations(ctx, actor.UserID, now)
	if err != nil {
		return domain.ListResult{}, err
	}
	total, err := s.repository.CountReceived(ctx, actor.UserID, organizations, now)
	if err != nil {
		return domain.ListResult{}, err
	}
	items, err := s.repository.ListReceived(ctx, actor.UserID, organizations, page, pageSize, now)
	if err != nil {
		return domain.ListResult{}, err
	}
	return domain.ListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) GetShare(ctx context.Context, actor domain.Actor, shareID uuid.UUID) (domain.Share, error) {
	share, err := s.repository.GetShare(ctx, shareID)
	if err != nil {
		return domain.Share{}, err
	}
	if err = s.requireVisible(ctx, actor, share, nil); err != nil {
		return domain.Share{}, err
	}
	return share, nil
}

func (s *Service) UpdateShare(ctx context.Context, actor domain.Actor, shareID uuid.UUID, input domain.UpdateInput) (domain.Share, error) {
	if input.RowVersion < 1 {
		return domain.Share{}, &domain.ValidationError{Field: "rowVersion"}
	}
	var actions []string
	var hasActions bool
	if input.Actions != nil {
		var err error
		actions, err = domain.NormalizeActions(*input.Actions)
		if err != nil {
			return domain.Share{}, err
		}
		hasActions = true
	}
	now := s.now().UTC()
	var result domain.Share
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetShareForUpdate(ctx, shareID)
		if err != nil {
			return err
		}
		if err = s.requireManage(ctx, repository, actor, current, now); err != nil {
			return err
		}
		if !hasActions {
			actions = current.Actions
		}
		validUntil := current.ValidUntil
		if input.ValidUntil != nil {
			validUntil = input.ValidUntil
		}
		if err = domain.ValidatePeriod(validUntil, now); err != nil {
			return err
		}
		allowReshare := current.AllowReshare
		if input.AllowReshare != nil {
			allowReshare = *input.AllowReshare
		}
		sourceType, sourceID := current.Source()
		source, err := repository.GetSourceResource(ctx, sourceType, sourceID)
		if err != nil {
			return err
		}
		for _, action := range actions {
			if err = s.requirePermission(ctx, actor, source, action); err != nil {
				return err
			}
		}
		result, err = repository.UpdateShare(ctx, shareID, actions, allowReshare, validUntil, input.RowVersion, now)
		if err != nil {
			return err
		}
		if err = repository.IncrementShareVersions(ctx, result, now); err != nil {
			return err
		}
		return insertEvent(ctx, repository, "SHARE_UPDATED", result, input.RequestID, now)
	})
	return result, err
}

func (s *Service) RevokeShare(ctx context.Context, actor domain.Actor, shareID uuid.UUID, input domain.RevokeInput) (domain.Share, error) {
	if input.RowVersion < 1 {
		return domain.Share{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if err := domain.ValidateReason(input.Reason); err != nil {
		return domain.Share{}, err
	}
	now := s.now().UTC()
	var result domain.Share
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetShareForUpdate(ctx, shareID)
		if err != nil {
			return err
		}
		if err = s.requireManage(ctx, repository, actor, current, now); err != nil {
			return err
		}
		result, err = repository.RevokeShare(ctx, shareID, actor.UserID, strings.TrimSpace(input.Reason), input.RowVersion, now)
		if err != nil {
			return err
		}
		if err = repository.IncrementShareVersions(ctx, result, now); err != nil {
			return err
		}
		return insertEvent(ctx, repository, "SHARE_REVOKED", result, input.RequestID, now)
	})
	return result, err
}

func (s *Service) OpenShare(ctx context.Context, actor domain.Actor, shareID uuid.UUID, input domain.OpenInput) (domain.Share, error) {
	now := s.now().UTC()
	_ = s.repository.ExpireShares(ctx, now)
	share, err := s.repository.GetShare(ctx, shareID)
	if err != nil {
		return domain.Share{}, err
	}
	if err = s.requireVisible(ctx, actor, share, input.ShareToken); err != nil {
		return domain.Share{}, err
	}
	_ = insertEvent(ctx, s.repository, "SHARE_OPENED", share, input.RequestID, now)
	return share, nil
}

func (s *Service) requireManage(ctx context.Context, repository Repository, actor domain.Actor, share domain.Share, now time.Time) error {
	if actor.Role == domain.SystemRoleAdmin || share.CreatorUserID == actor.UserID {
		return nil
	}
	sourceType, sourceID := share.Source()
	source, err := repository.GetSourceResource(ctx, sourceType, sourceID)
	if err != nil {
		return err
	}
	return s.requirePermission(ctx, actor, source, domain.ActionManagePerm)
}

func (s *Service) requireVisible(ctx context.Context, actor domain.Actor, share domain.Share, token *string) error {
	if actor.Role == domain.SystemRoleAdmin || share.CreatorUserID == actor.UserID {
		return nil
	}
	now := s.now().UTC()
	if share.Status != domain.StatusActive || share.ValidFrom.After(now) || (share.ValidUntil != nil && !share.ValidUntil.After(now)) {
		return domain.ErrForbidden
	}
	switch share.TargetKind {
	case domain.TargetUser:
		if share.TargetUserID != nil && *share.TargetUserID == actor.UserID {
			return nil
		}
	case domain.TargetOrganization:
		organizations, err := s.repository.ListActiveUserOrganizations(ctx, actor.UserID, now)
		if err != nil {
			return err
		}
		if share.TargetOrganizationID != nil && slices.Contains(organizations, *share.TargetOrganizationID) {
			return nil
		}
	case domain.TargetLink:
		if token == nil || strings.TrimSpace(*token) == "" {
			return domain.ErrShareTokenRequired
		}
		hash, err := tokenHash(*token)
		if err != nil {
			return err
		}
		if bytes.Equal(hash, share.TokenHash) {
			return nil
		}
		return domain.ErrShareTokenInvalid
	}
	return domain.ErrForbidden
}

func (s *Service) requirePermission(ctx context.Context, actor domain.Actor, source domain.SourceResource, action string) error {
	result, err := s.authorizer.EvaluatePermission(ctx, permissiondomain.Actor{UserID: actor.UserID, SessionID: actor.SessionID, Role: actor.Role}, source.Type, source.ID, action, nil, false)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return domain.ErrForbidden
	}
	return nil
}

func validateIdempotencyKey(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 128 {
		return &domain.ValidationError{Field: "Idempotency-Key"}
	}
	return nil
}

func newShareToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash, err := tokenHash(token)
	return token, hash, err
}

func tokenHash(value string) ([]byte, error) {
	if strings.TrimSpace(value) == "" {
		return nil, domain.ErrShareTokenRequired
	}
	sum := sha256.Sum256([]byte(value))
	return sum[:], nil
}

func claimIdempotency(ctx context.Context, repository Repository, actorID uuid.UUID, operation, key string, hash []byte, now time.Time) (*uuid.UUID, error) {
	recordID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	created, err := repository.TryCreateIdempotency(ctx, recordID, actorID, operation, key, hash, now.Add(idempotencyTTL), now)
	if err != nil {
		return nil, err
	}
	if created {
		return nil, nil
	}
	record, err := repository.GetIdempotency(ctx, actorID, operation, key)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(record.RequestHash, hash) {
		return nil, domain.ErrIdempotencyConflict
	}
	if record.Status == "COMPLETED" && record.ResultResourceID != nil {
		return record.ResultResourceID, nil
	}
	return nil, domain.ErrConflict
}

func requestHash(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(encoded)
	return hash[:], nil
}

func insertEvent(ctx context.Context, repository Repository, eventType string, share domain.Share, requestID uuid.UUID, now time.Time) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	sourceType, sourceID := share.Source()
	payload, err := json.Marshal(map[string]any{"shareId": share.ID, "sourceType": sourceType, "sourceId": sourceID, "targetKind": share.TargetKind})
	if err != nil {
		return err
	}
	return repository.InsertEvent(ctx, domain.Event{ID: id, AggregateType: domain.ResourceShare, AggregateID: share.ID, AggregateVersion: share.RowVersion, Type: eventType, Payload: payload, DeduplicationKey: fmt.Sprintf("%s:%s:%d", eventType, share.ID, share.RowVersion), CorrelationID: requestID, CreatedAt: now})
}
