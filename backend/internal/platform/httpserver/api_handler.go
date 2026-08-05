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

type APIHandler struct {
	health   HealthAPI
	identity IdentityAPI
}

func NewAPIHandler(health HealthAPI, identity IdentityAPI) *APIHandler {
	return &APIHandler{health: health, identity: identity}
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
