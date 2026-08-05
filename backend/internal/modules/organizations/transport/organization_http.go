package transport

import (
	"context"
	"errors"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/modules/organizations/application"
	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

func (h *Handler) ListCurrentUserOrganizations(ctx context.Context, request api.ListCurrentUserOrganizationsRequestObject) (api.ListCurrentUserOrganizationsResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ListCurrentUserOrganizations401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	result, err := h.service.ListCurrentUserMemberships(ginContext.Request.Context(), actor, page, pageSize)
	if errors.Is(err, domain.ErrInvalidInput) {
		return api.ListCurrentUserOrganizations400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]api.OrganizationMembership, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, apiMembership(item))
	}
	return api.ListCurrentUserOrganizations200JSONResponse(api.OrganizationMembershipListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) GetCurrentUserPersonalSpace(ctx context.Context, _ api.GetCurrentUserPersonalSpaceRequestObject) (api.GetCurrentUserPersonalSpaceResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.GetCurrentUserPersonalSpace401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	space, err := h.service.GetCurrentUserPersonalSpace(ginContext.Request.Context(), actor)
	if errors.Is(err, domain.ErrSpaceNotFound) {
		return api.GetCurrentUserPersonalSpace404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, nil
	}
	if err != nil {
		return nil, err
	}
	body, err := apiSpace(space)
	if err != nil {
		return nil, err
	}
	return api.GetCurrentUserPersonalSpace200JSONResponse(api.SpaceResponse{Space: body, RequestId: requestID}), nil
}

func (h *Handler) ListOrganizations(ctx context.Context, request api.ListOrganizationsRequestObject) (api.ListOrganizationsResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ListOrganizations401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	filter := domain.OrganizationListFilter{Page: page, PageSize: pageSize}
	if request.Params.ParentOrganizationId != nil {
		id := uuid.UUID(*request.Params.ParentOrganizationId)
		filter.ParentOrganizationID = &id
	}
	if request.Params.Status != nil {
		value := string(*request.Params.Status)
		filter.Status = &value
	}
	result, err := h.service.ListOrganizations(ginContext.Request.Context(), actor, filter)
	switch {
	case errors.Is(err, domain.ErrForbidden):
		return api.ListOrganizations403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case errors.Is(err, domain.ErrInvalidInput):
		return api.ListOrganizations400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	case err != nil:
		return nil, err
	}
	items := make([]api.Organization, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, apiOrganization(item))
	}
	return api.ListOrganizations200JSONResponse(api.OrganizationListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) CreateOrganization(ctx context.Context, request api.CreateOrganizationRequestObject) (api.CreateOrganizationResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.CreateOrganization401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CreateOrganization403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.CreateOrganization400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	var parentID *uuid.UUID
	if request.Body.ParentOrganizationId != nil {
		id := uuid.UUID(*request.Body.ParentOrganizationId)
		parentID = &id
	}
	sortOrder := int32(0)
	if request.Body.SortOrder != nil {
		sortOrder = int32(*request.Body.SortOrder)
	}
	result, err := h.service.CreateOrganization(ginContext.Request.Context(), actor, application.CreateOrganizationInput{ParentOrganizationID: parentID, Name: request.Body.Name, Code: request.Body.Code, TypeLabel: request.Body.TypeLabel, SortOrder: sortOrder, SpaceQuotaBytes: request.Body.SpaceQuotaBytes, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: databaseRequestID(requestID)})
	switch {
	case errors.Is(err, domain.ErrForbidden):
		return api.CreateOrganization403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case errors.Is(err, domain.ErrInvalidInput):
		return api.CreateOrganization400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	case isNotFound(err):
		return api.CreateOrganization404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, nil
	case isConflict(err):
		return api.CreateOrganization409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
	case err != nil:
		return nil, err
	}
	return api.CreateOrganization201JSONResponse(api.OrganizationResponse{Organization: apiOrganization(result), RequestId: requestID}), nil
}

func (h *Handler) GetOrganization(ctx context.Context, request api.GetOrganizationRequestObject) (api.GetOrganizationResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetOrganization401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	result, err := h.service.GetOrganization(ginContext.Request.Context(), actor, uuid.UUID(request.OrganizationId))
	switch {
	case errors.Is(err, domain.ErrForbidden):
		return api.GetOrganization403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case isNotFound(err):
		return api.GetOrganization404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, nil
	case err != nil:
		return nil, err
	}
	return api.GetOrganization200JSONResponse(api.OrganizationResponse{Organization: apiOrganization(result), RequestId: requestID}), nil
}

