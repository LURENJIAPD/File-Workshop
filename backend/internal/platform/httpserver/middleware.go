package httpserver

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"file-workshop/backend/internal/platform/requestid"

	"github.com/gin-gonic/gin"
)

func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin == "" {
			c.Next()
			return
		}
		if _, ok := allowed[origin]; !ok {
			if c.Request.Method == http.MethodOptions {
				writeError(c, http.StatusForbidden, errorCodeCORSOriginRejected, "请求来源不被允许")
				return
			}
			c.Next()
			return
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Expose-Headers", requestid.Header+", Retry-After")
		c.Header("Vary", "Origin")
		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-ID")
			c.Header("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := requestid.Resolve(c.GetHeader(requestid.Header))
		c.Request = c.Request.WithContext(requestid.WithContext(c.Request.Context(), requestID))
		c.Header(requestid.Header, requestID)
		c.Next()
	}
}

func accessLogMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := c.Writer.Status()
		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		} else if status >= http.StatusBadRequest {
			level = slog.LevelWarn
		}

		logger.Log(
			c.Request.Context(),
			level,
			"HTTP request completed",
			"requestId", requestid.FromContext(c.Request.Context()),
			"method", c.Request.Method,
			"route", route,
			"status", status,
			"responseBytes", c.Writer.Size(),
			"durationMs", time.Since(startedAt).Milliseconds(),
			"errorCount", len(c.Errors),
		)
	}
}

func recoveryMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(
					c.Request.Context(),
					"HTTP handler panic recovered",
					"requestId", requestid.FromContext(c.Request.Context()),
					"panicType", fmt.Sprintf("%T", recovered),
				)
				writeError(c, http.StatusInternalServerError, errorCodeInternal, "服务器内部错误")
			}
		}()
		c.Next()
	}
}
