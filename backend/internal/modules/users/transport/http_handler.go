package transport

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"file-workshop/backend/api"
	identityapplication "file-workshop/backend/internal/modules/identity/application"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/modules/users/application"
	"file-workshop/backend/internal/modules/users/domain"
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

func (h *Handler) GetCurrentUser(ctx context.Context, _ api.GetCurrentUserRequestObject) (api.GetCurrentUserResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.GetCurrentUser401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	user, err := h.service.GetCurrent(ginContext.Request.Context(), actor)
	if err != nil {
		return nil, err
	}
	return api.GetCurrentUser200JSONResponse{Body: userResponse(user, requestID), Headers: api.GetCurrentUser200ResponseHeaders{XRequestID: &requestID}}, nil
}

func (h *Handler) UpdateCurrentUser(ctx context.Context, request api.UpdateCurrentUserRequestObject) (api.UpdateCurrentUserResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.UpdateCurrentUser401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.UpdateCurrentUser403JSONResponse{SecurityRejectedJSONResponse: securityRejected(requestID)}, nil
	}
	if request.Body == nil {
		return api.UpdateCurrentUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	user, err := h.service.UpdateCurrent(ginContext.Request.Context(), actor, changesFromCurrentRequest(*request.Body), databaseRequestID(requestID))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidUserInput):
			return api.UpdateCurrentUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
		case isConflict(err):
			return api.UpdateCurrentUser409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, nil
		default:
			return nil, err
		}
	}
	return api.UpdateCurrentUser200JSONResponse{Body: userResponse(user, requestID), Headers: api.UpdateCurrentUser200ResponseHeaders{XRequestID: &requestID}}, nil
}

func (h *Handler) ListCurrentUserSessions(ctx context.Context, request api.ListCurrentUserSessionsRequestObject) (api.ListCurrentUserSessionsResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ListCurrentUserSessions401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	result, err := h.service.ListSessions(ginContext.Request.Context(), actor, page, pageSize)
	if errors.Is(err, domain.ErrInvalidUserInput) {
		return api.ListCurrentUserSessions400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]api.UserSession, 0, len(result.Items))
	for _, session := range result.Items {
		items = append(items, apiSession(session, actor.SessionID))
	}
	return api.ListCurrentUserSessions200JSONResponse{Body: api.UserSessionListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}, Headers: api.ListCurrentUserSessions200ResponseHeaders{XRequestID: &requestID}}, nil
}

func (h *Handler) RevokeCurrentUserSession(ctx context.Context, request api.RevokeCurrentUserSessionRequestObject) (api.RevokeCurrentUserSessionResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.RevokeCurrentUserSession401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RevokeCurrentUserSession403JSONResponse{SecurityRejectedJSONResponse: securityRejected(requestID)}, nil
	}
	err := h.service.RevokeSession(ginContext.Request.Context(), actor, uuid.UUID(request.SessionId), databaseRequestID(requestID))
	if errors.Is(err, domain.ErrUserNotFound) {
		return api.RevokeCurrentUserSession404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.RevokeCurrentUserSession204Response{Headers: api.RevokeCurrentUserSession204ResponseHeaders{XRequestID: &requestID}}, nil
}

func (h *Handler) ListUsers(ctx context.Context, request api.ListUsersRequestObject) (api.ListUsersResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ListUsers401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	filter := domain.ListFilter{Page: page, PageSize: pageSize}
	if request.Params.Status != nil {
		value := string(*request.Params.Status)
		filter.Status = &value
	}
	if request.Params.SystemRole != nil {
		value := string(*request.Params.SystemRole)
		filter.SystemRole = &value
	}
	result, err := h.service.List(ginContext.Request.Context(), actor, filter)
	if errors.Is(err, domain.ErrForbidden) {
		return api.ListUsers403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if errors.Is(err, domain.ErrInvalidUserInput) {
		return api.ListUsers400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]api.User, 0, len(result.Items))
	for _, user := range result.Items {
		items = append(items, apiUser(user))
	}
	return api.ListUsers200JSONResponse{Body: api.UserListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}, Headers: api.ListUsers200ResponseHeaders{XRequestID: &requestID}}, nil
}

