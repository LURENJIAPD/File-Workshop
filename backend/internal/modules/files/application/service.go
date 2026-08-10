package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"file-workshop/backend/internal/modules/files/domain"
	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"

	"github.com/google/uuid"
)

const idempotencyTTL = 24 * time.Hour

type Authorizer interface {
	EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error)
}

type Service struct {
	repository Repository
	transactor Transactor
	authorizer Authorizer
	now        func() time.Time
}

func NewService(repository Repository, transactor Transactor, authorizer Authorizer, now func() time.Time) *Service {
	return &Service{repository: repository, transactor: transactor, authorizer: authorizer, now: now}
}

func (s *Service) ListEntries(ctx context.Context, actor domain.Actor, filter domain.EntryListFilter) (domain.EntryListResult, error) {
	page, pageSize, err := domain.NormalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return domain.EntryListResult{}, err
	}
	if filter.EntryType != nil {
		if err = domain.ValidateEntryType(*filter.EntryType); err != nil {
			return domain.EntryListResult{}, err
		}
	}
	if filter.LifecycleStatus != nil {
		if err = domain.ValidateLifecycleStatus(*filter.LifecycleStatus); err != nil {
			return domain.EntryListResult{}, err
		}
	}
	space, parentID, err := s.resolveListParent(ctx, actor, filter.SpaceID, filter.ParentFolderID)
	if err != nil {
		return domain.EntryListResult{}, err
	}
	if parentID == nil {
		return domain.EntryListResult{Items: []domain.NamespaceEntry{}, SpaceID: filter.SpaceID, RootFolderID: space.RootFolderID, Page: page, PageSize: pageSize, Total: 0}, nil
	}
	filter.Page, filter.PageSize, filter.ParentFolderID = page, pageSize, parentID
	result, err := s.repository.ListEntries(ctx, filter)
	if err != nil {
		return domain.EntryListResult{}, err
	}
	result.RootFolderID = space.RootFolderID
	return result, nil
}

func (s *Service) GetEntry(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.NamespaceEntry, error) {
	entry, err := s.repository.GetEntry(ctx, id)
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	if err = s.requirePermission(ctx, actor, resourceType(entry), entry.ID, "READ_METADATA"); err != nil {
		return domain.NamespaceEntry{}, err
	}
	return entry, nil
}

func (s *Service) CreateFolder(ctx context.Context, actor domain.Actor, spaceID uuid.UUID, input domain.CreateFolderInput) (domain.NamespaceEntry, error) {
	name, normalized, err := validatedName(input.Name)
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	if err = validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.NamespaceEntry{}, err
	}
	now := s.now().UTC()
	hash, err := requestHash(struct {
		SpaceID uuid.UUID
		Parent  *uuid.UUID
		Name    string
	}{spaceID, input.ParentFolderID, name})
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	var result domain.NamespaceEntry
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, "CREATE_FOLDER", input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result, err = repository.GetEntry(ctx, *replayID)
			return err
		}
		parent, err := s.resolveWriteParent(ctx, repository, actor, spaceID, input.ParentFolderID, "CREATE_FOLDER", now)
		if err != nil {
			return err
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		result, err = repository.InsertNamespaceEntry(ctx, domain.NewNamespaceEntry{ID: id, SpaceID: spaceID, ParentFolderID: &parent.ID, EntryType: domain.EntryTypeFolder, Name: name, NormalizedName: normalized, PathCache: childPath(parent.PathCache, name), Depth: parent.Depth + 1, CreatedByUserID: actor.UserID, CreatedAt: now})
		if err != nil {
			return err
		}
		if err = repository.InsertFolder(ctx, result.ID, now); err != nil {
			return err
		}
		result, err = repository.GetEntry(ctx, result.ID)
		if err != nil {
			return err
		}
		if err = insertEvent(ctx, repository, "FOLDER", result.ID, result.RowVersion, "FOLDER_CREATED", input.RequestID, now); err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, "CREATE_FOLDER", input.IdempotencyKey, result.ID, "FOLDER", now)
	})
	return result, err
}

