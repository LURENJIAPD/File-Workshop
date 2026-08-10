-- name: GetShareSourceResource :one
SELECT
  e.namespace_entry_id AS resource_id,
  sqlc.arg('source_type')::text AS resource_type,
  e.space_id,
  e.name,
  s.space_type,
  s.owner_user_id AS space_owner_user_id,
  s.organization_id,
  COALESCE(d.owner_user_id, s.owner_user_id) AS owner_user_id,
  COALESCE(d.inheritance_mode, f.inheritance_mode) AS inheritance_mode,
  COALESCE(d.acl_version, f.acl_version) AS acl_version,
  COALESCE(d.row_version, f.row_version) AS row_version
FROM namespace_entries e
JOIN spaces s ON s.space_id = e.space_id
LEFT JOIN documents d ON d.document_id = e.namespace_entry_id AND sqlc.arg('source_type')::text = 'DOCUMENT'
LEFT JOIN folders f ON f.folder_id = e.namespace_entry_id AND sqlc.arg('source_type')::text = 'FOLDER'
WHERE e.namespace_entry_id = sqlc.arg('source_id')::uuid
  AND e.entry_type = sqlc.arg('source_type')::text
  AND e.lifecycle_status = 'ACTIVE'
  AND s.status <> 'DELETED';

-- name: InsertShare :one
INSERT INTO shares (
  share_id, source_document_id, source_folder_id, creator_user_id,
  target_kind, target_user_id, target_organization_id, target_space_id,
  token_hash, allow_reshare, valid_from, valid_until, created_at, updated_at
) VALUES (
  $1, $2, $3, $4,
  $5, $6, $7, $8,
  $9, $10, $11, $12, $13, $13
)
RETURNING *;

-- name: InsertShareAction :exec
INSERT INTO share_actions (share_id, action, created_at)
VALUES ($1, $2, $3);

-- name: DeleteShareActions :exec
DELETE FROM share_actions WHERE share_id = $1;

-- name: GetShareWithActions :one
SELECT s.*, COALESCE(array_agg(a.action ORDER BY a.action), ARRAY[]::varchar[])::text[] AS actions
FROM shares s
LEFT JOIN share_actions a ON a.share_id = s.share_id
WHERE s.share_id = $1
GROUP BY s.share_id;

-- name: GetShareWithActionsForUpdate :one
SELECT s.*, ARRAY(SELECT a.action FROM share_actions a WHERE a.share_id = s.share_id ORDER BY a.action)::text[] AS actions
FROM shares s
WHERE s.share_id = $1
FOR UPDATE;

-- name: CountCreatedShares :one
SELECT count(*)::bigint
FROM shares s
WHERE s.creator_user_id = $1;

-- name: ListCreatedShares :many
SELECT s.*, COALESCE(array_agg(a.action ORDER BY a.action), ARRAY[]::varchar[])::text[] AS actions
FROM shares s
LEFT JOIN share_actions a ON a.share_id = s.share_id
WHERE s.creator_user_id = $1
GROUP BY s.share_id
ORDER BY s.created_at DESC, s.share_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: CountReceivedShares :one
SELECT count(DISTINCT s.share_id)::bigint
FROM shares s
WHERE s.status = 'ACTIVE'
  AND s.valid_from <= sqlc.arg('effective_at')::timestamptz
  AND (s.valid_until IS NULL OR s.valid_until > sqlc.arg('effective_at'))
  AND (
    s.target_user_id = sqlc.arg('user_id')::uuid
    OR s.target_organization_id = ANY(sqlc.arg('organization_ids')::uuid[])
  );

-- name: ListReceivedShares :many
SELECT s.*, COALESCE(array_agg(a.action ORDER BY a.action), ARRAY[]::varchar[])::text[] AS actions
FROM shares s
LEFT JOIN share_actions a ON a.share_id = s.share_id
WHERE s.status = 'ACTIVE'
  AND s.valid_from <= sqlc.arg('effective_at')::timestamptz
  AND (s.valid_until IS NULL OR s.valid_until > sqlc.arg('effective_at'))
  AND (
    s.target_user_id = sqlc.arg('user_id')::uuid
    OR s.target_organization_id = ANY(sqlc.arg('organization_ids')::uuid[])
  )
GROUP BY s.share_id
ORDER BY s.created_at DESC, s.share_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: UpdateShare :one
UPDATE shares
SET allow_reshare = $2,
    valid_until = $3,
    updated_at = $4,
    row_version = row_version + 1
WHERE share_id = $1 AND row_version = $5 AND status = 'ACTIVE'
RETURNING *;

-- name: RevokeShare :one
UPDATE shares
SET status = 'REVOKED',
    revoked_at = $4,
    revoked_by_user_id = $2,
    revoke_reason = $3,
    updated_at = $4,
    row_version = row_version + 1
WHERE share_id = $1 AND row_version = $5 AND status = 'ACTIVE'
RETURNING *;

-- name: ExpireShares :exec
UPDATE shares
SET status = 'EXPIRED',
    updated_at = sqlc.arg('updated_at')::timestamptz,
    row_version = row_version + 1
WHERE status = 'ACTIVE'
  AND valid_until IS NOT NULL
  AND valid_until <= sqlc.arg('updated_at')::timestamptz;

-- name: IncrementSharePrincipalVersion :exec
UPDATE principal_security_versions
SET share_version = share_version + 1,
    global_authorization_version = global_authorization_version + 1,
    updated_at = $2
WHERE user_id = $1;

-- name: IncrementShareOrganizationVersion :exec
UPDATE organization_security_versions
SET share_version = share_version + 1,
    subtree_security_epoch = subtree_security_epoch + 1,
    updated_at = $2
WHERE organization_id IN (
  SELECT ancestor_organization_id FROM organization_closure WHERE descendant_organization_id = $1
);

-- name: TryCreateShareIdempotency :execrows
INSERT INTO idempotency_records (
  idempotency_record_id, principal_kind, user_id, operation, idempotency_key, request_hash,
  status, expires_at, created_at, updated_at
) VALUES ($1,'USER',$2,$3,$4,$5,'IN_PROGRESS',$6,$7,$7)
ON CONFLICT DO NOTHING;

-- name: GetShareIdempotency :one
SELECT request_hash, status, result_resource_id FROM idempotency_records
WHERE principal_kind = 'USER' AND user_id = $1 AND operation = $2 AND idempotency_key = $3;

-- name: CompleteShareIdempotency :exec
UPDATE idempotency_records
SET status = 'COMPLETED', result_resource_id = $4, result_resource_type = $5, completed_at = $6, updated_at = $6
WHERE principal_kind = 'USER' AND user_id = $1 AND operation = $2 AND idempotency_key = $3 AND status = 'IN_PROGRESS';

-- name: InsertShareOutboxEvent :exec
INSERT INTO outbox_events (
  outbox_event_id, aggregate_type, aggregate_id, aggregate_version,
  event_type, payload_json, deduplication_key, correlation_id, available_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9);
