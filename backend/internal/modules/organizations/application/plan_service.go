package application

import (
	"context"
	"encoding/json"
	"strings"

	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

const (
	operationCreateOrganizationChangePlan = "CREATE_ORGANIZATION_CHANGE_PLAN"
	operationAddPlanOperation             = "ADD_ORGANIZATION_CHANGE_OPERATION"
)

type CreatePlanInput struct {
	PlanType            string
	Name                string
	ExpectedTreeVersion int64
	IdempotencyKey      string
	RequestID           uuid.UUID
}

type AddPlanOperationInput struct {
	SequenceNumber         int32
	OperationType          string
	SourceOrganizationID   *uuid.UUID
	TargetOrganizationID   *uuid.UUID
	OperationSchemaVersion int32
	OperationJSON          json.RawMessage
	IdempotencyKey         string
	RequestID              uuid.UUID
}

type TransitionPlanInput struct {
	Action     string
	RowVersion int64
	Reason     string
	RequestID  uuid.UUID
}

func (s *Service) GetPlan(ctx context.Context, actor Actor, id uuid.UUID) (domain.OrganizationChangePlan, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	plan, err := s.repository.GetPlan(ctx, id)
	if err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	plan.Operations, err = s.repository.ListPlanOperations(ctx, id)
	return plan, err
}

func (s *Service) ListPlans(ctx context.Context, actor Actor, filter domain.PlanListFilter) (domain.PlanListResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.PlanListResult{}, err
	}
	page, pageSize, err := normalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return domain.PlanListResult{}, err
	}
	if filter.Status != nil {
		if err := domain.ValidatePlanStatus(*filter.Status); err != nil {
			return domain.PlanListResult{}, err
		}
	}
	filter.Page, filter.PageSize = page, pageSize
	return s.repository.ListPlans(ctx, filter)
}

func (s *Service) CreatePlan(ctx context.Context, actor Actor, input CreatePlanInput) (domain.OrganizationChangePlan, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	if err := domain.ValidatePlanType(input.PlanType); err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 256 {
		return domain.OrganizationChangePlan{}, &domain.ValidationError{Field: "name"}
	}
	if input.ExpectedTreeVersion < 1 {
		return domain.OrganizationChangePlan{}, &domain.ValidationError{Field: "expectedTreeVersion"}
	}
	if err := validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	id, err := newUUID("organization change plan")
	if err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	now := s.now().UTC()
	prepared := domain.OrganizationChangePlan{ID: id, PlanType: input.PlanType, Name: name, Status: domain.PlanStatusDraft, ExpectedTreeVersion: input.ExpectedTreeVersion, CreatedByUserID: actor.UserID, CreatedAt: now, UpdatedAt: now, RowVersion: 1, Operations: []domain.OrganizationChangeOperation{}}
	hash, err := requestHash(struct {
		PlanType            string
		Name                string
		ExpectedTreeVersion int64
	}{prepared.PlanType, prepared.Name, prepared.ExpectedTreeVersion})
	if err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	var result domain.OrganizationChangePlan
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, operationCreateOrganizationChangePlan, input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result, err = repository.GetPlan(ctx, *replayID)
			return err
		}
		result, err = repository.InsertPlan(ctx, prepared)
		if err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, operationCreateOrganizationChangePlan, input.IdempotencyKey, result.ID, "ORGANIZATION_CHANGE_PLAN", now)
	})
	return result, err
}

