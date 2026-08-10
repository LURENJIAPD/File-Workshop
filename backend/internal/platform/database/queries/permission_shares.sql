-- name: ListCandidateShareGrants :many
SELECT s.share_id, COALESCE(array_agg(a.action ORDER BY a.action), ARRAY[]::varchar[])::text[] AS actions
FROM shares s
JOIN share_actions a ON a.share_id = s.share_id
WHERE s.status = 'ACTIVE'
  AND s.valid_from <= sqlc.arg('effective_at')::timestamptz
  AND (s.valid_until IS NULL OR s.valid_until > sqlc.arg('effective_at'))
  AND s.target_kind IN ('USER', 'ORGANIZATION')
  AND (
    s.target_user_id = sqlc.arg('user_id')::uuid
    OR s.target_organization_id = ANY(sqlc.arg('organization_ids')::uuid[])
  )
  AND (
    s.source_document_id = sqlc.narg('document_id')::uuid
    OR s.source_folder_id = ANY(sqlc.arg('folder_ids')::uuid[])
  )
GROUP BY s.share_id
ORDER BY s.created_at, s.share_id;
