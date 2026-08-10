package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"file-workshop/backend/internal/modules/permissions/domain"

	"github.com/google/uuid"
)

const idempotencyTTL = 24 * time.Hour

type Service struct {
	repository Repository
	transactor Transactor
	cache      DecisionCache
	now        func() time.Time
}

func NewService(repository Repository, transactor Transactor, cache DecisionCache, now func() time.Time) *Service {
	return &Service{repository: repository, transactor: transactor, cache: cache, now: now}
}

func (s *Service) ListAdminDelegations(ctx context.Context, actor domain.Actor, organizationID *uuid.UUID, status *string, page, pageSize int) (domain.AdminDelegationListResult, error) {
	page, pageSize, err := domain.NormalizePage(page, pageSize)
	if err != nil {
		return domain.AdminDelegationListResult{}, err
	}
	if status != nil && *status != domain.StatusActive && *status != domain.StatusRevoked && *status != domain.StatusExpired && *status != domain.StatusInvalidated {
		return domain.AdminDelegationListResult{}, &domain.ValidationError{Field: "status"}
	}
	return s.repository.ListAdminDelegations(ctx, domain.AdminDelegationListFilter{ViewerUserID: actor.UserID, ViewerIsAdmin: actor.Role == domain.SystemRoleAdmin, OrganizationID: organizationID, Status: status, Page: page, PageSize: pageSize})
}

func (s *Service) GetAdminDelegation(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.AdminDelegation, error) {
	value, err := s.repository.GetAdminDelegation(ctx, id)
	if err != nil {
		return domain.AdminDelegation{}, err
	}
	if actor.Role != domain.SystemRoleAdmin && value.UserID != actor.UserID && value.GrantedByUserID != actor.UserID {
		return domain.AdminDelegation{}, domain.ErrDelegationNotFound
	}
	return value, nil
}

type CreateAdminDelegationInput struct {
	UserID             uuid.UUID
	OrganizationID     uuid.UUID
	Scope              string
	CanDelegate        bool
	ParentDelegationID *uuid.UUID
	Capabilities       []string
	ValidFrom          time.Time
	ValidUntil         *time.Time
	IdempotencyKey     string
	RequestID          uuid.UUID
}

func (s *Service) CreateAdminDelegation(ctx context.Context, actor domain.Actor, input CreateAdminDelegationInput) (domain.AdminDelegation, error) {
	now := s.now().UTC()
	id, err := uuid.NewV7()
	if err != nil {
		return domain.AdminDelegation{}, fmt.Errorf("generate admin delegation ID: %w", err)
	}
	value := domain.NewAdminDelegation{ID: id, UserID: input.UserID, OrganizationID: input.OrganizationID, Scope: input.Scope, CanDelegate: input.CanDelegate, ParentDelegationID: input.ParentDelegationID, Capabilities: slices.Clone(input.Capabilities), GrantedByUserID: actor.UserID, ValidFrom: input.ValidFrom.UTC(), ValidUntil: utcTime(input.ValidUntil), CreatedAt: now}
	if err := domain.ValidateDelegation(value); err != nil {
		return domain.AdminDelegation{}, err
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" || len(input.IdempotencyKey) > 128 {
		return domain.AdminDelegation{}, &domain.ValidationError{Field: "Idempotency-Key"}
	}
	if input.ParentDelegationID == nil && actor.Role != domain.SystemRoleAdmin {
		return domain.AdminDelegation{}, domain.ErrForbidden
	}
	hash, err := requestHash(input)
	if err != nil {
		return domain.AdminDelegation{}, err
	}
	var result domain.AdminDelegation
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replay, err := claimIdempotency(ctx, repository, actor.UserID, "CREATE_ADMIN_DELEGATION", input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replay != nil {
			result, err = repository.GetAdminDelegation(ctx, *replay)
			return err
		}
		if input.ParentDelegationID != nil {
			parent, err := repository.GetAdminDelegationForUpdate(ctx, *input.ParentDelegationID)
			if err != nil {
				return err
			}
			if parent.UserID != actor.UserID || !parent.CanDelegate || !slices.Contains(parent.Capabilities, domain.CapabilityDelegateAdmin) || !domain.Effective(parent.Status, parent.ValidFrom, parent.ValidUntil, now) {
				return domain.ErrForbidden
			}
			effective, effectiveErr := repository.AdminDelegationIsEffective(ctx, parent.ID, now)
			if effectiveErr != nil {
				return effectiveErr
			}
			if !effective {
				return domain.ErrForbidden
			}
			if parent.ParentDelegationID != nil && input.UserID == parent.GrantedByUserID {
				return domain.ErrInvalidDelegation
			}
		}
		result, err = repository.InsertAdminDelegation(ctx, value)
		if err != nil {
			return err
		}
		if err = repository.IncrementDelegationSecurityVersions(ctx, result.UserID, result.OrganizationID, now); err != nil {
			return err
		}
		if err = insertEvent(ctx, repository, "ADMIN_DELEGATION", result.ID, result.RowVersion, "ADMIN_DELEGATION_CREATED", input.RequestID, now); err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, "CREATE_ADMIN_DELEGATION", input.IdempotencyKey, result.ID, "ADMIN_DELEGATION", now)
	})
	return result, err
}

