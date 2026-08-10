package httpserver

import (
	"context"
	"fmt"

	"file-workshop/backend/api"
)

type HealthAPI interface {
	GetLiveness(context.Context, api.GetLivenessRequestObject) (api.GetLivenessResponseObject, error)
	GetReadiness(context.Context, api.GetReadinessRequestObject) (api.GetReadinessResponseObject, error)
}

type IdentityAPI interface {
	Login(context.Context, api.LoginRequestObject) (api.LoginResponseObject, error)
	Logout(context.Context, api.LogoutRequestObject) (api.LogoutResponseObject, error)
	RefreshSession(context.Context, api.RefreshSessionRequestObject) (api.RefreshSessionResponseObject, error)
	GetCurrentSession(context.Context, api.GetCurrentSessionRequestObject) (api.GetCurrentSessionResponseObject, error)
}

type UsersAPI interface {
	GetCurrentUser(context.Context, api.GetCurrentUserRequestObject) (api.GetCurrentUserResponseObject, error)
	UpdateCurrentUser(context.Context, api.UpdateCurrentUserRequestObject) (api.UpdateCurrentUserResponseObject, error)
	ListCurrentUserSessions(context.Context, api.ListCurrentUserSessionsRequestObject) (api.ListCurrentUserSessionsResponseObject, error)
	RevokeCurrentUserSession(context.Context, api.RevokeCurrentUserSessionRequestObject) (api.RevokeCurrentUserSessionResponseObject, error)
	ListUsers(context.Context, api.ListUsersRequestObject) (api.ListUsersResponseObject, error)
	CreateUser(context.Context, api.CreateUserRequestObject) (api.CreateUserResponseObject, error)
	GetUser(context.Context, api.GetUserRequestObject) (api.GetUserResponseObject, error)
	UpdateUser(context.Context, api.UpdateUserRequestObject) (api.UpdateUserResponseObject, error)
	DeleteUser(context.Context, api.DeleteUserRequestObject) (api.DeleteUserResponseObject, error)
	DisableUser(context.Context, api.DisableUserRequestObject) (api.DisableUserResponseObject, error)
	EnableUser(context.Context, api.EnableUserRequestObject) (api.EnableUserResponseObject, error)
	LockUser(context.Context, api.LockUserRequestObject) (api.LockUserResponseObject, error)
	ResetUserPassword(context.Context, api.ResetUserPasswordRequestObject) (api.ResetUserPasswordResponseObject, error)
}

