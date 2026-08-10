package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"file-workshop/backend/api"
	identityapplication "file-workshop/backend/internal/modules/identity/application"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	searchapplication "file-workshop/backend/internal/modules/search/application"
	"file-workshop/backend/internal/modules/search/domain"
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
	service       *searchapplication.Service
	authenticator SessionAuthenticator
	config        config.AuthConfig
}

func NewHandler(service *searchapplication.Service, authenticator *identityapplication.Service, cfg config.AuthConfig) *Handler {
	return &Handler{service: service, authenticator: authenticator, config: cfg}
}

func (h *Handler) SearchDirectoryEntries(ctx context.Context, request api.SearchDirectoryEntriesRequestObject) (api.SearchDirectoryEntriesResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.SearchDirectoryEntries401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	filter := domain.Filter{
		Query:           request.Params.Query,
		SpaceID:         optionalAPIUUID(request.Params.SpaceId),
		EntryType:       optionalEntryType(request.Params.EntryType),
		Extension:       request.Params.Extension,
		Classification:  request.Params.Classification,
		CreatedByUserID: optionalAPIUUID(request.Params.CreatedByUserId),
		UpdatedFrom:     request.Params.UpdatedFrom,
		UpdatedTo:       request.Params.UpdatedTo,
		MetadataKey:     request.Params.MetadataKey,
		MetadataValue:   request.Params.MetadataValue,
		Page:            page,
		PageSize:        pageSize,
	}
	result, err := h.service.Search(ginContext.Request.Context(), actor, filter)
	if bad, denied, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.SearchDirectoryEntries400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.SearchDirectoryEntries403JSONResponse{ForbiddenJSONResponse: denied}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	items := make([]api.SearchResult, 0, len(result.Items))
	for _, item := range result.Items {
		mapped, err := apiSearchResult(item)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return api.SearchDirectoryEntries200JSONResponse(api.SearchResultListResponse{Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, Degraded: result.Degraded, RequestId: requestID}), nil
}

func (h *Handler) authenticate(ctx context.Context) (*gin.Context, domain.Actor, string, error) {
	ginContext, ok := ctx.(*gin.Context)
	if !ok {
		return nil, domain.Actor{}, "", fmt.Errorf("search HTTP handler requires Gin context")
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

func invalidRequest(requestID string) api.InvalidRequestJSONResponse {
	return api.InvalidRequestJSONResponse{Body: api.ErrorResponse{Code: "INVALID_REQUEST", Message: "请求参数无效", RequestId: requestID}, Headers: api.InvalidRequestResponseHeaders{XRequestID: &requestID}}
}

func authRequired(requestID string) api.AuthRequiredJSONResponse {
	return api.AuthRequiredJSONResponse{Body: api.ErrorResponse{Code: "AUTH_REQUIRED", Message: "认证信息无效或已过期", RequestId: requestID}, Headers: api.AuthRequiredResponseHeaders{XRequestID: &requestID}}
}

func forbidden(requestID string) api.ForbiddenJSONResponse {
	return api.ForbiddenJSONResponse{Body: api.ErrorResponse{Code: "AUTH_FORBIDDEN", Message: "没有执行该操作的权限", RequestId: requestID}, Headers: api.ForbiddenResponseHeaders{XRequestID: &requestID}}
}

func mapError(err error, requestID string) (api.InvalidRequestJSONResponse, api.ForbiddenJSONResponse, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return invalidRequest(requestID), api.ForbiddenJSONResponse{}, "400"
	case errors.Is(err, domain.ErrForbidden):
		return api.InvalidRequestJSONResponse{}, forbidden(requestID), "403"
	default:
		return api.InvalidRequestJSONResponse{}, api.ForbiddenJSONResponse{}, ""
	}
}

func pagination(page *api.PageQuery, pageSize *api.PageSizeQuery) (int, int) {
	resultPage, resultPageSize := domain.DefaultPage, domain.DefaultPageSize
	if page != nil {
		resultPage = int(*page)
	}
	if pageSize != nil {
		resultPageSize = int(*pageSize)
	}
	return resultPage, resultPageSize
}

func optionalAPIUUID(value *openapi_types.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	id := uuid.UUID(*value)
	return &id
}

func optionalEntryType(value *api.DirectoryEntryType) *string {
	if value == nil {
		return nil
	}
	text := string(*value)
	return &text
}

func apiSearchResult(value domain.Result) (api.SearchResult, error) {
	entry, err := apiEntry(value.Entry)
	if err != nil {
		return api.SearchResult{}, err
	}
	matched := make([]api.SearchResultMatchedFields, 0, len(value.MatchedFields))
	for _, field := range value.MatchedFields {
		matched = append(matched, api.SearchResultMatchedFields(field))
	}
	result := api.SearchResult{Entry: entry, MatchedFields: matched, Source: api.SearchResultSource(value.Source)}
	if value.IndexStatus != nil {
		status := api.SearchResultIndexStatus(*value.IndexStatus)
		result.IndexStatus = &status
	}
	return result, nil
}

func apiEntry(value domain.Entry) (api.DirectoryEntry, error) {
	result := api.DirectoryEntry{EntryId: openapi_types.UUID(value.ID), SpaceId: openapi_types.UUID(value.SpaceID), EntryType: api.DirectoryEntryType(value.EntryType), Name: value.Name, NormalizedName: value.NormalizedName, PathCache: value.PathCache, Depth: int(value.Depth), LifecycleStatus: api.DirectoryLifecycleStatus(value.LifecycleStatus), IsRoot: value.IsRoot, CreatedByUserId: openapi_types.UUID(value.CreatedByUserID), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, DeletedAt: value.DeletedAt, RowVersion: value.RowVersion}
	if value.ParentFolderID != nil {
		parent := openapi_types.UUID(*value.ParentFolderID)
		result.ParentFolderId = &parent
	}
	if value.InheritanceMode != nil {
		mode := api.DirectoryEntryInheritanceMode(*value.InheritanceMode)
		result.InheritanceMode = &mode
	}
	result.AclVersion = value.ACLVersion
	if value.OwnerUserID != nil {
		id := openapi_types.UUID(*value.OwnerUserID)
		result.OwnerUserId = &id
	}
	if value.CurrentVersionID != nil {
		id := openapi_types.UUID(*value.CurrentVersionID)
		result.CurrentVersionId = &id
	}
	if value.AvailabilityStatus != nil {
		status := api.DocumentAvailabilityStatus(*value.AvailabilityStatus)
		result.AvailabilityStatus = &status
	}
	result.ExtensionNormalized = value.ExtensionNormalized
	result.Classification = value.Classification
	if value.MetadataSchemaVersion != nil {
		version := int(*value.MetadataSchemaVersion)
		result.MetadataSchemaVersion = &version
	}
	if len(value.MetadataJSON) > 0 {
		metadata := map[string]interface{}{}
		if err := json.Unmarshal(value.MetadataJSON, &metadata); err != nil {
			return api.DirectoryEntry{}, err
		}
		result.Metadata = &metadata
	}
	return result, nil
}
