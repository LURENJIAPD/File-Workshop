-- name: GetAdminDelegationWithCapabilities :one
SELECT d.*, COALESCE(array_agg(c.capability ORDER BY c.capability) FILTER (WHERE c.capability IS NOT NULL), ARRAY[]::varchar[])::text[] AS capabilities
FROM admin_delegations d
LEFT JOIN admin_delegation_capabilities c ON c.admin_delegation_id = d.admin_delegation_id
WHERE d.admin_delegation_id = $1
GROUP BY d.admin_delegation_id;

-- name: GetAdminDelegationWithCapabilitiesForUpdate :one
SELECT d.*, ARRAY(SELECT c.capability FROM admin_delegation_capabilities c WHERE c.admin_delegation_id = d.admin_delegation_id ORDER BY c.capability)::text[] AS capabilities
FROM admin_delegations d
WHERE d.admin_delegation_id = $1
FOR UPDATE;

-- name: CountVisibleAdminDelegations :one
SELECT count(*)::bigint
FROM admin_delegations d
WHERE (sqlc.arg('viewer_is_admin')::boolean OR d.user_id = sqlc.arg('viewer_user_id')::uuid OR d.granted_by_user_id = sqlc.arg('viewer_user_id')::uuid)
  AND (sqlc.narg('organization_id')::uuid IS NULL OR d.organization_id = sqlc.narg('organization_id'))
  AND (sqlc.narg('status')::text IS NULL OR d.status = sqlc.narg('status'));

-- name: ListVisibleAdminDelegations :many
SELECT d.*, COALESCE(array_agg(c.capability ORDER BY c.capability) FILTER (WHERE c.capability IS NOT NULL), ARRAY[]::varchar[])::text[] AS capabilities
FROM admin_delegations d
LEFT JOIN admin_delegation_capabilities c ON c.admin_delegation_id = d.admin_delegation_id
WHERE (sqlc.arg('viewer_is_admin')::boolean OR d.user_id = sqlc.arg('viewer_user_id')::uuid OR d.granted_by_user_id = sqlc.arg('viewer_user_id')::uuid)
  AND (sqlc.narg('organization_id')::uuid IS NULL OR d.organization_id = sqlc.narg('organization_id'))
  AND (sqlc.narg('status')::text IS NULL OR d.status = sqlc.narg('status'))
GROUP BY d.admin_delegation_id
ORDER BY d.created_at DESC, d.admin_delegation_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: CountOrganizationAdministrators :one
WITH RECURSIVE effective AS (
  SELECT d.admin_delegation_id, d.parent_admin_delegation_id,
         d.status = 'ACTIVE' AND d.valid_from <= sqlc.arg('effective_at')::timestamptz AND (d.valid_until IS NULL OR d.valid_until > sqlc.arg('effective_at')) AS valid
  FROM admin_delegations d
  UNION ALL
  SELECT child.admin_delegation_id, parent.parent_admin_delegation_id,
         child.valid AND parent.status = 'ACTIVE' AND parent.valid_from <= sqlc.arg('effective_at')::timestamptz AND (parent.valid_until IS NULL OR parent.valid_until > sqlc.arg('effective_at'))
  FROM effective child
  JOIN admin_delegations parent ON parent.admin_delegation_id = child.parent_admin_delegation_id
), valid_ids AS (
  SELECT admin_delegation_id FROM effective GROUP BY admin_delegation_id HAVING bool_and(valid)
)
SELECT count(*)::bigint
FROM admin_delegations d
JOIN valid_ids v ON v.admin_delegation_id = d.admin_delegation_id
WHERE (d.organization_id = sqlc.arg('organization_id')::uuid AND d.scope IN ('SELF','SUBTREE'))
   OR (d.scope = 'SUBTREE' AND EXISTS (
      SELECT 1 FROM organization_closure oc
      WHERE oc.ancestor_organization_id = d.organization_id AND oc.descendant_organization_id = sqlc.arg('organization_id')::uuid
   ));

-- name: PermissionOrganizationExists :one
SELECT EXISTS (SELECT 1 FROM organizations WHERE organization_id = $1 AND status <> 'DELETED')::boolean;

