-- name: GetLifecycleEntryForUpdate :one
SELECT
  e.namespace_entry_id, e.space_id, e.parent_folder_id, e.entry_type, e.name,
  e.normalized_name, e.path_cache, e.depth, e.lifecycle_status, e.created_by_user_id,
  e.created_at, e.updated_at, e.deleted_at, e.row_version,
  (s.root_folder_id = e.namespace_entry_id) AS is_root
FROM namespace_entries e
JOIN spaces s ON s.space_id = e.space_id
WHERE e.namespace_entry_id = $1
  AND e.lifecycle_status <> 'PURGED'
  AND s.status <> 'DELETED'
FOR UPDATE OF e;

-- name: GetLifecycleFolderForUpdate :one
SELECT
  e.namespace_entry_id, e.space_id, e.parent_folder_id, e.entry_type, e.name,
  e.normalized_name, e.path_cache, e.depth, e.lifecycle_status, e.created_by_user_id,
  e.created_at, e.updated_at, e.deleted_at, e.row_version,
  (s.root_folder_id = e.namespace_entry_id) AS is_root
FROM namespace_entries e
JOIN spaces s ON s.space_id = e.space_id
WHERE e.namespace_entry_id = $1
  AND e.entry_type = 'FOLDER'
  AND e.lifecycle_status = 'ACTIVE'
  AND s.status <> 'DELETED'
FOR UPDATE OF e;

-- name: LifecycleNameExists :one
SELECT EXISTS (
  SELECT 1
  FROM namespace_entries e
  WHERE e.space_id = $1
    AND (($2::uuid IS NULL AND e.parent_folder_id IS NULL) OR e.parent_folder_id = $2)
    AND e.normalized_name = $3
    AND e.lifecycle_status IN ('ACTIVE', 'ARCHIVED')
    AND e.namespace_entry_id <> $4
)::boolean;

-- name: TrashLifecycleEntrySubtree :execrows
WITH RECURSIVE tree AS (
  SELECT namespace_entry_id
  FROM namespace_entries
  WHERE namespace_entry_id = sqlc.arg('root_id')::uuid
  UNION ALL
  SELECT child.namespace_entry_id
  FROM namespace_entries child
  JOIN tree parent ON child.parent_folder_id = parent.namespace_entry_id
  WHERE child.lifecycle_status IN ('ACTIVE', 'ARCHIVED')
)
UPDATE namespace_entries e
SET lifecycle_status = 'TRASHED',
    updated_at = sqlc.arg('updated_at')::timestamptz,
    row_version = e.row_version + 1
FROM tree
WHERE e.namespace_entry_id = tree.namespace_entry_id
  AND e.lifecycle_status IN ('ACTIVE', 'ARCHIVED');

-- name: RestoreLifecycleEntrySubtree :execrows
WITH RECURSIVE tree AS (
  SELECT namespace_entry_id
  FROM namespace_entries
  WHERE namespace_entry_id = sqlc.arg('root_id')::uuid
  UNION ALL
  SELECT child.namespace_entry_id
  FROM namespace_entries child
  JOIN tree parent ON child.parent_folder_id = parent.namespace_entry_id
  WHERE child.lifecycle_status = 'TRASHED'
)
UPDATE namespace_entries e
SET lifecycle_status = 'ACTIVE',
    updated_at = sqlc.arg('updated_at')::timestamptz,
    row_version = e.row_version + 1
FROM tree
WHERE e.namespace_entry_id = tree.namespace_entry_id
  AND e.lifecycle_status = 'TRASHED';

-- name: MoveRestoreLifecycleRoot :one
UPDATE namespace_entries
SET parent_folder_id = $2,
    name = $3,
    normalized_name = $4,
    path_cache = $5,
    depth = $6,
    updated_at = $7,
    row_version = row_version + 1
WHERE namespace_entry_id = $1
  AND lifecycle_status = 'TRASHED'
RETURNING namespace_entry_id, space_id, parent_folder_id, entry_type, name, normalized_name, path_cache, depth, lifecycle_status, created_by_user_id, created_at, updated_at, deleted_at, row_version;

