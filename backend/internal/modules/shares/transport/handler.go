package transport

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"file-workshop/backend/api"
	identityapplication "file-workshop/backend/internal/modules/identity/application"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	shareapplication "file-workshop/backend/internal/modules/shares/application"
	"file-workshop/backend/internal/modules/shares/domain"
	"file-workshop/backend/internal/platform/config"
	"file-workshop/backend/internal/platform/requestid"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type SessionAuthenticator interface {
	CurrentSession(context.Context, string) (identitydomain.User, identitydomain.Session, error)
}

type Handler struct {
	service        *shareapplication.Service
	authenticator  SessionAuthenticator
	config         config.AuthConfig
	allowedOrigins map[string]struct{}
}

func NewHandler(service *shareapplication.Service, authenticator *identityapplication.Service, cfg config.AuthConfig) *Handler {
	origins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		origins[origin] = struct{}{}
	}
	return &Handler{service: service, authenticator: authenticator, config: cfg, allowedOrigins: origins}
}

func (h *Handler) CreateShare(ctx context.Context, request api.CreateShareRequestObject) (api.CreateShareResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.CreateShare401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CreateShare403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.CreateShare400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	input := createInput(*request.Body, string(request.Params.IdempotencyKey), databaseRequestID(requestID))
	result, err := h.service.CreateShare(ginContext.Request.Context(), actor, input)
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.CreateShare400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.CreateShare403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.CreateShare404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.CreateShare409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.CreateShare201JSONResponse(api.ShareResponse{Share: apiShare(result.Share), ShareToken: result.ShareToken, RequestId: requestID}), nil
}

func (h *Handler) ListCreatedShares(ctx context.Context, request api.ListCreatedSharesRequestObject) (api.ListCreatedSharesResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ListCreatedShares401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	result, err := h.service.ListCreated(ctx, actor, page, pageSize)
	if err != nil {
		if bad, _, _, _, code := mapError(err, requestID); code == "400" {
			return api.ListCreatedShares400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		}
		return nil, err
	}
	return api.ListCreatedShares200JSONResponse(apiList(result, requestID)), nil
}

func (h *Handler) ListReceivedShares(ctx context.Context, request api.ListReceivedSharesRequestObject) (api.ListReceivedSharesResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ListReceivedShares401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	result, err := h.service.ListReceived(ctx, actor, page, pageSize)
	if err != nil {
		if bad, _, _, _, code := mapError(err, requestID); code == "400" {
			return api.ListReceivedShares400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		}
		return nil, err
	}
	return api.ListReceivedShares200JSONResponse(apiList(result, requestID)), nil
}

func (h *Handler) GetShare(ctx context.Context, request api.GetShareRequestObject) (api.GetShareResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.GetShare401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	share, err := h.service.GetShare(ctx, actor, uuid.UUID(request.ShareId))
	if err != nil {
		if _, denied, missing, _, code := mapError(err, requestID); code != "" {
			switch code {
			case "403":
				return api.GetShare403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.GetShare404JSONResponse{NotFoundJSONResponse: missing}, nil
			}
		}
		return nil, err
	}
	return api.GetShare200JSONResponse(api.ShareResponse{Share: apiShare(share), RequestId: requestID}), nil
}

func (h *Handler) UpdateShare(ctx context.Context, request api.UpdateShareRequestObject) (api.UpdateShareResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.UpdateShare401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.UpdateShare403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.UpdateShare400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	input := updateInput(*request.Body, databaseRequestID(requestID))
	share, err := h.service.UpdateShare(ginContext.Request.Context(), actor, uuid.UUID(request.ShareId), input)
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.UpdateShare400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.UpdateShare403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.UpdateShare404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.UpdateShare409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.UpdateShare200JSONResponse(api.ShareResponse{Share: apiShare(share), RequestId: requestID}), nil
}

func (h *Handler) RevokeShare(ctx context.Context, request api.RevokeShareRequestObject) (api.RevokeShareResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.RevokeShare401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RevokeShare403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.RevokeShare400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	share, err := h.service.RevokeShare(ginContext.Request.Context(), actor, uuid.UUID(request.ShareId), domain.RevokeInput{Reason: request.Body.Reason, RowVersion: request.Body.RowVersion, RequestID: databaseRequestID(requestID)})
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.RevokeShare400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.RevokeShare403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.RevokeShare404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.RevokeShare409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.RevokeShare200JSONResponse(api.ShareResponse{Share: apiShare(share), RequestId: requestID}), nil
}

