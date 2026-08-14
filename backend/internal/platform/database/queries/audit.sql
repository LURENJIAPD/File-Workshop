-- name: InsertAuditEvent :exec
INSERT INTO audit_events (
  audit_event_id, event_type, risk_level, actor_type, actor_id, actor_display_name,
  actor_employee_no, effective_role, admin_delegation_id, share_id, resource_type,
  resource_id, resource_name, space_id, organization_id, document_id, document_version_id,
  action, result, failure_code, source_channel, request_id, trace_id, correlation_id,
  reason, metadata_schema_version, metadata_json, hash_schema_version, chain_id,
  sequence_number, previous_hash, event_hash, partition_date, created_at
) VALUES (
  $1::uuid, $2, $3, $4, $5::uuid, $6, $7, $8, $9::uuid, $10::uuid, $11,
  $12::uuid, $13, $14::uuid, $15::uuid, $16::uuid, $17::uuid, $18, $19, $20,
  $21, $22::uuid, $23, $24::uuid, $25, $26, $27::jsonb, $28, $29, $30,
  $31, $32, $33, $34
);

-- name: CountAuditEvents :one
SELECT count(*)::bigint
FROM audit_events
WHERE partition_date >= sqlc.arg('date_from')::date
  AND partition_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type')::text)
  AND (sqlc.narg('risk_level')::text IS NULL OR risk_level = sqlc.narg('risk_level')::text)
  AND (sqlc.narg('actor_type')::text IS NULL OR actor_type = sqlc.narg('actor_type')::text)
  AND (sqlc.narg('actor_id')::uuid IS NULL OR actor_id = sqlc.narg('actor_id')::uuid)
  AND (sqlc.narg('resource_type')::text IS NULL OR resource_type = sqlc.narg('resource_type')::text)
  AND (sqlc.narg('resource_id')::uuid IS NULL OR resource_id = sqlc.narg('resource_id')::uuid)
  AND (sqlc.narg('result')::text IS NULL OR result = sqlc.narg('result')::text)
  AND (sqlc.narg('request_id')::uuid IS NULL OR request_id = sqlc.narg('request_id')::uuid);

-- name: CountAuditEventsByRiskLevel :many
SELECT risk_level, count(*)::bigint AS event_count
FROM audit_events
WHERE partition_date >= sqlc.arg('date_from')::date
  AND partition_date <= sqlc.arg('date_to')::date
GROUP BY risk_level
ORDER BY risk_level ASC;

-- name: CountAuditEventsByResult :many
SELECT result, count(*)::bigint AS event_count
FROM audit_events
WHERE partition_date >= sqlc.arg('date_from')::date
  AND partition_date <= sqlc.arg('date_to')::date
GROUP BY result
ORDER BY result ASC;

-- name: CountAuditEventsByActorType :many
SELECT actor_type, count(*)::bigint AS event_count
FROM audit_events
WHERE partition_date >= sqlc.arg('date_from')::date
  AND partition_date <= sqlc.arg('date_to')::date
GROUP BY actor_type
ORDER BY actor_type ASC;

-- name: ListAuditEvents :many
SELECT
  audit_event_id, event_type, risk_level, actor_type, actor_id, actor_display_name,
  actor_employee_no, effective_role, admin_delegation_id, share_id, resource_type,
  resource_id, resource_name, space_id, organization_id, document_id, document_version_id,
  action, result, failure_code, source_channel, COALESCE(host(ip_address)::text, '') AS ip_address,
  user_agent, request_id, trace_id, correlation_id, reason, metadata_schema_version,
  metadata_json, hash_schema_version, chain_id, sequence_number, previous_hash,
  event_hash, partition_date, created_at
FROM audit_events
WHERE partition_date >= sqlc.arg('date_from')::date
  AND partition_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type')::text)
  AND (sqlc.narg('risk_level')::text IS NULL OR risk_level = sqlc.narg('risk_level')::text)
  AND (sqlc.narg('actor_type')::text IS NULL OR actor_type = sqlc.narg('actor_type')::text)
  AND (sqlc.narg('actor_id')::uuid IS NULL OR actor_id = sqlc.narg('actor_id')::uuid)
  AND (sqlc.narg('resource_type')::text IS NULL OR resource_type = sqlc.narg('resource_type')::text)
  AND (sqlc.narg('resource_id')::uuid IS NULL OR resource_id = sqlc.narg('resource_id')::uuid)
  AND (sqlc.narg('result')::text IS NULL OR result = sqlc.narg('result')::text)
  AND (sqlc.narg('request_id')::uuid IS NULL OR request_id = sqlc.narg('request_id')::uuid)