type OrganizationsAPI interface {
	ListOrganizationChangePlans(context.Context, api.ListOrganizationChangePlansRequestObject) (api.ListOrganizationChangePlansResponseObject, error)
	CreateOrganizationChangePlan(context.Context, api.CreateOrganizationChangePlanRequestObject) (api.CreateOrganizationChangePlanResponseObject, error)
	GetOrganizationChangePlan(context.Context, api.GetOrganizationChangePlanRequestObject) (api.GetOrganizationChangePlanResponseObject, error)
	AddOrganizationChangeOperation(context.Context, api.AddOrganizationChangeOperationRequestObject) (api.AddOrganizationChangeOperationResponseObject, error)
	TransitionOrganizationChangePlan(context.Context, api.TransitionOrganizationChangePlanRequestObject) (api.TransitionOrganizationChangePlanResponseObject, error)
	ListOrganizations(context.Context, api.ListOrganizationsRequestObject) (api.ListOrganizationsResponseObject, error)
	CreateOrganization(context.Context, api.CreateOrganizationRequestObject) (api.CreateOrganizationResponseObject, error)
	GetOrganization(context.Context, api.GetOrganizationRequestObject) (api.GetOrganizationResponseObject, error)
	UpdateOrganization(context.Context, api.UpdateOrganizationRequestObject) (api.UpdateOrganizationResponseObject, error)
	ListOrganizationMembers(context.Context, api.ListOrganizationMembersRequestObject) (api.ListOrganizationMembersResponseObject, error)
	AddOrganizationMember(context.Context, api.AddOrganizationMemberRequestObject) (api.AddOrganizationMemberResponseObject, error)
	RemoveOrganizationMember(context.Context, api.RemoveOrganizationMemberRequestObject) (api.RemoveOrganizationMemberResponseObject, error)
	UpdateOrganizationMember(context.Context, api.UpdateOrganizationMemberRequestObject) (api.UpdateOrganizationMemberResponseObject, error)
	MoveOrganization(context.Context, api.MoveOrganizationRequestObject) (api.MoveOrganizationResponseObject, error)
	ChangeOrganizationStatus(context.Context, api.ChangeOrganizationStatusRequestObject) (api.ChangeOrganizationStatusResponseObject, error)
	ListSpaces(context.Context, api.ListSpacesRequestObject) (api.ListSpacesResponseObject, error)
	CreatePublicSpace(context.Context, api.CreatePublicSpaceRequestObject) (api.CreatePublicSpaceResponseObject, error)
	GetSpace(context.Context, api.GetSpaceRequestObject) (api.GetSpaceResponseObject, error)
	UpdateSpace(context.Context, api.UpdateSpaceRequestObject) (api.UpdateSpaceResponseObject, error)
	ChangeSpaceStatus(context.Context, api.ChangeSpaceStatusRequestObject) (api.ChangeSpaceStatusResponseObject, error)
	ProvisionUserPersonalSpace(context.Context, api.ProvisionUserPersonalSpaceRequestObject) (api.ProvisionUserPersonalSpaceResponseObject, error)
	ListCurrentUserOrganizations(context.Context, api.ListCurrentUserOrganizationsRequestObject) (api.ListCurrentUserOrganizationsResponseObject, error)
	GetCurrentUserPersonalSpace(context.Context, api.GetCurrentUserPersonalSpaceRequestObject) (api.GetCurrentUserPersonalSpaceResponseObject, error)
}

type PermissionsAPI interface {
	ListAdminDelegations(context.Context, api.ListAdminDelegationsRequestObject) (api.ListAdminDelegationsResponseObject, error)
	CreateAdminDelegation(context.Context, api.CreateAdminDelegationRequestObject) (api.CreateAdminDelegationResponseObject, error)
	GetAdminDelegation(context.Context, api.GetAdminDelegationRequestObject) (api.GetAdminDelegationResponseObject, error)
	RevokeAdminDelegation(context.Context, api.RevokeAdminDelegationRequestObject) (api.RevokeAdminDelegationResponseObject, error)
	ListOrganizationAdministrators(context.Context, api.ListOrganizationAdministratorsRequestObject) (api.ListOrganizationAdministratorsResponseObject, error)
	EvaluateAdminDelegation(context.Context, api.EvaluateAdminDelegationRequestObject) (api.EvaluateAdminDelegationResponseObject, error)
	ListResourcePermissionGrants(context.Context, api.ListResourcePermissionGrantsRequestObject) (api.ListResourcePermissionGrantsResponseObject, error)
	CreatePermissionGrant(context.Context, api.CreatePermissionGrantRequestObject) (api.CreatePermissionGrantResponseObject, error)
	UpdatePermissionGrant(context.Context, api.UpdatePermissionGrantRequestObject) (api.UpdatePermissionGrantResponseObject, error)
	RevokePermissionGrant(context.Context, api.RevokePermissionGrantRequestObject) (api.RevokePermissionGrantResponseObject, error)
	EvaluatePermission(context.Context, api.EvaluatePermissionRequestObject) (api.EvaluatePermissionResponseObject, error)
	BatchEvaluatePermissions(context.Context, api.BatchEvaluatePermissionsRequestObject) (api.BatchEvaluatePermissionsResponseObject, error)
	BreakPermissionInheritance(context.Context, api.BreakPermissionInheritanceRequestObject) (api.BreakPermissionInheritanceResponseObject, error)
	RestorePermissionInheritance(context.Context, api.RestorePermissionInheritanceRequestObject) (api.RestorePermissionInheritanceResponseObject, error)
}

