package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"file-workshop/backend/internal/modules/identity/application"
	"file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/platform/config"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestOriginAllowList(t *testing.T) {
	handler := NewHandler(nil, config.AuthConfig{AllowedOrigins: []string{"http://127.0.0.1:5173"}, CookieSameSite: "lax"})
	tests := []struct {
		origin  string
		allowed bool
	}{
		{origin: "", allowed: true},
		{origin: "http://127.0.0.1:5173", allowed: true},
		{origin: "https://attacker.example", allowed: false},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		context.Request.Header.Set("Origin", test.origin)
		if actual := handler.originAllowed(context); actual != test.allowed {
			t.Fatalf("originAllowed(%q) = %v, want %v", test.origin, actual, test.allowed)
		}
	}
}

func TestAuthenticationCookiesAreHttpOnlyAndPathScoped(t *testing.T) {
	handler := NewHandler(nil, config.AuthConfig{
		AccessCookieName: "access", RefreshCookieName: "refresh",
		CookieSecure: true, CookieSameSite: "strict",
	})
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	now := time.Now().UTC()
	handler.setAuthenticationCookies(context, application.AuthenticationResult{
		AccessToken: "access-token", AccessExpiresAt: now.Add(15 * time.Minute), RefreshToken: "refresh-token",
		Session: domain.Session{ID: uuid.Must(uuid.NewV7()), ExpiresAt: now.Add(7 * 24 * time.Hour)},
	})
	cookies := recorder.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookie count = %d, want 2", len(cookies))
	}
	byName := map[string]*http.Cookie{cookies[0].Name: cookies[0], cookies[1].Name: cookies[1]}
	if cookie := byName["access"]; cookie == nil || cookie.Path != "/" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected access cookie: %#v", cookie)
	}
	if cookie := byName["refresh"]; cookie == nil || cookie.Path != "/api/v1/auth" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected refresh cookie: %#v", cookie)
	}
}