func (s *Service) CreateDocument(ctx context.Context, actor domain.Actor, spaceID uuid.UUID, input domain.CreateDocumentInput) (domain.NamespaceEntry, error) {
	name, normalized, err := validatedName(input.Name)
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	if err = domain.ValidateClassification(input.Classification); err != nil {
		return domain.NamespaceEntry{}, err
	}
	if len(input.MetadataJSON) == 0 {
		input.MetadataJSON = json.RawMessage("{}")
	}
	if err = domain.ValidateMetadata(input.MetadataJSON); err != nil {
		return domain.NamespaceEntry{}, err
	}
	if err = validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.NamespaceEntry{}, err
	}
	now := s.now().UTC()
	hash, err := requestHash(struct {
		SpaceID        uuid.UUID
		Parent         *uuid.UUID
		Name           string
		Classification *string
		Metadata       json.RawMessage
	}{spaceID, input.ParentFolderID, name, input.Classification, input.MetadataJSON})
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	var result domain.NamespaceEntry
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, "CREATE_DOCUMENT", input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result, err = repository.GetEntry(ctx, *replayID)
			return err
		}
		parent, err := s.resolveWriteParent(ctx, repository, actor, spaceID, input.ParentFolderID, "UPLOAD", now)
		if err != nil {
			return err
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if _, err = repository.InsertNamespaceEntry(ctx, domain.NewNamespaceEntry{ID: id, SpaceID: spaceID, ParentFolderID: &parent.ID, EntryType: domain.EntryTypeDocument, Name: name, NormalizedName: normalized, PathCache: childPath(parent.PathCache, name), Depth: parent.Depth + 1, CreatedByUserID: actor.UserID, CreatedAt: now}); err != nil {
			return err
		}
		result, err = repository.InsertDocument(ctx, domain.NewDocument{ID: id, OwnerUserID: actor.UserID, AvailabilityStatus: domain.AvailabilityBlocked, ExtensionNormalized: domain.ExtensionNormalized(name), Classification: trimmedOptional(input.Classification), MetadataJSON: input.MetadataJSON, CreatedAt: now})
		if err != nil {
			return err
		}
		if err = insertEvent(ctx, repository, "DOCUMENT", result.ID, result.RowVersion, "DOCUMENT_CREATED", input.RequestID, now); err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, "CREATE_DOCUMENT", input.IdempotencyKey, result.ID, "DOCUMENT", now)
	})
	return result, err
}

func (s *Service) RenameEntry(ctx context.Context, actor domain.Actor, id uuid.UUID, input domain.RenameEntryInput) (domain.NamespaceEntry, error) {
	if input.RowVersion < 1 {
		return domain.NamespaceEntry{}, &domain.ValidationError{Field: "rowVersion"}
	}
	name, normalized, err := validatedName(input.Name)
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	now := s.now().UTC()
	var result domain.NamespaceEntry
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetEntryForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if current.IsRoot {
			return domain.ErrRootOperation
		}
		if err = s.requirePermission(ctx, actor, resourceType(current), current.ID, "RENAME"); err != nil {
			return err
		}
		result, err = repository.RenameEntry(ctx, id, name, normalized, domain.ExtensionNormalized(name), input.RowVersion, now)
		if err != nil {
			return err
		}
		if result.EntryType == domain.EntryTypeFolder {
			if err = repository.UpdateDescendantPaths(ctx, result.ID, stringValue(result.PathCache), result.Depth, now); err != nil {
				return err
			}
		}
		return insertEvent(ctx, repository, resourceType(result), result.ID, result.RowVersion, "ENTRY_RENAMED", input.RequestID, now)
	})
	return result, err
}

