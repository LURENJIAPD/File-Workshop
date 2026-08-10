package application

import (
	"context"
	"strings"
	"time"

	"file-workshop/backend/internal/modules/audit/domain"

	"github.com/google/uuid"
)

type Repository interface {
	ListEvents(context.Context, domain.EventListFilter) (domain.EventListResult, error)
	GetEvent(context.Context, uuid.UUID, time.Time) (domain.Event, error)
	ListChainHeads(context.Context, domain.IntegrityFilter) (domain.IntegrityResult, error)
	VerifyChain(context.Context, string, time.Time, time.Time) (domain.VerificationResult, error)
	InsertEvent(context.Context, domain.Event) error
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, now: now}
}

func (s *Service) ListEvents(ctx context.Context, actor domain.Actor, filter domain.EventListFilter) (domain.EventListResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.EventListResult{}, err
	}
	if err := normalizeEventFilter(&filter); err != nil {
		return domain.EventListResult{}, err
	}
	return s.repository.ListEvents(ctx, filter)
}

func (s *Service) GetEvent(ctx context.Context, actor domain.Actor, id uuid.UUID, partitionDate time.Time) (domain.Event, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Event{}, err
	}
	if id == uuid.Nil {
		return domain.Event{}, &domain.ValidationError{Field: "auditEventId"}
	}
	if partitionDate.IsZero() {
		return domain.Event{}, &domain.ValidationError{Field: "partitionDate"}
	}
	return s.repository.GetEvent(ctx, id, partitionDate)
}

func (s *Service) GetIntegrity(ctx context.Context, actor domain.Actor, filter domain.IntegrityFilter) (domain.IntegrityResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.IntegrityResult{}, err
	}
	if err := normalizeIntegrityFilter(&filter); err != nil {
		return domain.IntegrityResult{}, err
	}
	return s.repository.ListChainHeads(ctx, filter)
}

func (s *Service) VerifyIntegrity(ctx context.Context, actor domain.Actor, chainID string, partitionDate time.Time) (domain.VerificationResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.VerificationResult{}, err
	}
	chainID = strings.TrimSpace(chainID)
	if chainID == "" {
		return domain.VerificationResult{}, &domain.ValidationError{Field: "chainId"}
	}
	if partitionDate.IsZero() {
		return domain.VerificationResult{}, &domain.ValidationError{Field: "partitionDate"}
	}
	return s.repository.VerifyChain(ctx, chainID, partitionDate, s.now().UTC())
}

func requireAdmin(actor domain.Actor) error {
	if actor.Role != domain.SystemRoleAdmin {
		return domain.ErrForbidden
	}
	return nil
}

func normalizeEventFilter(filter *domain.EventListFilter) error {
	if filter.DateFrom.IsZero() {
		return &domain.ValidationError{Field: "dateFrom"}
	}
	if filter.DateTo.IsZero() {
		return &domain.ValidationError{Field: "dateTo"}
	}
	filter.DateFrom = dateOnly(filter.DateFrom)
	filter.DateTo = dateOnly(filter.DateTo)
	if filter.DateFrom.After(filter.DateTo) {
		return &domain.ValidationError{Field: "dateFrom/dateTo"}
	}
	page, pageSize, err := domain.NormalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return err
	}
	filter.Page, filter.PageSize = page, pageSize
	if filter.RiskLevel != nil {
		trimmed := strings.TrimSpace(*filter.RiskLevel)
		if err = domain.ValidateRiskLevel(trimmed); err != nil {
			return err
		}
		filter.RiskLevel = &trimmed
	}
	if filter.ActorType != nil {
		trimmed := strings.TrimSpace(*filter.ActorType)
		if err = domain.ValidateActorType(trimmed); err != nil {
			return err
		}
		filter.ActorType = &trimmed
	}
	if filter.Result != nil {
		trimmed := strings.TrimSpace(*filter.Result)
		if err = domain.ValidateResult(trimmed); err != nil {
			return err
		}
		filter.Result = &trimmed
	}
	filter.EventType = trimmedOptional(filter.EventType)
	filter.ResourceType = trimmedOptional(filter.ResourceType)
	return nil
}

func normalizeIntegrityFilter(filter *domain.IntegrityFilter) error {
	if filter.DateFrom.IsZero() {
		return &domain.ValidationError{Field: "dateFrom"}
	}
	if filter.DateTo.IsZero() {
		return &domain.ValidationError{Field: "dateTo"}
	}
	filter.DateFrom = dateOnly(filter.DateFrom)
	filter.DateTo = dateOnly(filter.DateTo)
	if filter.DateFrom.After(filter.DateTo) {
		return &domain.ValidationError{Field: "dateFrom/dateTo"}
	}
	page, pageSize, err := domain.NormalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return err
	}
	filter.Page, filter.PageSize = page, pageSize
	if filter.Status != nil {
		trimmed := strings.TrimSpace(*filter.Status)
		if err = domain.ValidateChainStatus(trimmed); err != nil {
			return err
		}
		filter.Status = &trimmed
	}
	return nil
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
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
