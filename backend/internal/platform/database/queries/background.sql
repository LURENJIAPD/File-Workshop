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

-- name: CountOutboxFailuresByErrorCode :many
SELECT last_error_code, count(*)::bigint AS count, max(updated_at)::timestamptz AS latest_at
FROM outbox_events
WHERE status IN ('FAILED', 'DEAD')
  AND last_error_code IS NOT NULL
GROUP BY last_error_code
ORDER BY count(*) DESC, max(updated_at) DESC, last_error_code ASC
LIMIT 20;

-- name: CountOutboxEvents :one
SELECT count(*)::bigint
FROM outbox_events
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type')::text);

-- name: ListOutboxEvents :many
SELECT *
FROM outbox_events
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type')::text)
ORDER BY created_at DESC, outbox_event_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: GetOutboxEvent :one
SELECT *
FROM outbox_events
WHERE outbox_event_id = sqlc.arg('outbox_event_id')::uuid;

-- name: RetryOutboxEvent :one
UPDATE outbox_events
SET status = 'PENDING',
    attempt_count = 0,
    available_at = sqlc.arg('available_at')::timestamptz,
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    next_retry_at = NULL,
    published_at = NULL,
    last_error_code = 'MANUAL_RETRY',
    last_error_summary = sqlc.arg('reason')::text,
    updated_at = sqlc.arg('available_at')::timestamptz,
    row_version = row_version + 1
WHERE outbox_event_id = sqlc.arg('outbox_event_id')::uuid
  AND row_version = sqlc.arg('row_version')::bigint
  AND status IN ('FAILED', 'DEAD')
RETURNING *;

-- name: InsertBackgroundJob :one
INSERT INTO background_jobs (
  background_job_id, job_type, target_document_id, target_document_version_id,
  target_storage_object_id, payload_schema_version, payload_json,
  deduplication_key, priority, max_attempts, available_at, created_at, updated_at
) VALUES (
  sqlc.arg('background_job_id')::uuid,
  sqlc.arg('job_type')::varchar,
  sqlc.narg('target_document_id')::uuid,
  sqlc.narg('target_document_version_id')::uuid,
  sqlc.narg('target_storage_object_id')::uuid,
  sqlc.arg('payload_schema_version')::integer,
  sqlc.arg('payload_json')::jsonb,
  sqlc.arg('deduplication_key')::varchar,
  sqlc.arg('priority')::integer,
  sqlc.arg('max_attempts')::integer,
  sqlc.arg('available_at')::timestamptz,
  sqlc.arg('created_at')::timestamptz,
  sqlc.arg('created_at')::timestamptz
)
ON CONFLICT (job_type, deduplication_key) WHERE status IN ('PENDING', 'PROCESSING', 'FAILED')
DO UPDATE SET updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: ClaimBackgroundJobsByType :many
WITH candidates AS (
  SELECT background_job_id
  FROM background_jobs
  WHERE job_type = sqlc.arg('job_type')::varchar
    AND attempt_count < max_attempts
    AND (
      (status = 'PENDING' AND available_at <= sqlc.arg('now')::timestamptz)
      OR (status = 'FAILED' AND available_at <= sqlc.arg('now')::timestamptz)
      OR (status = 'PROCESSING' AND lease_until <= sqlc.arg('now')::timestamptz)
    )
  ORDER BY priority DESC, available_at, background_job_id
  LIMIT sqlc.arg('batch_size')::integer
  FOR UPDATE SKIP LOCKED
)
UPDATE background_jobs j
SET status = 'PROCESSING',
    attempt_count = j.attempt_count + 1,
    locked_by = sqlc.arg('locked_by')::varchar,
    locked_at = sqlc.arg('now')::timestamptz,
    lease_until = sqlc.arg('lease_until')::timestamptz,
    heartbeat_at = sqlc.arg('now')::timestamptz,
    started_at = COALESCE(j.started_at, sqlc.arg('now')::timestamptz),
    updated_at = sqlc.arg('now')::timestamptz,
    row_version = j.row_version + 1
FROM candidates c
WHERE j.background_job_id = c.background_job_id
RETURNING j.*;

