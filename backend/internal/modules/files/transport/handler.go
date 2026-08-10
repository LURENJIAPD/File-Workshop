package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/modules/files/application"
	"file-workshop/backend/internal/modules/files/domain"
	identityapplication "file-workshop/backend/internal/modules/identity/application"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
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
	service        *application.Service
	authenticator  SessionAuthenticator
	config         config.AuthConfig
	allowedOrigins map[string]struct{}
}

func NewHandler(service *application.Service, authenticator *identityapplication.Service, cfg config.AuthConfig) *Handler {
	origins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		origins[origin] = struct{}{}
	}
	return &Handler{service: service, authenticator: authenticator, config: cfg, allowedOrigins: origins}
}

func (h *Handler) authenticate(ctx context.Context) (*gin.Context, domain.Actor, string, error) {
	ginContext, ok := ctx.(*gin.Context)
	if !ok {
		return nil, domain.Actor{}, "", fmt.Errorf("file HTTP handler requires Gin context")
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
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: "DIRECTORY_ENTRY_NOT_FOUND", Message: "目录项不存在或不可见", RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}

func conflict(requestID string, err error) api.ConflictJSONResponse {
	code, message := "DIRECTORY_CONFLICT", "目录数据发生冲突"
	switch {
	case errors.Is(err, domain.ErrVersionConflict):
		code, message = "ROW_VERSION_CONFLICT", "资源已被其他请求修改"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		code, message = "IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同请求"
	case errors.Is(err, domain.ErrTreeCycle):
		code, message = "DIRECTORY_TREE_CYCLE", "文件夹移动会形成循环关系"
	case errors.Is(err, domain.ErrRootOperation):
		code, message = "DIRECTORY_ROOT_OPERATION_FORBIDDEN", "根文件夹不允许执行该操作"
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
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return invalidRequest(requestID), api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "400"
	case errors.Is(err, domain.ErrForbidden):
		return api.InvalidRequestJSONResponse{}, forbidden(requestID, false), api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "403"
	case errors.Is(err, domain.ErrEntryNotFound), errors.Is(err, domain.ErrSpaceNotFound):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, notFound(requestID), api.ConflictJSONResponse{}, "404"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrVersionConflict), errors.Is(err, domain.ErrIdempotencyConflict), errors.Is(err, domain.ErrTreeCycle), errors.Is(err, domain.ErrRootOperation):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, conflict(requestID, err), "409"
	default:
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, ""
	}
}

func apiEntry(value domain.NamespaceEntry) (api.DirectoryEntry, error) {
	result := api.DirectoryEntry{EntryId: openapi_types.UUID(value.ID), SpaceId: openapi_types.UUID(value.SpaceID), EntryType: api.DirectoryEntryType(value.EntryType), Name: value.Name, NormalizedName: value.NormalizedName, PathCache: value.PathCache, Depth: int(value.Depth), LifecycleStatus: api.DirectoryLifecycleStatus(value.LifecycleStatus), IsRoot: value.IsRoot, CreatedByUserId: openapi_types.UUID(value.CreatedByUserID), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, DeletedAt: value.DeletedAt, RowVersion: value.RowVersion}
	if value.ParentFolderID != nil {
		parent := openapi_types.UUID(*value.ParentFolderID)
		result.ParentFolderId = &parent
	}
	if value.InheritanceMode != nil {
		mode := api.DirectoryEntryInheritanceMode(*value.InheritanceMode)
		result.InheritanceMode = &mode
	}
	result.AclVersion = value.ACLVersion
	if value.OwnerUserID != nil {
		id := openapi_types.UUID(*value.OwnerUserID)
		result.OwnerUserId = &id
	}
	if value.CurrentVersionID != nil {
		id := openapi_types.UUID(*value.CurrentVersionID)
		result.CurrentVersionId = &id
	}
	if value.AvailabilityStatus != nil {
		status := api.DocumentAvailabilityStatus(*value.AvailabilityStatus)
		result.AvailabilityStatus = &status
	}
	result.ExtensionNormalized = value.ExtensionNormalized
	result.Classification = value.Classification
	if value.MetadataSchemaVersion != nil {
		version := int(*value.MetadataSchemaVersion)
		result.MetadataSchemaVersion = &version
	}
	if len(value.MetadataJSON) > 0 {
		metadata := map[string]interface{}{}
		if err := json.Unmarshal(value.MetadataJSON, &metadata); err != nil {
			return api.DirectoryEntry{}, err
		}
		result.Metadata = &metadata
	}
	return result, nil
}