func (s *Service) RevokeAdminDelegation(ctx context.Context, actor domain.Actor, id uuid.UUID, rowVersion int64, reason string, requestID uuid.UUID) (domain.AdminDelegation, error) {
	if rowVersion < 1 || strings.TrimSpace(reason) == "" {
		return domain.AdminDelegation{}, &domain.ValidationError{Field: "rowVersion/reason"}
	}
	now := s.now().UTC()
	var result domain.AdminDelegation
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetAdminDelegationForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if current.Status != domain.StatusActive {
			return domain.ErrConflict
		}
		if actor.Role != domain.SystemRoleAdmin {
			if current.UserID == actor.UserID || current.GrantedByUserID != actor.UserID {
				return domain.ErrForbidden
			}
			if current.ParentDelegationID == nil {
				return domain.ErrForbidden
			}
			parent, err := repository.FindEffectiveAdminDelegation(ctx, actor.UserID, current.OrganizationID, domain.CapabilityDelegateAdmin, now)
			if err != nil || parent == nil {
				return domain.ErrForbidden
			}
		}
		result, err = repository.RevokeAdminDelegation(ctx, id, rowVersion, strings.TrimSpace(reason), now)
		if err != nil {
			return err
		}
		invalidated, err := repository.InvalidateDescendantDelegations(ctx, id, now)
		if err != nil {
			return err
		}
		if err = repository.IncrementDelegationSecurityVersions(ctx, result.UserID, result.OrganizationID, now); err != nil {
			return err
		}
		for _, descendantID := range invalidated {
			descendant, getErr := repository.GetAdminDelegation(ctx, descendantID)
			if getErr != nil {
				return getErr
			}
			if err = repository.IncrementDelegationSecurityVersions(ctx, descendant.UserID, descendant.OrganizationID, now); err != nil {
				return err
			}
			if err = insertEvent(ctx, repository, "ADMIN_DELEGATION", descendantID, descendant.RowVersion, "ADMIN_DELEGATION_INVALIDATED", requestID, now); err != nil {
				return err
			}
		}
		return insertEvent(ctx, repository, "ADMIN_DELEGATION", result.ID, result.RowVersion, "ADMIN_DELEGATION_REVOKED", requestID, now)
	})
	return result, err
}

func (s *Service) EvaluateAdminDelegation(ctx context.Context, actor domain.Actor, organizationID uuid.UUID, capability string) (bool, string, *uuid.UUID, error) {
	if _, ok := domain.Capabilities[capability]; !ok {
		return false, "", nil, &domain.ValidationError{Field: "capability"}
	}
	if actor.Role == domain.SystemRoleAdmin {
		return true, "SYSTEM_ADMIN", nil, nil
	}
	value, err := s.repository.FindEffectiveAdminDelegation(ctx, actor.UserID, organizationID, capability, s.now().UTC())
	if err != nil {
		return false, "", nil, err
	}
	if value == nil {
		return false, "NONE", nil, nil
	}
	return true, "ADMIN_DELEGATION", &value.ID, nil
}

