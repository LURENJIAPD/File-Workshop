-- name: ClaimOutboxEventsByType :many
WITH candidates AS (
  SELECT outbox_event_id
  FROM outbox_events
  WHERE event_type = sqlc.arg('event_type')::varchar
    AND attempt_count < max_attempts
    AND (
      (status = 'PENDING' AND available_at <= sqlc.arg('now')::timestamptz)
      OR (status = 'FAILED' AND COALESCE(next_retry_at, available_at) <= sqlc.arg('now')::timestamptz)
      OR (status = 'PROCESSING' AND lease_until <= sqlc.arg('now')::timestamptz)
    )
  ORDER BY priority DESC, available_at, outbox_event_id
  LIMIT sqlc.arg('batch_size')::integer
  FOR UPDATE SKIP LOCKED
)
UPDATE outbox_events e
SET status = 'PROCESSING',
    attempt_count = e.attempt_count + 1,
    locked_by = sqlc.arg('locked_by')::varchar,
    locked_at = sqlc.arg('now')::timestamptz,
    lease_until = sqlc.arg('lease_until')::timestamptz,
    next_retry_at = NULL,
    updated_at = sqlc.arg('now')::timestamptz,
    row_version = e.row_version + 1
FROM candidates c
WHERE e.outbox_event_id = c.outbox_event_id
RETURNING e.*;

-- name: MarkOutboxEventPublished :execrows
UPDATE outbox_events
SET status = 'PUBLISHED',
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    next_retry_at = NULL,
    last_error_code = NULL,
    last_error_summary = NULL,
    published_at = sqlc.arg('published_at')::timestamptz,
    updated_at = sqlc.arg('published_at')::timestamptz,
    row_version = row_version + 1
WHERE outbox_event_id = sqlc.arg('outbox_event_id')::uuid
  AND status = 'PROCESSING'
  AND locked_by = sqlc.arg('locked_by')::varchar
  AND row_version = sqlc.arg('row_version')::bigint;

-- name: MarkOutboxEventFailed :execrows
UPDATE outbox_events
SET status = 'FAILED',
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    next_retry_at = sqlc.arg('next_retry_at')::timestamptz,
    last_error_code = sqlc.arg('last_error_code')::varchar,
    last_error_summary = sqlc.arg('last_error_summary')::text,
    updated_at = sqlc.arg('now')::timestamptz,
    row_version = row_version + 1
WHERE outbox_event_id = sqlc.arg('outbox_event_id')::uuid
  AND status = 'PROCESSING'
  AND locked_by = sqlc.arg('locked_by')::varchar
  AND row_version = sqlc.arg('row_version')::bigint
  AND attempt_count < max_attempts;

-- name: MarkOutboxEventDead :execrows
UPDATE outbox_events
SET status = 'DEAD',
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    next_retry_at = NULL,
    last_error_code = sqlc.arg('last_error_code')::varchar,
    last_error_summary = sqlc.arg('last_error_summary')::text,
    updated_at = sqlc.arg('now')::timestamptz,
    row_version = row_version + 1
WHERE outbox_event_id = sqlc.arg('outbox_event_id')::uuid
  AND status = 'PROCESSING'
  AND locked_by = sqlc.arg('locked_by')::varchar
  AND row_version = sqlc.arg('row_version')::bigint;

-- name: RenewOutboxEventLease :execrows
UPDATE outbox_events
SET lease_until = sqlc.arg('lease_until')::timestamptz,
    updated_at = sqlc.arg('now')::timestamptz,
    row_version = row_version + 1
WHERE outbox_event_id = sqlc.arg('outbox_event_id')::uuid
  AND status = 'PROCESSING'
  AND locked_by = sqlc.arg('locked_by')::varchar
  AND row_version = sqlc.arg('row_version')::bigint;

-- name: CountOutboxEventsByStatus :many
SELECT status, count(*)::bigint AS count
FROM outbox_events
GROUP BY status
ORDER BY status;