type FilesAPI interface {
	ListDirectoryEntries(context.Context, api.ListDirectoryEntriesRequestObject) (api.ListDirectoryEntriesResponseObject, error)
	CreateFolder(context.Context, api.CreateFolderRequestObject) (api.CreateFolderResponseObject, error)
	CreateDocument(context.Context, api.CreateDocumentRequestObject) (api.CreateDocumentResponseObject, error)
	GetDirectoryEntry(context.Context, api.GetDirectoryEntryRequestObject) (api.GetDirectoryEntryResponseObject, error)
	RenameDirectoryEntry(context.Context, api.RenameDirectoryEntryRequestObject) (api.RenameDirectoryEntryResponseObject, error)
	MoveDirectoryEntry(context.Context, api.MoveDirectoryEntryRequestObject) (api.MoveDirectoryEntryResponseObject, error)
}

type UploadsAPI interface {
	CreateUploadSession(context.Context, api.CreateUploadSessionRequestObject) (api.CreateUploadSessionResponseObject, error)
	GetUploadSession(context.Context, api.GetUploadSessionRequestObject) (api.GetUploadSessionResponseObject, error)
	PresignUploadPart(context.Context, api.PresignUploadPartRequestObject) (api.PresignUploadPartResponseObject, error)
	AbortUploadSession(context.Context, api.AbortUploadSessionRequestObject) (api.AbortUploadSessionResponseObject, error)
}

type VersionsAPI interface {
	ListDocumentVersions(context.Context, api.ListDocumentVersionsRequestObject) (api.ListDocumentVersionsResponseObject, error)
	RestoreDocumentVersion(context.Context, api.RestoreDocumentVersionRequestObject) (api.RestoreDocumentVersionResponseObject, error)
	GetDocumentLock(context.Context, api.GetDocumentLockRequestObject) (api.GetDocumentLockResponseObject, error)
	AcquireDocumentLock(context.Context, api.AcquireDocumentLockRequestObject) (api.AcquireDocumentLockResponseObject, error)
	HeartbeatDocumentLock(context.Context, api.HeartbeatDocumentLockRequestObject) (api.HeartbeatDocumentLockResponseObject, error)
	ReleaseDocumentLock(context.Context, api.ReleaseDocumentLockRequestObject) (api.ReleaseDocumentLockResponseObject, error)
	ForceReleaseDocumentLock(context.Context, api.ForceReleaseDocumentLockRequestObject) (api.ForceReleaseDocumentLockResponseObject, error)
}

type BackgroundAPI interface {
	ListBackgroundOutboxEvents(context.Context, api.ListBackgroundOutboxEventsRequestObject) (api.ListBackgroundOutboxEventsResponseObject, error)
	RetryBackgroundOutboxEvent(context.Context, api.RetryBackgroundOutboxEventRequestObject) (api.RetryBackgroundOutboxEventResponseObject, error)
	ListBackgroundJobs(context.Context, api.ListBackgroundJobsRequestObject) (api.ListBackgroundJobsResponseObject, error)
	RetryBackgroundJob(context.Context, api.RetryBackgroundJobRequestObject) (api.RetryBackgroundJobResponseObject, error)
}

type AuditAPI interface {
	ListAuditEvents(context.Context, api.ListAuditEventsRequestObject) (api.ListAuditEventsResponseObject, error)
	GetAuditEvent(context.Context, api.GetAuditEventRequestObject) (api.GetAuditEventResponseObject, error)
	GetAuditIntegrity(context.Context, api.GetAuditIntegrityRequestObject) (api.GetAuditIntegrityResponseObject, error)
	VerifyAuditIntegrity(context.Context, api.VerifyAuditIntegrityRequestObject) (api.VerifyAuditIntegrityResponseObject, error)
}

type APIHandler struct {
	health        HealthAPI
	identity      IdentityAPI
	users         UsersAPI
	organizations OrganizationsAPI
	permissions   PermissionsAPI
	files         FilesAPI
	uploads       UploadsAPI
	versions      VersionsAPI
	background    BackgroundAPI
	audit         AuditAPI
}

