package transport

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/modules/identity/application"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	uploadapplication "file-workshop/backend/internal/modules/uploads/application"
	"file-workshop/backend/internal/modules/uploads/domain"
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
	service        *uploadapplication.Service
	authenticator  SessionAuthenticator
	config         config.AuthConfig
	allowedOrigins map[string]struct{}
}

func NewHandler(service *uploadapplication.Service, authenticator *application.Service, cfg config.AuthConfig) *Handler {
	origins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		origins[origin] = struct{}{}
	}
	return &Handler{service: service, authenticator: authenticator, config: cfg, allowedOrigins: origins}
}

func (h *Handler) authenticate(ctx context.Context) (*gin.Context, domain.Actor, string, error) {
	ginContext, ok := ctx.(*gin.Context)
	if !ok {
		return nil, domain.Actor{}, "", fmt.Errorf("upload HTTP handler requires Gin context")
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

func (h *Handler) CreateUploadSession(ctx context.Context, request api.CreateUploadSessionRequestObject) (api.CreateUploadSessionResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.CreateUploadSession401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CreateUploadSession403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.CreateUploadSession400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	input, err := createInput(*request.Body, string(request.Params.IdempotencyKey), databaseRequestID(requestID))
	if err != nil {
		return api.CreateUploadSession400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	session, err := h.service.CreateSession(ginContext.Request.Context(), actor, input)
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.CreateUploadSession400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.CreateUploadSession403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.CreateUploadSession404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.CreateUploadSession409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.CreateUploadSession201JSONResponse(api.UploadSessionResponse{Session: apiSession(session), RequestId: requestID}), nil
}

func (h *Handler) GetUploadSession(ctx context.Context, request api.GetUploadSessionRequestObject) (api.GetUploadSessionResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.GetUploadSession401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	session, err := h.service.GetSession(ctx, actor, uuid.UUID(request.UploadSessionId))
	if err != nil {
		if _, denied, missing, _, code := mapError(err, requestID); code != "" {
			switch code {
			case "403":
				return api.GetUploadSession403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.GetUploadSession404JSONResponse{NotFoundJSONResponse: missing}, nil
			}
		}
		return nil, err
	}
	return api.GetUploadSession200JSONResponse(api.UploadSessionResponse{Session: apiSession(session), RequestId: requestID}), nil
}

func (h *Handler) PresignUploadPart(ctx context.Context, request api.PresignUploadPartRequestObject) (api.PresignUploadPartResponseObject, error) {
	_, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.PresignUploadPart401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	part, err := h.service.PresignUploadPart(ctx, actor, uuid.UUID(request.UploadSessionId), int32(request.PartNumber))
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.PresignUploadPart400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.PresignUploadPart403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.PresignUploadPart404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.PresignUploadPart409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.PresignUploadPart200JSONResponse(api.PresignedUploadPartResponse{Part: api.PresignedUploadPart{UploadSessionId: openapi_types.UUID(part.SessionID), PartNumber: int(part.PartNumber), Method: part.Method, Url: part.URL, Headers: &part.Headers, ExpiresAt: part.ExpiresAt}, RequestId: requestID}), nil
}

func (h *Handler) AbortUploadSession(ctx context.Context, request api.AbortUploadSessionRequestObject) (api.AbortUploadSessionResponseObject, error) {
	ginContext, actor, requestID, err := h.authenticate(ctx)
	if err != nil {
		return api.AbortUploadSession401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.AbortUploadSession403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.AbortUploadSession400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	session, err := h.service.AbortSession(ginContext.Request.Context(), actor, uuid.UUID(request.UploadSessionId), request.Body.RowVersion, request.Body.Reason, databaseRequestID(requestID))
	if err != nil {
		if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
			switch code {
			case "400":
				return api.AbortUploadSession400JSONResponse{InvalidRequestJSONResponse: bad}, nil
			case "403":
				return api.AbortUploadSession403JSONResponse{ForbiddenJSONResponse: denied}, nil
			case "404":
				return api.AbortUploadSession404JSONResponse{NotFoundJSONResponse: missing}, nil
			case "409":
				return api.AbortUploadSession409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
			}
		}
		return nil, err
	}
	return api.AbortUploadSession200JSONResponse(api.UploadSessionResponse{Session: apiSession(session), RequestId: requestID}), nil
}

func createInput(body api.CreateUploadSessionRequest, idempotencyKey string, requestID uuid.UUID) (domain.CreateSessionInput, error) {
	declaredHash, err := optionalHex(body.DeclaredSha256Hex)
	if err != nil {
		return domain.CreateSessionInput{}, err
	}
	lockHash, err := optionalHex(body.LockTokenHashHex)
	if err != nil {
		return domain.CreateSessionInput{}, err
	}
	return domain.CreateSessionInput{
		SpaceID:                  uuid.UUID(body.SpaceId),
		FolderID:                 uuid.UUID(body.FolderId),
		TargetDocumentID:         optionalAPIUUID(body.TargetDocumentId),
		UploadIntent:             string(body.UploadIntent),
		FileName:                 body.FileName,
		DeclaredSizeBytes:        body.DeclaredSizeBytes,
		DeclaredSHA256:           declaredHash,
		DeclaredMIMEType:         body.DeclaredMimeType,
		PartSizeBytes:            body.PartSizeBytes,
		ExpectedCurrentVersionID: optionalAPIUUID(body.ExpectedCurrentVersionId),
		ExpectedLockFencingToken: body.ExpectedLockFencingToken,
		LockTokenHash:            lockHash,
		IdempotencyKey:           idempotencyKey,
		RequestID:                requestID,
	}, nil
}

func optionalHex(value *string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(*value))
	if err != nil || len(decoded) != 32 {
		return nil, domain.ErrInvalidInput
	}
	return decoded, nil
}

func optionalAPIUUID(value *openapi_types.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	converted := uuid.UUID(*value)
	return &converted
}

func apiSession(value domain.Session) api.UploadSession {
	result := api.UploadSession{
		UploadSessionId:          openapi_types.UUID(value.ID),
		UserId:                   openapi_types.UUID(value.UserID),
		SpaceId:                  openapi_types.UUID(value.SpaceID),
		FolderId:                 openapi_types.UUID(value.FolderID),
		QuotaReservationId:       openapi_types.UUID(value.QuotaReservationID),
		UploadIntent:             api.UploadIntent(value.UploadIntent),
		FileName:                 value.FileName,
		NormalizedName:           value.NormalizedName,
		DeclaredSizeBytes:        value.DeclaredSizeBytes,
		DeclaredMimeType:         value.DeclaredMIMEType,
		PartSizeBytes:            value.PartSizeBytes,
		ExpectedPartCount:        int(value.ExpectedPartCount),
		ExpectedLockFencingToken: value.ExpectedLockFencingToken,
		Status:                   api.UploadSessionStatus(value.Status),
		ExpiresAt:                value.ExpiresAt,
		CreatedAt:                value.CreatedAt,
		UpdatedAt:                value.UpdatedAt,
		CompletedAt:              value.CompletedAt,
		FailureCode:              value.FailureCode,
		RowVersion:               value.RowVersion,
	}
	if len(value.DeclaredSHA256) == 32 {
		text := hex.EncodeToString(value.DeclaredSHA256)
		result.DeclaredSha256Hex = &text
	}
	if value.TargetDocumentID != nil {
		id := openapi_types.UUID(*value.TargetDocumentID)
		result.TargetDocumentId = &id
	}
	if value.ExpectedCurrentVersionID != nil {
		id := openapi_types.UUID(*value.ExpectedCurrentVersionID)
		result.ExpectedCurrentVersionId = &id
	}
	if value.ResultDocumentID != nil {
		id := openapi_types.UUID(*value.ResultDocumentID)
		result.ResultDocumentId = &id
	}
	if value.ResultVersionID != nil {
		id := openapi_types.UUID(*value.ResultVersionID)
		result.ResultVersionId = &id
	}
	return result
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

func notFound(requestID string) api.NotFoundJSONResponse {
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: "UPLOAD_SESSION_NOT_FOUND", Message: "上传会话不存在或不可见", RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}

func conflict(requestID string, err error) api.ConflictJSONResponse {
	code, message := "UPLOAD_CONFLICT", "上传会话状态冲突"
	switch {
	case errors.Is(err, domain.ErrVersionConflict):
		code, message = "ROW_VERSION_CONFLICT", "资源已被其他请求修改"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		code, message = "IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同请求"
	case errors.Is(err, domain.ErrStorageUnavailable):
		code, message = "STORAGE_UNAVAILABLE", "对象存储未启用或暂不可用"
	case errors.Is(err, domain.ErrQuotaExceeded):
		code, message = "SPACE_QUOTA_EXCEEDED", "空间容量不足"
	}
	return api.ConflictJSONResponse{Body: api.ErrorResponse{Code: code, Message: message, RequestId: requestID}, Headers: api.ConflictResponseHeaders{XRequestID: &requestID}}
}

func mapError(err error, requestID string) (api.InvalidRequestJSONResponse, api.ForbiddenJSONResponse, api.NotFoundJSONResponse, api.ConflictJSONResponse, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return invalidRequest(requestID), api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "400"
	case errors.Is(err, domain.ErrForbidden):
		return api.InvalidRequestJSONResponse{}, forbidden(requestID, false), api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "403"
	case errors.Is(err, domain.ErrNotFound):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, notFound(requestID), api.ConflictJSONResponse{}, "404"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrVersionConflict), errors.Is(err, domain.ErrIdempotencyConflict), errors.Is(err, domain.ErrStorageUnavailable), errors.Is(err, domain.ErrQuotaExceeded):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, conflict(requestID, err), "409"
	default:
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, ""
	}
}

func databaseRequestID(value string) uuid.UUID {
	if parsed, err := uuid.Parse(value); err == nil {
		return parsed
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("file-workshop/request/"+value))
}