func (s *Service) ListOrganizationAdministrators(ctx context.Context, actor domain.Actor, organizationID uuid.UUID, page, pageSize int) (domain.AdminDelegationListResult, error) {
	page, pageSize, err := domain.NormalizePage(page, pageSize)
	if err != nil {
		return domain.AdminDelegationListResult{}, err
	}
	exists, err := s.repository.OrganizationExists(ctx, organizationID)
	if err != nil {
		return domain.AdminDelegationListResult{}, err
	}
	if !exists {
		return domain.AdminDelegationListResult{}, domain.ErrNotFound
	}
	if actor.Role != domain.SystemRoleAdmin {
		allowed, _, _, err := s.EvaluateAdminDelegation(ctx, actor, organizationID, domain.CapabilityDelegateAdmin)
		if err != nil {
			return domain.AdminDelegationListResult{}, err
		}
		if !allowed {
			return domain.AdminDelegationListResult{}, domain.ErrForbidden
		}
	}
	return s.repository.ListOrganizationAdministrators(ctx, organizationID, page, pageSize, s.now().UTC())
}

type CreatePermissionGrantInput struct {
	SubjectType          string
	SubjectID            uuid.UUID
	ResourceType         string
	ResourceID           uuid.UUID
	Actions              []string
	InheritToDescendants bool
	GrantSource          string
	ValidFrom            time.Time
	ValidUntil           *time.Time
	GrantReason          *string
	IdempotencyKey       string
	RequestID            uuid.UUID
}

func (s *Service) CreatePermissionGrant(ctx context.Context, actor domain.Actor, input CreatePermissionGrantInput) (domain.PermissionGrant, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return domain.PermissionGrant{}, err
	}
	now := s.now().UTC()
	value := domain.NewPermissionGrant{ID: id, Actions: slices.Clone(input.Actions), InheritToDescendants: input.InheritToDescendants, GrantSource: input.GrantSource, ValidFrom: input.ValidFrom.UTC(), ValidUntil: utcTime(input.ValidUntil), GrantedByUserID: actor.UserID, GrantReason: domain.TrimmedOptional(input.GrantReason), CreatedAt: now}
	assignSubjectResource(&value, input.SubjectType, input.SubjectID, input.ResourceType, input.ResourceID)
	if err = domain.ValidateGrant(value); err != nil {
		return domain.PermissionGrant{}, err
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" || len(input.IdempotencyKey) > 128 {
		return domain.PermissionGrant{}, &domain.ValidationError{Field: "Idempotency-Key"}
	}
	hash, err := requestHash(input)
	if err != nil {
		return domain.PermissionGrant{}, err
	}
	var result domain.PermissionGrant
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replay, err := claimIdempotency(ctx, repository, actor.UserID, "CREATE_PERMISSION_GRANT", input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replay != nil {
			result, err = repository.GetPermissionGrant(ctx, *replay)
			return err
		}
		resource, err := repository.GetResource(ctx, input.ResourceType, input.ResourceID)
		if err != nil {
			return err
		}
		if err = s.requireManagePermission(ctx, repository, actor, resource, now); err != nil {
			return err
		}
		result, err = repository.InsertPermissionGrant(ctx, value)
		if err != nil {
			return err
		}
		if err = repository.IncrementGrantSecurityVersions(ctx, result, resource, now); err != nil {
			return err
		}
		if err = insertEvent(ctx, repository, "PERMISSION_GRANT", result.ID, result.RowVersion, "PERMISSION_GRANT_CREATED", input.RequestID, now); err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, "CREATE_PERMISSION_GRANT", input.IdempotencyKey, result.ID, "PERMISSION_GRANT", now)
	})
	return result, err
}

func (s *Service) ListResourcePermissionGrants(ctx context.Context, actor domain.Actor, resourceType string, resourceID uuid.UUID, page, pageSize int) (domain.PermissionGrantListResult, error) {
	page, pageSize, err := domain.NormalizePage(page, pageSize)
	if err != nil {
		return domain.PermissionGrantListResult{}, err
	}
	resource, err := s.repository.GetResource(ctx, resourceType, resourceID)
	if err != nil {
		return domain.PermissionGrantListResult{}, err
	}
	if err = s.requireManagePermission(ctx, s.repository, actor, resource, s.now().UTC()); err != nil {
		return domain.PermissionGrantListResult{}, err
	}
	return s.repository.ListDirectPermissionGrants(ctx, resourceType, resourceID, page, pageSize)
}

