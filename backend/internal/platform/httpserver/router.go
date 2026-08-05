package httpserver

import (
	"log/slog"
	"net/http"

	"file-workshop/backend/api"

	"github.com/gin-gonic/gin"
)

func NewRouter(handler api.StrictServerInterface, logger *slog.Logger, allowedOrigins []string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	// oapi-codegen strict Gin handlers receive *gin.Context as context.Context.
	// Enable fallback so values and cancellation from http.Request.Context are preserved.
	router.ContextWithFallback = true
	router.Use(
		requestIDMiddleware(),
		accessLogMiddleware(logger),
		recoveryMiddleware(logger),
		corsMiddleware(allowedOrigins),
	)

	strictHandler := api.NewStrictHandlerWithOptions(handler, nil, api.StrictGinServerOptions{
		RequestErrorHandlerFunc: func(c *gin.Context, err error) {
			logger.WarnContext(c.Request.Context(), "OpenAPI request decoding failed", "error", err)
			writeError(c, http.StatusBadRequest, errorCodeInvalidRequest, "请求参数无效")
		},
		HandlerErrorFunc: func(c *gin.Context, err error) {
			logger.ErrorContext(c.Request.Context(), "OpenAPI handler failed", "error", err)
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "服务器内部错误")
		},
		ResponseErrorHandlerFunc: func(c *gin.Context, err error) {
			logger.ErrorContext(c.Request.Context(), "OpenAPI response serialization failed", "error", err)
			if !c.Writer.Written() {
				writeError(c, http.StatusInternalServerError, errorCodeInternal, "服务器内部错误")
				return
			}
			c.Abort()
		},
	})
	api.RegisterHandlers(router, strictHandler)
	router.NoRoute(writeNotFound)
	router.NoMethod(writeMethodNotAllowed)
	return router
}