func (h *Handler) UpdateOrganization(ctx context.Context, request api.UpdateOrganizationRequestObject) (api.UpdateOrganizationResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.UpdateOrganization401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.UpdateOrganization403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.UpdateOrganization400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	changes := domain.OrganizationChanges{Name: request.Body.Name, Code: request.Body.Code, TypeLabel: request.Body.TypeLabel, RowVersion: request.Body.RowVersion}
	if request.Body.SortOrder != nil {
		value := int32(*request.Body.SortOrder)
		changes.SortOrder = &value
	}
	result, err := h.service.UpdateOrganization(ginContext.Request.Context(), actor, uuid.UUID(request.OrganizationId), changes, databaseRequestID(requestID))
	if response, handled := mapUpdateOrganizationError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateOrganization200JSONResponse(api.OrganizationResponse{Organization: apiOrganization(result), RequestId: requestID}), nil
}

func (h *Handler) MoveOrganization(ctx context.Context, request api.MoveOrganizationRequestObject) (api.MoveOrganizationResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.MoveOrganization401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.MoveOrganization403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.MoveOrganization400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	var parentID *uuid.UUID
	if request.Body.NewParentOrganizationId != nil {
		id := uuid.UUID(*request.Body.NewParentOrganizationId)
		parentID = &id
	}
	reason := ""
	if request.Body.Reason != nil {
		reason = *request.Body.Reason
	}
	result, err := h.service.MoveOrganization(ginContext.Request.Context(), actor, uuid.UUID(request.OrganizationId), application.MoveOrganizationInput{NewParentOrganizationID: parentID, RowVersion: request.Body.RowVersion, Reason: reason, RequestID: databaseRequestID(requestID)})
	if response, handled := mapMoveOrganizationError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	return api.MoveOrganization200JSONResponse(api.OrganizationResponse{Organization: apiOrganization(result), RequestId: requestID}), nil
}

func (h *Handler) ChangeOrganizationStatus(ctx context.Context, request api.ChangeOrganizationStatusRequestObject) (api.ChangeOrganizationStatusResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ChangeOrganizationStatus401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.ChangeOrganizationStatus403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.ChangeOrganizationStatus400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.ChangeOrganizationStatus(ginContext.Request.Context(), actor, uuid.UUID(request.OrganizationId), application.ChangeOrganizationStatusInput{Status: string(request.Body.Status), RowVersion: request.Body.RowVersion, Reason: request.Body.Reason, RequestID: databaseRequestID(requestID)})
	if response, handled := mapChangeOrganizationStatusError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	return api.ChangeOrganizationStatus200JSONResponse(api.OrganizationResponse{Organization: apiOrganization(result), RequestId: requestID}), nil
}

func (h *Handler) ListOrganizationMembers(ctx context.Context, request api.ListOrganizationMembersRequestObject) (api.ListOrganizationMembersResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ListOrganizationMembers401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	filter := domain.MembershipListFilter{Page: page, PageSize: pageSize}
	if request.Params.Status != nil {
		value := string(*request.Params.Status)
		filter.Status = &value
	}
	result, err := h.service.ListOrganizationMemberships(ginContext.Request.Context(), actor, uuid.UUID(request.OrganizationId), filter)
	switch {
	case errors.Is(err, domain.ErrForbidden):
		return api.ListOrganizationMembers403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, nil
	case errors.Is(err, domain.ErrInvalidInput):
		return api.ListOrganizationMembers400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	case isNotFound(err):
		return api.ListOrganizationMembers404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, nil
	case err != nil:
		return nil, err
	}
	items := make([]api.OrganizationMembership, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, apiMembership(item))
	}
	return api.ListOrganizationMembers200JSONResponse(api.OrganizationMembershipListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) AddOrganizationMember(ctx context.Context, request api.AddOrganizationMemberRequestObject) (api.AddOrganizationMemberResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.AddOrganizationMember401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.AddOrganizationMember403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.AddOrganizationMember400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.AddOrganizationMembership(ginContext.Request.Context(), actor, uuid.UUID(request.OrganizationId), application.AddMembershipInput{UserID: uuid.UUID(request.Body.UserId), MembershipType: string(request.Body.MembershipType), JobTitle: request.Body.JobTitle, EffectiveFrom: request.Body.EffectiveFrom, EffectiveUntil: request.Body.EffectiveUntil, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: databaseRequestID(requestID)})
	if response, handled := mapAddMemberError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	return api.AddOrganizationMember201JSONResponse(api.OrganizationMembershipResponse{Membership: apiMembership(result), RequestId: requestID}), nil
}

