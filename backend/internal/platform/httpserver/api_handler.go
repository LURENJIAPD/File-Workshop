package httpserver

import (
	"context"
	"fmt"

	"file-workshop/backend/api"
)

type HealthAPI interface {
	GetLiveness(context.Context, api.GetLivenessRequestObject) (api.GetLivenessResponseObject, error)
	GetReadiness(context.Context, api.GetReadinessRequestObject) (api.GetReadinessResponseObject, error)
}

type IdentityAPI interface {
	Login(context.Context, api.LoginRequestObject) (api.LoginResponseObject, error)
	Logout(context.Context, api.LogoutRequestObject) (api.LogoutResponseObject, error)
	RefreshSession(context.Context, api.RefreshSessionRequestObject) (api.RefreshSessionResponseObject, error)
	GetCurrentSession(context.Context, api.GetCurrentSessionRequestObject) (api.GetCurrentSessionResponseObject, error)
}

type UsersAPI interface {
	GetCurrentUser(context.Context, api.GetCurrentUserRequestObject) (api.GetCurrentUserResponseObject, error)
	UpdateCurrentUser(context.Context, api.UpdateCurrentUserRequestObject) (api.UpdateCurrentUserResponseObject, error)
	ListCurrentUserSessions(context.Context, api.ListCurrentUserSessionsRequestObject) (api.ListCurrentUserSessionsResponseObject, error)
	RevokeCurrentUserSession(context.Context, api.RevokeCurrentUserSessionRequestObject) (api.RevokeCurrentUserSessionResponseObject, error)
	ListUsers(context.Context, api.ListUsersRequestObject) (api.ListUsersResponseObject, error)
	CreateUser(context.Context, api.CreateUserRequestObject) (api.CreateUserResponseObject, error)
	GetUser(context.Context, api.GetUserRequestObject) (api.GetUserResponseObject, error)
	UpdateUser(context.Context, api.UpdateUserRequestObject) (api.UpdateUserResponseObject, error)
	DeleteUser(context.Context, api.DeleteUserRequestObject) (api.DeleteUserResponseObject, error)
	DisableUser(context.Context, api.DisableUserRequestObject) (api.DisableUserResponseObject, error)
	EnableUser(context.Context, api.EnableUserRequestObject) (api.EnableUserResponseObject, error)
	LockUser(context.Context, api.LockUserRequestObject) (api.LockUserResponseObject, error)
	ResetUserPassword(context.Context, api.ResetUserPasswordRequestObject) (api.ResetUserPasswordResponseObject, error)
}

type APIHandler struct {
	health   HealthAPI
	identity IdentityAPI
	users    UsersAPI
}

func NewAPIHandler(health HealthAPI, identity IdentityAPI, users UsersAPI) *APIHandler {
	return &APIHandler{health: health, identity: identity, users: users}
}

func (h *APIHandler) GetLiveness(ctx context.Context, request api.GetLivenessRequestObject) (api.GetLivenessResponseObject, error) {
	return h.health.GetLiveness(ctx, request)
}

func (h *APIHandler) GetReadiness(ctx context.Context, request api.GetReadinessRequestObject) (api.GetReadinessResponseObject, error) {
	return h.health.GetReadiness(ctx, request)
}

func (h *APIHandler) Login(ctx context.Context, request api.LoginRequestObject) (api.LoginResponseObject, error) {
	if h.identity == nil {
		return nil, fmt.Errorf("identity handler is not configured")
	}
	return h.identity.Login(ctx, request)
}

func (h *APIHandler) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	if h.identity == nil {
		return nil, fmt.Errorf("identity handler is not configured")
	}
	return h.identity.Logout(ctx, request)
}

func (h *APIHandler) RefreshSession(ctx context.Context, request api.RefreshSessionRequestObject) (api.RefreshSessionResponseObject, error) {
	if h.identity == nil {
		return nil, fmt.Errorf("identity handler is not configured")
	}
	return h.identity.RefreshSession(ctx, request)
}

func (h *APIHandler) GetCurrentSession(ctx context.Context, request api.GetCurrentSessionRequestObject) (api.GetCurrentSessionResponseObject, error) {
	if h.identity == nil {
		return nil, fmt.Errorf("identity handler is not configured")
	}
	return h.identity.GetCurrentSession(ctx, request)
}

func (h *APIHandler) GetCurrentUser(ctx context.Context, request api.GetCurrentUserRequestObject) (api.GetCurrentUserResponseObject, error) {
	return h.users.GetCurrentUser(ctx, request)
}

func (h *APIHandler) UpdateCurrentUser(ctx context.Context, request api.UpdateCurrentUserRequestObject) (api.UpdateCurrentUserResponseObject, error) {
	return h.users.UpdateCurrentUser(ctx, request)
}

func (h *APIHandler) ListCurrentUserSessions(ctx context.Context, request api.ListCurrentUserSessionsRequestObject) (api.ListCurrentUserSessionsResponseObject, error) {
	return h.users.ListCurrentUserSessions(ctx, request)
}

func (h *APIHandler) RevokeCurrentUserSession(ctx context.Context, request api.RevokeCurrentUserSessionRequestObject) (api.RevokeCurrentUserSessionResponseObject, error) {
	return h.users.RevokeCurrentUserSession(ctx, request)
}

func (h *APIHandler) ListUsers(ctx context.Context, request api.ListUsersRequestObject) (api.ListUsersResponseObject, error) {
	return h.users.ListUsers(ctx, request)
}

func (h *APIHandler) CreateUser(ctx context.Context, request api.CreateUserRequestObject) (api.CreateUserResponseObject, error) {
	return h.users.CreateUser(ctx, request)
}

func (h *APIHandler) GetUser(ctx context.Context, request api.GetUserRequestObject) (api.GetUserResponseObject, error) {
	return h.users.GetUser(ctx, request)
}

func (h *APIHandler) UpdateUser(ctx context.Context, request api.UpdateUserRequestObject) (api.UpdateUserResponseObject, error) {
	return h.users.UpdateUser(ctx, request)
}

func (h *APIHandler) DeleteUser(ctx context.Context, request api.DeleteUserRequestObject) (api.DeleteUserResponseObject, error) {
	return h.users.DeleteUser(ctx, request)
}

func (h *APIHandler) DisableUser(ctx context.Context, request api.DisableUserRequestObject) (api.DisableUserResponseObject, error) {
	return h.users.DisableUser(ctx, request)
}

func (h *APIHandler) EnableUser(ctx context.Context, request api.EnableUserRequestObject) (api.EnableUserResponseObject, error) {
	return h.users.EnableUser(ctx, request)
}

func (h *APIHandler) LockUser(ctx context.Context, request api.LockUserRequestObject) (api.LockUserResponseObject, error) {
	return h.users.LockUser(ctx, request)
}

func (h *APIHandler) ResetUserPassword(ctx context.Context, request api.ResetUserPasswordRequestObject) (api.ResetUserPasswordResponseObject, error) {
	return h.users.ResetUserPassword(ctx, request)
}
