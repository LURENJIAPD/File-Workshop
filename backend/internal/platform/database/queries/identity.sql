-- name: GetPasswordLoginIdentity :one
SELECT
    u.user_id,
    u.username,
    u.display_name,
    u.system_role,
    u.status AS user_status,
    u.locale,
    u.timezone,
    uc.user_credential_id,
    uc.secret_hash
FROM users AS u
JOIN user_credentials AS uc ON uc.user_id = u.user_id
WHERE u.username_normalized = $1
  AND uc.credential_type = 'PASSWORD'
  AND uc.status = 'ACTIVE'
  AND (uc.expires_at IS NULL OR uc.expires_at > $2)
LIMIT 1;

-- name: GetRecentLoginFailureState :one
SELECT
    count(*)::bigint AS failure_count,
    max(created_at)::timestamptz AS last_failure_at
FROM login_attempts AS attempt
WHERE attempt.username_normalized = $1
  AND attempt.result = 'FAILURE'
  AND attempt.created_at >= $2
  AND attempt.created_at > COALESCE((
      SELECT max(success.created_at)
      FROM login_attempts AS success
      WHERE success.username_normalized = $1
        AND success.result = 'SUCCESS'
        AND success.created_at >= $2
  ), $2);

-- name: InsertLoginAttempt :exec
INSERT INTO login_attempts (
    login_attempt_id,
    username_normalized,
    user_id,
    result,
    failure_code,
    ip_address,
    user_agent,
    request_id,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
);

-- name: CreateUserSession :one
INSERT INTO user_sessions (
    user_session_id,
    user_id,
    device_id,
    ip_address,
    user_agent,
    status,
    expires_at,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, 'ACTIVE', $6, $7, $7
)
RETURNING user_session_id, user_id, device_id, status, expires_at, last_seen_at, created_at;

-- name: CreateSessionRefreshToken :one
INSERT INTO session_refresh_tokens (
    refresh_token_id,
    user_session_id,
    token_family_id,
    parent_refresh_token_id,
    rotation_number,
    token_hash,
    status,
    issued_at,
    expires_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, 'ACTIVE', $7, $8, $7
)
RETURNING refresh_token_id, user_session_id, token_family_id, parent_refresh_token_id,
          rotation_number, status, issued_at, expires_at;

-- name: TouchUserAfterLogin :exec
UPDATE users
SET last_login_at = $2,
    updated_at = $2,
    row_version = row_version + 1
WHERE user_id = $1;

-- name: TouchCredentialAfterLogin :exec
UPDATE user_credentials
SET last_used_at = $2,
    updated_at = $2,
    row_version = row_version + 1
WHERE user_credential_id = $1;

-- name: GetRefreshTokenForUpdate :one
SELECT
    rt.refresh_token_id,
    rt.user_session_id,
    rt.token_family_id,
    rt.rotation_number,
    rt.status AS refresh_token_status,
    rt.expires_at AS refresh_token_expires_at,
    s.device_id,
    s.status AS session_status,
    s.expires_at AS session_expires_at,
    s.last_seen_at,
    s.created_at AS session_created_at,
    u.user_id,
    u.username,
    u.display_name,
    u.system_role,
    u.status AS user_status,
    u.locale,
    u.timezone
FROM session_refresh_tokens AS rt
JOIN user_sessions AS s ON s.user_session_id = rt.user_session_id
JOIN users AS u ON u.user_id = s.user_id
WHERE rt.token_hash = $1
FOR UPDATE OF rt, s;

-- name: MarkRefreshTokenUsed :execrows
UPDATE session_refresh_tokens
SET status = 'USED',
    used_at = $2,
    updated_at = $2,
    row_version = row_version + 1
WHERE refresh_token_id = $1
  AND status = 'ACTIVE';

-- name: MarkRefreshTokenReused :exec
UPDATE session_refresh_tokens
SET status = 'REUSED',
    revoked_at = $2,
    updated_at = $2,
    row_version = row_version + 1
WHERE refresh_token_id = $1
  AND status = 'USED';

-- name: RevokeUserSession :exec
UPDATE user_sessions
SET status = 'REVOKED',
    revoked_at = COALESCE(revoked_at, $2),
    revoke_reason = COALESCE(revoke_reason, $3),
    updated_at = $2,
    row_version = row_version + 1
WHERE user_session_id = $1
  AND status = 'ACTIVE';

-- name: RevokeActiveRefreshTokensForSession :exec
UPDATE session_refresh_tokens
SET status = 'REVOKED',
    revoked_at = $2,
    updated_at = $2,
    row_version = row_version + 1
WHERE user_session_id = $1
  AND status = 'ACTIVE';

-- name: GetCurrentSessionIdentity :one
SELECT
    s.user_session_id,
    s.device_id,
    s.status AS session_status,
    s.expires_at AS session_expires_at,
    s.last_seen_at,
    s.created_at AS session_created_at,
    u.user_id,
    u.username,
    u.display_name,
    u.system_role,
    u.status AS user_status,
    u.locale,
    u.timezone
FROM user_sessions AS s
JOIN users AS u ON u.user_id = s.user_id
WHERE s.user_session_id = $1;

-- name: GetSessionIDByRefreshTokenHash :one
SELECT user_session_id
FROM session_refresh_tokens
WHERE token_hash = $1;

-- name: TouchUserSession :exec
UPDATE user_sessions
SET last_seen_at = sqlc.arg(last_seen_at)::timestamptz,
    updated_at = sqlc.arg(last_seen_at)::timestamptz,
    row_version = row_version + 1
WHERE user_session_id = $1
  AND status = 'ACTIVE'
  AND (last_seen_at IS NULL OR last_seen_at < sqlc.arg(last_seen_at)::timestamptz - interval '5 minutes');