func NewAPIHandler(health HealthAPI, identity IdentityAPI, users UsersAPI, organizations OrganizationsAPI, optionalHandlers ...any) *APIHandler {
	var permissions PermissionsAPI
	var files FilesAPI
	var uploads UploadsAPI
	var versions VersionsAPI
	var background BackgroundAPI
	var audit AuditAPI
	for _, optional := range optionalHandlers {
		if handler, ok := optional.(PermissionsAPI); ok {
			permissions = handler
		}
		if handler, ok := optional.(FilesAPI); ok {
			files = handler
		}
		if handler, ok := optional.(UploadsAPI); ok {
			uploads = handler
		}
		if handler, ok := optional.(VersionsAPI); ok {
			versions = handler
		}
		if handler, ok := optional.(BackgroundAPI); ok {
			background = handler
		}
		if handler, ok := optional.(AuditAPI); ok {
			audit = handler
		}
	}
	return &APIHandler{health: health, identity: identity, users: users, organizations: organizations, permissions: permissions, files: files, uploads: uploads, versions: versions, background: background, audit: audit}
}

func (h *APIHandler) ListAuditEvents(ctx context.Context, request api.ListAuditEventsRequestObject) (api.ListAuditEventsResponseObject, error) {
	return h.audit.ListAuditEvents(ctx, request)
}
func (h *APIHandler) GetAuditEvent(ctx context.Context, request api.GetAuditEventRequestObject) (api.GetAuditEventResponseObject, error) {
	return h.audit.GetAuditEvent(ctx, request)
}
func (h *APIHandler) GetAuditIntegrity(ctx context.Context, request api.GetAuditIntegrityRequestObject) (api.GetAuditIntegrityResponseObject, error) {
	return h.audit.GetAuditIntegrity(ctx, request)
}
func (h *APIHandler) VerifyAuditIntegrity(ctx context.Context, request api.VerifyAuditIntegrityRequestObject) (api.VerifyAuditIntegrityResponseObject, error) {
	return h.audit.VerifyAuditIntegrity(ctx, request)
}

func (h *APIHandler) ListBackgroundOutboxEvents(ctx context.Context, request api.ListBackgroundOutboxEventsRequestObject) (api.ListBackgroundOutboxEventsResponseObject, error) {
	return h.background.ListBackgroundOutboxEvents(ctx, request)
}
func (h *APIHandler) RetryBackgroundOutboxEvent(ctx context.Context, request api.RetryBackgroundOutboxEventRequestObject) (api.RetryBackgroundOutboxEventResponseObject, error) {
	return h.background.RetryBackgroundOutboxEvent(ctx, request)
}
func (h *APIHandler) ListBackgroundJobs(ctx context.Context, request api.ListBackgroundJobsRequestObject) (api.ListBackgroundJobsResponseObject, error) {
	return h.background.ListBackgroundJobs(ctx, request)
}
func (h *APIHandler) RetryBackgroundJob(ctx context.Context, request api.RetryBackgroundJobRequestObject) (api.RetryBackgroundJobResponseObject, error) {
	return h.background.RetryBackgroundJob(ctx, request)
}

func (h *APIHandler) ListDirectoryEntries(ctx context.Context, request api.ListDirectoryEntriesRequestObject) (api.ListDirectoryEntriesResponseObject, error) {
	return h.files.ListDirectoryEntries(ctx, request)
}
func (h *APIHandler) CreateFolder(ctx context.Context, request api.CreateFolderRequestObject) (api.CreateFolderResponseObject, error) {
	return h.files.CreateFolder(ctx, request)
}
func (h *APIHandler) CreateDocument(ctx context.Context, request api.CreateDocumentRequestObject) (api.CreateDocumentResponseObject, error) {
	return h.files.CreateDocument(ctx, request)
}
func (h *APIHandler) GetDirectoryEntry(ctx context.Context, request api.GetDirectoryEntryRequestObject) (api.GetDirectoryEntryResponseObject, error) {
	return h.files.GetDirectoryEntry(ctx, request)
}
func (h *APIHandler) RenameDirectoryEntry(ctx context.Context, request api.RenameDirectoryEntryRequestObject) (api.RenameDirectoryEntryResponseObject, error) {
	return h.files.RenameDirectoryEntry(ctx, request)
}
func (h *APIHandler) MoveDirectoryEntry(ctx context.Context, request api.MoveDirectoryEntryRequestObject) (api.MoveDirectoryEntryResponseObject, error) {
	return h.files.MoveDirectoryEntry(ctx, request)
}

