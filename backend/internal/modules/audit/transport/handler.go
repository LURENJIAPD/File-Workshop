package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/modules/audit/application"
	"file-workshop/backend/internal/modules/audit/domain"
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

func (h *Handler) ListAuditEvents(ctx context.Context, request api.ListAuditEventsRequestObject) (api.ListAuditEventsResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ListAuditEvents401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	filter := domain.EventListFilter{DateFrom: request.Params.DateFrom.Time, DateTo: request.Params.DateTo.Time, Page: intValue(request.Params.Page), PageSize: intValue(request.Params.PageSize)}
	if request.Params.EventType != nil {
		filter.EventType = request.Params.EventType
	}
	if request.Params.RiskLevel != nil {
		value := string(*request.Params.RiskLevel)
		filter.RiskLevel = &value
	}
	if request.Params.ActorType != nil {
		value := string(*request.Params.ActorType)
		filter.ActorType = &value
	}
	if request.Params.ActorId != nil {
		value := uuid.UUID(*request.Params.ActorId)
		filter.ActorID = &value
	}
	if request.Params.ResourceType != nil {
		filter.ResourceType = request.Params.ResourceType
	}
	if request.Params.ResourceId != nil {
		value := uuid.UUID(*request.Params.ResourceId)
		filter.ResourceID = &value
	}
	if request.Params.Result != nil {
		value := string(*request.Params.Result)
		filter.Result = &value
	}
	if request.Params.RequestId != nil {
		value := uuid.UUID(*request.Params.RequestId)
		filter.RequestID = &value
	}
	result, err := h.service.ListEvents(ginContext.Request.Context(), actor, filter)
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.ListAuditEvents400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.ListAuditEvents403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	items, err := apiEvents(result.Items)
	if err != nil {
		return nil, err
	}
	return api.ListAuditEvents200JSONResponse(api.AuditEventListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) GetAuditEvent(ctx context.Context, request api.GetAuditEventRequestObject) (api.GetAuditEventResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetAuditEvent401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	event, err := h.service.GetEvent(ginContext.Request.Context(), actor, uuid.UUID(request.AuditEventId), request.Params.PartitionDate.Time)
	if bad, denied, missing, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.GetAuditEvent400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.GetAuditEvent403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.GetAuditEvent404JSONResponse{NotFoundJSONResponse: missing}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiEvent(event)
	if err != nil {
		return nil, err
	}
	return api.GetAuditEvent200JSONResponse(api.AuditEventResponse{Event: body, RequestId: requestID}), nil
}

func (h *Handler) GetAuditSummary(ctx context.Context, request api.GetAuditSummaryRequestObject) (api.GetAuditSummaryResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetAuditSummary401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	result, err := h.service.GetSummary(ginContext.Request.Context(), actor, domain.SummaryFilter{DateFrom: request.Params.DateFrom.Time, DateTo: request.Params.DateTo.Time})
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.GetAuditSummary400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.GetAuditSummary403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return api.GetAuditSummary200JSONResponse(apiSummary(result, requestID)), nil
}

func (h *Handler) GetAuditIntegrity(ctx context.Context, request api.GetAuditIntegrityRequestObject) (api.GetAuditIntegrityResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetAuditIntegrity401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	filter := domain.IntegrityFilter{DateFrom: request.Params.DateFrom.Time, DateTo: request.Params.DateTo.Time, Page: intValue(request.Params.Page), PageSize: intValue(request.Params.PageSize)}
	if request.Params.Status != nil {
		value := string(*request.Params.Status)
		filter.Status = &value
	}
	result, err := h.service.GetIntegrity(ginContext.Request.Context(), actor, filter)
	if bad, denied, _, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.GetAuditIntegrity400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.GetAuditIntegrity403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	items := make([]api.AuditChainHead, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, apiChainHead(item))
	}
	return api.GetAuditIntegrity200JSONResponse(api.AuditIntegrityResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}), nil
}

func (h *Handler) VerifyAuditIntegrity(ctx context.Context, request api.VerifyAuditIntegrityRequestObject) (api.VerifyAuditIntegrityResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.VerifyAuditIntegrity401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.VerifyAuditIntegrity403JSONResponse{ForbiddenJSONResponse: forbidden(requestID)}, nil
	}
	if request.Body == nil {
		return api.VerifyAuditIntegrity400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.VerifyIntegrity(ginContext.Request.Context(), actor, request.Body.ChainId, request.Body.PartitionDate.Time)
	if bad, denied, missing, conflict, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.VerifyAuditIntegrity400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.VerifyAuditIntegrity403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.VerifyAuditIntegrity404JSONResponse{NotFoundJSONResponse: missing}, nil
		case "409":
			return api.VerifyAuditIntegrity409JSONResponse{ConflictJSONResponse: conflict}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return api.VerifyAuditIntegrity200JSONResponse(api.AuditIntegrityVerificationResponse{ChainId: result.ChainID, PartitionDate: apiDate(result.PartitionDate), CheckedEvents: result.CheckedEvents, Verified: result.Verified, FailureReason: result.FailureReason, RequestId: requestID}), nil
}