ORDER BY created_at DESC, audit_event_id DESC
OFFSET sqlc.arg('page_offset')::bigint
LIMIT sqlc.arg('page_size')::integer;

-- name: GetAuditEvent :one
SELECT
  audit_event_id, event_type, risk_level, actor_type, actor_id, actor_display_name,
  actor_employee_no, effective_role, admin_delegation_id, share_id, resource_type,
  resource_id, resource_name, space_id, organization_id, document_id, document_version_id,
  action, result, failure_code, source_channel, COALESCE(host(ip_address)::text, '') AS ip_address,
  user_agent, request_id, trace_id, correlation_id, reason, metadata_schema_version,
  metadata_json, hash_schema_version, chain_id, sequence_number, previous_hash,
  event_hash, partition_date, created_at
FROM audit_events
WHERE partition_date = $1::date
  AND audit_event_id = $2::uuid;

-- name: GetAuditChainHeadForUpdate :one
SELECT chain_id, partition_date, last_sequence_number, last_event_id, last_hash,
  batch_root, anchor_location, status, verified_at, created_at, updated_at, row_version
FROM audit_chain_heads
WHERE chain_id = $1
  AND partition_date = $2::date
FOR UPDATE;

-- name: InsertAuditChainHead :exec
INSERT INTO audit_chain_heads (
  chain_id, partition_date, last_sequence_number, last_event_id, last_hash,
  status, created_at, updated_at
) VALUES (
  $1, $2::date, $3, $4::uuid, $5, 'ACTIVE', $6, $6
);

-- name: UpdateAuditChainHead :execrows
UPDATE audit_chain_heads
SET last_sequence_number = $3,
    last_event_id = $4::uuid,
    last_hash = $5,
    updated_at = $6,
    row_version = row_version + 1
WHERE chain_id = $1
  AND partition_date = $2::date
  AND status = 'ACTIVE';

-- name: ListAuditChainHeads :many
SELECT chain_id, partition_date, last_sequence_number, last_event_id, last_hash,
  batch_root, anchor_location, status, verified_at, created_at, updated_at, row_version
FROM audit_chain_heads
WHERE partition_date >= sqlc.arg('date_from')::date
  AND partition_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY partition_date DESC, chain_id ASC
OFFSET sqlc.arg('page_offset')::bigint
LIMIT sqlc.arg('page_size')::integer;

-- name: CountAuditChainHeads :one
SELECT count(*)::bigint
FROM audit_chain_heads
WHERE partition_date >= sqlc.arg('date_from')::date
  AND partition_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: CountAuditChainHeadsByStatus :many
SELECT status, count(*)::bigint AS chain_count
FROM audit_chain_heads
WHERE partition_date >= sqlc.arg('date_from')::date
  AND partition_date <= sqlc.arg('date_to')::date
GROUP BY status
ORDER BY status ASC;

-- name: ListAuditChainEventsForVerify :many
SELECT audit_event_id, event_type, risk_level, actor_type, actor_id, actor_display_name,
  actor_employee_no, effective_role, admin_delegation_id, share_id, resource_type,
  resource_id, resource_name, space_id, organization_id, document_id, document_version_id,
  action, result, failure_code, source_channel, request_id, trace_id, correlation_id,
  reason, metadata_schema_version, metadata_json, hash_schema_version, chain_id,
  sequence_number, previous_hash, event_hash, partition_date, created_at
FROM audit_events
WHERE partition_date = $1::date
  AND chain_id = $2
ORDER BY sequence_number ASC;

-- name: MarkAuditChainVerified :execrows
UPDATE audit_chain_heads
SET status = 'ACTIVE',
    verified_at = $3,
    updated_at = $3,
    row_version = row_version + 1
WHERE chain_id = $1
  AND partition_date = $2::date;

-- name: MarkAuditChainInvalid :execrows
UPDATE audit_chain_heads
SET status = 'INVALID',
    verified_at = $3,
    updated_at = $3,
    row_version = row_version + 1
WHERE chain_id = $1
  AND partition_date = $2::date;
