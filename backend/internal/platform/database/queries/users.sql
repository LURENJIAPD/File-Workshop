-- name: GetManagedUserByID :one
SELECT user_id, username, username_normalized, employee_no, employee_no_normalized,
       display_name, email, email_normalized, phone, system_role, status, locale,
       timezone, last_login_at, created_by_user_id, created_at, updated_at, deleted_at, row_version
FROM users
WHERE user_id = $1;

-- name: GetManagedUserForUpdate :one
SELECT user_id, username, username_normalized, employee_no, employee_no_normalized,
       display_name, email, email_normalized, phone, system_role, status, locale,
       timezone, last_login_at, created_by_user_id, created_at, updated_at, deleted_at, row_version
FROM users
WHERE user_id = $1
FOR UPDATE;

-- name: ListManagedUsers :many
SELECT user_id, username, username_normalized, employee_no, employee_no_normalized,
       display_name, email, email_normalized, phone, system_role, status, locale,
       timezone, last_login_at, created_by_user_id, created_at, updated_at, deleted_at, row_version
FROM users
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('system_role')::text IS NULL OR system_role = sqlc.narg('system_role')::text)
ORDER BY created_at DESC, user_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: CountManagedUsers :one
SELECT count(*)::bigint
FROM users
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('system_role')::text IS NULL OR system_role = sqlc.narg('system_role')::text);

-- name: InsertManagedUser :one
INSERT INTO users (
    user_id, username, username_normalized, employee_no, employee_no_normalized,
    display_name, email, email_normalized, phone, system_role, status, locale,
    timezone, created_by_user_id, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'ACTIVE', $11, $12, $13, $14, $14
)
RETURNING user_id, username, username_normalized, employee_no, employee_no_normalized,
          display_name, email, email_normalized, phone, system_role, status, locale,
          timezone, last_login_at, created_by_user_id, created_at, updated_at, deleted_at, row_version;

-- name: InsertManagedPasswordCredential :exec
INSERT INTO user_credentials (
    user_credential_id, user_id, credential_type, identifier, identifier_normalized,
    secret_hash, status, created_at, updated_at
) VALUES ($1, $2, 'PASSWORD', $3, $4, $5, 'ACTIVE', $6, $6);

-- name: InsertPrincipalSecurityVersions :exec
INSERT INTO principal_security_versions (user_id, updated_at)
VALUES ($1, $2);

-- name: UpdateManagedUser :one
UPDATE users
SET employee_no = $2,
    employee_no_normalized = $3,
    display_name = $4,
    email = $5,
    email_normalized = $6,
    phone = $7,
    system_role = $8,
    locale = $9,
    timezone = $10,
    updated_at = $11,
    row_version = row_version + 1
WHERE user_id = $1
  AND row_version = $12
RETURNING user_id, username, username_normalized, employee_no, employee_no_normalized,
          display_name, email, email_normalized, phone, system_role, status, locale,
          timezone, last_login_at, created_by_user_id, created_at, updated_at, deleted_at, row_version;

-- name: SetManagedUserStatus :one
UPDATE users
SET status = $2,
    deleted_at = $3,
    updated_at = $4,
    row_version = row_version + 1
WHERE user_id = $1
  AND row_version = $5
RETURNING user_id, username, username_normalized, employee_no, employee_no_normalized,
          display_name, email, email_normalized, phone, system_role, status, locale,
          timezone, last_login_at, created_by_user_id, created_at, updated_at, deleted_at, row_version;

-- name: TouchManagedUserForPasswordReset :one
UPDATE users
SET updated_at = $2,
    row_version = row_version + 1
WHERE user_id = $1
  AND row_version = $3
RETURNING row_version;

-- name: UpdateManagedPasswordCredential :execrows
UPDATE user_credentials
SET secret_hash = $2,
    updated_at = $3,
    row_version = row_version + 1
WHERE user_id = $1
  AND credential_type = 'PASSWORD'
  AND status = 'ACTIVE';

-- name: LockActiveSystemAdministrators :many
SELECT user_id
FROM users
WHERE system_role = 'SYSTEM_ADMIN'
  AND status = 'ACTIVE'
ORDER BY user_id
FOR UPDATE;

-- name: LockSystemAdminMutation :exec
SELECT pg_advisory_xact_lock(hashtext('file_workshop.system_admin_mutation'));

