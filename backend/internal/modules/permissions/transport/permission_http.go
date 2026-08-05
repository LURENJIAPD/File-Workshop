package transport

import (
	"context"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/modules/permissions/application"
	"file-workshop/backend/internal/modules/permissions/domain"

	"github.com/google/uuid"
)

func (h *Handler) ListResourcePermissionGrants(ctx context.Context, request api.ListResourcePermissionGrantsRequestObject) (api.ListResourcePermissionGrantsResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ListResourcePermissionGrants401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	result, err := h.service.ListResourcePermissionGrants(ctx, actor, string(request.ResourceType), uuid.UUID(request.ResourceId), page, pageSize)
	if err != nil {
		switch classify(err) {
		case errorInvalid:
			return api.ListResourcePermissionGrants400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		case errorForbidden:
			return api.ListResourcePermissionGrants403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
		case errorNotFound:
			return api.ListResourcePermissionGrants404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
		default:
			return nil, err
		}
	}
	return api.ListResourcePermissionGrants200JSONResponse(apiGrantList(result, requestID)), nil
}

func (h *Handler) CreatePermissionGrant(ctx context.Context, request api.CreatePermissionGrantRequestObject) (api.CreatePermissionGrantResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.CreatePermissionGrant401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CreatePermissionGrant403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.CreatePermissionGrant400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
	}
	actions := apiActions(request.Body.Actions)
	value, err := h.service.CreatePermissionGrant(ctx, actor, application.CreatePermissionGrantInput{SubjectType: string(request.Body.SubjectType), SubjectID: uuid.UUID(request.Body.SubjectId), ResourceType: string(request.Body.ResourceType), ResourceID: uuid.UUID(request.Body.ResourceId), Actions: actions, InheritToDescendants: request.Body.InheritToDescendants, GrantSource: string(request.Body.GrantSource), ValidFrom: request.Body.ValidFrom, ValidUntil: request.Body.ValidUntil, GrantReason: request.Body.GrantReason, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: requestUUID(requestID)})
	if err != nil {
		switch classify(err) {
		case errorInvalid:
			return api.CreatePermissionGrant400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		case errorForbidden:
			return api.CreatePermissionGrant403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
		case errorNotFound:
			return api.CreatePermissionGrant404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
		case errorConflict:
			return api.CreatePermissionGrant409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
		default:
			return nil, err
		}
	}
	return api.CreatePermissionGrant201JSONResponse(api.PermissionGrantResponse{Grant: apiGrant(value), RequestId: requestID}), nil
}

func (h *Handler) UpdatePermissionGrant(ctx context.Context, request api.UpdatePermissionGrantRequestObject) (api.UpdatePermissionGrantResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.UpdatePermissionGrant401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.UpdatePermissionGrant403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.UpdatePermissionGrant400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
	}
	value, err := h.service.UpdatePermissionGrant(ctx, actor, uuid.UUID(request.GrantId), apiActions(request.Body.Actions), request.Body.InheritToDescendants, request.Body.ValidUntil, request.Body.GrantReason, request.Body.RowVersion, requestUUID(requestID))
	if err != nil {
		switch classify(err) {
		case errorInvalid:
			return api.UpdatePermissionGrant400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		case errorForbidden:
			return api.UpdatePermissionGrant403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
		case errorNotFound:
			return api.UpdatePermissionGrant404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
		case errorConflict:
			return api.UpdatePermissionGrant409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
		default:
			return nil, err
		}
	}
	return api.UpdatePermissionGrant200JSONResponse(api.PermissionGrantResponse{Grant: apiGrant(value), RequestId: requestID}), nil
}

func (h *Handler) RevokePermissionGrant(ctx context.Context, request api.RevokePermissionGrantRequestObject) (api.RevokePermissionGrantResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.RevokePermissionGrant401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RevokePermissionGrant403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.RevokePermissionGrant400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
	}
	value, err := h.service.RevokePermissionGrant(ctx, actor, uuid.UUID(request.GrantId), request.Body.RowVersion, request.Body.Reason, requestUUID(requestID))
	if err != nil {
		switch classify(err) {
		case errorInvalid:
			return api.RevokePermissionGrant400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		case errorForbidden:
			return api.RevokePermissionGrant403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
		case errorNotFound:
			return api.RevokePermissionGrant404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
		case errorConflict:
			return api.RevokePermissionGrant409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
		default:
			return nil, err
		}
	}
	return api.RevokePermissionGrant200JSONResponse(api.PermissionGrantResponse{Grant: apiGrant(value), RequestId: requestID}), nil
}