func (h *APIHandler) CreateUploadSession(ctx context.Context, request api.CreateUploadSessionRequestObject) (api.CreateUploadSessionResponseObject, error) {
	return h.uploads.CreateUploadSession(ctx, request)
}
func (h *APIHandler) GetUploadSession(ctx context.Context, request api.GetUploadSessionRequestObject) (api.GetUploadSessionResponseObject, error) {
	return h.uploads.GetUploadSession(ctx, request)
}
func (h *APIHandler) PresignUploadPart(ctx context.Context, request api.PresignUploadPartRequestObject) (api.PresignUploadPartResponseObject, error) {
	return h.uploads.PresignUploadPart(ctx, request)
}
func (h *APIHandler) AbortUploadSession(ctx context.Context, request api.AbortUploadSessionRequestObject) (api.AbortUploadSessionResponseObject, error) {
	return h.uploads.AbortUploadSession(ctx, request)
}

func (h *APIHandler) ListDocumentVersions(ctx context.Context, request api.ListDocumentVersionsRequestObject) (api.ListDocumentVersionsResponseObject, error) {
	return h.versions.ListDocumentVersions(ctx, request)
}
func (h *APIHandler) RestoreDocumentVersion(ctx context.Context, request api.RestoreDocumentVersionRequestObject) (api.RestoreDocumentVersionResponseObject, error) {
	return h.versions.RestoreDocumentVersion(ctx, request)
}
func (h *APIHandler) GetDocumentLock(ctx context.Context, request api.GetDocumentLockRequestObject) (api.GetDocumentLockResponseObject, error) {
	return h.versions.GetDocumentLock(ctx, request)
}
func (h *APIHandler) AcquireDocumentLock(ctx context.Context, request api.AcquireDocumentLockRequestObject) (api.AcquireDocumentLockResponseObject, error) {
	return h.versions.AcquireDocumentLock(ctx, request)
}
func (h *APIHandler) HeartbeatDocumentLock(ctx context.Context, request api.HeartbeatDocumentLockRequestObject) (api.HeartbeatDocumentLockResponseObject, error) {
	return h.versions.HeartbeatDocumentLock(ctx, request)
}
func (h *APIHandler) ReleaseDocumentLock(ctx context.Context, request api.ReleaseDocumentLockRequestObject) (api.ReleaseDocumentLockResponseObject, error) {
	return h.versions.ReleaseDocumentLock(ctx, request)
}
func (h *APIHandler) ForceReleaseDocumentLock(ctx context.Context, request api.ForceReleaseDocumentLockRequestObject) (api.ForceReleaseDocumentLockResponseObject, error) {
	return h.versions.ForceReleaseDocumentLock(ctx, request)
}

