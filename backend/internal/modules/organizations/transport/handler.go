package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"file-workshop/backend/api"
	identityapplication "file-workshop/backend/internal/modules/identity/application"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/modules/organizations/application"
	"file-workshop/backend/internal/modules/organizations/domain"
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

func (h *Handler) authenticate(ctx context.Context) (*gin.Context, application.Actor, string, error) {
	ginContext, ok := ctx.(*gin.Context)
	if !ok {
		return nil, application.Actor{}, "", fmt.Errorf("organization HTTP handler requires Gin context")
	}
	requestID := requestid.FromContext(ginContext.Request.Context())
	user, session, err := h.authenticator.CurrentSession(ginContext.Request.Context(), h.accessToken(ginContext))
	if err != nil {
		return ginContext, application.Actor{}, requestID, err
	}
	return ginContext, application.Actor{UserID: user.ID, SessionID: session.ID, Role: user.SystemRole}, requestID, nil
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

func notFound(requestID string, err error) api.NotFoundJSONResponse {
	code, message := "RESOURCE_NOT_FOUND", "指定资源不存在"
	switch {
	case errors.Is(err, domain.ErrOrganizationNotFound):
		code, message = "ORGANIZATION_NOT_FOUND", "组织不存在"
	case errors.Is(err, domain.ErrMembershipNotFound):
		code, message = "ORGANIZATION_MEMBERSHIP_NOT_FOUND", "组织成员关系不存在"
	case errors.Is(err, domain.ErrSpaceNotFound):
		code, message = "SPACE_NOT_FOUND", "空间不存在"
	case errors.Is(err, domain.ErrPlanNotFound):
		code, message = "ORGANIZATION_CHANGE_PLAN_NOT_FOUND", "组织重组计划不存在"
	}
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: code, Message: message, RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}

func conflict(requestID string, err error) api.ConflictJSONResponse {
	code, message := "ORGANIZATION_CONFLICT", "组织或空间数据发生冲突"
	switch {
	case errors.Is(err, domain.ErrVersionConflict):
		code, message = "ROW_VERSION_CONFLICT", "资源已被其他请求修改"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		code, message = "IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同请求"
	case errors.Is(err, domain.ErrTreeCycle):
		code, message = "ORGANIZATION_TREE_CYCLE", "组织移动会形成循环关系"
	case errors.Is(err, domain.ErrDeletionBlocked):
		code, message = "RESOURCE_DELETE_BLOCKED", "资源仍有关联事实，不能删除"
	case errors.Is(err, domain.ErrQuotaExceeded):
		code, message = "SPACE_QUOTA_EXCEEDED", "空间可用容量不足"
	case errors.Is(err, domain.ErrUnsupportedOperation):
		code, message = "ORGANIZATION_PLAN_OPERATION_DEFERRED", "该重组操作需要后续文件模块或后台任务执行"
	case errors.Is(err, domain.ErrInvalidStateTransition):
		code, message = "RESOURCE_STATE_CONFLICT", "当前资源状态不允许该操作"
	}
	return api.ConflictJSONResponse{Body: api.ErrorResponse{Code: code, Message: message, RequestId: requestID}, Headers: api.ConflictResponseHeaders{XRequestID: &requestID}}
}

func isNotFound(err error) bool {
	return errors.Is(err, domain.ErrOrganizationNotFound) || errors.Is(err, domain.ErrMembershipNotFound) || errors.Is(err, domain.ErrSpaceNotFound) || errors.Is(err, domain.ErrPlanNotFound)
}

func isConflict(err error) bool {
	return errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrVersionConflict) || errors.Is(err, domain.ErrIdempotencyConflict) || errors.Is(err, domain.ErrTreeCycle) || errors.Is(err, domain.ErrDeletionBlocked) || errors.Is(err, domain.ErrQuotaExceeded) || errors.Is(err, domain.ErrUnsupportedOperation) || errors.Is(err, domain.ErrInvalidStateTransition)
}

func databaseRequestID(value string) uuid.UUID {
	if parsed, err := uuid.Parse(value); err == nil {
		return parsed
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("file-workshop/request/"+value))
}

func jsonObject(value *map[string]interface{}) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage("{}"), nil
	}
	return json.Marshal(*value)
}

func apiOrganization(value domain.Organization) api.Organization {
	result := api.Organization{OrganizationId: openapi_types.UUID(value.ID), Name: value.Name, NormalizedName: value.NormalizedName, Code: value.Code, NormalizedCode: value.NormalizedCode, TypeLabel: value.TypeLabel, SortOrder: int(value.SortOrder), PathCache: value.PathCache, Depth: int(value.Depth), TreeVersion: value.TreeVersion, Status: api.OrganizationStatus(value.Status), CreatedByUserId: openapi_types.UUID(value.CreatedByUserID), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, DeletedAt: value.DeletedAt, RowVersion: value.RowVersion}
	if value.ParentOrganizationID != nil {
		parent := openapi_types.UUID(*value.ParentOrganizationID)
		result.ParentOrganizationId = &parent
	}
	return result
}