func (h *Handler) CreateUser(ctx context.Context, request api.CreateUserRequestObject) (api.CreateUserResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.CreateUser401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CreateUser403JSONResponse{ForbiddenJSONResponse: originForbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.CreateUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	input := createInput(*request.Body, string(request.Params.IdempotencyKey), databaseRequestID(requestID))
	user, err := h.service.Create(ginContext.Request.Context(), actor, input)
	if response, handled := mapCreateError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	location := "/api/v1/admin/users/" + user.ID.String()
	return api.CreateUser201JSONResponse{Body: userResponse(user, requestID), Headers: api.CreateUser201ResponseHeaders{Location: &location, XRequestID: &requestID}}, nil
}

func (h *Handler) GetUser(ctx context.Context, request api.GetUserRequestObject) (api.GetUserResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetUser401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	user, err := h.service.Get(ginContext.Request.Context(), actor, uuid.UUID(request.UserId))
	if errors.Is(err, domain.ErrForbidden) {
		return api.GetUser403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if errors.Is(err, domain.ErrUserNotFound) {
		return api.GetUser404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetUser200JSONResponse(userResponse(user, requestID)), nil
}

func (h *Handler) UpdateUser(ctx context.Context, request api.UpdateUserRequestObject) (api.UpdateUserResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.UpdateUser401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.UpdateUser403JSONResponse{ForbiddenJSONResponse: originForbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.UpdateUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	user, err := h.service.Update(ginContext.Request.Context(), actor, uuid.UUID(request.UserId), changesFromAdminRequest(*request.Body), databaseRequestID(requestID))
	if response, handled := mapUpdateError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateUser200JSONResponse(userResponse(user, requestID)), nil
}

func (h *Handler) DisableUser(ctx context.Context, request api.DisableUserRequestObject) (api.DisableUserResponseObject, error) {
	response, err := h.changeStatus(ctx, request.UserId, request.Body, domain.UserStatusDisabled, statusResponseDisable)
	if err != nil {
		return nil, err
	}
	return response.(api.DisableUserResponseObject), nil
}

func (h *Handler) EnableUser(ctx context.Context, request api.EnableUserRequestObject) (api.EnableUserResponseObject, error) {
	response, err := h.changeStatus(ctx, request.UserId, request.Body, domain.UserStatusActive, statusResponseEnable)
	if err != nil {
		return nil, err
	}
	return response.(api.EnableUserResponseObject), nil
}

func (h *Handler) LockUser(ctx context.Context, request api.LockUserRequestObject) (api.LockUserResponseObject, error) {
	response, err := h.changeStatus(ctx, request.UserId, request.Body, domain.UserStatusLocked, statusResponseLock)
	if err != nil {
		return nil, err
	}
	return response.(api.LockUserResponseObject), nil
}

func (h *Handler) DeleteUser(ctx context.Context, request api.DeleteUserRequestObject) (api.DeleteUserResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.DeleteUser401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.DeleteUser403JSONResponse{ForbiddenJSONResponse: originForbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.DeleteUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	_, err := h.service.ChangeStatus(ginContext.Request.Context(), actor, uuid.UUID(request.UserId), domain.UserStatusDeleted, application.ChangeStatusInput{RowVersion: request.Body.RowVersion, Reason: request.Body.Reason, RequestID: databaseRequestID(requestID)})
	if response, handled := mapDeleteError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	return api.DeleteUser204Response{}, nil
}

func (h *Handler) ResetUserPassword(ctx context.Context, request api.ResetUserPasswordRequestObject) (api.ResetUserPasswordResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ResetUserPassword401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.ResetUserPassword403JSONResponse{ForbiddenJSONResponse: originForbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.ResetUserPassword400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	err := h.service.ResetPassword(ginContext.Request.Context(), actor, uuid.UUID(request.UserId), request.Body.Password, request.Body.RowVersion, databaseRequestID(requestID))
	if response, handled := mapResetError(err, requestID); handled {
		return response, nil
	}
	if err != nil {
		return nil, err
	}
	return api.ResetUserPassword204Response{}, nil
}

type statusResponseKind int

const (
	statusResponseDisable statusResponseKind = iota
	statusResponseEnable
	statusResponseLock
)

func (h *Handler) changeStatus(ctx context.Context, userID openapi_types.UUID, body *api.UserStateChangeRequest, target string, kind statusResponseKind) (any, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return statusAuthResponse(kind, requestID), nil
	}
	if !h.originAllowed(ginContext) {
		return statusForbiddenResponse(kind, requestID, true), nil
	}
	if body == nil {
		return statusInvalidResponse(kind, requestID), nil
	}
	user, err := h.service.ChangeStatus(ginContext.Request.Context(), actor, uuid.UUID(userID), target, application.ChangeStatusInput{RowVersion: body.RowVersion, Reason: body.Reason, RequestID: databaseRequestID(requestID)})
	if err != nil {
		if errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrInvalidUserInput) || errors.Is(err, domain.ErrUserNotFound) || isConflict(err) {
			return statusErrorResponse(kind, requestID, err), nil
		}
		return nil, err
	}
	return statusSuccessResponse(kind, userResponse(user, requestID)), nil
}

func (h *Handler) authenticate(ctx context.Context) (*gin.Context, application.Actor, string, error) {
	ginContext, ok := ctx.(*gin.Context)
	if !ok {
		return nil, application.Actor{}, "", fmt.Errorf("user HTTP handler requires Gin context")
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

func createInput(body api.CreateUserRequest, key string, requestID uuid.UUID) application.CreateUserInput {
	input := application.CreateUserInput{Username: body.Username, Password: body.Password, EmployeeNo: body.EmployeeNo, DisplayName: body.DisplayName, Phone: body.Phone, Locale: body.Locale, Timezone: body.Timezone, IdempotencyKey: key, RequestID: requestID}
	if body.Email != nil {
		value := string(*body.Email)
		input.Email = &value
	}
	if body.SystemRole != nil {
		value := string(*body.SystemRole)
		input.SystemRole = &value
	}
	return input
}

func changesFromCurrentRequest(body api.UpdateCurrentUserRequest) domain.UserChanges {
	changes := domain.UserChanges{DisplayName: body.DisplayName, Phone: body.Phone, Locale: body.Locale, Timezone: body.Timezone, RowVersion: body.RowVersion}
	if body.Email != nil {
		value := string(*body.Email)
		changes.Email = &value
	}
	return changes
}

func changesFromAdminRequest(body api.UpdateUserRequest) domain.UserChanges {
	changes := changesFromCurrentRequest(api.UpdateCurrentUserRequest{DisplayName: body.DisplayName, Email: body.Email, Locale: body.Locale, Phone: body.Phone, RowVersion: body.RowVersion, Timezone: body.Timezone})
	changes.EmployeeNo = body.EmployeeNo
	if body.SystemRole != nil {
		value := string(*body.SystemRole)
		changes.SystemRole = &value
	}
	return changes
}

func userResponse(user domain.User, requestID string) api.UserResponse {
	return api.UserResponse{User: apiUser(user), RequestId: requestID}
}

func apiUser(user domain.User) api.User {
	result := api.User{UserId: openapi_types.UUID(user.ID), Username: user.Username, EmployeeNo: user.EmployeeNo, DisplayName: user.DisplayName, Phone: user.Phone, SystemRole: api.SystemRole(user.SystemRole), Status: api.UserStatus(user.Status), Locale: user.Locale, Timezone: user.Timezone, LastLoginAt: user.LastLoginAt, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt, DeletedAt: user.DeletedAt, RowVersion: user.RowVersion}
	if user.Email != nil {
		value := openapi_types.Email(*user.Email)
		result.Email = &value
	}
	return result
}

func apiSession(session domain.Session, currentID uuid.UUID) api.UserSession {
	result := api.UserSession{SessionId: openapi_types.UUID(session.ID), DeviceId: session.DeviceID, UserAgent: session.UserAgent, Status: api.UserSessionStatus(session.Status), IsCurrent: session.ID == currentID, CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt, LastSeenAt: session.LastSeenAt, RevokedAt: session.RevokedAt, RevokeReason: session.RevokeReason, RowVersion: session.RowVersion}
	if session.IPAddress != nil {
		value := session.IPAddress.String()
		result.IpAddress = &value
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

func securityRejected(requestID string) api.SecurityRejectedJSONResponse {
	return api.SecurityRejectedJSONResponse{Body: api.ErrorResponse{Code: "AUTH_ORIGIN_REJECTED", Message: "请求来源不被允许", RequestId: requestID}, Headers: api.SecurityRejectedResponseHeaders{XRequestID: &requestID}}
}

func forbidden(requestID string) api.ForbiddenJSONResponse {
	return api.ForbiddenJSONResponse{Body: api.ErrorResponse{Code: "AUTH_FORBIDDEN", Message: "没有执行该操作的权限", RequestId: requestID}, Headers: api.ForbiddenResponseHeaders{XRequestID: &requestID}}
}

func originForbidden(requestID string) api.ForbiddenJSONResponse {
	response := forbidden(requestID)
	response.Body.Code = "AUTH_ORIGIN_REJECTED"
	response.Body.Message = "请求来源不被允许"
	return response
}

func notFound(requestID string) api.NotFoundJSONResponse {
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: "USER_NOT_FOUND", Message: "用户或会话不存在", RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}

func conflict(requestID string, err error) api.ConflictJSONResponse {
	code, message := "USER_CONFLICT", "用户数据发生冲突"
	switch {
	case errors.Is(err, domain.ErrVersionConflict):
		code, message = "USER_VERSION_CONFLICT", "用户资料已被其他请求修改"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		code, message = "IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同请求"
	case errors.Is(err, domain.ErrInvalidState):
		code, message = "USER_STATE_CONFLICT", "当前用户状态不允许该操作"
	case errors.Is(err, domain.ErrLastSystemAdmin):
		code, message = "USER_LAST_SYSTEM_ADMIN", "必须至少保留一个活动系统管理员"
	case errors.Is(err, domain.ErrPasswordCredential):
		code, message = "USER_PASSWORD_CREDENTIAL_NOT_FOUND", "用户没有可重置的活动密码凭据"
	}
	return api.ConflictJSONResponse{Body: api.ErrorResponse{Code: code, Message: message, RequestId: requestID}, Headers: api.ConflictResponseHeaders{XRequestID: &requestID}}
}

func isConflict(err error) bool {
	return errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrVersionConflict) || errors.Is(err, domain.ErrInvalidState) || errors.Is(err, domain.ErrIdempotencyConflict) || errors.Is(err, domain.ErrLastSystemAdmin) || errors.Is(err, domain.ErrPasswordCredential)
}

func databaseRequestID(value string) uuid.UUID {
	if parsed, err := uuid.Parse(value); err == nil {
		return parsed
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("file-workshop/request/"+value))
}

func mapCreateError(err error, requestID string) (api.CreateUserResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrForbidden):
		return api.CreateUser403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, true
	case errors.Is(err, domain.ErrInvalidUserInput):
		return api.CreateUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case isConflict(err):
		return api.CreateUser409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapUpdateError(err error, requestID string) (api.UpdateUserResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrForbidden):
		return api.UpdateUser403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, true
	case errors.Is(err, domain.ErrInvalidUserInput):
		return api.UpdateUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrUserNotFound):
		return api.UpdateUser404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, true
	case isConflict(err):
		return api.UpdateUser409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapDeleteError(err error, requestID string) (api.DeleteUserResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrForbidden):
		return api.DeleteUser403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, true
	case errors.Is(err, domain.ErrInvalidUserInput):
		return api.DeleteUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrUserNotFound):
		return api.DeleteUser404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, true
	case isConflict(err):
		return api.DeleteUser409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func mapResetError(err error, requestID string) (api.ResetUserPasswordResponseObject, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, domain.ErrForbidden):
		return api.ResetUserPassword403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, true
	case errors.Is(err, domain.ErrInvalidUserInput):
		return api.ResetUserPassword400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, true
	case errors.Is(err, domain.ErrUserNotFound):
		return api.ResetUserPassword404JSONResponse{NotFoundJSONResponse: notFound(requestID)}, true
	case isConflict(err):
		return api.ResetUserPassword409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}, true
	default:
		return nil, false
	}
}

func statusAuthResponse(kind statusResponseKind, requestID string) any {
	switch kind {
	case statusResponseDisable:
		return api.DisableUser401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}
	case statusResponseEnable:
		return api.EnableUser401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}
	default:
		return api.LockUser401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}
	}
}