-- name: ListOrganizationAdministrators :many
WITH RECURSIVE effective AS (
  SELECT d.admin_delegation_id, d.parent_admin_delegation_id,
         d.status = 'ACTIVE' AND d.valid_from <= sqlc.arg('effective_at')::timestamptz AND (d.valid_until IS NULL OR d.valid_until > sqlc.arg('effective_at')) AS valid
  FROM admin_delegations d
  UNION ALL
  SELECT child.admin_delegation_id, parent.parent_admin_delegation_id,
         child.valid AND parent.status = 'ACTIVE' AND parent.valid_from <= sqlc.arg('effective_at')::timestamptz AND (parent.valid_until IS NULL OR parent.valid_until > sqlc.arg('effective_at'))
  FROM effective child
  JOIN admin_delegations parent ON parent.admin_delegation_id = child.parent_admin_delegation_id
), valid_ids AS (
  SELECT admin_delegation_id FROM effective GROUP BY admin_delegation_id HAVING bool_and(valid)
)
SELECT d.*, COALESCE(array_agg(c.capability ORDER BY c.capability) FILTER (WHERE c.capability IS NOT NULL), ARRAY[]::varchar[])::text[] AS capabilities
FROM admin_delegations d
JOIN valid_ids v ON v.admin_delegation_id = d.admin_delegation_id
LEFT JOIN admin_delegation_capabilities c ON c.admin_delegation_id = d.admin_delegation_id
WHERE (d.organization_id = sqlc.arg('organization_id')::uuid AND d.scope IN ('SELF','SUBTREE'))
   OR (d.scope = 'SUBTREE' AND EXISTS (
      SELECT 1 FROM organization_closure oc
      WHERE oc.ancestor_organization_id = d.organization_id AND oc.descendant_organization_id = sqlc.arg('organization_id')::uuid
   ))