func (h *Handler) authenticate(ctx context.Context) (*gin.Context, domain.Actor, string, error) {
	ginContext, ok := ctx.(*gin.Context)
	if !ok {
		return nil, domain.Actor{}, "", fmt.Errorf("audit HTTP handler requires Gin context")
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

func apiEvents(values []domain.Event) ([]api.AuditEvent, error) {
	result := make([]api.AuditEvent, 0, len(values))
	for _, value := range values {
		event, err := apiEvent(value)
		if err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, nil
}

func apiEvent(value domain.Event) (api.AuditEvent, error) {
	metadata := map[string]any{}
	if len(value.MetadataJSON) > 0 {
		if err := json.Unmarshal(value.MetadataJSON, &metadata); err != nil {
			return api.AuditEvent{}, err
		}
	}
	hashSchemaVersion := intPtr32(value.HashSchemaVersion)
	return api.AuditEvent{
		AuditEventId:          openapi_types.UUID(value.ID),
		EventType:             value.EventType,
		RiskLevel:             api.AuditRiskLevel(value.RiskLevel),
		ActorType:             api.AuditActorType(value.ActorType),
		ActorId:               openapiUUIDPtr(value.ActorID),
		ActorDisplayName:      value.ActorDisplayName,
		ActorEmployeeNo:       value.ActorEmployeeNo,
		EffectiveRole:         value.EffectiveRole,
		AdminDelegationId:     openapiUUIDPtr(value.AdminDelegationID),
		ShareId:               openapiUUIDPtr(value.ShareID),
		ResourceType:          value.ResourceType,
		ResourceId:            openapiUUIDPtr(value.ResourceID),
		ResourceName:          value.ResourceName,
		SpaceId:               openapiUUIDPtr(value.SpaceID),
		OrganizationId:        openapiUUIDPtr(value.OrganizationID),
		DocumentId:            openapiUUIDPtr(value.DocumentID),
		DocumentVersionId:     openapiUUIDPtr(value.DocumentVersionID),
		Action:                value.Action,
		Result:                api.AuditResult(value.Result),
		FailureCode:           value.FailureCode,
		SourceChannel:         api.AuditSourceChannel(value.SourceChannel),
		IpAddress:             value.IPAddress,
		UserAgent:             value.UserAgent,
		RequestId:             openapi_types.UUID(value.RequestID),
		TraceId:               value.TraceID,
		CorrelationId:         openapiUUIDPtr(value.CorrelationID),
		Reason:                value.Reason,
		MetadataSchemaVersion: int(value.MetadataSchemaVersion),
		Metadata:              metadata,
		HashSchemaVersion:     hashSchemaVersion,
		ChainId:               value.ChainID,
		SequenceNumber:        value.SequenceNumber,
		PreviousHash:          bytesPtr(value.PreviousHash),
		EventHash:             bytesPtr(value.EventHash),
		PartitionDate:         apiDate(value.PartitionDate),
		CreatedAt:             value.CreatedAt,
	}, nil
}

func apiChainHead(value domain.ChainHead) api.AuditChainHead {
	return api.AuditChainHead{
		ChainId:            value.ChainID,
		PartitionDate:      apiDate(value.PartitionDate),
		LastSequenceNumber: value.LastSequenceNumber,
		LastEventId:        openapi_types.UUID(value.LastEventID),
		LastHash:           append([]byte(nil), value.LastHash...),
		BatchRoot:          bytesPtr(value.BatchRoot),
		AnchorLocation:     value.AnchorLocation,
		Status:             api.AuditChainStatus(value.Status),
		VerifiedAt:         value.VerifiedAt,
		CreatedAt:          value.CreatedAt,
		UpdatedAt:          value.UpdatedAt,
		RowVersion:         value.RowVersion,
	}
}

func apiSummary(value domain.Summary, requestID string) api.AuditSummaryResponse {
	return api.AuditSummaryResponse{
		DateFrom:           apiDate(value.DateFrom),
		DateTo:             apiDate(value.DateTo),
		TotalEvents:        value.TotalEvents,
		RiskLevelCounts:    apiRiskLevelCounts(value.RiskLevelCounts),
		ResultCounts:       apiResultCounts(value.ResultCounts),
		ActorTypeCounts:    apiActorTypeCounts(value.ActorTypeCounts),
		ChainStatusCounts:  apiChainStatusCounts(value.ChainStatusCounts),
		EventTypeCounts:    apiNamedCounts(value.EventTypeCounts),
		ResourceTypeCounts: apiNamedCounts(value.ResourceTypeCounts),
		FailureCodeCounts:  apiNamedCounts(value.FailureCodeCounts),
		RequestId:          requestID,
	}
}

func apiRiskLevelCounts(values []domain.CountByValue) []api.AuditRiskLevelCount {
	result := make([]api.AuditRiskLevelCount, 0, len(values))
	for _, value := range values {
		result = append(result, api.AuditRiskLevelCount{RiskLevel: api.AuditRiskLevel(value.Value), Count: value.Count})
	}
	return result
}

func apiResultCounts(values []domain.CountByValue) []api.AuditResultCount {
	result := make([]api.AuditResultCount, 0, len(values))
	for _, value := range values {
		result = append(result, api.AuditResultCount{Result: api.AuditResult(value.Value), Count: value.Count})
	}
	return result
}

func apiActorTypeCounts(values []domain.CountByValue) []api.AuditActorTypeCount {
	result := make([]api.AuditActorTypeCount, 0, len(values))
	for _, value := range values {
		result = append(result, api.AuditActorTypeCount{ActorType: api.AuditActorType(value.Value), Count: value.Count})
	}
	return result
}

func apiChainStatusCounts(values []domain.CountByValue) []api.AuditChainStatusCount {
	result := make([]api.AuditChainStatusCount, 0, len(values))
	for _, value := range values {
		result = append(result, api.AuditChainStatusCount{Status: api.AuditChainStatus(value.Value), Count: value.Count})
	}
	return result
}

func apiNamedCounts(values []domain.CountByValue) []api.AuditNamedCount {
	result := make([]api.AuditNamedCount, 0, len(values))
	for _, value := range values {
		result = append(result, api.AuditNamedCount{Name: value.Value, Count: value.Count})
	}
	return result
}

func apiDate(value time.Time) openapi_types.Date {
	return openapi_types.Date{Time: value}
}

func openapiUUIDPtr(value *uuid.UUID) *openapi_types.UUID {
	if value == nil {
		return nil
	}
	result := openapi_types.UUID(*value)
	return &result
}

func bytesPtr(value []byte) *[]byte {
	if len(value) == 0 {
		return nil
	}
	result := append([]byte(nil), value...)
	return &result
}

func intPtr32(value *int32) *int {
	if value == nil {
		return nil
	}
	result := int(*value)
	return &result
}

func invalidRequest(requestID string) api.InvalidRequestJSONResponse {
	return api.InvalidRequestJSONResponse{Body: api.ErrorResponse{Code: "INVALID_REQUEST", Message: "请求参数无效", RequestId: requestID}, Headers: api.InvalidRequestResponseHeaders{XRequestID: &requestID}}
}

func authRequired(requestID string) api.AuthRequiredJSONResponse {
	return api.AuthRequiredJSONResponse{Body: api.ErrorResponse{Code: "AUTH_REQUIRED", Message: "认证信息无效或已过期", RequestId: requestID}, Headers: api.AuthRequiredResponseHeaders{XRequestID: &requestID}}
}

func forbidden(requestID string) api.ForbiddenJSONResponse {
	return api.ForbiddenJSONResponse{Body: api.ErrorResponse{Code: "AUDIT_FORBIDDEN", Message: "没有执行该审计操作的权限", RequestId: requestID}, Headers: api.ForbiddenResponseHeaders{XRequestID: &requestID}}
}

func notFound(requestID string) api.NotFoundJSONResponse {
	return api.NotFoundJSONResponse{Body: api.ErrorResponse{Code: "AUDIT_NOT_FOUND", Message: "审计记录或哈希链不存在", RequestId: requestID}, Headers: api.NotFoundResponseHeaders{XRequestID: &requestID}}
}

func conflict(requestID string) api.ConflictJSONResponse {
	return api.ConflictJSONResponse{Body: api.ErrorResponse{Code: "AUDIT_STATE_CONFLICT", Message: "审计链当前状态不允许执行该操作", RequestId: requestID}, Headers: api.ConflictResponseHeaders{XRequestID: &requestID}}
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
