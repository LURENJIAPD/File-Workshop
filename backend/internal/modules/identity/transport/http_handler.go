package transport

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/modules/identity/application"
	"file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/platform/config"
	"file-workshop/backend/internal/platform/requestid"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

const noStore = "no-store"

type Handler struct {
	service        *application.Service
	config         config.AuthConfig
	allowedOrigins map[string]struct{}
	sameSite       http.SameSite
}

func NewHandler(service *application.Service, cfg config.AuthConfig) *Handler {
	origins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		origins[origin] = struct{}{}
	}
	sameSite := http.SameSiteLaxMode
	switch cfg.CookieSameSite {
	case "strict":
		sameSite = http.SameSiteStrictMode
	case "none":
		sameSite = http.SameSiteNoneMode
	}
	return &Handler{service: service, config: cfg, allowedOrigins: origins, sameSite: sameSite}
}

func (h *Handler) Login(ctx context.Context, request api.LoginRequestObject) (api.LoginResponseObject, error) {
	ginContext, err := ginContext(ctx)
	if err != nil {
		return nil, err
	}
	requestID := requestid.FromContext(ginContext.Request.Context())
	if !h.originAllowed(ginContext) {
		return api.Login403JSONResponse{SecurityRejectedJSONResponse: securityRejected(requestID)}, nil
	}
	if request.Body == nil {
		return api.Login400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.Login(ginContext.Request.Context(), application.LoginInput{
		Username: request.Body.Username,
		Password: request.Body.Password,
		Metadata: domain.RequestMetadata{
			DeviceID:  request.Body.DeviceId,
			IPAddress: clientIP(ginContext),
			UserAgent: ginContext.Request.UserAgent(),
			RequestID: databaseRequestID(requestID),
		},
	})
	if err != nil {
		var locked *domain.AccountLockedError
		var limited *domain.RateLimitedError
		switch {
		case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrAccountUnavailable):
			return api.Login401JSONResponse{InvalidCredentialsJSONResponse: invalidCredentials(requestID)}, nil
		case errors.As(err, &locked):
			return api.Login423JSONResponse{AccountLockedJSONResponse: accountLocked(requestID, locked.RetryAfter)}, nil
		case errors.As(err, &limited):
			retryAfter := max(1, int(math.Ceil(limited.RetryAfter.Seconds())))
			response := tooManyRequests(requestID)
			response.Headers.RetryAfter = &retryAfter
			return api.Login429JSONResponse{TooManyRequestsJSONResponse: response}, nil
		default:
			return nil, err
		}
	}
	h.setAuthenticationCookies(ginContext, result)
	return api.Login200JSONResponse{
		Body: authenticationResponse(result, requestID),
		Headers: api.Login200ResponseHeaders{
			CacheControl: stringPointer(noStore),
			XRequestID:   &requestID,
		},
	}, nil
}

func (h *Handler) RefreshSession(ctx context.Context, _ api.RefreshSessionRequestObject) (api.RefreshSessionResponseObject, error) {
	ginContext, err := ginContext(ctx)
	if err != nil {
		return nil, err
	}
	requestID := requestid.FromContext(ginContext.Request.Context())
	if !h.originAllowed(ginContext) {
		return api.RefreshSession403JSONResponse{SecurityRejectedJSONResponse: securityRejected(requestID)}, nil
	}
	rawRefreshToken, _ := ginContext.Cookie(h.config.RefreshCookieName)
	result, err := h.service.Refresh(ginContext.Request.Context(), rawRefreshToken)
	if err != nil {
		if errors.Is(err, domain.ErrAuthentication) || errors.Is(err, domain.ErrTokenReused) {
			h.clearAuthenticationCookies(ginContext)
			return api.RefreshSession401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
		}
		return nil, err
	}
	h.setAuthenticationCookies(ginContext, result)
	return api.RefreshSession200JSONResponse{
		Body: authenticationResponse(result, requestID),
		Headers: api.RefreshSession200ResponseHeaders{
			CacheControl: stringPointer(noStore),
			XRequestID:   &requestID,
		},
	}, nil
}

func (h *Handler) Logout(ctx context.Context, _ api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	ginContext, err := ginContext(ctx)
	if err != nil {
		return nil, err
	}
	requestID := requestid.FromContext(ginContext.Request.Context())
	if !h.originAllowed(ginContext) {
		return api.Logout403JSONResponse{SecurityRejectedJSONResponse: securityRejected(requestID)}, nil
	}
	accessToken := h.accessToken(ginContext)
	refreshToken, _ := ginContext.Cookie(h.config.RefreshCookieName)
	if err := h.service.Logout(ginContext.Request.Context(), accessToken, refreshToken); err != nil {
		return nil, err
	}
	h.clearAuthenticationCookies(ginContext)
	return api.Logout204Response{Headers: api.Logout204ResponseHeaders{XRequestID: &requestID}}, nil
}

func (h *Handler) GetCurrentSession(ctx context.Context, _ api.GetCurrentSessionRequestObject) (api.GetCurrentSessionResponseObject, error) {
	ginContext, err := ginContext(ctx)
	if err != nil {
		return nil, err
	}
	requestID := requestid.FromContext(ginContext.Request.Context())
	user, session, err := h.service.CurrentSession(ginContext.Request.Context(), h.accessToken(ginContext))
	if err != nil {
		if errors.Is(err, domain.ErrAuthentication) {
			return api.GetCurrentSession401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
		}
		return nil, err
	}
	return api.GetCurrentSession200JSONResponse{
		Body: api.CurrentSessionResponse{
			RequestId: requestID,
			User:      apiUser(user),
			Session:   apiSession(session),
		},
		Headers: api.GetCurrentSession200ResponseHeaders{
			CacheControl: stringPointer(noStore),
			XRequestID:   &requestID,
		},
	}, nil
}

