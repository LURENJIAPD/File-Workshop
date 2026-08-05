package transport

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"file-workshop/backend/api"
	identityapplication "file-workshop/backend/internal/modules/identity/application"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/modules/permissions/application"
	"file-workshop/backend/internal/modules/permissions/domain"
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
		return nil, domain.Actor{}, "", fmt.Errorf("permission HTTP handler requires Gin context")
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
func pagination(page *api.PageQuery, pageSize *api.PageSizeQuery) (int, int) {
	p, s := domain.DefaultPage, domain.DefaultPageSize
	if page != nil {
		p = int(*page)
	}
	if pageSize != nil {
		s = int(*pageSize)
	}
	return p, s
}
func requestUUID(value string) uuid.UUID {
	if parsed, err := uuid.Parse(value); err == nil {
		return parsed
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("file-workshop/request/"+value))
}

type errorKind int

const (
	errorInternal errorKind = iota
	errorInvalid
	errorAuth
	errorForbidden
	errorNotFound
	errorConflict
)

func classify(err error) errorKind {
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &validation):
		return errorInvalid
	case errors.Is(err, identitydomain.ErrAuthentication):
		return errorAuth
	case errors.Is(err, domain.ErrForbidden):
		return errorForbidden
	case errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrDelegationNotFound) || errors.Is(err, domain.ErrGrantNotFound):
		return errorNotFound
	case errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrVersionConflict) || errors.Is(err, domain.ErrIdempotencyConflict) || errors.Is(err, domain.ErrInvalidDelegation):
		return errorConflict
	default:
		return errorInternal
	}
}
func invalid(requestID string) api.InvalidRequestJSONResponse {
	return api.InvalidRequestJSONResponse{Body: api.ErrorResponse{Code: "INVALID_REQUEST", Message: "请求参数无效", RequestId: requestID}, Headers: api.InvalidRequestResponseHeaders{XRequestID: &requestID}}
}
func authRequired(requestID string) api.AuthRequiredJSONResponse {
	return api.AuthRequiredJSONResponse{Body: api.ErrorResponse{Code: "AUTH_REQUIRED", Message: "认证信息无效或已过期", RequestId: requestID}, Headers: api.AuthRequiredResponseHeaders{XRequestID: &requestID}}
}
func forbidden(requestID string) api.ForbiddenJSONResponse {
	return api.ForbiddenJSONResponse{Body: api.ErrorResponse{Code: "AUTH_FORBIDDEN", Message: "没有执行该操作的权限", RequestId: requestID}, Headers: api.ForbiddenResponseHeaders{XRequestID: &requestID}}
}
func notFound(requestID string) api.NotFoundJSONResponse {
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: "RESOURCE_NOT_FOUND", Message: "指定资源不存在或不可见", RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}
func conflict(requestID string, err error) api.ConflictJSONResponse {
	code, message := "AUTHORIZATION_CONFLICT", "权限数据发生冲突"
	if errors.Is(err, domain.ErrVersionConflict) {
		code, message = "ROW_VERSION_CONFLICT", "资源已被其他请求修改"
	} else if errors.Is(err, domain.ErrIdempotencyConflict) {
		code, message = "IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同请求"
	} else if errors.Is(err, domain.ErrInvalidDelegation) {
		code, message = "ADMIN_DELEGATION_SCOPE_EXCEEDED", "管理委派超出授权者范围或能力"
	}
	return api.ConflictJSONResponse{Body: api.ErrorResponse{Code: code, Message: message, RequestId: requestID}, Headers: api.ConflictResponseHeaders{XRequestID: &requestID}}
}

func apiDelegation(value domain.AdminDelegation) api.AdminDelegation {
	capabilities := make([]api.AdminCapability, 0, len(value.Capabilities))
	for _, item := range value.Capabilities {
		capabilities = append(capabilities, api.AdminCapability(item))
	}
	result := api.AdminDelegation{DelegationId: openapi_types.UUID(value.ID), UserId: openapi_types.UUID(value.UserID), OrganizationId: openapi_types.UUID(value.OrganizationID), Scope: api.AdminDelegationScope(value.Scope), CanDelegate: value.CanDelegate, Capabilities: capabilities, GrantedByUserId: openapi_types.UUID(value.GrantedByUserID), ValidFrom: value.ValidFrom, ValidUntil: value.ValidUntil, Status: api.AdminDelegationStatus(value.Status), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, RevokedAt: value.RevokedAt, RevokeReason: value.RevokeReason, RowVersion: value.RowVersion}
	if value.ParentDelegationID != nil {
		id := openapi_types.UUID(*value.ParentDelegationID)
		result.ParentDelegationId = &id
	}
	return result
}
func apiDelegationList(value domain.AdminDelegationListResult, requestID string) api.AdminDelegationListResponse {
	items := make([]api.AdminDelegation, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, apiDelegation(item))
	}
	return api.AdminDelegationListResponse{Items: items, Page: api.Page(value.Page), PageSize: api.PageSize(value.PageSize), Total: value.Total, RequestId: requestID}
}
func apiGrant(value domain.PermissionGrant) api.PermissionGrant {
	subjectType, subjectID := value.Subject()
	resourceType, resourceID := value.Resource()
	actions := make([]api.PermissionAction, 0, len(value.Actions))
	for _, item := range value.Actions {
		actions = append(actions, api.PermissionAction(item))
	}
	result := api.PermissionGrant{GrantId: openapi_types.UUID(value.ID), SubjectType: api.PermissionSubjectType(subjectType), SubjectId: openapi_types.UUID(subjectID), ResourceType: api.PermissionResourceType(resourceType), ResourceId: openapi_types.UUID(resourceID), Actions: actions, InheritToDescendants: value.InheritToDescendants, GrantSource: api.PermissionGrantSource(value.GrantSource), ValidFrom: value.ValidFrom, ValidUntil: value.ValidUntil, Status: api.PermissionGrantStatus(value.Status), GrantedByUserId: openapi_types.UUID(value.GrantedByUserID), GrantReason: value.GrantReason, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, RevokedAt: value.RevokedAt, RevokeReason: value.RevokeReason, RowVersion: value.RowVersion}
	if value.RevokedByUserID != nil {
		id := openapi_types.UUID(*value.RevokedByUserID)
		result.RevokedByUserId = &id
	}
	return result
}
func apiGrantList(value domain.PermissionGrantListResult, requestID string) api.PermissionGrantListResponse {
	items := make([]api.PermissionGrant, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, apiGrant(item))
	}
	return api.PermissionGrantListResponse{Items: items, Page: api.Page(value.Page), PageSize: api.PageSize(value.PageSize), Total: value.Total, RequestId: requestID}
}
func apiEvaluation(value domain.PermissionEvaluation) api.PermissionEvaluationResult {
	ids := make([]openapi_types.UUID, 0, len(value.MatchedGrantIDs))
	for _, id := range value.MatchedGrantIDs {
		ids = append(ids, openapi_types.UUID(id))
	}
	return api.PermissionEvaluationResult{ResourceType: api.PermissionResourceType(value.ResourceType), ResourceId: openapi_types.UUID(value.ResourceID), Action: api.PermissionAction(value.Action), Allowed: value.Allowed, Source: api.PermissionEvaluationResultSource(value.Source), MatchedGrantIds: &ids, PrivilegedAccessRequired: value.PrivilegedAccessRequired}
}
