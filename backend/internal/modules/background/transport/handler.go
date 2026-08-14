package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/modules/background/application"
	"file-workshop/backend/internal/modules/background/domain"
	identityapplication "file-workshop/backend/internal/modules/identity/application"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
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

func (h *Handler) ListBackgroundOutboxEvents(ctx context.Context, request api.ListBackgroundOutboxEventsRequestObject) (api.ListBackgroundOutboxEventsResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ListBackgroundOutboxEvents401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	filter := domain.OutboxListFilter{Page: intValue(request.Params.Page), PageSize: intValue(request.Params.PageSize)}
	if request.Params.Status != nil {
		value := string(*request.Params.Status)
		filter.Status = &value
	}
	if request.Params.EventType != nil {
		value := *request.Params.EventType
		filter.EventType = &value
	}
	result, err := h.service.ListOutboxEvents(ginContext.Request.Context(), actor, filter)
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.ListBackgroundOutboxEvents400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.ListBackgroundOutboxEvents403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	items := make([]api.BackgroundOutboxEvent, 0, len(result.Items))
	for _, event := range result.Items {
		mapped, err := apiOutboxEvent(event)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return api.ListBackgroundOutboxEvents200JSONResponse(api.BackgroundOutboxEventListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) RetryBackgroundOutboxEvent(ctx context.Context, request api.RetryBackgroundOutboxEventRequestObject) (api.RetryBackgroundOutboxEventResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.RetryBackgroundOutboxEvent401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RetryBackgroundOutboxEvent403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.RetryBackgroundOutboxEvent400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.RetryOutboxEvent(ginContext.Request.Context(), actor, uuid.UUID(request.OutboxEventId), request.Body.RowVersion, request.Body.Reason)
	if bad, denied, missing, conflict, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.RetryBackgroundOutboxEvent400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.RetryBackgroundOutboxEvent403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.RetryBackgroundOutboxEvent404JSONResponse{NotFoundJSONResponse: missing}, nil
		case "409":
			return api.RetryBackgroundOutboxEvent409JSONResponse{ConflictJSONResponse: conflict}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiOutboxEvent(result)
	if err != nil {
		return nil, err
	}
	return api.RetryBackgroundOutboxEvent200JSONResponse(api.BackgroundOutboxEventResponse{Event: body, RequestId: requestID}), nil
}

func (h *Handler) BatchRetryBackgroundOutboxEvents(ctx context.Context, request api.BatchRetryBackgroundOutboxEventsRequestObject) (api.BatchRetryBackgroundOutboxEventsResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.BatchRetryBackgroundOutboxEvents401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.BatchRetryBackgroundOutboxEvents403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.BatchRetryBackgroundOutboxEvents400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.BatchRetryOutboxEvents(ginContext.Request.Context(), actor, batchOutboxRequestItems(request.Body.Items), request.Body.Reason)
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.BatchRetryBackgroundOutboxEvents400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.BatchRetryBackgroundOutboxEvents403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiOutboxBatchResult(result, requestID)
	if err != nil {
		return nil, err
	}
	return api.BatchRetryBackgroundOutboxEvents200JSONResponse(body), nil
}

func (h *Handler) ListBackgroundJobs(ctx context.Context, request api.ListBackgroundJobsRequestObject) (api.ListBackgroundJobsResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ListBackgroundJobs401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	filter := domain.JobListFilter{Page: intValue(request.Params.Page), PageSize: intValue(request.Params.PageSize)}
	if request.Params.Status != nil {
		value := string(*request.Params.Status)
		filter.Status = &value
	}
	if request.Params.JobType != nil {
		value := *request.Params.JobType
		filter.JobType = &value
	}
	result, err := h.service.ListBackgroundJobs(ginContext.Request.Context(), actor, filter)
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.ListBackgroundJobs400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.ListBackgroundJobs403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	items := make([]api.BackgroundJob, 0, len(result.Items))
	for _, job := range result.Items {
		mapped, err := apiJob(job)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return api.ListBackgroundJobs200JSONResponse(api.BackgroundJobListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) GetBackgroundAdministrationSummary(ctx context.Context, request api.GetBackgroundAdministrationSummaryRequestObject) (api.GetBackgroundAdministrationSummaryResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetBackgroundAdministrationSummary401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	result, err := h.service.GetSummary(ginContext.Request.Context(), actor)
	if _, denied, _, _, code := mapError(err, requestID); code != "" {
		if code == "403" {
			return api.GetBackgroundAdministrationSummary403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return api.GetBackgroundAdministrationSummary200JSONResponse(apiSummary(result, requestID)), nil
}

func (h *Handler) GetBackgroundQueueLagSummary(ctx context.Context, request api.GetBackgroundQueueLagSummaryRequestObject) (api.GetBackgroundQueueLagSummaryResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetBackgroundQueueLagSummary401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	result, err := h.service.GetQueueLagSummary(ginContext.Request.Context(), actor)
	if _, denied, _, _, code := mapError(err, requestID); code != "" {
		if code == "403" {
			return api.GetBackgroundQueueLagSummary403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return api.GetBackgroundQueueLagSummary200JSONResponse(apiQueueLagSummary(result, requestID)), nil
}

func (h *Handler) GetBackgroundFailureSummary(ctx context.Context, request api.GetBackgroundFailureSummaryRequestObject) (api.GetBackgroundFailureSummaryResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetBackgroundFailureSummary401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	result, err := h.service.GetFailureSummary(ginContext.Request.Context(), actor)
	if _, denied, _, _, code := mapError(err, requestID); code != "" {
		if code == "403" {
			return api.GetBackgroundFailureSummary403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return api.GetBackgroundFailureSummary200JSONResponse(apiFailureSummary(result, requestID)), nil
}

func (h *Handler) RecoverExpiredBackgroundLeases(ctx context.Context, request api.RecoverExpiredBackgroundLeasesRequestObject) (api.RecoverExpiredBackgroundLeasesResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.RecoverExpiredBackgroundLeases401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RecoverExpiredBackgroundLeases403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.RecoverExpiredBackgroundLeases400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.RecoverExpiredLeases(ginContext.Request.Context(), actor, intValue(request.Body.BatchSize), request.Body.Reason)
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.RecoverExpiredBackgroundLeases400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.RecoverExpiredBackgroundLeases403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return api.RecoverExpiredBackgroundLeases200JSONResponse(apiLeaseRecovery(result, requestID)), nil
}

func (h *Handler) RetryBackgroundJob(ctx context.Context, request api.RetryBackgroundJobRequestObject) (api.RetryBackgroundJobResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.RetryBackgroundJob401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RetryBackgroundJob403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.RetryBackgroundJob400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.RetryBackgroundJob(ginContext.Request.Context(), actor, uuid.UUID(request.BackgroundJobId), request.Body.RowVersion, request.Body.Reason)
	if bad, denied, missing, conflict, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.RetryBackgroundJob400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.RetryBackgroundJob403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.RetryBackgroundJob404JSONResponse{NotFoundJSONResponse: missing}, nil
		case "409":
			return api.RetryBackgroundJob409JSONResponse{ConflictJSONResponse: conflict}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiJob(result)
	if err != nil {
		return nil, err
	}
	return api.RetryBackgroundJob200JSONResponse(api.BackgroundJobResponse{Job: body, RequestId: requestID}), nil
}

func (h *Handler) BatchRetryBackgroundJobs(ctx context.Context, request api.BatchRetryBackgroundJobsRequestObject) (api.BatchRetryBackgroundJobsResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.BatchRetryBackgroundJobs401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.BatchRetryBackgroundJobs403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.BatchRetryBackgroundJobs400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.BatchRetryBackgroundJobs(ginContext.Request.Context(), actor, batchRequestItems(request.Body.Items), request.Body.Reason)
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.BatchRetryBackgroundJobs400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.BatchRetryBackgroundJobs403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiBatchResult(result, requestID)
	if err != nil {
		return nil, err
	}
	return api.BatchRetryBackgroundJobs200JSONResponse(body), nil
}

func (h *Handler) BatchCancelBackgroundJobs(ctx context.Context, request api.BatchCancelBackgroundJobsRequestObject) (api.BatchCancelBackgroundJobsResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.BatchCancelBackgroundJobs401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.BatchCancelBackgroundJobs403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.BatchCancelBackgroundJobs400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.BatchCancelBackgroundJobs(ginContext.Request.Context(), actor, batchRequestItems(request.Body.Items), request.Body.Reason)
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.BatchCancelBackgroundJobs400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.BatchCancelBackgroundJobs403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiBatchResult(result, requestID)
	if err != nil {
		return nil, err
	}
	return api.BatchCancelBackgroundJobs200JSONResponse(body), nil
}

func (h *Handler) BatchMarkBackgroundJobsDead(ctx context.Context, request api.BatchMarkBackgroundJobsDeadRequestObject) (api.BatchMarkBackgroundJobsDeadResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.BatchMarkBackgroundJobsDead401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.BatchMarkBackgroundJobsDead403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.BatchMarkBackgroundJobsDead400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.BatchMarkBackgroundJobsDead(ginContext.Request.Context(), actor, batchRequestItems(request.Body.Items), request.Body.Reason)
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.BatchMarkBackgroundJobsDead400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.BatchMarkBackgroundJobsDead403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiBatchResult(result, requestID)
	if err != nil {
		return nil, err
	}
	return api.BatchMarkBackgroundJobsDead200JSONResponse(body), nil
}

func (h *Handler) BatchSkipBackgroundJobs(ctx context.Context, request api.BatchSkipBackgroundJobsRequestObject) (api.BatchSkipBackgroundJobsResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.BatchSkipBackgroundJobs401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.BatchSkipBackgroundJobs403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.BatchSkipBackgroundJobs400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.BatchSkipBackgroundJobs(ginContext.Request.Context(), actor, batchRequestItems(request.Body.Items), request.Body.Reason)
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.BatchSkipBackgroundJobs400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.BatchSkipBackgroundJobs403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiBatchResult(result, requestID)
	if err != nil {
		return nil, err
	}
	return api.BatchSkipBackgroundJobs200JSONResponse(body), nil
}

func (h *Handler) CancelBackgroundJob(ctx context.Context, request api.CancelBackgroundJobRequestObject) (api.CancelBackgroundJobResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.CancelBackgroundJob401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CancelBackgroundJob403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.CancelBackgroundJob400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.CancelBackgroundJob(ginContext.Request.Context(), actor, uuid.UUID(request.BackgroundJobId), request.Body.RowVersion, request.Body.Reason)
	if bad, denied, missing, conflict, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.CancelBackgroundJob400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.CancelBackgroundJob403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.CancelBackgroundJob404JSONResponse{NotFoundJSONResponse: missing}, nil
		case "409":
			return api.CancelBackgroundJob409JSONResponse{ConflictJSONResponse: conflict}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiJob(result)
	if err != nil {
		return nil, err
	}
	return api.CancelBackgroundJob200JSONResponse(api.BackgroundJobResponse{Job: body, RequestId: requestID}), nil
}

func (h *Handler) authenticate(ctx context.Context) (*gin.Context, domain.Actor, string, error) {
	ginContext, ok := ctx.(*gin.Context)
	if !ok {
		return nil, domain.Actor{}, "", fmt.Errorf("background HTTP handler requires Gin context")
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

func intValue[T ~int](value *T) int {
	if value == nil {
		return 0
	}
	return int(*value)
}

func invalidRequest(requestID string) api.InvalidRequestJSONResponse {
	return api.InvalidRequestJSONResponse{Body: api.ErrorResponse{Code: "INVALID_REQUEST", Message: "请求参数无效", RequestId: requestID}, Headers: api.InvalidRequestResponseHeaders{XRequestID: &requestID}}
}

func authRequired(requestID string) api.AuthRequiredJSONResponse {
	return api.AuthRequiredJSONResponse{Body: api.ErrorResponse{Code: "AUTH_REQUIRED", Message: "认证信息无效或已过期", RequestId: requestID}, Headers: api.AuthRequiredResponseHeaders{XRequestID: &requestID}}
}

func forbidden(requestID string) api.ForbiddenJSONResponse {
	return api.ForbiddenJSONResponse{Body: api.ErrorResponse{Code: "AUTH_FORBIDDEN", Message: "没有执行该后台运维操作的权限", RequestId: requestID}, Headers: api.ForbiddenResponseHeaders{XRequestID: &requestID}}
}

func notFound(requestID string) api.NotFoundJSONResponse {
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: "BACKGROUND_ITEM_NOT_FOUND", Message: "后台任务或 Outbox 事件不存在", RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}

func conflict(requestID string) api.ConflictJSONResponse {
	return api.ConflictJSONResponse{Body: api.ErrorResponse{Code: "BACKGROUND_STATE_CONFLICT", Message: "当前状态或 rowVersion 不允许执行该操作", RequestId: requestID}, Headers: api.ConflictResponseHeaders{XRequestID: &requestID}}
}

func mapError(err error, requestID string) (api.InvalidRequestJSONResponse, api.ForbiddenJSONResponse, api.NotFoundJSONResponse, api.ConflictJSONResponse, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return invalidRequest(requestID), api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "400"
	case errors.Is(err, domain.ErrForbidden):
		return api.InvalidRequestJSONResponse{}, forbidden(requestID), api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, "403"
	case errors.Is(err, domain.ErrNotFound):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, notFound(requestID), api.ConflictJSONResponse{}, "404"
	case errors.Is(err, domain.ErrConflict):
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, conflict(requestID), "409"
	default:
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, api.NotFoundJSONResponse{}, api.ConflictJSONResponse{}, ""
	}
}

func apiOutboxEvent(value domain.OutboxEvent) (api.BackgroundOutboxEvent, error) {
	payload, err := jsonObject(value.PayloadJSON)
	if err != nil {
		return api.BackgroundOutboxEvent{}, err
	}
	result := api.BackgroundOutboxEvent{OutboxEventId: openapi_types.UUID(value.ID), AggregateType: value.AggregateType, AggregateId: openapi_types.UUID(value.AggregateID), AggregateVersion: value.AggregateVersion, EventType: value.EventType, EventSchemaVersion: int(value.EventSchemaVersion), Payload: payload, DeduplicationKey: value.DeduplicationKey, Priority: int(value.Priority), Status: api.BackgroundOutboxStatus(value.Status), AttemptCount: int(value.AttemptCount), MaxAttempts: int(value.MaxAttempts), AvailableAt: value.AvailableAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, RowVersion: value.RowVersion}
	result.CorrelationId = optionalAPIUUID(value.CorrelationID)
	result.CausationId = optionalAPIUUID(value.CausationID)
	result.LockedBy = value.LockedBy
	result.LockedAt = value.LockedAt
	result.LeaseUntil = value.LeaseUntil
	result.NextRetryAt = value.NextRetryAt
	result.PublishedAt = value.PublishedAt
	result.LastErrorCode = value.LastErrorCode
	result.LastErrorSummary = value.LastErrorSummary
	return result, nil
}

func apiJob(value domain.BackgroundJob) (api.BackgroundJob, error) {
	payload, err := jsonObject(value.PayloadJSON)
	if err != nil {
		return api.BackgroundJob{}, err
	}
	result := api.BackgroundJob{BackgroundJobId: openapi_types.UUID(value.ID), JobType: value.JobType, PayloadSchemaVersion: int(value.PayloadSchemaVersion), Payload: payload, DeduplicationKey: value.DeduplicationKey, Priority: int(value.Priority), Status: api.BackgroundJobStatus(value.Status), AttemptCount: int(value.AttemptCount), MaxAttempts: int(value.MaxAttempts), AvailableAt: value.AvailableAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, RowVersion: value.RowVersion}
	result.TargetDocumentId = optionalAPIUUID(value.TargetDocumentID)
	result.TargetDocumentVersionId = optionalAPIUUID(value.TargetDocumentVersionID)
	result.TargetStorageObjectId = optionalAPIUUID(value.TargetStorageObjectID)
	result.LockedBy = value.LockedBy
	result.LockedAt = value.LockedAt
	result.LeaseUntil = value.LeaseUntil
	result.HeartbeatAt = value.HeartbeatAt
	result.StartedAt = value.StartedAt
	result.CompletedAt = value.CompletedAt
	result.LastErrorCode = value.LastErrorCode
	result.LastErrorSummary = value.LastErrorSummary
	return result, nil
}

func apiSummary(value domain.AdministrationSummary, requestID string) api.BackgroundAdministrationSummaryResponse {
	outbox := make([]api.BackgroundOutboxStatusCount, 0, len(value.OutboxEvents))
	for _, item := range value.OutboxEvents {
		outbox = append(outbox, api.BackgroundOutboxStatusCount{Status: api.BackgroundOutboxStatus(item.Status), Count: item.Count})
	}
	jobs := make([]api.BackgroundJobStatusCount, 0, len(value.BackgroundJobs))
	for _, item := range value.BackgroundJobs {
		jobs = append(jobs, api.BackgroundJobStatusCount{Status: api.BackgroundJobStatus(item.Status), Count: item.Count})
	}
	return api.BackgroundAdministrationSummaryResponse{OutboxEvents: outbox, BackgroundJobs: jobs, RequestId: requestID}
}

func apiQueueLagSummary(value domain.QueueLagSummary, requestID string) api.BackgroundQueueLagSummaryResponse {
	return api.BackgroundQueueLagSummaryResponse{
		OutboxEvents:   apiQueueLagItem(value.OutboxEvents),
		BackgroundJobs: apiQueueLagItem(value.BackgroundJobs),
		RequestId:      requestID,
	}
}

func apiQueueLagItem(value domain.QueueLagItem) api.BackgroundQueueLagItem {
	return api.BackgroundQueueLagItem{
		DuePendingCount:        value.DuePendingCount,
		DueFailedCount:         value.DueFailedCount,
		ExpiredProcessingCount: value.ExpiredProcessingCount,
		OldestDueAt:            value.OldestDueAt,
	}
}

func apiFailureSummary(value domain.FailureSummary, requestID string) api.BackgroundFailureSummaryResponse {
	return api.BackgroundFailureSummaryResponse{OutboxEvents: apiFailureSummaryItems(value.OutboxEvents), BackgroundJobs: apiFailureSummaryItems(value.BackgroundJobs), RequestId: requestID}
}

func apiFailureSummaryItems(values []domain.FailureSummaryItem) []api.BackgroundFailureSummaryItem {
	result := make([]api.BackgroundFailureSummaryItem, 0, len(values))
	for _, value := range values {
		result = append(result, api.BackgroundFailureSummaryItem{ErrorCode: value.ErrorCode, Count: value.Count, LatestAt: value.LatestAt})
	}
	return result
}

func apiLeaseRecovery(value domain.LeaseRecoveryResult, requestID string) api.ExpiredBackgroundLeaseRecoveryResponse {
	return api.ExpiredBackgroundLeaseRecoveryResponse{
		OutboxEvents:   apiLeaseRecoveryItem(value.OutboxEvents),
		BackgroundJobs: apiLeaseRecoveryItem(value.BackgroundJobs),
		RequestId:      requestID,
	}
}

func apiLeaseRecoveryItem(value domain.LeaseRecoveryItem) api.ExpiredBackgroundLeaseRecoveryItem {
	return api.ExpiredBackgroundLeaseRecoveryItem{Recovered: value.Recovered, Retryable: value.Retryable, Dead: value.Dead}
}

func batchOutboxRequestItems(items []api.BatchBackgroundOutboxEventOperationItemRequest) []domain.BatchOutboxEventItem {
	result := make([]domain.BatchOutboxEventItem, 0, len(items))
	for _, item := range items {
		result = append(result, domain.BatchOutboxEventItem{ID: uuid.UUID(item.OutboxEventId), RowVersion: item.RowVersion})
	}
	return result
}

func apiOutboxBatchResult(value domain.BatchOutboxEventOperationResult, requestID string) (api.BatchBackgroundOutboxEventOperationResponse, error) {
	items := make([]api.BatchBackgroundOutboxEventOperationItemResult, 0, len(value.Items))
	for _, item := range value.Items {
		result := api.BatchBackgroundOutboxEventOperationItemResult{OutboxEventId: openapi_types.UUID(item.ID), Success: item.Success}
		if item.Event != nil {
			event, err := apiOutboxEvent(*item.Event)
			if err != nil {
				return api.BatchBackgroundOutboxEventOperationResponse{}, err
			}
			result.Event = &event
		}
		result.ErrorCode = item.ErrorCode
		result.ErrorMessage = item.ErrorMessage
		items = append(items, result)
	}
	return api.BatchBackgroundOutboxEventOperationResponse{Items: items, Succeeded: value.Succeeded, Failed: value.Failed, RequestId: requestID}, nil
}

func batchRequestItems(items []api.BatchBackgroundJobOperationItemRequest) []domain.BatchJobItem {
	result := make([]domain.BatchJobItem, 0, len(items))
	for _, item := range items {
		result = append(result, domain.BatchJobItem{ID: uuid.UUID(item.BackgroundJobId), RowVersion: item.RowVersion})
	}
	return result
}

func apiBatchResult(value domain.BatchJobOperationResult, requestID string) (api.BatchBackgroundJobOperationResponse, error) {
	items := make([]api.BatchBackgroundJobOperationItemResult, 0, len(value.Items))
	for _, item := range value.Items {
		result := api.BatchBackgroundJobOperationItemResult{BackgroundJobId: openapi_types.UUID(item.ID), Success: item.Success}
		if item.Job != nil {
			job, err := apiJob(*item.Job)
			if err != nil {
				return api.BatchBackgroundJobOperationResponse{}, err
			}
			result.Job = &job
		}
		result.ErrorCode = item.ErrorCode
		result.ErrorMessage = item.ErrorMessage
		items = append(items, result)
	}
	return api.BatchBackgroundJobOperationResponse{Items: items, Succeeded: value.Succeeded, Failed: value.Failed, RequestId: requestID}, nil
}

func optionalAPIUUID(value *uuid.UUID) *openapi_types.UUID {
	if value == nil {
		return nil
	}
	id := openapi_types.UUID(*value)
	return &id
}

func jsonObject(value []byte) (map[string]any, error) {
	if len(value) == 0 {
		return map[string]any{}, nil
	}
	var object map[string]any
	if err := json.Unmarshal(value, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return map[string]any{}, nil
	}
	return object, nil
}