func (s *Service) UpdatePermissionGrant(ctx context.Context, actor domain.Actor, id uuid.UUID, actions []string, inherit bool, validUntil *time.Time, reason *string, rowVersion int64, requestID uuid.UUID) (domain.PermissionGrant, error) {
	if rowVersion < 1 {
		return domain.PermissionGrant{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if err := domain.ValidateActions(actions); err != nil {
		return domain.PermissionGrant{}, err
	}
	now := s.now().UTC()
	var result domain.PermissionGrant
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetPermissionGrantForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if current.Status != domain.StatusActive {
			return domain.ErrConflict
		}
		if current.DocumentID != nil && inherit {
			return &domain.ValidationError{Field: "inheritToDescendants"}
		}
		until := utcTime(validUntil)
		if until != nil && !until.After(current.ValidFrom) {
			return &domain.ValidationError{Field: "validUntil"}
		}
		resourceType, resourceID := current.Resource()
		resource, err := repository.GetResource(ctx, resourceType, resourceID)
		if err != nil {
			return err
		}
		if err = s.requireManagePermission(ctx, repository, actor, resource, now); err != nil {
			return err
		}
		result, err = repository.UpdatePermissionGrant(ctx, id, actions, inherit, until, domain.TrimmedOptional(reason), rowVersion, now)
		if err != nil {
			return err
		}
		if err = repository.IncrementGrantSecurityVersions(ctx, result, resource, now); err != nil {
			return err
		}
		return insertEvent(ctx, repository, "PERMISSION_GRANT", result.ID, result.RowVersion, "PERMISSION_GRANT_UPDATED", requestID, now)
	})
	return result, err
}

func (s *Service) RevokePermissionGrant(ctx context.Context, actor domain.Actor, id uuid.UUID, rowVersion int64, reason string, requestID uuid.UUID) (domain.PermissionGrant, error) {
	if rowVersion < 1 || strings.TrimSpace(reason) == "" {
		return domain.PermissionGrant{}, &domain.ValidationError{Field: "rowVersion/reason"}
	}
	now := s.now().UTC()
	var result domain.PermissionGrant
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetPermissionGrantForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if current.Status != domain.StatusActive {
			return domain.ErrConflict
		}
		resourceType, resourceID := current.Resource()
		resource, err := repository.GetResource(ctx, resourceType, resourceID)
		if err != nil {
			return err
		}
		if err = s.requireManagePermission(ctx, repository, actor, resource, now); err != nil {
			return err
		}
		result, err = repository.RevokePermissionGrant(ctx, id, actor.UserID, strings.TrimSpace(reason), rowVersion, now)
		if err != nil {
			return err
		}
		if err = repository.IncrementGrantSecurityVersions(ctx, result, resource, now); err != nil {
			return err
		}
		return insertEvent(ctx, repository, "PERMISSION_GRANT", result.ID, result.RowVersion, "PERMISSION_GRANT_REVOKED", requestID, now)
	})
	return result, err
}

func (s *Service) EvaluatePermission(ctx context.Context, actor domain.Actor, resourceType string, resourceID uuid.UUID, action string, privilegedReason *string, privilegedAccessConfirmed bool) (domain.PermissionEvaluation, error) {
	if _, ok := domain.Actions[action]; !ok {
		return domain.PermissionEvaluation{}, &domain.ValidationError{Field: "action"}
	}
	resource, err := s.repository.GetResource(ctx, resourceType, resourceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return denied(resourceType, resourceID, action), nil
		}
		return domain.PermissionEvaluation{}, err
	}
	return s.evaluateResource(ctx, s.repository, actor, resource, action, privilegedReason, privilegedAccessConfirmed, s.now().UTC())
}

