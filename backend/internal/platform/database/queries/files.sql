-- name: GetFileSpaceDirectoryInfo :one
SELECT space_id, space_type, owner_user_id, organization_id, root_folder_id, status, row_version
FROM spaces
WHERE space_id = $1 AND status <> 'DELETED';

-- name: GetFileSpaceDirectoryInfoForUpdate :one
SELECT space_id, space_type, owner_user_id, organization_id, root_folder_id, status, row_version
FROM spaces
WHERE space_id = $1 AND status <> 'DELETED'
FOR UPDATE;

-- name: UpdateFileSpaceRootFolder :exec
UPDATE spaces
SET root_folder_id = $2,
    updated_at = $3,
    row_version = row_version + 1
WHERE space_id = $1 AND root_folder_id IS NULL AND status <> 'DELETED';

-- name: TouchFileSpaceSecurityEpoch :exec
UPDATE spaces
SET security_epoch = security_epoch + 1,
    updated_at = $2,
    row_version = row_version + 1
WHERE space_id = $1 AND status <> 'DELETED';

-- name: InsertFileNamespaceEntry :one
INSERT INTO namespace_entries (
  namespace_entry_id, space_id, parent_folder_id, entry_type,
  name, normalized_name, path_cache, depth, lifecycle_status,
  created_by_user_id, created_at, updated_at
) VALUES (
  $1, $2, $3, $4,
  $5, $6, $7, $8, 'ACTIVE',
  $9, $10, $10
)
RETURNING *;

-- name: InsertFileFolder :exec
INSERT INTO folders (folder_id, inheritance_mode, created_at, updated_at)
VALUES ($1, 'INHERIT', $2, $2);

-- name: InsertFileDocument :one
INSERT INTO documents (
  document_id, owner_user_id, current_version_id, availability_status,
  extension_normalized, inheritance_mode, classification,
  metadata_schema_version, metadata_json, created_at, updated_at
) VALUES (
  $1, $2, NULL, $3,
  $4, 'INHERIT', $5,
  1, $6, $7, $7
)
RETURNING *;

-- name: GetFileNamespaceEntry :one
SELECT
  e.*,
  (s.root_folder_id = e.namespace_entry_id) AS is_root,
  f.inheritance_mode AS folder_inheritance_mode,
  f.acl_version AS folder_acl_version,
  d.owner_user_id,
  d.current_version_id,
  d.availability_status,
  d.extension_normalized,
  d.inheritance_mode AS document_inheritance_mode,
  d.acl_version AS document_acl_version,
  d.classification,
  d.metadata_schema_version,
  d.metadata_json
FROM namespace_entries e
JOIN spaces s ON s.space_id = e.space_id
LEFT JOIN folders f ON f.folder_id = e.namespace_entry_id
LEFT JOIN documents d ON d.document_id = e.namespace_entry_id
WHERE e.namespace_entry_id = $1 AND e.lifecycle_status <> 'PURGED' AND s.status <> 'DELETED';

-- name: GetFileNamespaceEntryForUpdate :one
SELECT
  e.*,
  (s.root_folder_id = e.namespace_entry_id) AS is_root,
  f.inheritance_mode AS folder_inheritance_mode,
  f.acl_version AS folder_acl_version,
  d.owner_user_id,
  d.current_version_id,
  d.availability_status,
  d.extension_normalized,
  d.inheritance_mode AS document_inheritance_mode,
  d.acl_version AS document_acl_version,
  d.classification,
  d.metadata_schema_version,
  d.metadata_json
FROM namespace_entries e
JOIN spaces s ON s.space_id = e.space_id
LEFT JOIN folders f ON f.folder_id = e.namespace_entry_id
LEFT JOIN documents d ON d.document_id = e.namespace_entry_id
WHERE e.namespace_entry_id = $1 AND e.lifecycle_status <> 'PURGED' AND s.status <> 'DELETED'
FOR UPDATE OF e;

-- name: CountFileChildEntries :one
SELECT count(*)::bigint
FROM namespace_entries e
WHERE e.space_id = sqlc.arg('space_id')::uuid
  AND e.parent_folder_id = sqlc.arg('parent_folder_id')::uuid
  AND (sqlc.narg('entry_type')::text IS NULL OR e.entry_type = sqlc.narg('entry_type'))
  AND (sqlc.narg('lifecycle_status')::text IS NULL OR e.lifecycle_status = sqlc.narg('lifecycle_status'));

-- name: ListFileChildEntries :many
SELECT
  e.*,
  false::boolean AS is_root,
  f.inheritance_mode AS folder_inheritance_mode,
  f.acl_version AS folder_acl_version,
  d.owner_user_id,
  d.current_version_id,
  d.availability_status,
  d.extension_normalized,
  d.inheritance_mode AS document_inheritance_mode,
  d.acl_version AS document_acl_version,
  d.classification,
  d.metadata_schema_version,
  d.metadata_json