func (h *APIHandler) ListAdminDelegations(ctx context.Context, request api.ListAdminDelegationsRequestObject) (api.ListAdminDelegationsResponseObject, error) {
	return h.permissions.ListAdminDelegations(ctx, request)
}
func (h *APIHandler) CreateAdminDelegation(ctx context.Context, request api.CreateAdminDelegationRequestObject) (api.CreateAdminDelegationResponseObject, error) {
	return h.permissions.CreateAdminDelegation(ctx, request)
}
func (h *APIHandler) GetAdminDelegation(ctx context.Context, request api.GetAdminDelegationRequestObject) (api.GetAdminDelegationResponseObject, error) {
	return h.permissions.GetAdminDelegation(ctx, request)
}
func (h *APIHandler) RevokeAdminDelegation(ctx context.Context, request api.RevokeAdminDelegationRequestObject) (api.RevokeAdminDelegationResponseObject, error) {
	return h.permissions.RevokeAdminDelegation(ctx, request)
}
func (h *APIHandler) ListOrganizationAdministrators(ctx context.Context, request api.ListOrganizationAdministratorsRequestObject) (api.ListOrganizationAdministratorsResponseObject, error) {
	return h.permissions.ListOrganizationAdministrators(ctx, request)
}
func (h *APIHandler) EvaluateAdminDelegation(ctx context.Context, request api.EvaluateAdminDelegationRequestObject) (api.EvaluateAdminDelegationResponseObject, error) {
	return h.permissions.EvaluateAdminDelegation(ctx, request)
}
func (h *APIHandler) ListResourcePermissionGrants(ctx context.Context, request api.ListResourcePermissionGrantsRequestObject) (api.ListResourcePermissionGrantsResponseObject, error) {
	return h.permissions.ListResourcePermissionGrants(ctx, request)
}
func (h *APIHandler) CreatePermissionGrant(ctx context.Context, request api.CreatePermissionGrantRequestObject) (api.CreatePermissionGrantResponseObject, error) {
	return h.permissions.CreatePermissionGrant(ctx, request)
}
func (h *APIHandler) UpdatePermissionGrant(ctx context.Context, request api.UpdatePermissionGrantRequestObject) (api.UpdatePermissionGrantResponseObject, error) {
	return h.permissions.UpdatePermissionGrant(ctx, request)
}
func (h *APIHandler) RevokePermissionGrant(ctx context.Context, request api.RevokePermissionGrantRequestObject) (api.RevokePermissionGrantResponseObject, error) {
	return h.permissions.RevokePermissionGrant(ctx, request)
}
func (h *APIHandler) EvaluatePermission(ctx context.Context, request api.EvaluatePermissionRequestObject) (api.EvaluatePermissionResponseObject, error) {
	return h.permissions.EvaluatePermission(ctx, request)
}
func (h *APIHandler) BatchEvaluatePermissions(ctx context.Context, request api.BatchEvaluatePermissionsRequestObject) (api.BatchEvaluatePermissionsResponseObject, error) {
	return h.permissions.BatchEvaluatePermissions(ctx, request)
}
func (h *APIHandler) BreakPermissionInheritance(ctx context.Context, request api.BreakPermissionInheritanceRequestObject) (api.BreakPermissionInheritanceResponseObject, error) {
	return h.permissions.BreakPermissionInheritance(ctx, request)
}
func (h *APIHandler) RestorePermissionInheritance(ctx context.Context, request api.RestorePermissionInheritanceRequestObject) (api.RestorePermissionInheritanceResponseObject, error) {
	return h.permissions.RestorePermissionInheritance(ctx, request)
}

func (h *APIHandler) GetLiveness(ctx context.Context, request api.GetLivenessRequestObject) (api.GetLivenessResponseObject, error) {
	return h.health.GetLiveness(ctx, request)
}

func (h *APIHandler) GetReadiness(ctx context.Context, request api.GetReadinessRequestObject) (api.GetReadinessResponseObject, error) {
	return h.health.GetReadiness(ctx, request)
}

func (h *APIHandler) Login(ctx context.Context, request api.LoginRequestObject) (api.LoginResponseObject, error) {
	if h.identity == nil {
		return nil, fmt.Errorf("identity handler is not configured")
	}
	return h.identity.Login(ctx, request)
}

func (h *APIHandler) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	if h.identity == nil {
		return nil, fmt.Errorf("identity handler is not configured")
	}
	return h.identity.Logout(ctx, request)
}

func (h *APIHandler) RefreshSession(ctx context.Context, request api.RefreshSessionRequestObject) (api.RefreshSessionResponseObject, error) {
	if h.identity == nil {
		return nil, fmt.Errorf("identity handler is not configured")
	}
	return h.identity.RefreshSession(ctx, request)
}

func (h *APIHandler) GetCurrentSession(ctx context.Context, request api.GetCurrentSessionRequestObject) (api.GetCurrentSessionResponseObject, error) {
	if h.identity == nil {
		return nil, fmt.Errorf("identity handler is not configured")
	}
	return h.identity.GetCurrentSession(ctx, request)
}

func (h *APIHandler) GetCurrentUser(ctx context.Context, request api.GetCurrentUserRequestObject) (api.GetCurrentUserResponseObject, error) {
	return h.users.GetCurrentUser(ctx, request)
}

func (h *APIHandler) UpdateCurrentUser(ctx context.Context, request api.UpdateCurrentUserRequestObject) (api.UpdateCurrentUserResponseObject, error) {
	return h.users.UpdateCurrentUser(ctx, request)
}

