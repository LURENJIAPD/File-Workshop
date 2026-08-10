-- name: SearchDirectoryEntries :many
SELECT
  e.namespace_entry_id, e.space_id, e.parent_folder_id, e.entry_type, e.name, e.normalized_name,
  e.path_cache, e.depth, e.lifecycle_status, e.created_by_user_id, e.created_at, e.updated_at,
  e.deleted_at, e.row_version, (sp.root_folder_id = e.namespace_entry_id) AS is_root,
  f.inheritance_mode AS folder_inheritance_mode,
  f.acl_version AS folder_acl_version,
  d.owner_user_id, d.current_version_id, d.availability_status, d.extension_normalized,
  d.inheritance_mode AS document_inheritance_mode,
  d.acl_version AS document_acl_version,
  d.classification, d.metadata_schema_version, d.metadata_json,
  s.status AS index_status
FROM namespace_entries e
JOIN spaces sp ON sp.space_id = e.space_id
LEFT JOIN folders f ON f.folder_id = e.namespace_entry_id
LEFT JOIN documents d ON d.document_id = e.namespace_entry_id
LEFT JOIN document_index_states s ON s.document_id = d.document_id
WHERE e.lifecycle_status = 'ACTIVE'
  AND sp.status <> 'DELETED'
  AND (sqlc.narg('space_id')::uuid IS NULL OR e.space_id = sqlc.narg('space_id')::uuid)
  AND (sqlc.narg('entry_type')::text IS NULL OR e.entry_type = sqlc.narg('entry_type')::text)
  AND (
    sqlc.narg('query')::text IS NULL
    OR e.normalized_name LIKE '%' || sqlc.narg('query')::text || '%'
    OR lower(e.name) LIKE '%' || sqlc.narg('query')::text || '%'
    OR lower(COALESCE(d.extension_normalized, '')) LIKE '%' || sqlc.narg('query')::text || '%'
    OR lower(COALESCE(d.classification, '')) LIKE '%' || sqlc.narg('query')::text || '%'
  )
  AND (sqlc.narg('extension')::text IS NULL OR lower(COALESCE(d.extension_normalized, '')) = sqlc.narg('extension')::text)
  AND (sqlc.narg('classification')::text IS NULL OR lower(COALESCE(d.classification, '')) = sqlc.narg('classification')::text)
  AND (sqlc.narg('created_by_user_id')::uuid IS NULL OR e.created_by_user_id = sqlc.narg('created_by_user_id')::uuid)
  AND (sqlc.narg('updated_from')::timestamptz IS NULL OR e.updated_at >= sqlc.narg('updated_from')::timestamptz)
  AND (sqlc.narg('updated_to')::timestamptz IS NULL OR e.updated_at <= sqlc.narg('updated_to')::timestamptz)
  AND (
    sqlc.narg('metadata_key')::text IS NULL
    OR d.metadata_json ->> sqlc.narg('metadata_key')::text IS NOT NULL
  )
  AND (
    sqlc.narg('metadata_value')::text IS NULL
    OR lower(COALESCE(d.metadata_json ->> sqlc.narg('metadata_key')::text, '')) LIKE '%' || sqlc.narg('metadata_value')::text || '%'
  )
ORDER BY
  CASE
    WHEN sqlc.narg('query')::text IS NOT NULL AND e.normalized_name = sqlc.narg('query')::text THEN 0
    WHEN sqlc.narg('query')::text IS NOT NULL AND e.normalized_name LIKE sqlc.narg('query')::text || '%' THEN 1
    ELSE 2
  END,
  e.updated_at DESC,
  e.namespace_entry_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: GetSearchIndexDocumentTarget :one
SELECT
  d.document_id,
  d.current_version_id,
  d.acl_version,
  sp.security_epoch AS space_security_epoch
FROM documents d
JOIN namespace_entries e ON e.namespace_entry_id = d.document_id
JOIN spaces sp ON sp.space_id = e.space_id
WHERE d.document_id = sqlc.arg('document_id')::uuid
  AND e.lifecycle_status = 'ACTIVE'
  AND sp.status <> 'DELETED';

-- name: UpsertDocumentIndexPending :one
INSERT INTO document_index_states (
  document_id, status, last_error_code, updated_at
) VALUES (
  sqlc.arg('document_id')::uuid,
  'PENDING',
  NULL,
  sqlc.arg('updated_at')::timestamptz
)
ON CONFLICT (document_id)
DO UPDATE SET
  status = 'PENDING',
  last_error_code = NULL,
  updated_at = EXCLUDED.updated_at,
  row_version = document_index_states.row_version + 1
RETURNING *;