func apiMembership(value domain.Membership) api.OrganizationMembership {
	return api.OrganizationMembership{MembershipId: openapi_types.UUID(value.ID), UserId: openapi_types.UUID(value.UserID), OrganizationId: openapi_types.UUID(value.OrganizationID), MembershipType: api.OrganizationMembershipType(value.MembershipType), JobTitle: value.JobTitle, Status: api.OrganizationMembershipStatus(value.Status), EffectiveFrom: value.EffectiveFrom, EffectiveUntil: value.EffectiveUntil, CreatedByUserId: openapi_types.UUID(value.CreatedByUserID), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, RowVersion: value.RowVersion}
}

func apiSpace(value domain.Space) (api.Space, error) {
	config := map[string]interface{}{}
	if len(value.ConfigJSON) > 0 {
		if err := json.Unmarshal(value.ConfigJSON, &config); err != nil {
			return api.Space{}, fmt.Errorf("decode space config: %w", err)
		}
	}
	result := api.Space{SpaceId: openapi_types.UUID(value.ID), SpaceType: api.SpaceType(value.SpaceType), Name: value.Name, NormalizedName: value.NormalizedName, QuotaBytes: value.QuotaBytes, UsedBytes: value.UsedBytes, ReservedBytes: value.ReservedBytes, AclVersion: value.ACLVersion, SecurityEpoch: value.SecurityEpoch, ConfigSchemaVersion: int(value.ConfigSchemaVersion), Config: config, Status: api.SpaceStatus(value.Status), CreatedByUserId: openapi_types.UUID(value.CreatedByUserID), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, DeletedAt: value.DeletedAt, RowVersion: value.RowVersion}
	if value.OwnerUserID != nil {
		owner := openapi_types.UUID(*value.OwnerUserID)
		result.OwnerUserId = &owner
	}
	if value.OrganizationID != nil {
		organizationID := openapi_types.UUID(*value.OrganizationID)
		result.OrganizationId = &organizationID
	}
	if value.RootFolderID != nil {
		root := openapi_types.UUID(*value.RootFolderID)
		result.RootFolderId = &root
	}
	return result, nil
}

func apiPlan(value domain.OrganizationChangePlan) (api.OrganizationChangePlan, error) {
	operations := make([]api.OrganizationChangeOperation, 0, len(value.Operations))
	for _, operation := range value.Operations {
		object := map[string]interface{}{}
		if len(operation.OperationJSON) > 0 {
			if err := json.Unmarshal(operation.OperationJSON, &object); err != nil {
				return api.OrganizationChangePlan{}, fmt.Errorf("decode organization operation: %w", err)
			}
		}
		item := api.OrganizationChangeOperation{OperationId: openapi_types.UUID(operation.ID), PlanId: openapi_types.UUID(operation.PlanID), SequenceNumber: int(operation.SequenceNumber), OperationType: api.OrganizationChangeOperationType(operation.OperationType), OperationSchemaVersion: int(operation.OperationSchemaVersion), Operation: object, Status: api.OrganizationChangeOperationStatus(operation.Status), CreatedAt: operation.CreatedAt, UpdatedAt: operation.UpdatedAt, CompletedAt: operation.CompletedAt, FailureCode: operation.FailureCode, RowVersion: operation.RowVersion}
		if operation.SourceOrganizationID != nil {
			id := openapi_types.UUID(*operation.SourceOrganizationID)
			item.SourceOrganizationId = &id
		}
		if operation.TargetOrganizationID != nil {
			id := openapi_types.UUID(*operation.TargetOrganizationID)
			item.TargetOrganizationId = &id
		}
		operations = append(operations, item)
	}
	result := api.OrganizationChangePlan{PlanId: openapi_types.UUID(value.ID), PlanType: api.OrganizationChangePlanType(value.PlanType), Name: value.Name, Status: api.OrganizationChangePlanStatus(value.Status), ExpectedTreeVersion: value.ExpectedTreeVersion, CreatedByUserId: openapi_types.UUID(value.CreatedByUserID), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, ValidatedAt: value.ValidatedAt, ApprovedAt: value.ApprovedAt, StartedAt: value.StartedAt, CompletedAt: value.CompletedAt, FailureCode: value.FailureCode, RowVersion: value.RowVersion, Operations: operations}
	if value.ApprovedByUserID != nil {
		id := openapi_types.UUID(*value.ApprovedByUserID)
		result.ApprovedByUserId = &id
	}
	return result, nil
}