func (h *APIHandler) ListCurrentUserSessions(ctx context.Context, request api.ListCurrentUserSessionsRequestObject) (api.ListCurrentUserSessionsResponseObject, error) {
	return h.users.ListCurrentUserSessions(ctx, request)
}

func (h *APIHandler) RevokeCurrentUserSession(ctx context.Context, request api.RevokeCurrentUserSessionRequestObject) (api.RevokeCurrentUserSessionResponseObject, error) {
	return h.users.RevokeCurrentUserSession(ctx, request)
}

func (h *APIHandler) ListUsers(ctx context.Context, request api.ListUsersRequestObject) (api.ListUsersResponseObject, error) {
	return h.users.ListUsers(ctx, request)
}

func (h *APIHandler) CreateUser(ctx context.Context, request api.CreateUserRequestObject) (api.CreateUserResponseObject, error) {
	return h.users.CreateUser(ctx, request)
}

func (h *APIHandler) GetUser(ctx context.Context, request api.GetUserRequestObject) (api.GetUserResponseObject, error) {
	return h.users.GetUser(ctx, request)
}

func (h *APIHandler) UpdateUser(ctx context.Context, request api.UpdateUserRequestObject) (api.UpdateUserResponseObject, error) {
	return h.users.UpdateUser(ctx, request)
}

func (h *APIHandler) DeleteUser(ctx context.Context, request api.DeleteUserRequestObject) (api.DeleteUserResponseObject, error) {
	return h.users.DeleteUser(ctx, request)
}

func (h *APIHandler) DisableUser(ctx context.Context, request api.DisableUserRequestObject) (api.DisableUserResponseObject, error) {
	return h.users.DisableUser(ctx, request)
}

func (h *APIHandler) EnableUser(ctx context.Context, request api.EnableUserRequestObject) (api.EnableUserResponseObject, error) {
	return h.users.EnableUser(ctx, request)
}

func (h *APIHandler) LockUser(ctx context.Context, request api.LockUserRequestObject) (api.LockUserResponseObject, error) {
	return h.users.LockUser(ctx, request)
}

func (h *APIHandler) ResetUserPassword(ctx context.Context, request api.ResetUserPasswordRequestObject) (api.ResetUserPasswordResponseObject, error) {
	return h.users.ResetUserPassword(ctx, request)
}

func (h *APIHandler) ListOrganizationChangePlans(ctx context.Context, request api.ListOrganizationChangePlansRequestObject) (api.ListOrganizationChangePlansResponseObject, error) {
	return h.organizations.ListOrganizationChangePlans(ctx, request)
}

func (h *APIHandler) CreateOrganizationChangePlan(ctx context.Context, request api.CreateOrganizationChangePlanRequestObject) (api.CreateOrganizationChangePlanResponseObject, error) {
	return h.organizations.CreateOrganizationChangePlan(ctx, request)
}

func (h *APIHandler) GetOrganizationChangePlan(ctx context.Context, request api.GetOrganizationChangePlanRequestObject) (api.GetOrganizationChangePlanResponseObject, error) {
	return h.organizations.GetOrganizationChangePlan(ctx, request)
}

func (h *APIHandler) AddOrganizationChangeOperation(ctx context.Context, request api.AddOrganizationChangeOperationRequestObject) (api.AddOrganizationChangeOperationResponseObject, error) {
	return h.organizations.AddOrganizationChangeOperation(ctx, request)
}

func (h *APIHandler) TransitionOrganizationChangePlan(ctx context.Context, request api.TransitionOrganizationChangePlanRequestObject) (api.TransitionOrganizationChangePlanResponseObject, error) {
	return h.organizations.TransitionOrganizationChangePlan(ctx, request)
}

func (h *APIHandler) ListOrganizations(ctx context.Context, request api.ListOrganizationsRequestObject) (api.ListOrganizationsResponseObject, error) {
	return h.organizations.ListOrganizations(ctx, request)
}