func (h *Handler) EvaluatePermission(ctx context.Context, request api.EvaluatePermissionRequestObject) (api.EvaluatePermissionResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.EvaluatePermission401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if request.Body == nil {
		return api.EvaluatePermission400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
	}
	result, err := h.service.EvaluatePermission(ctx, actor, string(request.Body.ResourceType), uuid.UUID(request.Body.ResourceId), string(request.Body.Action), request.Body.PrivilegedReason, boolValue(request.Body.PrivilegedAccessConfirmed))
	if err != nil {
		if classify(err) == errorInvalid {
			return api.EvaluatePermission400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		}
		return nil, err
	}
	return api.EvaluatePermission200JSONResponse(api.PermissionEvaluationResponse{Result: apiEvaluation(result), RequestId: requestID}), nil
}

func (h *Handler) BatchEvaluatePermissions(ctx context.Context, request api.BatchEvaluatePermissionsRequestObject) (api.BatchEvaluatePermissionsResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.BatchEvaluatePermissions401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if request.Body == nil {
		return api.BatchEvaluatePermissions400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
	}
	inputs := make([]application.PermissionCheckInput, 0, len(request.Body.Items))
	for _, item := range request.Body.Items {
		inputs = append(inputs, application.PermissionCheckInput{ResourceType: string(item.ResourceType), ResourceID: uuid.UUID(item.ResourceId), Action: string(item.Action), PrivilegedReason: item.PrivilegedReason, PrivilegedAccessConfirmed: boolValue(item.PrivilegedAccessConfirmed)})
	}
	values, err := h.service.BatchEvaluatePermissions(ctx, actor, inputs)
	if err != nil {
		if classify(err) == errorInvalid {
			return api.BatchEvaluatePermissions400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		}
		return nil, err
	}
	items := make([]api.PermissionEvaluationResult, 0, len(values))
	for _, value := range values {
		items = append(items, apiEvaluation(value))
	}
	return api.BatchEvaluatePermissions200JSONResponse(api.BatchPermissionEvaluationResponse{Items: items, RequestId: requestID}), nil
}

func (h *Handler) BreakPermissionInheritance(ctx context.Context, request api.BreakPermissionInheritanceRequestObject) (api.BreakPermissionInheritanceResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.BreakPermissionInheritance401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.BreakPermissionInheritance403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.BreakPermissionInheritance400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
	}
	value, err := h.service.ChangeInheritance(ctx, actor, string(request.ResourceType), uuid.UUID(request.ResourceId), domain.InheritanceBreak, request.Body.RowVersion, requestUUID(requestID))
	if err != nil {
		switch classify(err) {
		case errorInvalid:
			return api.BreakPermissionInheritance400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		case errorForbidden:
			return api.BreakPermissionInheritance403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
		case errorNotFound:
			return api.BreakPermissionInheritance404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
		case errorConflict:
			return api.BreakPermissionInheritance409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
		default:
			return nil, err
		}
	}
	return api.BreakPermissionInheritance200JSONResponse(apiInheritance(value, requestID)), nil
}

func (h *Handler) RestorePermissionInheritance(ctx context.Context, request api.RestorePermissionInheritanceRequestObject) (api.RestorePermissionInheritanceResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.RestorePermissionInheritance401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RestorePermissionInheritance403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.RestorePermissionInheritance400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
	}
	value, err := h.service.ChangeInheritance(ctx, actor, string(request.ResourceType), uuid.UUID(request.ResourceId), domain.InheritanceInherit, request.Body.RowVersion, requestUUID(requestID))
	if err != nil {
		switch classify(err) {
		case errorInvalid:
			return api.RestorePermissionInheritance400JSONResponse{InvalidRequestJSONResponse: invalid(requestID)}, nil
		case errorForbidden:
			return api.RestorePermissionInheritance403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
		case errorNotFound:
			return api.RestorePermissionInheritance404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
		case errorConflict:
			return api.RestorePermissionInheritance409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
		default:
			return nil, err
		}
	}
	return api.RestorePermissionInheritance200JSONResponse(apiInheritance(value, requestID)), nil
}

func apiActions(values []api.PermissionAction) []string {
	items := make([]string, 0, len(values))
	for _, item := range values {
		items = append(items, string(item))
	}
	return items
}

func boolValue(value *bool) bool { return value != nil && *value }
func apiInheritance(value domain.InheritanceResult, requestID string) api.PermissionInheritanceResponse {
	return api.PermissionInheritanceResponse{ResourceType: api.PermissionResourceType(value.ResourceType), ResourceId: value.ResourceID, InheritanceMode: api.PermissionInheritanceResponseInheritanceMode(value.Mode), AclVersion: value.ACLVersion, RowVersion: value.RowVersion, RequestId: requestID}
}
