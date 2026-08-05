package transport

import (
	"context"
	"errors"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/modules/organizations/application"
	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

func (h *Handler) ProvisionUserPersonalSpace(ctx context.Context, request api.ProvisionUserPersonalSpaceRequestObject) (api.ProvisionUserPersonalSpaceResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ProvisionUserPersonalSpace401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.ProvisionUserPersonalSpace403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.ProvisionUserPersonalSpace400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	config, err := jsonObject(request.Body.Config)
	if err != nil {
		return api.ProvisionUserPersonalSpace400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.ProvisionPersonalSpace(ginContext.Request.Context(), actor, uuid.UUID(request.UserId), application.CreateSpaceInput{Name: request.Body.Name, QuotaBytes: request.Body.QuotaBytes, ConfigJSON: config, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: databaseRequestID(requestID)})
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return api.ProvisionUserPersonalSpace400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return api.ProvisionUserPersonalSpace403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case isNotFound(err):
		return api.ProvisionUserPersonalSpace404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, nil
	case isConflict(err):
		return api.ProvisionUserPersonalSpace409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
	case err != nil:
		return nil, err
	}
	body, err := apiSpace(result)
	if err != nil {
		return nil, err
	}
	return api.ProvisionUserPersonalSpace201JSONResponse(api.SpaceResponse{Space: body, RequestId: requestID}), nil
}

func (h *Handler) ListSpaces(ctx context.Context, request api.ListSpacesRequestObject) (api.ListSpacesResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ListSpaces401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	filter := domain.SpaceListFilter{Page: page, PageSize: pageSize}
	if request.Params.SpaceType != nil {
		value := string(*request.Params.SpaceType)
		filter.SpaceType = &value
	}
	if request.Params.Status != nil {
		value := string(*request.Params.Status)
		filter.Status = &value
	}
	if request.Params.OrganizationId != nil {
		value := uuid.UUID(*request.Params.OrganizationId)
		filter.OrganizationID = &value
	}
	if request.Params.OwnerUserId != nil {
		value := uuid.UUID(*request.Params.OwnerUserId)
		filter.OwnerUserID = &value
	}
	result, err := h.service.ListSpaces(ginContext.Request.Context(), actor, filter)
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return api.ListSpaces400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return api.ListSpaces403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case err != nil:
		return nil, err
	}
	items := make([]api.Space, 0, len(result.Items))
	for _, item := range result.Items {
		mapped, err := apiSpace(item)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return api.ListSpaces200JSONResponse(api.SpaceListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) CreatePublicSpace(ctx context.Context, request api.CreatePublicSpaceRequestObject) (api.CreatePublicSpaceResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.CreatePublicSpace401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CreatePublicSpace403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.CreatePublicSpace400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	config, err := jsonObject(request.Body.Config)
	if err != nil {
		return api.CreatePublicSpace400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.CreatePublicSpace(ginContext.Request.Context(), actor, application.CreateSpaceInput{Name: request.Body.Name, QuotaBytes: request.Body.QuotaBytes, ConfigJSON: config, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: databaseRequestID(requestID)})
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return api.CreatePublicSpace400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return api.CreatePublicSpace403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case isConflict(err):
		return api.CreatePublicSpace409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
	case err != nil:
		return nil, err
	}
	body, err := apiSpace(result)
	if err != nil {
		return nil, err
	}
	return api.CreatePublicSpace201JSONResponse(api.SpaceResponse{Space: body, RequestId: requestID}), nil
}

func (h *Handler) GetSpace(ctx context.Context, request api.GetSpaceRequestObject) (api.GetSpaceResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetSpace401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	result, err := h.service.GetSpace(ginContext.Request.Context(), actor, uuid.UUID(request.SpaceId))
	switch {
	case errors.Is(err, domain.ErrForbidden):
		return api.GetSpace403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case isNotFound(err):
		return api.GetSpace404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, nil
	case err != nil:
		return nil, err
	}
	body, err := apiSpace(result)
	if err != nil {
		return nil, err
	}
	return api.GetSpace200JSONResponse(api.SpaceResponse{Space: body, RequestId: requestID}), nil
}

func (h *Handler) UpdateSpace(ctx context.Context, request api.UpdateSpaceRequestObject) (api.UpdateSpaceResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.UpdateSpace401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.UpdateSpace403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.UpdateSpace400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	changes := domain.SpaceChanges{Name: request.Body.Name, QuotaBytes: request.Body.QuotaBytes, RowVersion: request.Body.RowVersion}
	if request.Body.ConfigSchemaVersion != nil {
		value := int32(*request.Body.ConfigSchemaVersion)
		changes.ConfigSchemaVersion = &value
	}
	if request.Body.Config != nil {
		changes.ConfigJSON, _ = jsonObject(request.Body.Config)
	}
	result, err := h.service.UpdateSpace(ginContext.Request.Context(), actor, uuid.UUID(request.SpaceId), changes, databaseRequestID(requestID))
	if response, handled := mapUpdateSpaceError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	body, err := apiSpace(result)
	if err != nil {
		return nil, err
	}
	return api.UpdateSpace200JSONResponse(api.SpaceResponse{Space: body, RequestId: requestID}), nil
}

func (h *Handler) ChangeSpaceStatus(ctx context.Context, request api.ChangeSpaceStatusRequestObject) (api.ChangeSpaceStatusResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ChangeSpaceStatus401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.ChangeSpaceStatus403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.ChangeSpaceStatus400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.ChangeSpaceStatus(ginContext.Request.Context(), actor, uuid.UUID(request.SpaceId), application.ChangeSpaceStatusInput{Status: string(request.Body.Status), RowVersion: request.Body.RowVersion, Reason: request.Body.Reason, RequestID: databaseRequestID(requestID)})
	if response, handled := mapChangeSpaceStatusError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	body, err := apiSpace(result)
	if err != nil {
		return nil, err
	}
	return api.ChangeSpaceStatus200JSONResponse(api.SpaceResponse{Space: body, RequestId: requestID}), nil
}

func (h *Handler) ListOrganizationChangePlans(ctx context.Context, request api.ListOrganizationChangePlansRequestObject) (api.ListOrganizationChangePlansResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ListOrganizationChangePlans401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	filter := domain.PlanListFilter{Page: page, PageSize: pageSize}
	if request.Params.Status != nil {
		value := string(*request.Params.Status)
		filter.Status = &value
	}
	result, err := h.service.ListPlans(ginContext.Request.Context(), actor, filter)
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return api.ListOrganizationChangePlans400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return api.ListOrganizationChangePlans403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case err != nil:
		return nil, err
	}
	items := make([]api.OrganizationChangePlan, 0, len(result.Items))
	for _, item := range result.Items {
		mapped, err := apiPlan(item)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return api.ListOrganizationChangePlans200JSONResponse(api.OrganizationChangePlanListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) CreateOrganizationChangePlan(ctx context.Context, request api.CreateOrganizationChangePlanRequestObject) (api.CreateOrganizationChangePlanResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.CreateOrganizationChangePlan401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CreateOrganizationChangePlan403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.CreateOrganizationChangePlan400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.CreatePlan(ginContext.Request.Context(), actor, application.CreatePlanInput{PlanType: string(request.Body.PlanType), Name: request.Body.Name, ExpectedTreeVersion: request.Body.ExpectedTreeVersion, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: databaseRequestID(requestID)})
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return api.CreateOrganizationChangePlan400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return api.CreateOrganizationChangePlan403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case isConflict(err):
		return api.CreateOrganizationChangePlan409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
	case err != nil:
		return nil, err
	}
	body, err := apiPlan(result)
	if err != nil {
		return nil, err
	}
	return api.CreateOrganizationChangePlan201JSONResponse(api.OrganizationChangePlanResponse{Plan: body, RequestId: requestID}), nil
}

func (h *Handler) GetOrganizationChangePlan(ctx context.Context, request api.GetOrganizationChangePlanRequestObject) (api.GetOrganizationChangePlanResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetOrganizationChangePlan401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	result, err := h.service.GetPlan(ginContext.Request.Context(), actor, uuid.UUID(request.PlanId))
	switch {
	case errors.Is(err, domain.ErrForbidden):
		return api.GetOrganizationChangePlan403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case isNotFound(err):
		return api.GetOrganizationChangePlan404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, nil
	case err != nil:
		return nil, err
	}
	body, err := apiPlan(result)
	if err != nil {
		return nil, err
	}
	return api.GetOrganizationChangePlan200JSONResponse(api.OrganizationChangePlanResponse{Plan: body, RequestId: requestID}), nil
}

func (h *Handler) AddOrganizationChangeOperation(ctx context.Context, request api.AddOrganizationChangeOperationRequestObject) (api.AddOrganizationChangeOperationResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.AddOrganizationChangeOperation401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.AddOrganizationChangeOperation403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.AddOrganizationChangeOperation400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	operationJSON, err := jsonObject(&request.Body.Operation)
	if err != nil {
		return api.AddOrganizationChangeOperation400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	var sourceID, targetID *uuid.UUID
	if request.Body.SourceOrganizationId != nil {
		value := uuid.UUID(*request.Body.SourceOrganizationId)
		sourceID = &value
	}
	if request.Body.TargetOrganizationId != nil {
		value := uuid.UUID(*request.Body.TargetOrganizationId)
		targetID = &value
	}
	result, err := h.service.AddPlanOperation(ginContext.Request.Context(), actor, uuid.UUID(request.PlanId), application.AddPlanOperationInput{SequenceNumber: int32(request.Body.SequenceNumber), OperationType: string(request.Body.OperationType), SourceOrganizationID: sourceID, TargetOrganizationID: targetID, OperationSchemaVersion: int32(request.Body.OperationSchemaVersion), OperationJSON: operationJSON, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: databaseRequestID(requestID)})
	if response, handled := mapAddPlanOperationError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	body, err := apiPlan(result)
	if err != nil {
		return nil, err
	}
	return api.AddOrganizationChangeOperation201JSONResponse(api.OrganizationChangePlanResponse{Plan: body, RequestId: requestID}), nil
}

func (h *Handler) TransitionOrganizationChangePlan(ctx context.Context, request api.TransitionOrganizationChangePlanRequestObject) (api.TransitionOrganizationChangePlanResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.TransitionOrganizationChangePlan401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.TransitionOrganizationChangePlan403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.TransitionOrganizationChangePlan400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	reason := ""
	if request.Body.Reason != nil {
		reason = *request.Body.Reason
	}
	result, err := h.service.TransitionPlan(ginContext.Request.Context(), actor, uuid.UUID(request.PlanId), application.TransitionPlanInput{Action: string(request.Body.Action), RowVersion: request.Body.RowVersion, Reason: reason, RequestID: databaseRequestID(requestID)})
	if response, handled := mapTransitionPlanError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	body, err := apiPlan(result)
	if err != nil {
		return nil, err
	}
	return api.TransitionOrganizationChangePlan200JSONResponse(api.OrganizationChangePlanResponse{Plan: body, RequestId: requestID}), nil
}

func mapUpdateSpaceError(err error, requestID string) (api.UpdateSpaceResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrInvalidInput):
		return api.UpdateSpace400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrForbidden):
		return api.UpdateSpace403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, true
	case isNotFound(err):
		return api.UpdateSpace404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, true
	case isConflict(err):
		return api.UpdateSpace409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapChangeSpaceStatusError(err error, requestID string) (api.ChangeSpaceStatusResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrInvalidInput):
		return api.ChangeSpaceStatus400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrForbidden):
		return api.ChangeSpaceStatus403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, true
	case isNotFound(err):
		return api.ChangeSpaceStatus404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, true
	case isConflict(err):
		return api.ChangeSpaceStatus409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapAddPlanOperationError(err error, requestID string) (api.AddOrganizationChangeOperationResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrInvalidInput):
		return api.AddOrganizationChangeOperation400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrForbidden):
		return api.AddOrganizationChangeOperation403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, true
	case isNotFound(err):
		return api.AddOrganizationChangeOperation404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, true
	case isConflict(err):
		return api.AddOrganizationChangeOperation409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapTransitionPlanError(err error, requestID string) (api.TransitionOrganizationChangePlanResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrInvalidInput):
		return api.TransitionOrganizationChangePlan400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrForbidden):
		return api.TransitionOrganizationChangePlan403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, true
	case isNotFound(err):
		return api.TransitionOrganizationChangePlan404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, true
	case isConflict(err):
		return api.TransitionOrganizationChangePlan409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}
