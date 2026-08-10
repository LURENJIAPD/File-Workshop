package transport

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"file-workshop/backend/api"
	identityapplication "file-workshop/backend/internal/modules/identity/application"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	lifecycleapplication "file-workshop/backend/internal/modules/lifecycle/application"
	"file-workshop/backend/internal/modules/lifecycle/domain"
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
	service        *lifecycleapplication.Service
	authenticator  SessionAuthenticator
	config         config.AuthConfig
	allowedOrigins map[string]struct{}
}

func NewHandler(service *lifecycleapplication.Service, authenticator *identityapplication.Service, cfg config.AuthConfig) *Handler {
	origins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		origins[origin] = struct{}{}
	}
	return &Handler{service: service, authenticator: authenticator, config: cfg, allowedOrigins: origins}
}

func (h *Handler) TrashDirectoryEntry(ctx context.Context, request api.TrashDirectoryEntryRequestObject) (api.TrashDirectoryEntryResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.TrashDirectoryEntry401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.TrashDirectoryEntry403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.TrashDirectoryEntry400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	item, err := h.service.TrashEntry(ginContext.Request.Context(), actor, uuid.UUID(request.EntryId), domain.TrashInput{
		Reason:         request.Body.Reason,
		RowVersion:     request.Body.RowVersion,
		IdempotencyKey: string(request.Params.IdempotencyKey),
		RequestID:      databaseRequestID(requestID),
	})
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.TrashDirectoryEntry400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.TrashDirectoryEntry403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.TrashDirectoryEntry404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.TrashDirectoryEntry409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.TrashDirectoryEntry201JSONResponse(api.RecycleItemResponse{Item: apiRecycleItem(item), RequestId: requestID}), nil
}

func (h *Handler) ListRecycleBinItems(ctx context.Context, request api.ListRecycleBinItemsRequestObject) (api.ListRecycleBinItemsResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ListRecycleBinItems401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	filter := domain.ListFilter{SpaceID: optionalAPIUUID(request.Params.SpaceId), Page: page, PageSize: pageSize}
	result, err := h.service.ListRecycleItems(ctx, actor, filter)
	if err != nil {
		if bad, denied, _, _, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.ListRecycleBinItems400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.ListRecycleBinItems403JSONResponse{ForbiddenJSONResponse: denied}, nil
			}
		}
		return nil, err
	}
	items := make([]api.RecycleItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, apiRecycleItem(item))
	}
	return api.ListRecycleBinItems200JSONResponse(api.RecycleItemListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) RestoreRecycleItem(ctx context.Context, request api.RestoreRecycleItemRequestObject) (api.RestoreRecycleItemResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.RestoreRecycleItem401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RestoreRecycleItem403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.RestoreRecycleItem400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	item, err := h.service.RestoreRecycleItem(ginContext.Request.Context(), actor, uuid.UUID(request.RecycleItemId), domain.RestoreInput{
		TargetParentFolderID: optionalAPIUUID(request.Body.TargetParentFolderId),
		Name:                 request.Body.Name,
		RowVersion:           request.Body.RowVersion,
		RequestID:            databaseRequestID(requestID),
	})
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.RestoreRecycleItem400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.RestoreRecycleItem403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.RestoreRecycleItem404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.RestoreRecycleItem409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.RestoreRecycleItem200JSONResponse(api.RecycleItemResponse{Item: apiRecycleItem(item), RequestId: requestID}), nil
}

func (h *Handler) PurgeRecycleItem(ctx context.Context, request api.PurgeRecycleItemRequestObject) (api.PurgeRecycleItemResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.PurgeRecycleItem401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.PurgeRecycleItem403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.PurgeRecycleItem400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	item, err := h.service.PurgeRecycleItem(ginContext.Request.Context(), actor, uuid.UUID(request.RecycleItemId), domain.PurgeInput{
		Reason:     request.Body.Reason,
		RowVersion: request.Body.RowVersion,
		RequestID:  databaseRequestID(requestID),
	})
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.PurgeRecycleItem400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.PurgeRecycleItem403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.PurgeRecycleItem404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.PurgeRecycleItem409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.PurgeRecycleItem200JSONResponse(api.RecycleItemResponse{Item: apiRecycleItem(item), RequestId: requestID}), nil
}

