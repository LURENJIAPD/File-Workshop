package transport

import (
	"context"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/modules/permissions/application"
	"file-workshop/backend/internal/modules/permissions/domain"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (h *Handler) ListAdminDelegations(ctx context.Context, request api.ListAdminDelegationsRequestObject) (api.ListAdminDelegationsResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ListAdminDelegations401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	var organizationID *uuid.UUID
	if request.Params.OrganizationId != nil {
		id := uuid.UUID(*request.Params.OrganizationId)
		organizationID = &id
	}
	var status *string
	if request.Params.Status != nil {
		value := string(*request.Params.Status)
		status = &value
	}
	result, err := h.service.ListAdminDelegations(ctx, actor, organizationID, status, page, pageSize)
	if err != nil {
		if classify(err) == errorInvalid {
			return api.ListAdminDelegations400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		}
		return nil, err
	}
	return api.ListAdminDelegations200JSONResponse(apiDelegationList(result, requestID)), nil
}

func (h *Handler) CreateAdminDelegation(ctx context.Context, request api.CreateAdminDelegationRequestObject) (api.CreateAdminDelegationResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.CreateAdminDelegation401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CreateAdminDelegation403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.CreateAdminDelegation400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
	}
	capabilities := make([]string, 0, len(request.Body.Capabilities))
	for _, item := range request.Body.Capabilities {
		capabilities = append(capabilities, string(item))
	}
	var parentID *uuid.UUID
	if request.Body.ParentDelegationId != nil {
		id := uuid.UUID(*request.Body.ParentDelegationId)
		parentID = &id
	}
	value, err := h.service.CreateAdminDelegation(ctx, actor, application.CreateAdminDelegationInput{UserID: uuid.UUID(request.Body.UserId), OrganizationID: uuid.UUID(request.Body.OrganizationId), Scope: string(request.Body.Scope), CanDelegate: request.Body.CanDelegate, ParentDelegationID: parentID, Capabilities: capabilities, ValidFrom: request.Body.ValidFrom, ValidUntil: request.Body.ValidUntil, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: requestUUID(requestID)})
	if err != nil {
		switch classify(err) {
		case errorInvalid:
			return api.CreateAdminDelegation400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		case errorForbidden:
			return api.CreateAdminDelegation403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
		case errorNotFound:
			return api.CreateAdminDelegation404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
		case errorConflict:
			return api.CreateAdminDelegation409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
		default:
			return nil, err
		}
	}
	return api.CreateAdminDelegation201JSONResponse(api.AdminDelegationResponse{Delegation: apiDelegation(value), RequestId: requestID}), nil
}

func (h *Handler) GetAdminDelegation(ctx context.Context, request api.GetAdminDelegationRequestObject) (api.GetAdminDelegationResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.GetAdminDelegation401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	value, err := h.service.GetAdminDelegation(ctx, actor, uuid.UUID(request.DelegationId))
	if err != nil {
		if classify(err) == errorNotFound {
			return api.GetAdminDelegation404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
		}
		return nil, err
	}
	return api.GetAdminDelegation200JSONResponse(api.AdminDelegationResponse{Delegation: apiDelegation(value), RequestId: requestID}), nil
}

func (h *Handler) RevokeAdminDelegation(ctx context.Context, request api.RevokeAdminDelegationRequestObject) (api.RevokeAdminDelegationResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.RevokeAdminDelegation401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RevokeAdminDelegation403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.RevokeAdminDelegation400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
	}
	value, err := h.service.RevokeAdminDelegation(ctx, actor, uuid.UUID(request.DelegationId), request.Body.RowVersion, request.Body.Reason, requestUUID(requestID))
	if err != nil {
		switch classify(err) {
		case errorInvalid:
			return api.RevokeAdminDelegation400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		case errorForbidden:
			return api.RevokeAdminDelegation403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
		case errorNotFound:
			return api.RevokeAdminDelegation404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
		case errorConflict:
			return api.RevokeAdminDelegation409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
		default:
			return nil, err
		}
	}
	return api.RevokeAdminDelegation200JSONResponse(api.AdminDelegationResponse{Delegation: apiDelegation(value), RequestId: requestID}), nil
}

func (h *Handler) ListOrganizationAdministrators(ctx context.Context, request api.ListOrganizationAdministratorsRequestObject) (api.ListOrganizationAdministratorsResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ListOrganizationAdministrators401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	result, err := h.service.ListOrganizationAdministrators(ctx, actor, uuid.UUID(request.OrganizationId), page, pageSize)
	if err != nil {
		switch classify(err) {
		case errorInvalid:
			return api.ListOrganizationAdministrators400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		case errorForbidden:
			return api.ListOrganizationAdministrators403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
		case errorNotFound:
			return api.ListOrganizationAdministrators404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
		default:
			return nil, err
		}
	}
	return api.ListOrganizationAdministrators200JSONResponse(apiDelegationList(result, requestID)), nil
}

func (h *Handler) EvaluateAdminDelegation(ctx context.Context, request api.EvaluateAdminDelegationRequestObject) (api.EvaluateAdminDelegationResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.EvaluateAdminDelegation401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if request.Body == nil {
		return api.EvaluateAdminDelegation400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
	}
	allowed, source, id, err := h.service.EvaluateAdminDelegation(ctx, actor, uuid.UUID(request.Body.OrganizationId), string(request.Body.Capability))
	if err != nil {
		if classify(err) == errorInvalid {
			return api.EvaluateAdminDelegation400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		}
		return nil, err
	}
	response := api.AdminDelegationEvaluationResponse{Allowed: allowed, Source: api.AdminDelegationEvaluationResponseSource(source), RequestId: requestID}
	if id != nil {
		value := openapi_types.UUID(*id)
		response.DelegationId = &value
	}
	return api.EvaluateAdminDelegation200JSONResponse(response), nil
}

var _ = domain.StatusActive