-- name: IncrementGlobalAuthorizationVersion :exec
UPDATE principal_security_versions
SET global_authorization_version = global_authorization_version + 1,
    updated_at = $2
WHERE user_id = $1;

-- name: RevokeManagedUserSessions :exec
UPDATE user_sessions
SET status = 'REVOKED',
    revoked_at = $2,
    revoke_reason = $3,
    updated_at = $2,
    row_version = row_version + 1
WHERE user_id = $1
  AND status = 'ACTIVE';

-- name: RevokeManagedUserRefreshTokens :exec
UPDATE session_refresh_tokens AS token
SET status = 'REVOKED',
    revoked_at = $2,
    updated_at = $2,
    row_version = token.row_version + 1
WHERE token.status = 'ACTIVE'
  AND EXISTS (
      SELECT 1 FROM user_sessions AS session
      WHERE session.user_session_id = token.user_session_id
        AND session.user_id = $1
  );

-- name: RevokeManagedUserCredentials :exec
UPDATE user_credentials
SET status = 'REVOKED',
    revoked_at = $2,
    revoke_reason = $3,
    updated_at = $2,
    row_version = row_version + 1
WHERE user_id = $1
  AND status = 'ACTIVE';

-- name: ListManagedUserSessions :many
SELECT user_session_id, user_id, device_id, ip_address, user_agent, status, expires_at,
       last_seen_at, created_at, updated_at, revoked_at, revoke_reason, row_version
FROM user_sessions
WHERE user_id = $1
ORDER BY created_at DESC, user_session_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: CountManagedUserSessions :one
SELECT count(*)::bigint
FROM user_sessions
WHERE user_id = $1;

-- name: GetOwnedUserSession :one
SELECT user_session_id
FROM user_sessions
WHERE user_session_id = $1
  AND user_id = $2;

-- name: RevokeOwnedUserSessionTokens :exec
UPDATE session_refresh_tokens AS token
SET status = 'REVOKED',
    revoked_at = $3,
    updated_at = $3,
    row_version = row_version + 1
WHERE token.user_session_id = $1
  AND token.status = 'ACTIVE'
  AND EXISTS (
      SELECT 1 FROM user_sessions AS session
      WHERE session.user_session_id = $1
        AND session.user_id = $2
  );

-- name: RevokeOwnedUserSession :execrows
UPDATE user_sessions
SET status = 'REVOKED',
    revoked_at = $3,
    revoke_reason = 'USER_SESSION_REVOKED',
    updated_at = $3,
    row_version = row_version + 1
WHERE user_session_id = $1
  AND user_id = $2
  AND status = 'ACTIVE';

-- name: TryCreateUserIdempotencyRecord :execrows
INSERT INTO idempotency_records (
    idempotency_record_id, principal_kind, user_id, operation, idempotency_key,
    request_hash, status, expires_at, created_at, updated_at
) VALUES ($1, 'USER', $2, 'CREATE_USER', $3, $4, 'IN_PROGRESS', $5, $6, $6)
ON CONFLICT DO NOTHING;

-- name: GetUserIdempotencyRecordForUpdate :one
SELECT request_hash, status, result_resource_id
FROM idempotency_records
WHERE principal_kind = 'USER'
  AND user_id = $1
  AND operation = 'CREATE_USER'
  AND idempotency_key = $2
FOR UPDATE;

-- name: CompleteUserIdempotencyRecord :exec
UPDATE idempotency_records
SET status = 'COMPLETED',
    response_status_code = 201,
    response_schema_version = 1,
    response_json = jsonb_build_object('userId', $3::uuid),
    result_resource_type = 'USER',
    result_resource_id = $3,
    completed_at = $4,
    updated_at = $4,
    row_version = row_version + 1
WHERE user_id = $1
  AND operation = 'CREATE_USER'
  AND idempotency_key = $2
  AND status = 'IN_PROGRESS';

-- name: InsertUserOutboxEvent :exec
INSERT INTO outbox_events (
    outbox_event_id, aggregate_type, aggregate_id, aggregate_version, event_type,
    event_schema_version, payload_json, deduplication_key, correlation_id,
    max_attempts, available_at, created_at, updated_at
) VALUES ($1, 'USER', $2, $3, $4, 1, $5, $6, $7, 10, $8, $8, $8);