func (s *Service) MoveEntry(ctx context.Context, actor domain.Actor, id uuid.UUID, input domain.MoveEntryInput) (domain.NamespaceEntry, error) {
	if input.RowVersion < 1 {
		return domain.NamespaceEntry{}, &domain.ValidationError{Field: "rowVersion"}
	}
	now := s.now().UTC()
	var result domain.NamespaceEntry
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetEntryForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if current.IsRoot {
			return domain.ErrRootOperation
		}
		if err = s.requirePermission(ctx, actor, resourceType(current), current.ID, "MOVE"); err != nil {
			return err
		}
		targetParent, err := s.resolveWriteParent(ctx, repository, actor, current.SpaceID, input.TargetParentFolderID, targetCreateAction(current.EntryType), now)
		if err != nil {
			return err
		}
		if current.EntryType == domain.EntryTypeFolder {
			if targetParent.ID == current.ID {
				return domain.ErrTreeCycle
			}
			descendant, err := repository.FolderIsDescendantOf(ctx, targetParent.ID, current.ID)
			if err != nil {
				return err
			}
			if descendant {
				return domain.ErrTreeCycle
			}
		}
		result, err = repository.MoveEntry(ctx, current.ID, targetParent.ID, childPath(targetParent.PathCache, current.Name), targetParent.Depth+1, input.RowVersion, now)
		if err != nil {
			return err
		}
		if current.EntryType == domain.EntryTypeFolder {
			if err = repository.UpdateDescendantPaths(ctx, result.ID, stringValue(result.PathCache), result.Depth, now); err != nil {
				return err
			}
		}
		if err = repository.TouchSpaceSecurityEpoch(ctx, current.SpaceID, now); err != nil {
			return err
		}
		return insertEvent(ctx, repository, resourceType(result), result.ID, result.RowVersion, "ENTRY_MOVED", input.RequestID, now)
	})
	return result, err
}

func (s *Service) resolveListParent(ctx context.Context, actor domain.Actor, spaceID uuid.UUID, parentID *uuid.UUID) (domain.SpaceDirectoryInfo, *uuid.UUID, error) {
	space, err := s.repository.GetSpaceDirectoryInfo(ctx, spaceID)
	if err != nil {
		return domain.SpaceDirectoryInfo{}, nil, err
	}
	if parentID != nil {
		parent, err := s.repository.GetEntry(ctx, *parentID)
		if err != nil {
			return domain.SpaceDirectoryInfo{}, nil, err
		}
		if parent.SpaceID != spaceID || parent.EntryType != domain.EntryTypeFolder || parent.LifecycleStatus != domain.LifecycleActive {
			return domain.SpaceDirectoryInfo{}, nil, domain.ErrEntryNotFound
		}
		if err = s.requirePermission(ctx, actor, permissiondomain.ResourceFolder, parent.ID, "LIST"); err != nil {
			return domain.SpaceDirectoryInfo{}, nil, err
		}
		return space, parentID, nil
	}
	if space.RootFolderID == nil {
		if err = s.requirePermission(ctx, actor, permissiondomain.ResourceSpace, spaceID, "LIST"); err != nil {
			return domain.SpaceDirectoryInfo{}, nil, err
		}
		return space, nil, nil
	}
	if err = s.requirePermission(ctx, actor, permissiondomain.ResourceFolder, *space.RootFolderID, "LIST"); err != nil {
		return domain.SpaceDirectoryInfo{}, nil, err
	}
	return space, space.RootFolderID, nil
}

