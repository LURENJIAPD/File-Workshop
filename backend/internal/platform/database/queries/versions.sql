-- name: GetVersionDocumentContext :one
SELECT
  d.document_id,
  e.space_id,
  d.owner_user_id,
  d.current_version_id,
  d.availability_status,
  d.row_version,
  e.lifecycle_status
FROM documents d
JOIN namespace_entries e ON e.namespace_entry_id = d.document_id
JOIN spaces s ON s.space_id = e.space_id
WHERE d.document_id = $1
  AND e.entry_type = 'DOCUMENT'
  AND e.lifecycle_status = 'ACTIVE'
  AND s.status = 'ACTIVE';

-- name: CountDocumentVersions :one
SELECT count(*)::bigint
FROM document_versions
WHERE document_id = $1;

-- name: ListDocumentVersions :many
SELECT *
FROM document_versions
WHERE document_id = sqlc.arg('document_id')::uuid
ORDER BY version_number DESC, document_version_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: GetDocumentVersion :one
SELECT *
FROM document_versions
WHERE document_id = $1
  AND document_version_id = $2;

-- name: GetVersionDocumentForUpdate :one
SELECT *
FROM documents
WHERE document_id = $1
FOR UPDATE;

-- name: InsertRestoredDocumentVersion :one
WITH source_version AS (
  SELECT *
  FROM document_versions
  WHERE document_id = sqlc.arg('document_id')::uuid
    AND document_version_id = sqlc.arg('restored_from_version_id')::uuid
),
next_number AS (
  SELECT COALESCE(MAX(version_number), 0) + 1 AS version_number
  FROM document_versions
  WHERE document_id = sqlc.arg('document_id')::uuid
)
INSERT INTO document_versions (
  document_version_id, document_id, version_number, storage_object_id,
  size_bytes, sha256, mime_type, change_note, source_type,
  restored_from_version_id, created_by_user_id, created_at
)
SELECT
  sqlc.arg('document_version_id')::uuid,
  sqlc.arg('document_id')::uuid,
  next_number.version_number,
  source_version.storage_object_id,
  source_version.size_bytes,
  source_version.sha256,
  source_version.mime_type,
  sqlc.narg('change_note')::text,
  'RESTORE',
  source_version.document_version_id,
  sqlc.arg('created_by_user_id')::uuid,
  sqlc.arg('created_at')::timestamptz
FROM source_version, next_number
RETURNING *;

-- name: SetDocumentCurrentVersion :execrows
UPDATE documents
SET current_version_id = $2,
    availability_status = 'PENDING_SCAN',
    updated_at = $3,
    row_version = row_version + 1
WHERE document_id = $1
  AND row_version = $4;

-- name: ExpireDocumentLocks :exec
UPDATE document_locks
SET status = 'EXPIRED',
    released_at = sqlc.arg('now')::timestamptz,
    release_reason = 'LOCK_EXPIRED',
    updated_at = sqlc.arg('now')::timestamptz,
    row_version = row_version + 1
WHERE document_id = sqlc.arg('document_id')::uuid
  AND status = 'ACTIVE'
  AND expires_at <= sqlc.arg('now')::timestamptz;

-- name: EnsureDocumentLockCounter :exec
INSERT INTO document_lock_counters (document_id, last_fencing_token, updated_at)
VALUES ($1, 0, $2)
ON CONFLICT (document_id) DO NOTHING;

-- name: IncrementDocumentLockCounter :one
UPDATE document_lock_counters
SET last_fencing_token = last_fencing_token + 1,
    updated_at = $2
WHERE document_id = $1
RETURNING last_fencing_token;

-- name: InsertDocumentLock :one
INSERT INTO document_locks (
  document_lock_id, document_id, user_id, token_hash, fencing_token,
  source, status, acquired_at, heartbeat_at, expires_at, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5,
  $6, 'ACTIVE', $7, $7, $8, $7, $7
)
RETURNING *;

-- name: GetActiveDocumentLock :one
SELECT *
FROM document_locks
WHERE document_id = $1
  AND status = 'ACTIVE';

-- name: GetActiveDocumentLockForUpdate :one
SELECT *
FROM document_locks
WHERE document_id = $1
  AND status = 'ACTIVE'
FOR UPDATE;

-- name: HeartbeatDocumentLock :one
UPDATE document_locks
SET heartbeat_at = $4,
    expires_at = $5,
    updated_at = $4,
    row_version = row_version + 1
WHERE document_id = $1
  AND token_hash = $2
  AND row_version = $3
  AND user_id = $6
  AND status = 'ACTIVE'
  AND expires_at > $4
RETURNING *;

-- name: ReleaseDocumentLock :one
UPDATE document_locks
SET status = 'RELEASED',
    released_at = $5,
    released_by_user_id = $6,
    release_reason = $7,
    updated_at = $5,
    row_version = row_version + 1
WHERE document_id = $1
  AND token_hash = $2
  AND row_version = $3
  AND user_id = $4
  AND status = 'ACTIVE'
RETURNING *;

-- name: ForceReleaseDocumentLock :one
UPDATE document_locks
SET status = 'FORCED',
    released_at = $3,
    released_by_user_id = $4,
    release_reason = $5,
    updated_at = $3,
    row_version = row_version + 1
WHERE document_id = $1
  AND row_version = $2
  AND status = 'ACTIVE'
RETURNING *;

-- name: InsertVersionOutboxEvent :exec
INSERT INTO outbox_events (
  outbox_event_id, aggregate_type, aggregate_id, aggregate_version,
  event_type, event_schema_version, payload_json, deduplication_key, correlation_id,
  max_attempts, available_at, created_at, updated_at
) VALUES (
  $1, $2, $3, $4,
  $5, 1, $6, $7, $8,
  10, $9, $9, $9
);