-- name: UpdateLifecycleDescendantPaths :exec
WITH RECURSIVE tree AS (
  SELECT child.namespace_entry_id,
         sqlc.arg('root_path')::text || '/' || child.name AS new_path,
         sqlc.arg('root_depth')::integer + 1 AS new_depth
  FROM namespace_entries child
  WHERE child.parent_folder_id = sqlc.arg('root_id')::uuid
  UNION ALL
  SELECT child.namespace_entry_id,
         tree.new_path || '/' || child.name AS new_path,
         tree.new_depth + 1
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

-- name: MarkLifecycleEntrySubtreePurging :execrows
WITH RECURSIVE tree AS (
  SELECT namespace_entry_id
  FROM namespace_entries
  WHERE namespace_entry_id = sqlc.arg('root_id')::uuid
  UNION ALL
  SELECT child.namespace_entry_id
  FROM namespace_entries child
  JOIN tree parent ON child.parent_folder_id = parent.namespace_entry_id
  WHERE child.lifecycle_status = 'TRASHED'
)
UPDATE namespace_entries e
SET lifecycle_status = 'PURGING',
    deleted_at = sqlc.arg('deleted_at')::timestamptz,
    updated_at = sqlc.arg('deleted_at')::timestamptz,
    row_version = e.row_version + 1
FROM tree
WHERE e.namespace_entry_id = tree.namespace_entry_id
  AND e.lifecycle_status = 'TRASHED';

-- name: InsertRecycleItem :one
INSERT INTO recycle_items (
  recycle_item_id, namespace_entry_id, original_space_id, original_parent_folder_id,
  original_name, deleted_by_user_id, deleted_at, expires_at, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$7,$7)
RETURNING *;

-- name: GetRecycleItemWithEntry :one
SELECT
  r.recycle_item_id, r.namespace_entry_id, r.original_space_id, r.original_parent_folder_id,
  r.original_name, r.deleted_by_user_id, r.deleted_at, r.expires_at, r.status,
  r.restored_to_folder_id, r.restored_at, r.created_at, r.updated_at, r.row_version,
  e.entry_type, e.name AS current_name, e.lifecycle_status
FROM recycle_items r
JOIN namespace_entries e ON e.namespace_entry_id = r.namespace_entry_id
WHERE r.recycle_item_id = $1;

-- name: GetRecycleItemWithEntryForUpdate :one
SELECT
  r.recycle_item_id, r.namespace_entry_id, r.original_space_id, r.original_parent_folder_id,
  r.original_name, r.deleted_by_user_id, r.deleted_at, r.expires_at, r.status,
  r.restored_to_folder_id, r.restored_at, r.created_at, r.updated_at, r.row_version,
  e.entry_type, e.name AS current_name, e.lifecycle_status
FROM recycle_items r
JOIN namespace_entries e ON e.namespace_entry_id = r.namespace_entry_id
WHERE r.recycle_item_id = $1
FOR UPDATE OF r;

-- name: CountRecycleItems :one
SELECT count(*)::bigint
FROM recycle_items r
JOIN namespace_entries e ON e.namespace_entry_id = r.namespace_entry_id
WHERE r.status = 'ACTIVE'
  AND (sqlc.narg('space_id')::uuid IS NULL OR r.original_space_id = sqlc.narg('space_id')::uuid);

-- name: ListRecycleItems :many
SELECT
  r.recycle_item_id, r.namespace_entry_id, r.original_space_id, r.original_parent_folder_id,
  r.original_name, r.deleted_by_user_id, r.deleted_at, r.expires_at, r.status,
  r.restored_to_folder_id, r.restored_at, r.created_at, r.updated_at, r.row_version,
  e.entry_type, e.name AS current_name, e.lifecycle_status
FROM recycle_items r
JOIN namespace_entries e ON e.namespace_entry_id = r.namespace_entry_id
WHERE r.status = 'ACTIVE'
  AND (sqlc.narg('space_id')::uuid IS NULL OR r.original_space_id = sqlc.narg('space_id')::uuid)
ORDER BY r.deleted_at DESC, r.recycle_item_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: ListExpiredActiveRecycleItems :many
SELECT
  r.recycle_item_id, r.namespace_entry_id, r.original_space_id, r.original_parent_folder_id,
  r.original_name, r.deleted_by_user_id, r.deleted_at, r.expires_at, r.status,
  r.restored_to_folder_id, r.restored_at, r.created_at, r.updated_at, r.row_version,
  e.entry_type, e.name AS current_name, e.lifecycle_status
FROM recycle_items r
JOIN namespace_entries e ON e.namespace_entry_id = r.namespace_entry_id
WHERE r.status = 'ACTIVE'
  AND r.expires_at <= sqlc.arg('now')::timestamptz
  AND e.lifecycle_status = 'TRASHED'
ORDER BY r.expires_at ASC, r.recycle_item_id ASC
LIMIT sqlc.arg('batch_size')::integer;

-- name: RestoreRecycleItem :one
UPDATE recycle_items
SET status = 'RESTORED',
    restored_to_folder_id = $2,
    restored_at = $3,
    updated_at = $3,
    row_version = row_version + 1
WHERE recycle_item_id = $1
  AND row_version = $4
  AND status = 'ACTIVE'
RETURNING *;

-- name: MarkRecycleItemPurging :one
UPDATE recycle_items
SET status = 'PURGING',
    updated_at = $3,
    row_version = row_version + 1
WHERE recycle_item_id = $1
  AND row_version = $2
  AND status = 'ACTIVE'
RETURNING *;

-- name: ActiveLegalHoldExistsForEntrySubtree :one
WITH RECURSIVE tree AS (
  SELECT e.namespace_entry_id, e.entry_type
  FROM namespace_entries e
  WHERE e.namespace_entry_id = $1
  UNION ALL
  SELECT child.namespace_entry_id, child.entry_type
  FROM namespace_entries child
  JOIN tree parent ON child.parent_folder_id = parent.namespace_entry_id
)
SELECT EXISTS (
  SELECT 1
  FROM legal_holds hold
  JOIN tree ON tree.namespace_entry_id = hold.document_id
  WHERE hold.status = 'ACTIVE'
)::boolean;

-- name: MarkSharesSourceUnavailableForEntrySubtree :exec
WITH RECURSIVE tree AS (
  SELECT namespace_entry_id, entry_type
  FROM namespace_entries
  WHERE namespace_entry_id = sqlc.arg('root_id')::uuid
  UNION ALL
  SELECT child.namespace_entry_id, child.entry_type
  FROM namespace_entries child
  JOIN tree parent ON child.parent_folder_id = parent.namespace_entry_id
)
UPDATE shares s
SET status = 'SOURCE_UNAVAILABLE',
    updated_at = sqlc.arg('updated_at')::timestamptz,
    row_version = s.row_version + 1
WHERE s.status = 'ACTIVE'
  AND (
    s.source_document_id IN (SELECT namespace_entry_id FROM tree WHERE entry_type = 'DOCUMENT')
    OR s.source_folder_id IN (SELECT namespace_entry_id FROM tree WHERE entry_type = 'FOLDER')
  );

-- name: TouchLifecycleSpaceSecurityEpoch :exec
UPDATE spaces
SET security_epoch = security_epoch + 1,
    updated_at = $2,
    row_version = row_version + 1
WHERE space_id = $1 AND status <> 'DELETED';

-- name: TryCreateLifecycleIdempotency :execrows
INSERT INTO idempotency_records (
  idempotency_record_id, principal_kind, user_id, operation, idempotency_key, request_hash,
  status, expires_at, created_at, updated_at
) VALUES ($1,'USER',$2,$3,$4,$5,'IN_PROGRESS',$6,$7,$7)
ON CONFLICT DO NOTHING;

-- name: GetLifecycleIdempotency :one
SELECT request_hash, status, result_resource_id FROM idempotency_records
WHERE principal_kind = 'USER' AND user_id = $1 AND operation = $2 AND idempotency_key = $3;

-- name: CompleteLifecycleIdempotency :exec
UPDATE idempotency_records
SET status = 'COMPLETED', result_resource_id = $4, result_resource_type = $5, completed_at = $6, updated_at = $6
WHERE principal_kind = 'USER' AND user_id = $1 AND operation = $2 AND idempotency_key = $3 AND status = 'IN_PROGRESS';

-- name: InsertLifecycleOutboxEvent :exec
INSERT INTO outbox_events (
  outbox_event_id, aggregate_type, aggregate_id, aggregate_version,
  event_type, payload_json, deduplication_key, correlation_id, available_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9);