func (s *Service) BatchEvaluatePermissions(ctx context.Context, actor domain.Actor, inputs []PermissionCheckInput) ([]domain.PermissionEvaluation, error) {
	if len(inputs) == 0 || len(inputs) > 100 {
		return nil, &domain.ValidationError{Field: "items"}
	}
	results := make([]domain.PermissionEvaluation, 0, len(inputs))
	for _, input := range inputs {
		result, err := s.EvaluatePermission(ctx, actor, input.ResourceType, input.ResourceID, input.Action, input.PrivilegedReason, input.PrivilegedAccessConfirmed)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

type PermissionCheckInput struct {
	ResourceType              string
	ResourceID                uuid.UUID
	Action                    string
	PrivilegedReason          *string
	PrivilegedAccessConfirmed bool
}

func (s *Service) ChangeInheritance(ctx context.Context, actor domain.Actor, resourceType string, resourceID uuid.UUID, mode string, rowVersion int64, requestID uuid.UUID) (domain.InheritanceResult, error) {
	if resourceType != domain.ResourceFolder && resourceType != domain.ResourceDocument {
		return domain.InheritanceResult{}, &domain.ValidationError{Field: "resourceType"}
	}
	if mode != domain.InheritanceBreak && mode != domain.InheritanceInherit {
		return domain.InheritanceResult{}, &domain.ValidationError{Field: "inheritanceMode"}
	}
	if rowVersion < 1 {
		return domain.InheritanceResult{}, &domain.ValidationError{Field: "rowVersion"}
	}
	now := s.now().UTC()
	var result domain.InheritanceResult
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		resource, err := repository.GetResource(ctx, resourceType, resourceID)
		if err != nil {
			return err
		}
		if err = s.requireManagePermission(ctx, repository, actor, resource, now); err != nil {
			return err
		}
		result, err = repository.ChangeInheritance(ctx, resourceType, resourceID, mode, rowVersion, now)
		if err != nil {
			return err
		}
		placeholder := domain.PermissionGrant{GrantedByUserID: actor.UserID, SpaceID: &resource.SpaceID}
		spaceResource := domain.Resource{Type: domain.ResourceSpace, ID: resource.SpaceID, SpaceID: resource.SpaceID}
		if err = repository.IncrementGrantSecurityVersions(ctx, placeholder, spaceResource, now); err != nil {
			return err
		}
		eventType := "PERMISSION_INHERITANCE_RESTORED"
		if mode == domain.InheritanceBreak {
			eventType = "PERMISSION_INHERITANCE_BROKEN"
		}
		return insertEvent(ctx, repository, resourceType, resourceID, result.RowVersion, eventType, requestID, now)
	})
	return result, err
}

func (s *Service) requireManagePermission(ctx context.Context, repository Repository, actor domain.Actor, resource domain.Resource, now time.Time) error {
	result, err := s.evaluateResource(ctx, repository, actor, resource, "MANAGE_PERMISSION", nil, false, now)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return domain.ErrForbidden
	}
	return nil
}

func (s *Service) evaluateResource(ctx context.Context, repository Repository, actor domain.Actor, resource domain.Resource, action string, privilegedReason *string, privilegedAccessConfirmed bool, now time.Time) (domain.PermissionEvaluation, error) {
	result := denied(resource.Type, resource.ID, action)
	if resource.SpaceType == "PERSONAL" && resource.SpaceOwnerUserID != nil && *resource.SpaceOwnerUserID == actor.UserID {
		result.Allowed, result.Source = true, "PERSONAL_OWNER"
		return result, nil
	}
	if actor.Role == domain.SystemRoleAdmin {
		result.Allowed, result.Source = true, "SYSTEM_ADMIN"
		result.PrivilegedAccessRequired = resource.SpaceType == "PERSONAL" || action == "DOWNLOAD" || action == "PREVIEW" || action == "RESTORE" || action == "PURGE"
		if result.PrivilegedAccessRequired && (domain.TrimmedOptional(privilegedReason) == nil || !privilegedAccessConfirmed) {
			result.Allowed = false
		}
		return result, nil
	}
	cacheKey := ""
	if s.cache != nil {
		version, err := repository.GetAuthorizationVersion(ctx, actor.UserID, resource.SpaceID, now)
		if err != nil {
			return domain.PermissionEvaluation{}, err
		}
		digest := sha256.Sum256([]byte(version + ":" + actor.UserID.String() + ":" + resource.Type + ":" + resource.ID.String() + ":" + action))
		cacheKey = "file-workshop:authorization:" + hex.EncodeToString(digest[:])
		if cached, ok := s.cache.Get(ctx, cacheKey); ok {
			return cached, nil
		}
	}
	if resource.SpaceType == "ORGANIZATION" && resource.OrganizationID != nil {
		capability := capabilityForAction(action)
		delegation, err := repository.FindEffectiveAdminDelegation(ctx, actor.UserID, *resource.OrganizationID, capability, now)
		if err != nil {
			return domain.PermissionEvaluation{}, err
		}
		if delegation != nil {
			result.Allowed, result.Source = true, "ADMIN_DELEGATION"
			if cacheKey != "" {
				s.cache.Set(ctx, cacheKey, result)
			}
			return result, nil
		}
	}
	organizations, err := repository.ListActiveUserOrganizations(ctx, actor.UserID, now)
	if err != nil {
		return domain.PermissionEvaluation{}, err
	}
	grants, err := repository.ListCandidatePermissionGrants(ctx, actor.UserID, organizations, resource, now)
	if err != nil {
		return domain.PermissionEvaluation{}, err
	}
	direct := false
	for _, grant := range grants {
		if !slices.Contains(grant.Actions, action) || !grantApplies(grant, resource) {
			continue
		}
		result.MatchedGrantIDs = append(result.MatchedGrantIDs, grant.ID)
		grantType, grantID := grant.Resource()
		if grantType == resource.Type && grantID == resource.ID {
			direct = true
		}
	}
	if len(result.MatchedGrantIDs) > 0 {
		result.Allowed = true
		result.Source = "INHERITED_GRANT"
		if direct {
			result.Source = "DIRECT_GRANT"
		}
	}
	if !result.Allowed && isShareAction(action) {
		shares, err := repository.ListCandidateShareGrants(ctx, actor.UserID, organizations, resource, now)
		if err != nil {
			return domain.PermissionEvaluation{}, err
		}
		for _, share := range shares {
			if slices.Contains(share.Actions, action) {
				result.Allowed = true
				result.Source = "SHARE"
				result.MatchedGrantIDs = append(result.MatchedGrantIDs, share.ID)
			}
		}
	}
	if cacheKey != "" {
		s.cache.Set(ctx, cacheKey, result)
	}
	return result, nil
}