func (h *Handler) ScanExpiredRecycleItems(ctx context.Context, request api.ScanExpiredRecycleItemsRequestObject) (api.ScanExpiredRecycleItemsResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ScanExpiredRecycleItems401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.ScanExpiredRecycleItems403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	batchSize := 0
	if request.Body != nil && request.Body.BatchSize != nil {
		batchSize = *request.Body.BatchSize
	}
	result, err := h.service.ScanExpiredRecycleItems(ginContext.Request.Context(), actor, domain.ExpiredScanInput{BatchSize: batchSize, RequestID: databaseRequestID(requestID)})
	if err != nil {
		if bad, denied, _, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.ScanExpiredRecycleItems400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.ScanExpiredRecycleItems403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "409":
				return api.ScanExpiredRecycleItems409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.ScanExpiredRecycleItems200JSONResponse(api.ScanExpiredRecycleItemsResponse{
		Scanned:                 result.Scanned,
		Enqueued:                result.Enqueued,
		SkippedPreservationHold: result.SkippedPreservationHold,
		JobType:                 api.ScanExpiredRecycleItemsResponseJobType(result.JobType),
		RequestId:               requestID,
	}), nil
}

func (h *Handler) PlacePreservationHold(ctx context.Context, request api.PlacePreservationHoldRequestObject) (api.PlacePreservationHoldResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.PlacePreservationHold401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.PlacePreservationHold403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.PlacePreservationHold400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	hold, err := h.service.PlacePreservationHold(ginContext.Request.Context(), actor, uuid.UUID(request.DocumentId), domain.PlacePreservationHoldInput{
		DocumentVersionID: optionalAPIUUID(request.Body.DocumentVersionId),
		CaseReference:     request.Body.CaseReference,
		Reason:            request.Body.Reason,
		IdempotencyKey:    string(request.Params.IdempotencyKey),
		RequestID:         databaseRequestID(requestID),
	})
	if err != nil {
		if bad, denied, _, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.PlacePreservationHold400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.PlacePreservationHold403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.PlacePreservationHold404JSONResponse{NotFoundJSONResponse: preservationNotFound(requestID)}, nil
			case "409":
				return api.PlacePreservationHold409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.PlacePreservationHold201JSONResponse(api.PreservationHoldResponse{Hold: apiPreservationHold(hold), RequestId: requestID}), nil
}

func (h *Handler) ListPreservationHolds(ctx context.Context, request api.ListPreservationHoldsRequestObject) (api.ListPreservationHoldsResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ListPreservationHolds401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	filter := domain.PreservationHoldListFilter{DocumentID: optionalAPIUUID(request.Params.DocumentId), Page: page, PageSize: pageSize}
	if request.Params.Status != nil {
		status := string(*request.Params.Status)
		filter.Status = &status
	}
	if request.Params.CaseReference != nil {
		value := *request.Params.CaseReference
		filter.CaseReference = &value
	}
	result, err := h.service.ListPreservationHolds(ctx, actor, filter)
	if err != nil {
		if bad, denied, _, _, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.ListPreservationHolds400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.ListPreservationHolds403JSONResponse{ForbiddenJSONResponse: denied}, nil
			}
		}
		return nil, err
	}
	items := make([]api.PreservationHold, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, apiPreservationHold(item))
	}
	return api.ListPreservationHolds200JSONResponse(api.PreservationHoldListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) GetPreservationHold(ctx context.Context, request api.GetPreservationHoldRequestObject) (api.GetPreservationHoldResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.GetPreservationHold401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	hold, err := h.service.GetPreservationHold(ctx, actor, uuid.UUID(request.PreservationHoldId))
	if err != nil {
		if _, denied, _, _, code := mapError(err, requestID); code != "" {
			switch code {
			case "403":
				return api.GetPreservationHold403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.GetPreservationHold404JSONResponse{NotFoundJSONResponse: preservationNotFound(requestID)}, nil
			}
		}
		return nil, err
	}
	return api.GetPreservationHold200JSONResponse(api.PreservationHoldResponse{Hold: apiPreservationHold(hold), RequestId: requestID}), nil
}