func (h *Handler) UpdateOrganizationMember(ctx context.Context, request api.UpdateOrganizationMemberRequestObject) (api.UpdateOrganizationMemberResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.UpdateOrganizationMember401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.UpdateOrganizationMember403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.UpdateOrganizationMember400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	changes := domain.MembershipChanges{JobTitle: request.Body.JobTitle, EffectiveUntil: request.Body.EffectiveUntil, RowVersion: request.Body.RowVersion}
	if request.Body.MembershipType != nil {
		value := string(*request.Body.MembershipType)
		changes.MembershipType = &value
	}
	if request.Body.Status != nil {
		value := string(*request.Body.Status)
		changes.Status = &value
	}
	result, err := h.service.UpdateOrganizationMembership(ginContext.Request.Context(), actor, uuid.UUID(request.OrganizationId), uuid.UUID(request.MembershipId), changes, databaseRequestID(requestID))
	if response, handled := mapUpdateMemberError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateOrganizationMember200JSONResponse(api.OrganizationMembershipResponse{Membership: apiMembership(result), RequestId: requestID}), nil
}

func (h *Handler) RemoveOrganizationMember(ctx context.Context, request api.RemoveOrganizationMemberRequestObject) (api.RemoveOrganizationMemberResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.RemoveOrganizationMember401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RemoveOrganizationMember403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.RemoveOrganizationMember400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	err := h.service.RemoveOrganizationMembership(ginContext.Request.Context(), actor, uuid.UUID(request.OrganizationId), uuid.UUID(request.MembershipId), application.RemoveMembershipInput{RowVersion: request.Body.RowVersion, Reason: request.Body.Reason, RequestID: databaseRequestID(requestID)})
	if response, handled := mapRemoveMemberError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	return api.RemoveOrganizationMember204Response{}, nil
}

func mapUpdateOrganizationError(err error, requestID string) (api.UpdateOrganizationResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrInvalidInput):
		return api.UpdateOrganization400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrForbidden):
		return api.UpdateOrganization403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, true
	case isNotFound(err):
		return api.UpdateOrganization404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, true
	case isConflict(err):
		return api.UpdateOrganization409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapMoveOrganizationError(err error, requestID string) (api.MoveOrganizationResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrInvalidInput):
		return api.MoveOrganization400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrForbidden):
		return api.MoveOrganization403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, true
	case isNotFound(err):
		return api.MoveOrganization404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, true
	case isConflict(err):
		return api.MoveOrganization409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapChangeOrganizationStatusError(err error, requestID string) (api.ChangeOrganizationStatusResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrInvalidInput):
		return api.ChangeOrganizationStatus400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrForbidden):
		return api.ChangeOrganizationStatus403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, true
	case isNotFound(err):
		return api.ChangeOrganizationStatus404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, true
	case isConflict(err):
		return api.ChangeOrganizationStatus409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapAddMemberError(err error, requestID string) (api.AddOrganizationMemberResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrInvalidInput):
		return api.AddOrganizationMember400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrForbidden):
		return api.AddOrganizationMember403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, true
	case isNotFound(err):
		return api.AddOrganizationMember404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, true
	case isConflict(err):
		return api.AddOrganizationMember409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapUpdateMemberError(err error, requestID string) (api.UpdateOrganizationMemberResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrInvalidInput):
		return api.UpdateOrganizationMember400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrForbidden):
		return api.UpdateOrganizationMember403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, true
	case isNotFound(err):
		return api.UpdateOrganizationMember404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, true
	case isConflict(err):
		return api.UpdateOrganizationMember409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapRemoveMemberError(err error, requestID string) (api.RemoveOrganizationMemberResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrInvalidInput):
		return api.RemoveOrganizationMember400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrForbidden):
		return api.RemoveOrganizationMember403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, false)}, true
	case isNotFound(err):
		return api.RemoveOrganizationMember404JSONResponse{NotFoundJSONResponse: notFound(requestID, err)}, true
	case isConflict(err):
		return api.RemoveOrganizationMember409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}