func (s *Service) AddPlanOperation(ctx context.Context, actor Actor, planID uuid.UUID, input AddPlanOperationInput) (domain.OrganizationChangePlan, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	if input.SequenceNumber < 1 || input.OperationSchemaVersion < 1 {
		return domain.OrganizationChangePlan{}, &domain.ValidationError{Field: "sequenceNumber/operationSchemaVersion"}
	}
	if err := domain.ValidateOperationType(input.OperationType); err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	if err := validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	if len(input.OperationJSON) == 0 {
		input.OperationJSON = json.RawMessage("{}")
	}
	if err := domain.ValidateJSONObject(input.OperationJSON); err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	if err := validateOperationReferences(input.OperationType, input.SourceOrganizationID, input.TargetOrganizationID); err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	id, err := newUUID("organization change operation")
	if err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	now := s.now().UTC()
	prepared := domain.OrganizationChangeOperation{ID: id, PlanID: planID, SequenceNumber: input.SequenceNumber, OperationType: input.OperationType, SourceOrganizationID: input.SourceOrganizationID, TargetOrganizationID: input.TargetOrganizationID, OperationSchemaVersion: input.OperationSchemaVersion, OperationJSON: input.OperationJSON, Status: domain.OperationStatusPending, CreatedAt: now, UpdatedAt: now, RowVersion: 1}
	hash, err := requestHash(struct {
		SequenceNumber         int32
		OperationType          string
		SourceOrganizationID   *uuid.UUID
		TargetOrganizationID   *uuid.UUID
		OperationSchemaVersion int32
		OperationJSON          json.RawMessage
	}{prepared.SequenceNumber, prepared.OperationType, prepared.SourceOrganizationID, prepared.TargetOrganizationID, prepared.OperationSchemaVersion, prepared.OperationJSON})
	if err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	operation := operationAddPlanOperation + ":" + planID.String()
	var result domain.OrganizationChangePlan
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, operation, input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result, err = repository.GetPlan(ctx, *replayID)
			if err == nil {
				result.Operations, err = repository.ListPlanOperations(ctx, *replayID)
			}
			return err
		}
		plan, err := repository.GetPlanForUpdate(ctx, planID)
		if err != nil {
			return err
		}
		if plan.Status != domain.PlanStatusDraft {
			return domain.ErrInvalidStateTransition
		}
		if plan.PlanType == domain.PlanTypeMove && input.OperationType != domain.OperationTypeMoveNode {
			return &domain.ValidationError{Field: "operationType"}
		}
		if _, err := repository.InsertPlanOperation(ctx, prepared); err != nil {
			return err
		}
		result, err = repository.TouchDraftPlan(ctx, planID, now)
		if err != nil {
			return err
		}
		result.Operations, err = repository.ListPlanOperations(ctx, planID)
		if err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, operation, input.IdempotencyKey, planID, "ORGANIZATION_CHANGE_PLAN", now)
	})
	return result, err
}