func (h *Handler) OpenShare(ctx context.Context, request api.OpenShareRequestObject) (api.OpenShareResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.OpenShare401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	var token *string
	if request.Body != nil {
		token = request.Body.ShareToken
	}
	share, err := h.service.OpenShare(ctx, actor, uuid.UUID(request.ShareId), domain.OpenInput{ShareToken: token, RequestID: databaseRequestID(requestID)})
	if err != nil {
		if _, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "403":
				return api.OpenShare403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.OpenShare404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.OpenShare409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	sourceType, sourceID := share.Source()
	actions := make([]api.ShareAction, 0, len(share.Actions))
	for _, action := range share.Actions {
		actions = append(actions, api.ShareAction(action))
	}
	return api.OpenShare200JSONResponse(api.OpenShareResponse{Share: apiShare(share), SourceType: api.ShareResourceType(sourceType), SourceId: openapi_types.UUID(sourceID), Actions: actions, RequestId: requestID}), nil
}

func (h *Handler) authenticate(ctx context.Context) (*gin.Context, domain.Actor, string, error) {
	ginContext, ok := ctx.(*gin.Context)
	if !ok {
		return nil, domain.Actor{}, "", fmt.Errorf("share HTTP handler requires Gin context")
	}
	requestID := requestid.FromContext(ginContext.Request.Context())
	user, session, err := h.authenticator.CurrentSession(ginContext.Request.Context(), h.accessToken(ginContext))
	if err != nil {
		return ginContext, domain.Actor{}, requestID, err
	}
	return ginContext, domain.Actor{UserID: user.ID, SessionID: session.ID, Role: user.SystemRole}, requestID, nil
}

func (h *Handler) accessToken(ctx *gin.Context) string {
	authorization := strings.Fields(strings.TrimSpace(ctx.GetHeader("Authorization")))
	if len(authorization) == 2 && strings.EqualFold(authorization[0], "Bearer") {
		return authorization[1]
	}
	token, _ := ctx.Cookie(h.config.AccessCookieName)
	return token
}

func (h *Handler) originAllowed(ctx *gin.Context) bool {
	origin := strings.TrimSpace(ctx.GetHeader("Origin"))
	if origin == "" {
		return true
	}
	_, ok := h.allowedOrigins[origin]
	return ok
}

func createInput(body api.CreateShareRequest, idempotencyKey string, requestID uuid.UUID) domain.CreateInput {
	actions := make([]string, 0, len(body.Actions))
	for _, action := range body.Actions {
		actions = append(actions, string(action))
	}
	return domain.CreateInput{SourceType: string(body.SourceType), SourceID: uuid.UUID(body.SourceId), TargetKind: string(body.TargetKind), TargetUserID: optionalAPIUUID(body.TargetUserId), TargetOrganizationID: optionalAPIUUID(body.TargetOrganizationId), AllowReshare: boolValue(body.AllowReshare), Actions: actions, ValidUntil: body.ValidUntil, Note: body.Note, IdempotencyKey: idempotencyKey, RequestID: requestID}
}

func updateInput(body api.UpdateShareRequest, requestID uuid.UUID) domain.UpdateInput {
	var actions *[]string
	if body.Actions != nil {
		values := make([]string, 0, len(*body.Actions))
		for _, action := range *body.Actions {
			values = append(values, string(action))
		}
		actions = &values
	}
	return domain.UpdateInput{Actions: actions, ValidUntil: body.ValidUntil, AllowReshare: body.AllowReshare, RowVersion: body.RowVersion, RequestID: requestID}
}

func apiList(value domain.ListResult, requestID string) api.ShareListResponse {
	items := make([]api.Share, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, apiShare(item))
	}
	return api.ShareListResponse{Items: items, Page: value.Page, PageSize: value.PageSize, Total: value.Total, RequestId: requestID}
}

