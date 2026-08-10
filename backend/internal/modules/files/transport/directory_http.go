package transport

import (
	"context"
	"encoding/json"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/modules/files/domain"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (h *Handler) ListDirectoryEntries(ctx context.Context, request api.ListDirectoryEntriesRequestObject) (api.ListDirectoryEntriesResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.ListDirectoryEntries401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	filter := domain.EntryListFilter{SpaceID: uuid.UUID(request.SpaceId), Page: page, PageSize: pageSize}
	if request.Params.ParentFolderId != nil {
		value := uuid.UUID(*request.Params.ParentFolderId)
		filter.ParentFolderID = &value
	}
	if request.Params.EntryType != nil {
		value := string(*request.Params.EntryType)
		filter.EntryType = &value
	}
	if request.Params.LifecycleStatus != nil {
		value := string(*request.Params.LifecycleStatus)
		filter.LifecycleStatus = &value
	}
	result, err := h.service.ListEntries(ginContext.Request.Context(), actor, filter)
	if bad, denied, missing, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.ListDirectoryEntries400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.ListDirectoryEntries403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.ListDirectoryEntries404JSONResponse{NotFoundJSONResponse: missing}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	items := make([]api.DirectoryEntry, 0, len(result.Items))
	for _, item := range result.Items {
		mapped, err := apiEntry(item)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	spaceID := openapi_types.UUID(result.SpaceID)
	body := api.DirectoryEntryListResponse{Items: items, SpaceId: &spaceID, Page: result.Page, PageSize: result.PageSize, Total: result.Total, RequestId: requestID}
	if result.ParentFolderID != nil {
		id := openapi_types.UUID(*result.ParentFolderID)
		body.ParentFolderId = &id
	}
	if result.RootFolderID != nil {
		id := openapi_types.UUID(*result.RootFolderID)
		body.RootFolderId = &id
	}
	return api.ListDirectoryEntries200JSONResponse(body), nil
}

func (h *Handler) CreateFolder(ctx context.Context, request api.CreateFolderRequestObject) (api.CreateFolderResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.CreateFolder401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CreateFolder403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.CreateFolder400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	var parentID *uuid.UUID
	if request.Body.ParentFolderId != nil {
		value := uuid.UUID(*request.Body.ParentFolderId)
		parentID = &value
	}
	result, err := h.service.CreateFolder(ginContext.Request.Context(), actor, uuid.UUID(request.SpaceId), domain.CreateFolderInput{ParentFolderID: parentID, Name: request.Body.Name, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: databaseRequestID(requestID)})
	if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.CreateFolder400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.CreateFolder403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.CreateFolder404JSONResponse{NotFoundJSONResponse: missing}, nil
		case "409":
			return api.CreateFolder409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiEntry(result)
	if err != nil {
		return nil, err
	}
	return api.CreateFolder201JSONResponse(api.DirectoryEntryResponse{Entry: body, RequestId: requestID}), nil
}

func (h *Handler) CreateDocument(ctx context.Context, request api.CreateDocumentRequestObject) (api.CreateDocumentResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.CreateDocument401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.CreateDocument403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.CreateDocument400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	var parentID *uuid.UUID
	if request.Body.ParentFolderId != nil {
		value := uuid.UUID(*request.Body.ParentFolderId)
		parentID = &value
	}
	metadata, err := jsonMap(request.Body.Metadata)
	if err != nil {
		return api.CreateDocument400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.CreateDocument(ginContext.Request.Context(), actor, uuid.UUID(request.SpaceId), domain.CreateDocumentInput{ParentFolderID: parentID, Name: request.Body.Name, Classification: request.Body.Classification, MetadataJSON: metadata, IdempotencyKey: string(request.Params.IdempotencyKey), RequestID: databaseRequestID(requestID)})
	if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.CreateDocument400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.CreateDocument403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.CreateDocument404JSONResponse{NotFoundJSONResponse: missing}, nil
		case "409":
			return api.CreateDocument409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiEntry(result)
	if err != nil {
		return nil, err
	}
	return api.CreateDocument201JSONResponse(api.DirectoryEntryResponse{Entry: body, RequestId: requestID}), nil
}

func (h *Handler) GetDirectoryEntry(ctx context.Context, request api.GetDirectoryEntryRequestObject) (api.GetDirectoryEntryResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.GetDirectoryEntry401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	result, err := h.service.GetEntry(ginContext.Request.Context(), actor, uuid.UUID(request.EntryId))
	if _, denied, missing, _, code := mapError(err, requestID); code != "" {
		switch code {
		case "403":
			return api.GetDirectoryEntry403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.GetDirectoryEntry404JSONResponse{NotFoundJSONResponse: missing}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiEntry(result)
	if err != nil {
		return nil, err
	}
	return api.GetDirectoryEntry200JSONResponse(api.DirectoryEntryResponse{Entry: body, RequestId: requestID}), nil
}

func (h *Handler) RenameDirectoryEntry(ctx context.Context, request api.RenameDirectoryEntryRequestObject) (api.RenameDirectoryEntryResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.RenameDirectoryEntry401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.RenameDirectoryEntry403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.RenameDirectoryEntry400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	result, err := h.service.RenameEntry(ginContext.Request.Context(), actor, uuid.UUID(request.EntryId), domain.RenameEntryInput{Name: request.Body.Name, RowVersion: request.Body.RowVersion, RequestID: databaseRequestID(requestID)})
	if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.RenameDirectoryEntry400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.RenameDirectoryEntry403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.RenameDirectoryEntry404JSONResponse{NotFoundJSONResponse: missing}, nil
		case "409":
			return api.RenameDirectoryEntry409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiEntry(result)
	if err != nil {
		return nil, err
	}
	return api.RenameDirectoryEntry200JSONResponse(api.DirectoryEntryResponse{Entry: body, RequestId: requestID}), nil
}

func (h *Handler) MoveDirectoryEntry(ctx context.Context, request api.MoveDirectoryEntryRequestObject) (api.MoveDirectoryEntryResponseObject, error) {
	ginContext, actor, requestID, authErr := h.authenticate(ctx)
	if authErr != nil {
		return api.MoveDirectoryEntry401JSONResponse{AuthRequiredJSONResponse: authRequired(requestID)}, nil
	}
	if !h.originAllowed(ginContext) {
		return api.MoveDirectoryEntry403JSONResponse{ForbiddenJSONResponse: forbidden(requestID, true)}, nil
	}
	if request.Body == nil {
		return api.MoveDirectoryEntry400JSONResponse{InvalidRequestJSONResponse: invalidRequest(requestID)}, nil
	}
	var targetParentID *uuid.UUID
	if request.Body.TargetParentFolderId != nil {
		value := uuid.UUID(*request.Body.TargetParentFolderId)
		targetParentID = &value
	}
	result, err := h.service.MoveEntry(ginContext.Request.Context(), actor, uuid.UUID(request.EntryId), domain.MoveEntryInput{TargetParentFolderID: targetParentID, RowVersion: request.Body.RowVersion, RequestID: databaseRequestID(requestID)})
	if bad, denied, missing, conflictResponse, code := mapError(err, requestID); code != "" {
		switch code {
		case "400":
			return api.MoveDirectoryEntry400JSONResponse{InvalidRequestJSONResponse: bad}, nil
		case "403":
			return api.MoveDirectoryEntry403JSONResponse{ForbiddenJSONResponse: denied}, nil
		case "404":
			return api.MoveDirectoryEntry404JSONResponse{NotFoundJSONResponse: missing}, nil
		case "409":
			return api.MoveDirectoryEntry409JSONResponse{ConflictJSONResponse: conflictResponse}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := apiEntry(result)
	if err != nil {
		return nil, err
	}
	return api.MoveDirectoryEntry200JSONResponse(api.DirectoryEntryResponse{Entry: body, RequestId: requestID}), nil
}

func jsonMap(value *map[string]interface{}) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(*value)
}