func (h *APIHandler) CreateOrganization(ctx context.Context, request api.CreateOrganizationRequestObject) (api.CreateOrganizationResponseObject, error) {
	return h.organizations.CreateOrganization(ctx, request)
}

func (h *APIHandler) GetOrganization(ctx context.Context, request api.GetOrganizationRequestObject) (api.GetOrganizationResponseObject, error) {
	return h.organizations.GetOrganization(ctx, request)
}

func (h *APIHandler) UpdateOrganization(ctx context.Context, request api.UpdateOrganizationRequestObject) (api.UpdateOrganizationResponseObject, error) {
	return h.organizations.UpdateOrganization(ctx, request)
}

func (h *APIHandler) ListOrganizationMembers(ctx context.Context, request api.ListOrganizationMembersRequestObject) (api.ListOrganizationMembersResponseObject, error) {
	return h.organizations.ListOrganizationMembers(ctx, request)
}

func (h *APIHandler) AddOrganizationMember(ctx context.Context, request api.AddOrganizationMemberRequestObject) (api.AddOrganizationMemberResponseObject, error) {
	return h.organizations.AddOrganizationMember(ctx, request)
}

func (h *APIHandler) RemoveOrganizationMember(ctx context.Context, request api.RemoveOrganizationMemberRequestObject) (api.RemoveOrganizationMemberResponseObject, error) {
	return h.organizations.RemoveOrganizationMember(ctx, request)
}

func (h *APIHandler) UpdateOrganizationMember(ctx context.Context, request api.UpdateOrganizationMemberRequestObject) (api.UpdateOrganizationMemberResponseObject, error) {
	return h.organizations.UpdateOrganizationMember(ctx, request)
}

func (h *APIHandler) MoveOrganization(ctx context.Context, request api.MoveOrganizationRequestObject) (api.MoveOrganizationResponseObject, error) {
	return h.organizations.MoveOrganization(ctx, request)
}

func (h *APIHandler) ChangeOrganizationStatus(ctx context.Context, request api.ChangeOrganizationStatusRequestObject) (api.ChangeOrganizationStatusResponseObject, error) {
	return h.organizations.ChangeOrganizationStatus(ctx, request)
}

func (h *APIHandler) ListSpaces(ctx context.Context, request api.ListSpacesRequestObject) (api.ListSpacesResponseObject, error) {
	return h.organizations.ListSpaces(ctx, request)
}

func (h *APIHandler) CreatePublicSpace(ctx context.Context, request api.CreatePublicSpaceRequestObject) (api.CreatePublicSpaceResponseObject, error) {
	return h.organizations.CreatePublicSpace(ctx, request)
}

func (h *APIHandler) GetSpace(ctx context.Context, request api.GetSpaceRequestObject) (api.GetSpaceResponseObject, error) {
	return h.organizations.GetSpace(ctx, request)
}

func (h *APIHandler) UpdateSpace(ctx context.Context, request api.UpdateSpaceRequestObject) (api.UpdateSpaceResponseObject, error) {
	return h.organizations.UpdateSpace(ctx, request)
}

func (h *APIHandler) ChangeSpaceStatus(ctx context.Context, request api.ChangeSpaceStatusRequestObject) (api.ChangeSpaceStatusResponseObject, error) {
	return h.organizations.ChangeSpaceStatus(ctx, request)
}

func (h *APIHandler) ProvisionUserPersonalSpace(ctx context.Context, request api.ProvisionUserPersonalSpaceRequestObject) (api.ProvisionUserPersonalSpaceResponseObject, error) {
	return h.organizations.ProvisionUserPersonalSpace(ctx, request)
}

func (h *APIHandler) ListCurrentUserOrganizations(ctx context.Context, request api.ListCurrentUserOrganizationsRequestObject) (api.ListCurrentUserOrganizationsResponseObject, error) {
	return h.organizations.ListCurrentUserOrganizations(ctx, request)
}

func (h *APIHandler) GetCurrentUserPersonalSpace(ctx context.Context, request api.GetCurrentUserPersonalSpaceRequestObject) (api.GetCurrentUserPersonalSpaceResponseObject, error) {
	return h.organizations.GetCurrentUserPersonalSpace(ctx, request)
}
