package transport

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"file-workshop/backend/api"
	identityapplication "file-workshop/backend/internal/modules/identity/application"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	versionapplication "file-workshop/backend/internal/modules/versions/application"
	"file-workshop/backend/internal/modules/versions/domain"
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
	service        *versionapplication.Service
	authenticator  SessionAuthenticator
	config         config.AuthConfig
	allowedOrigins map[string]struct{}
}

func NewHandler(service *versionapplication.Service, authenticator *identityapplication.Service, cfg config.AuthConfig) *Handler {
	origins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		origins[origin] = struct{}{}
	}
	return &Handler{service: service, authenticator: authenticator, config: cfg, allowedOrigins: origins}
}

func (h *Handler) ListDocumentVersions(ctx context.Context, request api.ListDocumentVersionsRequestObject) (api.ListDocumentVersionsResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ListDocumentVersions401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	result, err := h.service.ListVersions(ctx, actor, uuid.UUID(request.DocumentId), page, pageSize)
	if err != nil {
		if bad, denied, missing, _, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.ListDocumentVersions400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.ListDocumentVersions403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.ListDocumentVersions404JSONResponse{NotFoundJSONResponse: missing}, nil
			}
		}
		return nil, err
	}
	items := make([]api.DocumentVersion, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, apiVersion(item))
	}
	return api.ListDocumentVersions200JSONResponse(api.DocumentVersionListResponse{Items: items, Page: api.Page(result.Page), PageSize: api.PageSize(result.PageSize), Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) RestoreDocumentVersion(ctx context.Context, request api.RestoreDocumentVersionRequestObject) (api.RestoreDocumentVersionResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.RestoreDocumentVersion401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RestoreDocumentVersion403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.RestoreDocumentVersion400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	version, err := h.service.RestoreVersion(ginContext.Request.Context(), actor, uuid.UUID(request.DocumentId), uuid.UUID(request.DocumentVersionId), domain.RestoreVersionInput{RowVersion: request.Body.RowVersion, ChangeNote: request.Body.ChangeNote, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: databaseRequestID(requestID)})
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.RestoreDocumentVersion400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.RestoreDocumentVersion403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.RestoreDocumentVersion404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.RestoreDocumentVersion409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.RestoreDocumentVersion201JSONResponse(api.DocumentVersionResponse{Version: apiVersion(version), RequestId: requestID}), nil
}

func (h *Handler) GetDocumentLock(ctx context.Context, request api.GetDocumentLockRequestObject) (api.GetDocumentLockResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.GetDocumentLock401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	lock, err := h.service.GetLock(ctx, actor, uuid.UUID(request.DocumentId))
	if err != nil {
		if _, denied, missing, _, code := mapError(err, requestID); code != "" {
			switch code {
			case "403":
				return api.GetDocumentLock403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.GetDocumentLock404JSONResponse{NotFoundJSONResponse: missing}, nil
			}
		}
		return nil, err
	}
	return api.GetDocumentLock200JSONResponse(api.DocumentLockResponse{Lock: apiLockPointer(lock), RequestId: requestID}), nil
}

func (h *Handler) AcquireDocumentLock(ctx context.Context, request api.AcquireDocumentLockRequestObject) (api.AcquireDocumentLockResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.AcquireDocumentLock401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) || request.Body == nil {
		if request.Body == nil {
			return api.AcquireDocumentLock400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
		}
		return api.AcquireDocumentLock403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	result, err := h.service.AcquireLock(ginContext.Request.Context(), actor, uuid.UUID(request.DocumentId), domain.AcquireLockInput{Source: string(request.Body.Source), TTLSeconds: request.Body.TtlSeconds, RequestID: databaseRequestID(requestID)})
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.AcquireDocumentLock400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.AcquireDocumentLock403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.AcquireDocumentLock404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.AcquireDocumentLock409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.AcquireDocumentLock201JSONResponse(api.AcquireDocumentLockResponse{Lock: apiLock(result.Lock), LockToken: result.LockToken, RequestId: requestID}), nil
}

func (h *Handler) HeartbeatDocumentLock(ctx context.Context, request api.HeartbeatDocumentLockRequestObject) (api.HeartbeatDocumentLockResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.HeartbeatDocumentLock401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if request.Body == nil {
		return api.HeartbeatDocumentLock400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	lock, err := h.service.HeartbeatLock(ginContext.Request.Context(), actor, uuid.UUID(request.DocumentId), domain.HeartbeatLockInput{LockToken: request.Body.LockToken, RowVersion: request.Body.RowVersion, TTLSeconds: request.Body.TtlSeconds, RequestID: databaseRequestID(requestID)})
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.HeartbeatDocumentLock400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.HeartbeatDocumentLock403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.HeartbeatDocumentLock404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.HeartbeatDocumentLock409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.HeartbeatDocumentLock200JSONResponse(api.DocumentLockResponse{Lock: apiLockPointer(&lock), RequestId: requestID}), nil
}

func (h *Handler) ReleaseDocumentLock(ctx context.Context, request api.ReleaseDocumentLockRequestObject) (api.ReleaseDocumentLockResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ReleaseDocumentLock401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if request.Body == nil {
		return api.ReleaseDocumentLock400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	lock, err := h.service.ReleaseLock(ginContext.Request.Context(), actor, uuid.UUID(request.DocumentId), domain.ReleaseLockInput{LockToken: request.Body.LockToken, RowVersion: request.Body.RowVersion, Reason: request.Body.Reason, RequestID: databaseRequestID(requestID)})
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.ReleaseDocumentLock400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.ReleaseDocumentLock403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.ReleaseDocumentLock404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.ReleaseDocumentLock409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.ReleaseDocumentLock200JSONResponse(api.DocumentLockResponse{Lock: apiLockPointer(&lock), RequestId: requestID}), nil
}

func (h *Handler) ForceReleaseDocumentLock(ctx context.Context, request api.ForceReleaseDocumentLockRequestObject) (api.ForceReleaseDocumentLockResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.ForceReleaseDocumentLock401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) || request.Body == nil {
		if request.Body == nil {
			return api.ForceReleaseDocumentLock400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
		}
		return api.ForceReleaseDocumentLock403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	lock, err := h.service.ForceReleaseLock(ginContext.Request.Context(), actor, uuid.UUID(request.DocumentId), domain.ForceReleaseLockInput{RowVersion: request.Body.RowVersion, Reason: request.Body.Reason, RequestID: databaseRequestID(requestID)})
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.ForceReleaseDocumentLock400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.ForceReleaseDocumentLock403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.ForceReleaseDocumentLock404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.ForceReleaseDocumentLock409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.ForceReleaseDocumentLock200JSONResponse(api.DocumentLockResponse{Lock: apiLockPointer(&lock), RequestId: requestID}), nil
}

func (h *Handler) authenticate(ctx context.Context) (*gin.Context, domain.Actor, string, error) {
	ginContext, ok := ctx.(*gin.Context)
	if !ok {
		return nil, domain.Actor{}, "", fmt.Errorf("version HTTP handler requires Gin context")
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

func apiVersion(value domain.Version) api.DocumentVersion {
	result := api.DocumentVersion{DocumentVersionId: openapi_types.UUID(value.ID), DocumentId: openapi_types.UUID(value.DocumentID), VersionNumber: value.VersionNumber, StorageObjectId: openapi_types.UUID(value.StorageObjectID), SizeBytes: value.SizeBytes, Sha256Hex: hex.EncodeToString(value.SHA256), MimeType: value.MIMEType, ChangeNote: value.ChangeNote, SourceType: api.DocumentVersionSourceType(value.SourceType), CreatedByUserId: openapi_types.UUID(value.CreatedByUserID), CreatedAt: value.CreatedAt}
	if value.RestoredFromVersionID != nil {
		id := openapi_types.UUID(*value.RestoredFromVersionID)
		result.RestoredFromVersionId = &id
	}
	return result
}

func apiLockPointer(value *domain.Lock) *api.DocumentLock {
	if value == nil {
		return nil
	}
	result := apiLock(*value)
	return &result
}

func apiLock(value domain.Lock) api.DocumentLock {
	result := api.DocumentLock{DocumentLockId: openapi_types.UUID(value.ID), DocumentId: openapi_types.UUID(value.DocumentID), UserId: openapi_types.UUID(value.UserID), FencingToken: value.FencingToken, Source: api.DocumentLockSource(value.Source), Status: api.DocumentLockStatus(value.Status), AcquiredAt: value.AcquiredAt, HeartbeatAt: value.HeartbeatAt, ExpiresAt: value.ExpiresAt, ReleasedAt: value.ReleasedAt, ReleaseReason: value.ReleaseReason, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, RowVersion: value.RowVersion}
	if value.ReleasedByUserID != nil {
		id := openapi_types.UUID(*value.ReleasedByUserID)
		result.ReleasedByUserId = &id
	}
	return result
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
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: "DOCUMENT_VERSION_NOT_FOUND", Message: "文档、版本或锁不存在", RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}

func conflict(requestID string, err error) api.ConflictJSONResponse {
	code, message := "VERSION_CONFLICT", "版本或锁状态冲突"
	switch {
	case errors.Is(err, domain.ErrVersionConflict):
		code, message = "ROW_VERSION_CONFLICT", "资源已被其他请求修改"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		code, message = "IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同请求"
	case errors.Is(err, domain.ErrLocked):
		code, message = "FILE_LOCKED", "文件已被其他租约锁定"
	}
	return api.ConflictJSONResponse{Body: api.ErrorResponse{Code: code, Message: message, RequestId: requestID}, Headers: api.ConflictResponseHeaders{XRequestID: &requestID}}
}

func mapError(err error, requestID string) (api.InvalidRequestJSONResponse, api.ForbiddenJSONResponse, api.NotFoundJSONResponse, api.ConflictJSONResponse, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return invalidRequest(requestID), api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "400"
	case errors.Is(err, domain.ErrForbidden):
		return api.InvalidRequestJSONResponse{}, forbidden(requestID, false), api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "403"
	case errors.Is(err, domain.ErrNotFound):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, notFound(requestID), api.ConflictJSONResponse{}, "404"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrVersionConflict), errors.Is(err, domain.ErrIdempotencyConflict), errors.Is(err, domain.ErrLocked):
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