func (s *Service) TransitionPlan(ctx context.Context, actor Actor, planID uuid.UUID, input TransitionPlanInput) (domain.OrganizationChangePlan, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	if input.RowVersion < 1 {
		return domain.OrganizationChangePlan{}, &domain.ValidationError{Field: "rowVersion"}
	}
	switch input.Action {
	case domain.PlanActionValidate, domain.PlanActionApprove, domain.PlanActionExecute, domain.PlanActionCancel:
	default:
		return domain.OrganizationChangePlan{}, &domain.ValidationError{Field: "action"}
	}
	now := s.now().UTC()
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		plan, err := repository.GetPlanForUpdate(ctx, planID)
		if err != nil {
			return err
		}
		if plan.RowVersion != input.RowVersion {
			return domain.ErrVersionConflict
		}
		operations, err := repository.ListPlanOperations(ctx, planID)
		if err != nil {
			return err
		}
		switch input.Action {
		case domain.PlanActionValidate:
			if plan.Status != domain.PlanStatusDraft || len(operations) == 0 {
				return domain.ErrInvalidStateTransition
			}
			if err := s.validatePlanOperations(ctx, repository, plan, operations); err != nil {
				return err
			}
			_, err = repository.SetPlanStatus(ctx, planID, domain.PlanStatusValidated, nil, nil, plan.RowVersion, now)
		case domain.PlanActionApprove:
			if plan.Status != domain.PlanStatusValidated {
				return domain.ErrInvalidStateTransition
			}
			approver := actor.UserID
			_, err = repository.SetPlanStatus(ctx, planID, domain.PlanStatusApproved, &approver, nil, plan.RowVersion, now)
		case domain.PlanActionCancel:
			if plan.Status != domain.PlanStatusDraft && plan.Status != domain.PlanStatusValidated && plan.Status != domain.PlanStatusApproved {
				return domain.ErrInvalidStateTransition
			}
			_, err = repository.SetPlanStatus(ctx, planID, domain.PlanStatusCancelled, nil, nil, plan.RowVersion, now)
		case domain.PlanActionExecute:
			if plan.Status != domain.PlanStatusApproved {
				return domain.ErrInvalidStateTransition
			}
			if plan.PlanType != domain.PlanTypeMove {
				return domain.ErrUnsupportedOperation
			}
			if err := s.validatePlanOperations(ctx, repository, plan, operations); err != nil {
				return err
			}
			executing, err := repository.SetPlanStatus(ctx, planID, domain.PlanStatusExecuting, nil, nil, plan.RowVersion, now)
			if err != nil {
				return err
			}
			for _, operation := range operations {
				if operation.OperationType != domain.OperationTypeMoveNode || operation.SourceOrganizationID == nil {
					return domain.ErrUnsupportedOperation
				}
				organization, err := repository.GetOrganizationForUpdate(ctx, *operation.SourceOrganizationID)
				if err != nil {
					return err
				}
				if _, err := s.moveOrganizationTx(ctx, repository, actor, organization.ID, operation.TargetOrganizationID, organization.RowVersion, input.Reason, input.RequestID, now); err != nil {
					return err
				}
				if _, err := repository.MarkPlanOperation(ctx, operation.ID, domain.OperationStatusSuccess, nil, now); err != nil {
					return err
				}
			}
			_, err = repository.SetPlanStatus(ctx, planID, domain.PlanStatusCompleted, nil, nil, executing.RowVersion, now)
		default:
			return &domain.ValidationError{Field: "action"}
		}
		return err
	})
	if err != nil {
		return domain.OrganizationChangePlan{}, err
	}
	return s.GetPlan(ctx, actor, planID)
}

func (s *Service) validatePlanOperations(ctx context.Context, repository Repository, plan domain.OrganizationChangePlan, operations []domain.OrganizationChangeOperation) error {
	for _, operation := range operations {
		if operation.Status != domain.OperationStatusPending {
			return domain.ErrInvalidStateTransition
		}
		if err := validateOperationReferences(operation.OperationType, operation.SourceOrganizationID, operation.TargetOrganizationID); err != nil {
			return err
		}
		if operation.SourceOrganizationID != nil {
			source, err := repository.GetOrganization(ctx, *operation.SourceOrganizationID)
			if err != nil {
				return err
			}
			if source.TreeVersion != plan.ExpectedTreeVersion {
				return domain.ErrConflict
			}
			if operation.OperationType == domain.OperationTypeMoveNode && operation.TargetOrganizationID != nil {
				if _, err := repository.GetOrganization(ctx, *operation.TargetOrganizationID); err != nil {
					return err
				}
				cycle, err := repository.OrganizationWouldCreateCycle(ctx, source.ID, *operation.TargetOrganizationID)
				if err != nil {
					return err
				}
				if cycle {
					return domain.ErrTreeCycle
				}
			}
		}
	}
	return nil
}

func validateOperationReferences(operationType string, sourceID, targetID *uuid.UUID) error {
	switch operationType {
	case domain.OperationTypeMoveNode:
		if sourceID == nil {
			return &domain.ValidationError{Field: "sourceOrganizationId"}
		}
	case domain.OperationTypeMergeNode, domain.OperationTypeMoveMember, domain.OperationTypeMoveSpaceContent:
		if sourceID == nil || targetID == nil {
			return &domain.ValidationError{Field: "sourceOrganizationId/targetOrganizationId"}
		}
	case domain.OperationTypeCreateNode:
		// 根组织创建允许 targetOrganizationId 为空，具体节点属性保存在版本化 operation 对象中。
	default:
		return &domain.ValidationError{Field: "operationType"}
	}
	return nil
}