GROUP BY d.admin_delegation_id
ORDER BY d.created_at DESC, d.admin_delegation_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: InsertAdminDelegation :one
INSERT INTO admin_delegations (
  admin_delegation_id, user_id, organization_id, scope, can_delegate,
  parent_admin_delegation_id, granted_by_user_id, valid_from, valid_until,
  created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
RETURNING *;

-- name: InsertAdminDelegationCapability :exec
INSERT INTO admin_delegation_capabilities (admin_delegation_id, capability, created_at)
VALUES ($1,$2,$3);

-- name: RevokeAdminDelegation :one
UPDATE admin_delegations
SET status = 'REVOKED', revoked_at = $3, revoke_reason = $4, updated_at = $3, row_version = row_version + 1
WHERE admin_delegation_id = $1 AND row_version = $2 AND status = 'ACTIVE'
RETURNING *;

-- name: InvalidateDescendantAdminDelegations :many
WITH RECURSIVE descendants AS (
  SELECT root.admin_delegation_id FROM admin_delegations root WHERE root.parent_admin_delegation_id = $1
  UNION ALL
  SELECT child.admin_delegation_id FROM admin_delegations child JOIN descendants parent ON child.parent_admin_delegation_id = parent.admin_delegation_id
)
UPDATE admin_delegations
SET status = 'INVALIDATED', updated_at = $2, row_version = row_version + 1
WHERE admin_delegation_id IN (SELECT admin_delegation_id FROM descendants) AND status = 'ACTIVE'
RETURNING admin_delegation_id;

-- name: FindEffectiveAdminDelegation :one
WITH RECURSIVE candidates AS (
  SELECT d.*
  FROM admin_delegations d
  JOIN admin_delegation_capabilities c ON c.admin_delegation_id = d.admin_delegation_id AND c.capability = sqlc.arg('capability')::text
  WHERE d.user_id = sqlc.arg('user_id')::uuid
    AND d.status = 'ACTIVE' AND d.valid_from <= sqlc.arg('effective_at')::timestamptz AND (d.valid_until IS NULL OR d.valid_until > sqlc.arg('effective_at'))
    AND (
      d.organization_id = sqlc.arg('organization_id')::uuid
      OR (d.scope = 'SUBTREE' AND EXISTS (
        SELECT 1 FROM organization_closure oc
        WHERE oc.ancestor_organization_id = d.organization_id AND oc.descendant_organization_id = sqlc.arg('organization_id')::uuid
      ))
    )
), ancestor_state AS (
  SELECT c.admin_delegation_id AS candidate_id, c.parent_admin_delegation_id,
         true AS valid
  FROM candidates c
  UNION ALL
  SELECT state.candidate_id, parent.parent_admin_delegation_id,
         state.valid AND parent.status = 'ACTIVE' AND parent.valid_from <= sqlc.arg('effective_at')::timestamptz AND (parent.valid_until IS NULL OR parent.valid_until > sqlc.arg('effective_at'))
  FROM ancestor_state state
  JOIN admin_delegations parent ON parent.admin_delegation_id = state.parent_admin_delegation_id
), valid_candidates AS (
  SELECT candidate_id FROM ancestor_state GROUP BY candidate_id HAVING bool_and(valid)
)
SELECT c.*, COALESCE(array_agg(cap.capability ORDER BY cap.capability), ARRAY[]::varchar[])::text[] AS capabilities
FROM candidates c
JOIN valid_candidates v ON v.candidate_id = c.admin_delegation_id
LEFT JOIN admin_delegation_capabilities cap ON cap.admin_delegation_id = c.admin_delegation_id
GROUP BY c.admin_delegation_id, c.user_id, c.organization_id, c.scope, c.can_delegate,
 c.parent_admin_delegation_id, c.granted_by_user_id, c.valid_from, c.valid_until,
 c.status, c.created_at, c.updated_at, c.revoked_at, c.revoke_reason, c.row_version
ORDER BY c.created_at DESC, c.admin_delegation_id DESC
LIMIT 1;

-- name: AdminDelegationIsEffective :one
WITH RECURSIVE chain AS (
  SELECT d.admin_delegation_id, d.parent_admin_delegation_id, d.status, d.valid_from, d.valid_until
  FROM admin_delegations d WHERE d.admin_delegation_id = $1
  UNION ALL
  SELECT parent.admin_delegation_id, parent.parent_admin_delegation_id, parent.status, parent.valid_from, parent.valid_until
  FROM admin_delegations parent JOIN chain child ON parent.admin_delegation_id = child.parent_admin_delegation_id
)
SELECT COALESCE(bool_and(c.status = 'ACTIVE' AND c.valid_from <= sqlc.arg('effective_at')::timestamptz AND (c.valid_until IS NULL OR c.valid_until > sqlc.arg('effective_at'))), false)::boolean
FROM chain c;

-- name: IncrementPrincipalDelegationVersion :exec
UPDATE principal_security_versions
SET delegation_version = delegation_version + 1, global_authorization_version = global_authorization_version + 1, updated_at = $2
WHERE user_id = $1;

-- name: IncrementOrganizationDelegationVersion :exec
UPDATE organization_security_versions
SET delegation_version = delegation_version + 1, subtree_security_epoch = subtree_security_epoch + 1, updated_at = $2
WHERE organization_id IN (
  SELECT ancestor_organization_id FROM organization_closure WHERE descendant_organization_id = $1
);

-- name: GetSpaceAuthorizationResource :one
SELECT s.space_id, s.space_type, s.owner_user_id, s.organization_id, s.acl_version, s.row_version
FROM spaces s WHERE s.space_id = $1 AND s.status <> 'DELETED';

-- name: GetFolderAuthorizationResource :one
SELECT e.space_id, s.space_type, s.owner_user_id, s.organization_id, f.inheritance_mode, f.acl_version, f.row_version
FROM folders f
JOIN namespace_entries e ON e.namespace_entry_id = f.folder_id
JOIN spaces s ON s.space_id = e.space_id
WHERE f.folder_id = $1 AND e.lifecycle_status <> 'PURGED' AND s.status <> 'DELETED';

-- name: GetDocumentAuthorizationResource :one
SELECT e.space_id, s.space_type, s.owner_user_id, s.organization_id, d.inheritance_mode, d.acl_version, d.row_version
FROM documents d
JOIN namespace_entries e ON e.namespace_entry_id = d.document_id
JOIN spaces s ON s.space_id = e.space_id
WHERE d.document_id = $1 AND e.lifecycle_status <> 'PURGED' AND s.status <> 'DELETED';

-- name: ListFolderAuthorizationAncestors :many
WITH RECURSIVE ancestors AS (
  SELECT e.parent_folder_id AS folder_id, 1::integer AS distance
  FROM namespace_entries e WHERE e.namespace_entry_id = sqlc.arg('resource_id')::uuid
  UNION ALL
  SELECT e.parent_folder_id, a.distance + 1
  FROM ancestors a
  JOIN namespace_entries e ON e.namespace_entry_id = a.folder_id
  WHERE a.folder_id IS NOT NULL
)
SELECT a.folder_id, a.distance, f.inheritance_mode
FROM ancestors a JOIN folders f ON f.folder_id = a.folder_id
ORDER BY a.distance;

-- name: GetPermissionGrantWithActions :one
SELECT g.*, COALESCE(array_agg(a.action ORDER BY a.action), ARRAY[]::varchar[])::text[] AS actions
FROM permission_grants g
LEFT JOIN permission_grant_actions a ON a.permission_grant_id = g.permission_grant_id
WHERE g.permission_grant_id = $1
GROUP BY g.permission_grant_id;

-- name: GetPermissionGrantWithActionsForUpdate :one
SELECT g.*, ARRAY(SELECT a.action FROM permission_grant_actions a WHERE a.permission_grant_id = g.permission_grant_id ORDER BY a.action)::text[] AS actions
FROM permission_grants g
WHERE g.permission_grant_id = $1
FOR UPDATE;

-- name: CountDirectPermissionGrants :one
SELECT count(*)::bigint FROM permission_grants g
WHERE (sqlc.arg('resource_type')::text = 'SPACE' AND g.space_id = sqlc.arg('resource_id')::uuid)
   OR (sqlc.arg('resource_type')::text = 'FOLDER' AND g.folder_id = sqlc.arg('resource_id')::uuid)
   OR (sqlc.arg('resource_type')::text = 'DOCUMENT' AND g.document_id = sqlc.arg('resource_id')::uuid);

-- name: ListDirectPermissionGrants :many
SELECT g.*, COALESCE(array_agg(a.action ORDER BY a.action), ARRAY[]::varchar[])::text[] AS actions
FROM permission_grants g
LEFT JOIN permission_grant_actions a ON a.permission_grant_id = g.permission_grant_id
WHERE (sqlc.arg('resource_type')::text = 'SPACE' AND g.space_id = sqlc.arg('resource_id')::uuid)
   OR (sqlc.arg('resource_type')::text = 'FOLDER' AND g.folder_id = sqlc.arg('resource_id')::uuid)
   OR (sqlc.arg('resource_type')::text = 'DOCUMENT' AND g.document_id = sqlc.arg('resource_id')::uuid)
GROUP BY g.permission_grant_id
ORDER BY g.created_at DESC, g.permission_grant_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: ListCandidatePermissionGrants :many
SELECT g.*, COALESCE(array_agg(a.action ORDER BY a.action), ARRAY[]::varchar[])::text[] AS actions
FROM permission_grants g
JOIN permission_grant_actions a ON a.permission_grant_id = g.permission_grant_id
WHERE g.status = 'ACTIVE' AND g.valid_from <= sqlc.arg('effective_at')::timestamptz AND (g.valid_until IS NULL OR g.valid_until > sqlc.arg('effective_at'))
  AND (g.subject_user_id = sqlc.arg('user_id')::uuid OR g.subject_organization_id = ANY(sqlc.arg('organization_ids')::uuid[]))
  AND (
    g.space_id = sqlc.arg('space_id')::uuid
    OR g.folder_id = ANY(sqlc.arg('folder_ids')::uuid[])
    OR g.document_id = sqlc.narg('document_id')::uuid
  )
GROUP BY g.permission_grant_id
ORDER BY g.created_at, g.permission_grant_id;

-- name: ListActivePermissionUserOrganizations :many
SELECT organization_id FROM user_organizations
WHERE user_id = $1 AND status = 'ACTIVE' AND effective_from <= $2 AND (effective_until IS NULL OR effective_until > $2)
ORDER BY organization_id;

-- name: InsertPermissionGrant :one
INSERT INTO permission_grants (
  permission_grant_id, subject_user_id, subject_organization_id, space_id, folder_id,
  document_id, inherit_to_descendants, grant_source, valid_from, valid_until,
  granted_by_user_id, grant_reason, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13)
RETURNING *;

-- name: InsertPermissionGrantAction :exec
INSERT INTO permission_grant_actions (permission_grant_id, action, created_at) VALUES ($1,$2,$3);

-- name: DeletePermissionGrantActions :exec
DELETE FROM permission_grant_actions WHERE permission_grant_id = $1;

-- name: UpdatePermissionGrant :one
UPDATE permission_grants
SET inherit_to_descendants = $2, valid_until = $3, grant_reason = $4,
    updated_at = $5, row_version = row_version + 1
WHERE permission_grant_id = $1 AND row_version = $6 AND status = 'ACTIVE'
RETURNING *;

-- name: RevokePermissionGrant :one
UPDATE permission_grants
SET status = 'REVOKED', revoked_at = $4, revoked_by_user_id = $2, revoke_reason = $3,
    updated_at = $4, row_version = row_version + 1
WHERE permission_grant_id = $1 AND row_version = $5 AND status = 'ACTIVE'
RETURNING *;

-- name: ChangeFolderInheritance :one
UPDATE folders SET inheritance_mode = $2, acl_version = acl_version + 1, updated_at = $3, row_version = row_version + 1
WHERE folder_id = $1 AND row_version = $4 AND inheritance_mode <> $2
RETURNING inheritance_mode, acl_version, row_version;

-- name: ChangeDocumentInheritance :one
UPDATE documents SET inheritance_mode = $2, acl_version = acl_version + 1, updated_at = $3, row_version = row_version + 1
WHERE document_id = $1 AND row_version = $4 AND inheritance_mode <> $2
RETURNING inheritance_mode, acl_version, row_version;

-- name: IncrementSpaceACLVersion :exec
UPDATE spaces SET acl_version = acl_version + 1, security_epoch = security_epoch + 1, updated_at = $2 WHERE space_id = $1;

-- name: IncrementFolderACLVersion :exec
UPDATE folders SET acl_version = acl_version + 1, updated_at = $2 WHERE folder_id = $1;

-- name: IncrementDocumentACLVersion :exec
UPDATE documents SET acl_version = acl_version + 1, updated_at = $2 WHERE document_id = $1;

-- name: IncrementPrincipalGrantVersion :exec
UPDATE principal_security_versions
SET direct_grant_version = direct_grant_version + 1, global_authorization_version = global_authorization_version + 1, updated_at = $2
WHERE user_id = $1;

-- name: IncrementOrganizationGrantVersion :exec
UPDATE organization_security_versions
SET grant_version = grant_version + 1, subtree_security_epoch = subtree_security_epoch + 1, updated_at = $2
WHERE organization_id IN (SELECT ancestor_organization_id FROM organization_closure WHERE descendant_organization_id = $1);

-- name: GetPermissionAuthorizationVersion :one
SELECT concat_ws(':',
  p.organization_membership_version, p.delegation_version, p.direct_grant_version,
  p.share_version, p.global_authorization_version, s.acl_version, s.security_epoch,
  COALESCE((
    SELECT string_agg(concat_ws(',', osv.organization_id, osv.membership_version, osv.delegation_version, osv.grant_version, osv.share_version, osv.subtree_security_epoch), ';' ORDER BY osv.organization_id)
    FROM user_organizations uo
    JOIN organization_security_versions osv ON osv.organization_id = uo.organization_id
    WHERE uo.user_id = sqlc.arg('user_id')::uuid AND uo.status = 'ACTIVE'
      AND uo.effective_from <= sqlc.arg('effective_at')::timestamptz
      AND (uo.effective_until IS NULL OR uo.effective_until > sqlc.arg('effective_at'))
  ), ''),
  COALESCE((
    SELECT string_agg(concat_ws(',', target.organization_id, target.membership_version, target.delegation_version, target.grant_version, target.share_version, target.subtree_security_epoch), ';' ORDER BY closure.depth DESC, target.organization_id)
    FROM organization_closure closure
    JOIN organization_security_versions target ON target.organization_id = closure.ancestor_organization_id
    WHERE closure.descendant_organization_id = s.organization_id
  ), '')
)::text AS version_key
FROM principal_security_versions p
JOIN spaces s ON s.space_id = sqlc.arg('space_id')::uuid
WHERE p.user_id = sqlc.arg('user_id')::uuid;

-- name: TryCreatePermissionIdempotency :execrows
INSERT INTO idempotency_records (
  idempotency_record_id, principal_kind, user_id, operation, idempotency_key, request_hash,
  status, expires_at, created_at, updated_at
) VALUES ($1,'USER',$2,$3,$4,$5,'IN_PROGRESS',$6,$7,$7)
ON CONFLICT DO NOTHING;

-- name: GetPermissionIdempotency :one
SELECT request_hash, status, result_resource_id FROM idempotency_records
WHERE principal_kind = 'USER' AND user_id = $1 AND operation = $2 AND idempotency_key = $3
FOR UPDATE;

-- name: CompletePermissionIdempotency :exec
UPDATE idempotency_records
SET status = 'COMPLETED', response_status_code = 201, response_schema_version = 1,
    response_json = jsonb_build_object('resourceId', $4::uuid), result_resource_id = $4,
    result_resource_type = $5, completed_at = $6, updated_at = $6, row_version = row_version + 1
WHERE principal_kind = 'USER' AND user_id = $1 AND operation = $2 AND idempotency_key = $3 AND status = 'IN_PROGRESS';

-- name: InsertPermissionOutboxEvent :exec
INSERT INTO outbox_events (
  outbox_event_id, aggregate_type, aggregate_id, aggregate_version, event_type,
  event_schema_version, payload_json, deduplication_key, correlation_id,
  max_attempts, available_at, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,1,$6,$7,$8,10,$9,$9,$9);