func (h *Handler) ReleasePreservationHold(ctx context.Context, request api.ReleasePreservationHoldRequestObject) (api.ReleasePreservationHoldResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ReleasePreservationHold401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.ReleasePreservationHold403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.ReleasePreservationHold400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	hold, err := h.service.ReleasePreservationHold(ginContext.Request.Context(), actor, uuid.UUID(request.PreservationHoldId), domain.ReleasePreservationHoldInput{Reason: request.Body.Reason, RowVersion: request.Body.RowVersion, RequestID: databaseRequestID(requestID)})
	if err != nil {
		if bad, denied, _, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.ReleasePreservationHold400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.ReleasePreservationHold403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.ReleasePreservationHold404JSONResponse{NotFoundJSONResponse: preservationNotFound(requestID)}, nil
			case "409":
				return api.ReleasePreservationHold409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.ReleasePreservationHold200JSONResponse(api.PreservationHoldResponse{Hold: apiPreservationHold(hold), RequestId: requestID}), nil
}

func (h *Handler) authenticate(ctx context.Context) (*gin.Context, domain.Actor, string, error) {
	ginContext, ok := ctx.(*gin.Context)
	if !ok {
		return nil, domain.Actor{}, "", fmt.Errorf("lifecycle HTTP handler requires Gin context")
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
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: "RECYCLE_ITEM_NOT_FOUND", Message: "回收站项目不存在或不可见", RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}

func preservationNotFound(requestID string) api.NotFoundJSONResponse {
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: "PRESERVATION_HOLD_NOT_FOUND", Message: "资料或资料保全记录不存在", RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}

func conflict(requestID string, err error) api.ConflictJSONResponse {
	code, message := "RECYCLE_CONFLICT", "回收生命周期数据发生冲突"
	switch {
	case errors.Is(err, domain.ErrVersionConflict):
		code, message = "ROW_VERSION_CONFLICT", "资源已被其他请求修改"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		code, message = "IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同请求"
	case errors.Is(err, domain.ErrRootOperation):
		code, message = "RECYCLE_ROOT_OPERATION_FORBIDDEN", "根文件夹不允许执行该操作"
	case errors.Is(err, domain.ErrNameConflict):
		code, message = "RECYCLE_NAME_CONFLICT", "目标位置已存在同名文件或文件夹"
	case errors.Is(err, domain.ErrPreservationHoldActive):
		code, message = "PRESERVATION_HOLD_ACTIVE", "资源存在有效资料保全，不能永久清理"
	}
	return api.ConflictJSONResponse{Body: api.ErrorResponse{Code: code, Message: message, RequestId: requestID}, Headers: api.ConflictResponseHeaders{XRequestID: &requestID}}
}

func pagination(page *api.PageQuery, pageSize *api.PageSizeQuery) (int, int) {
	resultPage, resultPageSize := domain.DefaultPage, domain.DefaultPageSize
	if page != nil {
		resultPage = int(*page)
	}
	if pageSize != nil {
		resultPageSize = int(*pageSize)
	}
	return resultPage, resultPageSize
}

func databaseRequestID(value string) uuid.UUID {
	if parsed, err := uuid.Parse(value); err == nil {
		return parsed
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("file-workshop/request/"+value))
}

func mapError(err error, requestID string) (api.InvalidRequestJSONResponse, api.ForbiddenJSONResponse, api.NotFoundJSONResponse, api.ConflictJSONResponse, string) {
	var validationError *domain.ValidationError
	switch {
	case errors.As(err, &validationError):
		return invalidRequest(requestID), api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "400"
	case errors.Is(err, domain.ErrInvalidInput):
		return invalidRequest(requestID), api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "400"
	case errors.Is(err, domain.ErrForbidden):
		return api.InvalidRequestJSONResponse{}, forbidden(requestID, false), api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "403"
	case errors.Is(err, domain.ErrNotFound):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, notFound(requestID), api.ConflictJSONResponse{}, "404"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrVersionConflict), errors.Is(err, domain.ErrIdempotencyConflict), errors.Is(err, domain.ErrRootOperation), errors.Is(err, domain.ErrNameConflict), errors.Is(err, domain.ErrPreservationHoldActive):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, conflict(requestID, err), "409"
	default:
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, ""
	}
}

func apiRecycleItem(value domain.RecycleItem) api.RecycleItem {
	result := api.RecycleItem{
		RecycleItemId:   openapi_types.UUID(value.ID),
		EntryId:         openapi_types.UUID(value.EntryID),
		EntryType:       api.DirectoryEntryType(value.EntryType),
		OriginalSpaceId: openapi_types.UUID(value.OriginalSpaceID),
		OriginalName:    value.OriginalName,
		DeletedByUserId: openapi_types.UUID(value.DeletedByUserID),
		DeletedAt:       value.DeletedAt,
		ExpiresAt:       value.ExpiresAt,
		Status:          api.RecycleItemStatus(value.Status),
		CreatedAt:       value.CreatedAt,
		UpdatedAt:       value.UpdatedAt,
		RowVersion:      value.RowVersion,
	}
	if value.OriginalParentFolderID != nil {
		parent := openapi_types.UUID(*value.OriginalParentFolderID)
		result.OriginalParentFolderId = &parent
	}
	if value.CurrentName != "" {
		result.CurrentName = &value.CurrentName
	}
	if value.LifecycleStatus != "" {
		status := api.DirectoryLifecycleStatus(value.LifecycleStatus)
		result.LifecycleStatus = &status
	}
	if value.RestoredToFolderID != nil {
		folderID := openapi_types.UUID(*value.RestoredToFolderID)
		result.RestoredToFolderId = &folderID
	}
	result.RestoredAt = value.RestoredAt
	return result
}

func apiPreservationHold(value domain.PreservationHold) api.PreservationHold {
	result := api.PreservationHold{
		PreservationHoldId: openapi_types.UUID(value.ID),
		DocumentId:         openapi_types.UUID(value.DocumentID),
		CaseReference:      value.CaseReference,
		Reason:             value.Reason,
		Status:             api.PreservationHoldStatus(value.Status),
		PlacedByUserId:     openapi_types.UUID(value.PlacedByUserID),
		PlacedAt:           value.PlacedAt,
		CreatedAt:          value.CreatedAt,
		UpdatedAt:          value.UpdatedAt,
		RowVersion:         value.RowVersion,
	}
	if value.DocumentVersionID != nil {
		versionID := openapi_types.UUID(*value.DocumentVersionID)
		result.DocumentVersionId = &versionID
	}
	if value.ReleasedByUserID != nil {
		userID := openapi_types.UUID(*value.ReleasedByUserID)
		result.ReleasedByUserId = &userID
	}
	result.ReleasedAt = value.ReleasedAt
	result.ReleaseReason = value.ReleaseReason
	return result
}

func optionalAPIUUID(value *openapi_types.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	id := uuid.UUID(*value)
	return &id
}