-- name: MarkBackgroundJobSuccess :execrows
UPDATE background_jobs
SET status = 'SUCCESS',
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    heartbeat_at = NULL,
    completed_at = sqlc.arg('completed_at')::timestamptz,
    last_error_code = NULL,
    last_error_summary = NULL,
    updated_at = sqlc.arg('completed_at')::timestamptz,
    row_version = row_version + 1
WHERE background_job_id = sqlc.arg('background_job_id')::uuid
  AND status = 'PROCESSING'
  AND locked_by = sqlc.arg('locked_by')::varchar
  AND row_version = sqlc.arg('row_version')::bigint;

-- name: MarkBackgroundJobFailed :execrows
UPDATE background_jobs
SET status = 'FAILED',
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    heartbeat_at = NULL,
    available_at = sqlc.arg('next_retry_at')::timestamptz,
    last_error_code = sqlc.arg('last_error_code')::varchar,
    last_error_summary = sqlc.arg('last_error_summary')::text,
    updated_at = sqlc.arg('now')::timestamptz,
    row_version = row_version + 1
WHERE background_job_id = sqlc.arg('background_job_id')::uuid
  AND status = 'PROCESSING'
  AND locked_by = sqlc.arg('locked_by')::varchar
  AND row_version = sqlc.arg('row_version')::bigint
  AND attempt_count < max_attempts;

-- name: MarkBackgroundJobDead :execrows
UPDATE background_jobs
SET status = 'DEAD',
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    heartbeat_at = NULL,
    completed_at = sqlc.arg('now')::timestamptz,
    last_error_code = sqlc.arg('last_error_code')::varchar,
    last_error_summary = sqlc.arg('last_error_summary')::text,
    updated_at = sqlc.arg('now')::timestamptz,
    row_version = row_version + 1
WHERE background_job_id = sqlc.arg('background_job_id')::uuid
  AND status = 'PROCESSING'
  AND locked_by = sqlc.arg('locked_by')::varchar
  AND row_version = sqlc.arg('row_version')::bigint;

-- name: RenewBackgroundJobLease :execrows
UPDATE background_jobs
SET lease_until = sqlc.arg('lease_until')::timestamptz,
    heartbeat_at = sqlc.arg('now')::timestamptz,
    updated_at = sqlc.arg('now')::timestamptz,
    row_version = row_version + 1
WHERE background_job_id = sqlc.arg('background_job_id')::uuid
  AND status = 'PROCESSING'
  AND locked_by = sqlc.arg('locked_by')::varchar
  AND row_version = sqlc.arg('row_version')::bigint;

-- name: CountBackgroundJobs :one
SELECT count(*)::bigint
FROM background_jobs
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('job_type')::text IS NULL OR job_type = sqlc.narg('job_type')::text);

-- name: ListBackgroundJobs :many
SELECT *
FROM background_jobs
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('job_type')::text IS NULL OR job_type = sqlc.narg('job_type')::text)
ORDER BY created_at DESC, background_job_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: GetBackgroundJob :one
SELECT *
FROM background_jobs
WHERE background_job_id = sqlc.arg('background_job_id')::uuid;

-- name: CountBackgroundJobsByStatus :many
SELECT status, count(*)::bigint AS count
FROM background_jobs
GROUP BY status
ORDER BY status;

