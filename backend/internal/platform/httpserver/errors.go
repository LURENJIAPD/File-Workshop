package httpserver

import (
	"net/http"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/platform/requestid"

	"github.com/gin-gonic/gin"
)

const (
	errorCodeInvalidRequest     = "INVALID_REQUEST"
	errorCodeInternal           = "INTERNAL_ERROR"
	errorCodeNotFound           = "ROUTE_NOT_FOUND"
	errorCodeMethodNotAllowed   = "METHOD_NOT_ALLOWED"
	errorCodeCORSOriginRejected = "CORS_ORIGIN_REJECTED"
)

func writeError(c *gin.Context, status int, code, message string) {
	requestID := requestid.FromContext(c.Request.Context())
	if requestID == "" {
		requestID = requestid.Resolve(c.GetHeader(requestid.Header))
		c.Header(requestid.Header, requestID)
	}
	c.AbortWithStatusJSON(status, api.ErrorResponse{
		Code:      code,
		Message:   message,
		RequestId: requestID,
	})
}

func writeNotFound(c *gin.Context) {
	writeError(c, http.StatusNotFound, errorCodeNotFound, "请求的接口不存在")
}

func writeMethodNotAllowed(c *gin.Context) {
	writeError(c, http.StatusMethodNotAllowed, errorCodeMethodNotAllowed, "请求方法不被允许")
}