FROM namespace_entries e
LEFT JOIN folders f ON f.folder_id = e.namespace_entry_id
LEFT JOIN documents d ON d.document_id = e.namespace_entry_id
WHERE e.space_id = sqlc.arg('space_id')::uuid
  AND e.parent_folder_id = sqlc.arg('parent_folder_id')::uuid
  AND (sqlc.narg('entry_type')::text IS NULL OR e.entry_type = sqlc.narg('entry_type'))
  AND (sqlc.narg('lifecycle_status')::text IS NULL OR e.lifecycle_status = sqlc.narg('lifecycle_status'))
ORDER BY e.entry_type, e.normalized_name, e.namespace_entry_id
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: RenameFileNamespaceEntry :one
UPDATE namespace_entries
SET name = sqlc.arg('name')::varchar,
    normalized_name = sqlc.arg('normalized_name')::varchar,
    path_cache = CASE
      WHEN parent_folder_id IS NULL THEN path_cache
      ELSE regexp_replace(COALESCE(path_cache, ''), '[^/]+$', sqlc.arg('name')::varchar::text)
    END,
    updated_at = sqlc.arg('updated_at')::timestamptz,
    row_version = row_version + 1
WHERE namespace_entry_id = sqlc.arg('namespace_entry_id')::uuid
  AND row_version = sqlc.arg('row_version')::bigint
  AND lifecycle_status IN ('ACTIVE', 'ARCHIVED')
RETURNING *;

-- name: UpdateFileDocumentExtension :exec
UPDATE documents
SET extension_normalized = $2,
    updated_at = $3,
    row_version = row_version + 1
WHERE document_id = $1;

-- name: MoveFileNamespaceEntry :one
UPDATE namespace_entries
SET parent_folder_id = $2,
    path_cache = $3,
    depth = $4,
    updated_at = $5,
    row_version = row_version + 1
WHERE namespace_entry_id = $1
  AND row_version = $6
  AND parent_folder_id IS NOT NULL
  AND lifecycle_status IN ('ACTIVE', 'ARCHIVED')
RETURNING *;

-- name: UpdateFileDescendantPaths :exec
WITH RECURSIVE tree AS (
  SELECT child.namespace_entry_id,
         sqlc.arg('root_path')::text || '/' || child.name AS new_path,
         sqlc.arg('root_depth')::integer + 1 AS new_depth
  FROM namespace_entries child
  WHERE child.parent_folder_id = sqlc.arg('root_id')::uuid
  UNION ALL
  SELECT child.namespace_entry_id,
         tree.new_path || '/' || child.name AS new_path,
         tree.new_depth + 1 AS new_depth
  FROM namespace_entries child
  JOIN tree ON child.parent_folder_id = tree.namespace_entry_id
)
UPDATE namespace_entries e
SET path_cache = tree.new_path,
    depth = tree.new_depth,
    updated_at = sqlc.arg('updated_at')::timestamptz,
    row_version = e.row_version + 1
FROM tree
WHERE e.namespace_entry_id = tree.namespace_entry_id;

-- name: FileFolderIsDescendantOf :one
WITH RECURSIVE parents AS (
  SELECT e.parent_folder_id
  FROM namespace_entries e
  WHERE e.namespace_entry_id = sqlc.arg('folder_id')::uuid
  UNION ALL
  SELECT e.parent_folder_id
  FROM namespace_entries e
  JOIN parents p ON e.namespace_entry_id = p.parent_folder_id
  WHERE p.parent_folder_id IS NOT NULL
)
SELECT EXISTS (
  SELECT 1 FROM parents WHERE parent_folder_id = sqlc.arg('ancestor_folder_id')::uuid
)::boolean;

-- name: TryCreateFileIdempotencyRecord :execrows
INSERT INTO idempotency_records (
  idempotency_record_id, principal_kind, user_id, operation, idempotency_key, request_hash,
  status, expires_at, created_at, updated_at
) VALUES (
  $1, 'USER', $2, $3, $4, $5, 'IN_PROGRESS', $6, $7, $7
)
ON CONFLICT DO NOTHING;

-- name: GetFileIdempotencyRecord :one
SELECT request_hash, status, result_resource_id
FROM idempotency_records
WHERE principal_kind = 'USER'
  AND user_id = $1
  AND operation = $2
  AND idempotency_key = $3;

-- name: CompleteFileIdempotencyRecord :exec
UPDATE idempotency_records
SET status = 'COMPLETED',
    response_status_code = 201,
    response_schema_version = 1,
    response_json = jsonb_build_object('resourceId', $4::uuid, 'resourceType', $5::text),
    result_resource_id = $4,
    result_resource_type = $5,
    completed_at = $6,
    updated_at = $6,
    row_version = row_version + 1
WHERE principal_kind = 'USER'
  AND user_id = $1
  AND operation = $2
  AND idempotency_key = $3
  AND status = 'IN_PROGRESS';

-- name: InsertFileOutboxEvent :exec
INSERT INTO outbox_events (
  outbox_event_id, aggregate_type, aggregate_id, aggregate_version,
  event_type, event_schema_version, payload_json, deduplication_key, correlation_id,
  max_attempts, available_at, created_at, updated_at
) VALUES (
  $1, $2, $3, $4,
  $5, 1, $6, $7, $8,
  10, $9, $9, $9
)
ON CONFLICT (deduplication_key) DO NOTHING;