func (h *Handler) accessToken(c *gin.Context) string {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	parts := strings.Fields(authorization)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	token, _ := c.Cookie(h.config.AccessCookieName)
	return token
}

func (h *Handler) originAllowed(c *gin.Context) bool {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin == "" {
		return true
	}
	_, allowed := h.allowedOrigins[origin]
	return allowed
}

func (h *Handler) setAuthenticationCookies(c *gin.Context, result application.AuthenticationResult) {
	h.setCookie(c, h.config.AccessCookieName, result.AccessToken, "/", result.AccessExpiresAt)
	h.setCookie(c, h.config.RefreshCookieName, result.RefreshToken, "/api/v1/auth", result.Session.ExpiresAt)
}

func (h *Handler) clearAuthenticationCookies(c *gin.Context) {
	expired := time.Unix(1, 0).UTC()
	h.setCookie(c, h.config.AccessCookieName, "", "/", expired)
	h.setCookie(c, h.config.RefreshCookieName, "", "/api/v1/auth", expired)
}

func (h *Handler) setCookie(c *gin.Context, name, value, path string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if expiresAt.Before(time.Now()) {
		maxAge = -1
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Domain:   h.config.CookieDomain,
		Expires:  expiresAt,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: h.sameSite,
	})
}

func authenticationResponse(result application.AuthenticationResult, requestID string) api.AuthTokenResponse {
	return api.AuthTokenResponse{
		AccessToken: result.AccessToken,
		ExpiresIn:   max(0, int64(time.Until(result.AccessExpiresAt).Seconds())),
		RequestId:   requestID,
		Session:     apiSession(result.Session),
		TokenType:   api.Bearer,
		User:        apiUser(result.User),
	}
}

func apiUser(user domain.User) api.AuthenticatedUser {
	return api.AuthenticatedUser{
		UserId:      openapi_types.UUID(user.ID),
		Username:    user.Username,
		DisplayName: user.DisplayName,
		SystemRole:  api.AuthenticatedUserSystemRole(user.SystemRole),
		Locale:      user.Locale,
		Timezone:    user.Timezone,
	}
}

func apiSession(session domain.Session) api.SessionSummary {
	return api.SessionSummary{
		SessionId:  openapi_types.UUID(session.ID),
		DeviceId:   session.DeviceID,
		Status:     api.SessionSummaryStatus(session.Status),
		ExpiresAt:  session.ExpiresAt,
		LastSeenAt: session.LastSeenAt,
		CreatedAt:  session.CreatedAt,
	}
}

func invalidRequest(requestID string) api.InvalidRequestJSONResponse {
	return api.InvalidRequestJSONResponse{
		Body:    api.ErrorResponse{Code: "INVALID_REQUEST", Message: "请求参数无效", RequestId: requestID},
		Headers: api.InvalidRequestResponseHeaders{XRequestID: &requestID},
	}
}

func invalidCredentials(requestID string) api.InvalidCredentialsJSONResponse {
	return api.InvalidCredentialsJSONResponse{
		Body:    api.ErrorResponse{Code: "AUTH_INVALID_CREDENTIALS", Message: "用户名或密码错误", RequestId: requestID},
		Headers: api.InvalidCredentialsResponseHeaders{XRequestID: &requestID},
	}
}

func accountLocked(requestID string, retryAfter time.Duration) api.AccountLockedJSONResponse {
	details := map[string]any{"retryAfterSeconds": max(1, int(math.Ceil(retryAfter.Seconds())))}
	return api.AccountLockedJSONResponse{
		Body: api.ErrorResponse{
			Code:      "AUTH_ACCOUNT_LOCKED",
			Message:   "登录失败次数过多，账号暂时锁定",
			RequestId: requestID,
			Details:   &details,
		},
		Headers: api.AccountLockedResponseHeaders{XRequestID: &requestID},
	}
}

func tooManyRequests(requestID string) api.TooManyRequestsJSONResponse {
	return api.TooManyRequestsJSONResponse{
		Body:    api.ErrorResponse{Code: "AUTH_RATE_LIMITED", Message: "登录请求过于频繁，请稍后重试", RequestId: requestID},
		Headers: api.TooManyRequestsResponseHeaders{XRequestID: &requestID},
	}
}

func authRequired(requestID string) api.AuthRequiredJSONResponse {
	return api.AuthRequiredJSONResponse{
		Body:    api.ErrorResponse{Code: "AUTH_REQUIRED", Message: "认证信息无效或已过期", RequestId: requestID},
		Headers: api.AuthRequiredResponseHeaders{XRequestID: &requestID},
	}
}

func securityRejected(requestID string) api.SecurityRejectedJSONResponse {
	return api.SecurityRejectedJSONResponse{
		Body:    api.ErrorResponse{Code: "AUTH_ORIGIN_REJECTED", Message: "请求来源不被允许", RequestId: requestID},
		Headers: api.SecurityRejectedResponseHeaders{XRequestID: &requestID},
	}
}

func ginContext(ctx context.Context) (*gin.Context, error) {
	value, ok := ctx.(*gin.Context)
	if !ok {
		return nil, fmt.Errorf("identity HTTP handler requires Gin context")
	}
	return value, nil
}

func clientIP(c *gin.Context) *netip.Addr {
	value, err := netip.ParseAddr(strings.TrimSpace(c.ClientIP()))
	if err != nil {
		return nil
	}
	return &value
}

func databaseRequestID(value string) uuid.UUID {
	if parsed, err := uuid.Parse(value); err == nil {
		return parsed
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("file-workshop/request/"+value))
}

func stringPointer(value string) *string {
	return &value
}