func statusForbiddenResponse(kind statusResponseKind, requestID string, origin bool) any {
	response := forbidden(requestID)
	if origin {
		response = originForbidden(requestID)
	}
	switch kind {
	case statusResponseDisable:
		return api.DisableUser403JSONResponse{ForbiddenJSONResponse: response}
	case statusResponseEnable:
		return api.EnableUser403JSONResponse{ForbiddenJSONResponse: response}
	default:
		return api.LockUser403JSONResponse{ForbiddenJSONResponse: response}
	}
}

func statusInvalidResponse(kind statusResponseKind, requestID string) any {
	switch kind {
	case statusResponseDisable:
		return api.DisableUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}
	case statusResponseEnable:
		return api.EnableUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}
	default:
		return api.LockUser400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}
	}
}

func statusErrorResponse(kind statusResponseKind, requestID string, err error) any {
	if errors.Is(err, domain.ErrForbidden) {
		return statusForbiddenResponse(kind, requestID, false)
	}
	if errors.Is(err, domain.ErrInvalidUserInput) {
		return statusInvalidResponse(kind, requestID)
	}
	if errors.Is(err, domain.ErrUserNotFound) {
		switch kind {
		case statusResponseDisable:
			return api.DisableUser404JSONResponse{NotFoundJSONResponse: notFound(requestID)}
		case statusResponseEnable:
			return api.EnableUser404JSONResponse{NotFoundJSONResponse: notFound(requestID)}
		default:
			return api.LockUser404JSONResponse{NotFoundJSONResponse: notFound(requestID)}
		}
	}
	switch kind {
	case statusResponseDisable:
		return api.DisableUser409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}
	case statusResponseEnable:
		return api.EnableUser409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}
	default:
		return api.LockUser409JSONResponse{ConflictJSONResponse: conflict(requestID, err)}
	}
}

func statusSuccessResponse(kind statusResponseKind, response api.UserResponse) any {
	switch kind {
	case statusResponseDisable:
		return api.DisableUser200JSONResponse(response)
	case statusResponseEnable:
		return api.EnableUser200JSONResponse(response)
	default:
		return api.LockUser200JSONResponse(response)
	}
}