func isShareAction(action string) bool {
	switch action {
	case "READ_METADATA", "PREVIEW", "DOWNLOAD", "WRITE_CONTENT":
		return true
	default:
		return false
	}
}

func grantApplies(grant domain.PermissionGrant, resource domain.Resource) bool {
	grantType, grantID := grant.Resource()
	if grantType == resource.Type && grantID == resource.ID {
		return true
	}
	if !grant.InheritToDescendants || resource.InheritanceMode == domain.InheritanceBreak {
		return false
	}
	if grantType == domain.ResourceDocument {
		return false
	}
	if grantType == domain.ResourceSpace {
		for _, ancestor := range resource.FolderAncestors {
			if ancestor.InheritanceMode == domain.InheritanceBreak {
				return false
			}
		}
		return grantID == resource.SpaceID
	}
	for _, ancestor := range resource.FolderAncestors {
		if ancestor.ID == grantID {
			return true
		}
		if ancestor.InheritanceMode == domain.InheritanceBreak {
			return false
		}
	}
	return false
}

func capabilityForAction(action string) string {
	switch action {
	case "MANAGE_PERMISSION":
		return domain.CapabilityManagePermission
	case "RESTORE", "PURGE":
		return domain.CapabilityManageRecycleBin
	case "LOCK":
		return domain.CapabilityForceUnlock
	default:
		return domain.CapabilityManageContent
	}
}

func denied(resourceType string, resourceID uuid.UUID, action string) domain.PermissionEvaluation {
	return domain.PermissionEvaluation{ResourceType: resourceType, ResourceID: resourceID, Action: action, Allowed: false, Source: "NONE", MatchedGrantIDs: []uuid.UUID{}}
}

func assignSubjectResource(value *domain.NewPermissionGrant, subjectType string, subjectID uuid.UUID, resourceType string, resourceID uuid.UUID) {
	if subjectType == domain.SubjectUser {
		value.SubjectUserID = &subjectID
	} else if subjectType == domain.SubjectOrganization {
		value.SubjectOrganizationID = &subjectID
	}
	if resourceType == domain.ResourceSpace {
		value.SpaceID = &resourceID
	} else if resourceType == domain.ResourceFolder {
		value.FolderID = &resourceID
	} else if resourceType == domain.ResourceDocument {
		value.DocumentID = &resourceID
	}
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
	if !slices.Equal(record.RequestHash, hash) {
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
	dedup := hex.EncodeToString(dedupHash[:])
	return repository.InsertEvent(ctx, domain.Event{ID: id, AggregateType: aggregateType, AggregateID: aggregateID, AggregateVersion: version, Type: eventType, Payload: payload, DeduplicationKey: dedup, CorrelationID: requestID, CreatedAt: now})
}

func utcTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	converted := value.UTC()
	return &converted
}