func apiShare(value domain.Share) api.Share {
	sourceType, sourceID := value.Source()
	actions := make([]api.ShareAction, 0, len(value.Actions))
	for _, action := range value.Actions {
		actions = append(actions, api.ShareAction(action))
	}
	result := api.Share{ShareId: openapi_types.UUID(value.ID), SourceType: api.ShareResourceType(sourceType), SourceId: openapi_types.UUID(sourceID), CreatorUserId: openapi_types.UUID(value.CreatorUserID), TargetKind: api.ShareTargetKind(value.TargetKind), AllowReshare: value.AllowReshare, Actions: actions, ValidFrom: value.ValidFrom, ValidUntil: value.ValidUntil, Status: api.ShareStatus(value.Status), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, RevokedAt: value.RevokedAt, RevokeReason: value.RevokeReason, RowVersion: value.RowVersion}
	if value.TargetUserID != nil {
		id := openapi_types.UUID(*value.TargetUserID)
		result.TargetUserId = &id
	}
	if value.TargetOrganizationID != nil {
		id := openapi_types.UUID(*value.TargetOrganizationID)
		result.TargetOrganizationId = &id
	}
	if value.TargetSpaceID != nil {
		id := openapi_types.UUID(*value.TargetSpaceID)
		result.TargetSpaceId = &id
	}
	if value.RevokedByUserID != nil {
		id := openapi_types.UUID(*value.RevokedByUserID)
		result.RevokedByUserId = &id
	}
	return result
}

func optionalAPIUUID(value *openapi_types.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	converted := uuid.UUID(*value)
	return &converted
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func pagination(page *api.PageQuery, pageSize *api.PageSizeQuery) (int, int) {
	resultPage, resultSize := 0, 0
	if page != nil {
		resultPage = int(*page)
	}
	if pageSize != nil {
		resultSize = int(*pageSize)
	}
	return resultPage, resultSize
}

func invalidRequest(requestID string) api.InvalidRequestJSONResponse {
	return api.InvalidRequestJSONResponse{Body: api.ErrorResponse{Code: "INVALID_REQUEST", Message: "请求参数无效", RequestId: requestID}, Headers: api.InvalidRequestResponseHeaders{XRequestID: &requestID}}
}

func authRequired(requestID string) api.AuthRequiredJSONResponse {
	return api.AuthRequiredJSONResponse{Body: api.ErrorResponse{Code: "AUTH_REQUIRED", Message: "认证信息无效或已过期", RequestId: requestID}, Headers: api.AuthRequiredResponseHeaders{XRequestID: &requestID}}
}

func forbidden(requestID string, originRejected bool) api.ForbiddenJSONResponse {
	code, message := "AUTH_FORBIDDEN", "没有执行该操作的权限"
	if originRejected {
		code, message = "AUTH_ORIGIN_REJECTED", "请求来源不被允许"
	}
	return api.ForbiddenJSONResponse{Body: api.ErrorResponse{Code: code, Message: message, RequestId: requestID}, Headers: api.ForbiddenResponseHeaders{XRequestID: &requestID}}
}

func notFound(requestID string) api.NotFoundJSONResponse {
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: "SHARE_NOT_FOUND", Message: "共享不存在或不可见", RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}

func conflict(requestID string, err error) api.ConflictJSONResponse {
	code, message := "SHARE_CONFLICT", "共享状态冲突"
	switch {
	case errors.Is(err, domain.ErrVersionConflict):
		code, message = "ROW_VERSION_CONFLICT", "资源已被其他请求修改"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		code, message = "IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同请求"
	case errors.Is(err, domain.ErrShareTokenRequired), errors.Is(err, domain.ErrShareTokenInvalid):
		code, message = "SHARE_TOKEN_INVALID", "共享令牌缺失或无效"
	}
	return api.ConflictJSONResponse{Body: api.ErrorResponse{Code: code, Message: message, RequestId: requestID}, Headers: api.ConflictResponseHeaders{XRequestID: &requestID}}
}

func mapError(err error, requestID string) (api.InvalidRequestJSONResponse, api.ForbiddenJSONResponse, api.NotFoundJSONResponse, api.ConflictJSONResponse, string) {
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &validation), errors.Is(err, domain.ErrInvalidInput), errors.Is(err, domain.ErrTargetUnsupported):
		return invalidRequest(requestID), api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "400"
	case errors.Is(err, domain.ErrForbidden):
		return api.InvalidRequestJSONResponse{}, forbidden(requestID, false), api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "403"
	case errors.Is(err, domain.ErrNotFound):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, notFound(requestID), api.ConflictJSONResponse{}, "404"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrVersionConflict), errors.Is(err, domain.ErrIdempotencyConflict), errors.Is(err, domain.ErrShareTokenRequired), errors.Is(err, domain.ErrShareTokenInvalid):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, conflict(requestID, err), "409"
	default:
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, ""
	}
}

func databaseRequestID(value string) uuid.UUID {
	if parsed, err := uuid.Parse(value); err == nil {
		return parsed
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("file-workshop/request/"+value))
}