-- name: GetBackgroundQueueLagSummary :one
WITH outbox_due AS (
  SELECT 'PENDING'::text AS status, available_at AS due_at
  FROM outbox_events
  WHERE status = 'PENDING'
    AND available_at <= sqlc.arg('now')::timestamptz
    AND attempt_count < max_attempts
  UNION ALL
  SELECT 'FAILED'::text AS status, COALESCE(next_retry_at, available_at) AS due_at
  FROM outbox_events
  WHERE status = 'FAILED'
    AND COALESCE(next_retry_at, available_at) <= sqlc.arg('now')::timestamptz
    AND attempt_count < max_attempts
  UNION ALL
  SELECT 'PROCESSING'::text AS status, lease_until AS due_at
  FROM outbox_events
  WHERE status = 'PROCESSING'
    AND lease_until IS NOT NULL
    AND lease_until <= sqlc.arg('now')::timestamptz
),
job_due AS (
  SELECT 'PENDING'::text AS status, available_at AS due_at
  FROM background_jobs
  WHERE status = 'PENDING'
    AND available_at <= sqlc.arg('now')::timestamptz
    AND attempt_count < max_attempts
  UNION ALL
  SELECT 'FAILED'::text AS status, available_at AS due_at
  FROM background_jobs
  WHERE status = 'FAILED'
    AND available_at <= sqlc.arg('now')::timestamptz
    AND attempt_count < max_attempts
  UNION ALL
  SELECT 'PROCESSING'::text AS status, lease_until AS due_at
  FROM background_jobs
  WHERE status = 'PROCESSING'
    AND lease_until IS NOT NULL
    AND lease_until <= sqlc.arg('now')::timestamptz
)
SELECT
  (SELECT count(*)::bigint FROM outbox_due WHERE status = 'PENDING') AS outbox_due_pending_count,
  (SELECT count(*)::bigint FROM outbox_due WHERE status = 'FAILED') AS outbox_due_failed_count,
  (SELECT count(*)::bigint FROM outbox_due WHERE status = 'PROCESSING') AS outbox_expired_processing_count,
  (SELECT min(due_at)::timestamptz FROM outbox_due) AS outbox_oldest_due_at,
  (SELECT count(*)::bigint FROM job_due WHERE status = 'PENDING') AS background_jobs_due_pending_count,
  (SELECT count(*)::bigint FROM job_due WHERE status = 'FAILED') AS background_jobs_due_failed_count,
  (SELECT count(*)::bigint FROM job_due WHERE status = 'PROCESSING') AS background_jobs_expired_processing_count,
  (SELECT min(due_at)::timestamptz FROM job_due) AS background_jobs_oldest_due_at;

-- name: CountBackgroundJobFailuresByErrorCode :many
SELECT last_error_code, count(*)::bigint AS count, max(updated_at)::timestamptz AS latest_at
FROM background_jobs
WHERE status IN ('FAILED', 'DEAD')
  AND last_error_code IS NOT NULL
GROUP BY last_error_code
ORDER BY count(*) DESC, max(updated_at) DESC, last_error_code ASC
LIMIT 20;

-- name: RecoverExpiredBackgroundLeases :one
WITH outbox_candidates AS (
  SELECT outbox_event_id
  FROM outbox_events
  WHERE status = 'PROCESSING'
    AND lease_until IS NOT NULL
    AND lease_until <= sqlc.arg('now')::timestamptz
  ORDER BY priority DESC, lease_until, outbox_event_id
  LIMIT sqlc.arg('batch_size')::integer
  FOR UPDATE SKIP LOCKED
),
updated_outbox AS (
  UPDATE outbox_events e
  SET status = CASE WHEN e.attempt_count < e.max_attempts THEN 'FAILED' ELSE 'DEAD' END,
      locked_by = NULL,
      locked_at = NULL,
      lease_until = NULL,
      next_retry_at = CASE WHEN e.attempt_count < e.max_attempts THEN sqlc.arg('now')::timestamptz ELSE NULL END,
      last_error_code = 'LEASE_EXPIRED',
      last_error_summary = sqlc.arg('reason')::text,
      updated_at = sqlc.arg('now')::timestamptz,
      row_version = e.row_version + 1
  FROM outbox_candidates c
  WHERE e.outbox_event_id = c.outbox_event_id
  RETURNING e.status
),
job_candidates AS (
  SELECT background_job_id
  FROM background_jobs
  WHERE status = 'PROCESSING'
    AND lease_until IS NOT NULL
    AND lease_until <= sqlc.arg('now')::timestamptz
  ORDER BY priority DESC, lease_until, background_job_id
  LIMIT sqlc.arg('batch_size')::integer
  FOR UPDATE SKIP LOCKED
),
updated_jobs AS (
  UPDATE background_jobs j
  SET status = CASE WHEN j.attempt_count < j.max_attempts THEN 'FAILED' ELSE 'DEAD' END,
      locked_by = NULL,
      locked_at = NULL,
      lease_until = NULL,
      heartbeat_at = NULL,
      available_at = CASE WHEN j.attempt_count < j.max_attempts THEN sqlc.arg('now')::timestamptz ELSE j.available_at END,
      completed_at = CASE WHEN j.attempt_count < j.max_attempts THEN NULL ELSE sqlc.arg('now')::timestamptz END,
      last_error_code = 'LEASE_EXPIRED',
      last_error_summary = sqlc.arg('reason')::text,
      updated_at = sqlc.arg('now')::timestamptz,
      row_version = j.row_version + 1
  FROM job_candidates c
  WHERE j.background_job_id = c.background_job_id
  RETURNING j.status
)
SELECT
  (SELECT count(*)::bigint FROM updated_outbox) AS outbox_recovered_count,
  (SELECT count(*)::bigint FROM updated_outbox WHERE status = 'FAILED') AS outbox_retryable_count,
  (SELECT count(*)::bigint FROM updated_outbox WHERE status = 'DEAD') AS outbox_dead_count,
  (SELECT count(*)::bigint FROM updated_jobs) AS background_jobs_recovered_count,
  (SELECT count(*)::bigint FROM updated_jobs WHERE status = 'FAILED') AS background_jobs_retryable_count,
  (SELECT count(*)::bigint FROM updated_jobs WHERE status = 'DEAD') AS background_jobs_dead_count;