func (s *Service) resolveWriteParent(ctx context.Context, repository Repository, actor domain.Actor, spaceID uuid.UUID, parentID *uuid.UUID, action string, now time.Time) (domain.NamespaceEntry, error) {
	if parentID != nil {
		parent, err := repository.GetEntry(ctx, *parentID)
		if err != nil {
			return domain.NamespaceEntry{}, err
		}
		if parent.SpaceID != spaceID || parent.EntryType != domain.EntryTypeFolder || parent.LifecycleStatus != domain.LifecycleActive {
			return domain.NamespaceEntry{}, domain.ErrEntryNotFound
		}
		if err = s.requirePermission(ctx, actor, permissiondomain.ResourceFolder, parent.ID, action); err != nil {
			return domain.NamespaceEntry{}, err
		}
		return parent, nil
	}
	space, err := repository.GetSpaceDirectoryInfoForUpdate(ctx, spaceID)
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	if space.RootFolderID != nil {
		parent, err := repository.GetEntry(ctx, *space.RootFolderID)
		if err != nil {
			return domain.NamespaceEntry{}, err
		}
		if err = s.requirePermission(ctx, actor, permissiondomain.ResourceFolder, parent.ID, action); err != nil {
			return domain.NamespaceEntry{}, err
		}
		return parent, nil
	}
	if err = s.requirePermission(ctx, actor, permissiondomain.ResourceSpace, spaceID, action); err != nil {
		return domain.NamespaceEntry{}, err
	}
	rootID, err := uuid.NewV7()
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	root, err := repository.InsertNamespaceEntry(ctx, domain.NewNamespaceEntry{ID: rootID, SpaceID: spaceID, EntryType: domain.EntryTypeFolder, Name: "Root", NormalizedName: "root", PathCache: "/", Depth: 0, CreatedByUserID: actor.UserID, CreatedAt: now})
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	if err = repository.InsertFolder(ctx, root.ID, now); err != nil {
		return domain.NamespaceEntry{}, err
	}
	if err = repository.UpdateSpaceRootFolder(ctx, spaceID, root.ID, now); err != nil {
		return domain.NamespaceEntry{}, err
	}
	return repository.GetEntry(ctx, root.ID)
}

func (s *Service) requirePermission(ctx context.Context, actor domain.Actor, resourceType string, resourceID uuid.UUID, action string) error {
	result, err := s.authorizer.EvaluatePermission(ctx, permissiondomain.Actor{UserID: actor.UserID, SessionID: actor.SessionID, Role: actor.Role}, resourceType, resourceID, action, nil, false)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return domain.ErrForbidden
	}
	return nil
}

func validatedName(value string) (string, string, error) {
	if err := domain.ValidateEntryName(value); err != nil {
		return "", "", err
	}
	name := strings.TrimSpace(value)
	return name, domain.NormalizeName(name), nil
}

func resourceType(entry domain.NamespaceEntry) string {
	if entry.EntryType == domain.EntryTypeDocument {
		return permissiondomain.ResourceDocument
	}
	return permissiondomain.ResourceFolder
}

func targetCreateAction(entryType string) string {
	if entryType == domain.EntryTypeDocument {
		return "UPLOAD"
	}
	return "CREATE_FOLDER"
}

func childPath(parentPath *string, name string) string {
	if parentPath == nil || *parentPath == "" || *parentPath == "/" {
		return "/" + name
	}
	return strings.TrimRight(*parentPath, "/") + "/" + name
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func trimmedOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func validateIdempotencyKey(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 128 {
		return &domain.ValidationError{Field: "Idempotency-Key"}
	}
	return nil
}

func requestHash(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(encoded)
	return hash[:], nil
}

func claimIdempotency(ctx context.Context, repository Repository, actorID uuid.UUID, operation, key string, hash []byte, now time.Time) (*uuid.UUID, error) {
	recordID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	created, err := repository.TryCreateIdempotency(ctx, recordID, actorID, operation, key, hash, now.Add(idempotencyTTL), now)
	if err != nil {
		return nil, err
	}
	if created {
		return nil, nil
	}
	record, err := repository.GetIdempotency(ctx, actorID, operation, key)
	if err != nil {
		return nil, err
	}
	if string(record.RequestHash) != string(hash) {
		return nil, domain.ErrIdempotencyConflict
	}
	if record.Status != "COMPLETED" || record.ResultResourceID == nil {
		return nil, domain.ErrConflict
	}
	return record.ResultResourceID, nil
}

func insertEvent(ctx context.Context, repository Repository, aggregateType string, aggregateID uuid.UUID, version int64, eventType string, requestID uuid.UUID, now time.Time) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"aggregateId": aggregateID, "eventType": eventType, "occurredAt": now})
	if err != nil {
		return err
	}
	dedupHash := sha256.Sum256([]byte(eventType + aggregateID.String() + fmt.Sprint(version)))
	return repository.InsertEvent(ctx, domain.Event{ID: id, AggregateType: aggregateType, AggregateID: aggregateID, AggregateVersion: version, Type: eventType, Payload: payload, DeduplicationKey: hex.EncodeToString(dedupHash[:]), CorrelationID: requestID, CreatedAt: now})
}
