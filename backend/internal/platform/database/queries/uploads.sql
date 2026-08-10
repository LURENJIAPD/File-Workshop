-- name: GetUploadFolderContext :one
SELECT
  e.namespace_entry_id AS folder_id,
  e.space_id,
  e.lifecycle_status,
  s.status AS space_status
FROM namespace_entries e
JOIN folders f ON f.folder_id = e.namespace_entry_id
JOIN spaces s ON s.space_id = e.space_id
WHERE e.namespace_entry_id = sqlc.arg('folder_id')::uuid
  AND e.space_id = sqlc.arg('space_id')::uuid
  AND e.entry_type = 'FOLDER'
  AND e.lifecycle_status = 'ACTIVE'
  AND s.status = 'ACTIVE';

-- name: GetUploadDocumentContext :one
SELECT
  e.namespace_entry_id AS document_id,
  e.space_id,
  e.lifecycle_status,
  d.current_version_id,
  d.availability_status,
  d.row_version,
  s.status AS space_status
FROM namespace_entries e
JOIN documents d ON d.document_id = e.namespace_entry_id
JOIN spaces s ON s.space_id = e.space_id
WHERE e.namespace_entry_id = sqlc.arg('document_id')::uuid
  AND e.space_id = sqlc.arg('space_id')::uuid
  AND e.entry_type = 'DOCUMENT'
  AND e.lifecycle_status = 'ACTIVE'
  AND s.status = 'ACTIVE';

-- name: InsertUploadSession :one
INSERT INTO upload_sessions (
  upload_session_id, user_id, space_id, folder_id, quota_reservation_id,
  target_document_id, upload_intent, file_name, normalized_name,
  declared_size_bytes, declared_sha256, declared_mime_type,
  provider_upload_id, temporary_object_key,
  part_size_bytes, expected_part_count,
  expected_current_version_id, expected_lock_fencing_token, lock_token_hash,
  status, expires_at, created_at, updated_at
) VALUES (
  sqlc.arg('upload_session_id')::uuid,
  sqlc.arg('user_id')::uuid,
  sqlc.arg('space_id')::uuid,
  sqlc.arg('folder_id')::uuid,
  sqlc.arg('quota_reservation_id')::uuid,
  sqlc.narg('target_document_id')::uuid,
  sqlc.arg('upload_intent')::varchar,
  sqlc.arg('file_name')::varchar,
  sqlc.arg('normalized_name')::varchar,
  sqlc.arg('declared_size_bytes')::bigint,
  sqlc.narg('declared_sha256')::bytea,
  sqlc.narg('declared_mime_type')::varchar,
  sqlc.narg('provider_upload_id')::varchar,
  sqlc.arg('temporary_object_key')::varchar,
  sqlc.arg('part_size_bytes')::bigint,
  sqlc.arg('expected_part_count')::integer,
  sqlc.narg('expected_current_version_id')::uuid,
  sqlc.narg('expected_lock_fencing_token')::bigint,
  sqlc.narg('lock_token_hash')::bytea,
  'INITIATED',
  sqlc.arg('expires_at')::timestamptz,
  sqlc.arg('created_at')::timestamptz,
  sqlc.arg('created_at')::timestamptz
)
RETURNING *;

-- name: GetUploadSession :one
SELECT *
FROM upload_sessions
WHERE upload_session_id = $1;

-- name: GetUploadSessionForUpdate :one
SELECT *
FROM upload_sessions
WHERE upload_session_id = $1
FOR UPDATE;

-- name: MarkUploadSessionUploading :one
UPDATE upload_sessions
SET status = 'UPLOADING',
    updated_at = $2,
    row_version = row_version + 1
WHERE upload_session_id = $1
  AND status = 'INITIATED'
RETURNING *;

-- name: AbortUploadSession :one
UPDATE upload_sessions
SET status = 'ABORTED',
    failure_code = $3,
    updated_at = $4,
    row_version = row_version + 1
WHERE upload_session_id = $1
  AND row_version = $2
  AND status IN ('INITIATED', 'UPLOADING', 'FAILED')
RETURNING *;