-- name: RetryBackgroundJob :one
UPDATE background_jobs
SET status = 'PENDING',
    attempt_count = 0,
    available_at = sqlc.arg('available_at')::timestamptz,
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    heartbeat_at = NULL,
    completed_at = NULL,
    last_error_code = 'MANUAL_RETRY',
    last_error_summary = sqlc.arg('reason')::text,
    updated_at = sqlc.arg('available_at')::timestamptz,
    row_version = row_version + 1
WHERE background_job_id = sqlc.arg('background_job_id')::uuid
  AND row_version = sqlc.arg('row_version')::bigint
  AND status IN ('FAILED', 'DEAD')
RETURNING *;

-- name: CancelBackgroundJob :one
UPDATE background_jobs
SET status = 'CANCELLED',
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    heartbeat_at = NULL,
    completed_at = sqlc.arg('completed_at')::timestamptz,
    last_error_code = 'MANUAL_CANCEL',
    last_error_summary = sqlc.arg('reason')::text,
    updated_at = sqlc.arg('completed_at')::timestamptz,
    row_version = row_version + 1
WHERE background_job_id = sqlc.arg('background_job_id')::uuid
  AND row_version = sqlc.arg('row_version')::bigint
  AND status IN ('PENDING', 'FAILED', 'DEAD')
RETURNING *;

-- name: MarkBackgroundJobManuallyDead :one
UPDATE background_jobs
SET status = 'DEAD',
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    heartbeat_at = NULL,
    completed_at = sqlc.arg('completed_at')::timestamptz,
    last_error_code = 'MANUAL_DEAD_LETTER',
    last_error_summary = sqlc.arg('reason')::text,
    updated_at = sqlc.arg('completed_at')::timestamptz,
    row_version = row_version + 1
WHERE background_job_id = sqlc.arg('background_job_id')::uuid
  AND row_version = sqlc.arg('row_version')::bigint
  AND status = 'FAILED'
RETURNING *;

-- name: SkipBackgroundJob :one
UPDATE background_jobs
SET status = 'SKIPPED',
    locked_by = NULL,
    locked_at = NULL,
    lease_until = NULL,
    heartbeat_at = NULL,
    completed_at = sqlc.arg('completed_at')::timestamptz,
    last_error_code = 'MANUAL_SKIP',
    last_error_summary = sqlc.arg('reason')::text,
    updated_at = sqlc.arg('completed_at')::timestamptz,
    row_version = row_version + 1
WHERE background_job_id = sqlc.arg('background_job_id')::uuid
  AND row_version = sqlc.arg('row_version')::bigint
  AND status IN ('PENDING', 'FAILED', 'DEAD')
RETURNING *;
