-- +goose Up
/*
 File Workshop V1.0 - Goose PostgreSQL 首版基线 Migration
 设计依据: docs/File-Workshop-V1.0-数据库设计说明.md

 Goose 执行说明:
 1. 使用具有 CREATE SCHEMA、CREATE TABLE 权限的 Migration 账号连接目标 PostgreSQL 数据库。
 2. 通过仓库固定版本的 Goose 在空库执行，不随应用启动隐式执行。
 3. 脚本不创建数据库、不删除旧对象、不创建应用角色，也不写入业务种子数据。
 4. 业务 UUID 由 Go 应用生成 UUIDv7，本脚本不会为主键设置随机 UUID 默认值。
 5. 若服务器提供 pgvector 且当前账号可创建扩展，脚本会自动创建 vector 扩展和
    chunk_embeddings 表；否则跳过 AI 向量表，并在消息与末尾验证结果中明确提示。
 6. 脚本按当前日期创建上月、当月和未来 3 个月的审计、登录尝试与 Agent 调用分区，
    同时创建 DEFAULT 应急分区。生产环境仍需由维护任务持续提前创建分区。
 7. 本文件仅用于首版空库初始化，不是可重复执行脚本；Goose 在事务中执行，任一语句失败时整体回滚。

 兼容目标: PostgreSQL 15 及以上。
*/

-- +goose StatementBegin

SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '0';
SET LOCAL idle_in_transaction_session_timeout = '15min';
SET LOCAL timezone = 'UTC';

CREATE SCHEMA file_workshop;
SET LOCAL search_path = file_workshop, public;

-- ============================================================================
-- 1. 身份、用户与会话
-- ============================================================================

CREATE TABLE users (
    user_id uuid PRIMARY KEY,
    username varchar(128) NOT NULL,
    username_normalized varchar(128) NOT NULL,
    employee_no varchar(128),
    employee_no_normalized varchar(128),
    display_name varchar(128) NOT NULL,
    email varchar(256),
    email_normalized varchar(256),
    phone varchar(64),
    avatar_storage_object_id uuid,
    system_role varchar(32) NOT NULL DEFAULT 'USER',
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    locale varchar(16) NOT NULL DEFAULT 'zh-CN',
    timezone varchar(64) NOT NULL DEFAULT 'UTC',
    last_login_at timestamptz,
    created_by_user_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT ck_users_username_nonblank CHECK (btrim(username_normalized) <> ''),
    CONSTRAINT ck_users_system_role CHECK (system_role IN ('USER', 'SYSTEM_ADMIN')),
    CONSTRAINT ck_users_status CHECK (status IN ('ACTIVE', 'DISABLED', 'LOCKED', 'DELETED')),
    CONSTRAINT ck_users_deleted_state CHECK ((status = 'DELETED') = (deleted_at IS NOT NULL)),
    CONSTRAINT ck_users_row_version CHECK (row_version >= 1),
    CONSTRAINT fk_users_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE SET NULL
);

CREATE TABLE user_credentials (
    user_credential_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    credential_type varchar(32) NOT NULL,
    provider varchar(64),
    identifier varchar(256) NOT NULL,
    identifier_normalized varchar(256) NOT NULL,
    secret_hash text,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    expires_at timestamptz,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at timestamptz,
    revoke_reason text,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_user_credentials_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_user_credentials_type CHECK (credential_type IN ('PASSWORD', 'LDAP', 'OIDC', 'APP_PASSWORD')),
    CONSTRAINT ck_user_credentials_identifier CHECK (btrim(identifier_normalized) <> ''),
    CONSTRAINT ck_user_credentials_secret CHECK (
        (credential_type IN ('PASSWORD', 'APP_PASSWORD') AND secret_hash IS NOT NULL)
        OR (credential_type IN ('LDAP', 'OIDC') AND secret_hash IS NULL)
    ),
    CONSTRAINT ck_user_credentials_status CHECK (status IN ('ACTIVE', 'REVOKED', 'EXPIRED')),
    CONSTRAINT ck_user_credentials_revoked_state CHECK ((status = 'REVOKED') = (revoked_at IS NOT NULL)),
    CONSTRAINT ck_user_credentials_expiry CHECK (expires_at IS NULL OR expires_at > created_at),
    CONSTRAINT ck_user_credentials_row_version CHECK (row_version >= 1)
);

CREATE TABLE user_mfa_methods (
    user_mfa_method_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    method_type varchar(32) NOT NULL,
    label varchar(128) NOT NULL,
    secret_ref varchar(512),
    credential_id bytea,
    public_key bytea,
    sign_count bigint,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    verified_at timestamptz,
    last_used_at timestamptz,
    revoked_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_user_mfa_methods_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT uq_user_mfa_methods_credential_id UNIQUE (credential_id),
    CONSTRAINT ck_user_mfa_methods_type CHECK (method_type IN ('TOTP', 'WEBAUTHN')),
    CONSTRAINT ck_user_mfa_methods_payload CHECK (
        (method_type = 'TOTP' AND secret_ref IS NOT NULL AND credential_id IS NULL AND public_key IS NULL AND sign_count IS NULL)
        OR
        (method_type = 'WEBAUTHN' AND secret_ref IS NULL AND credential_id IS NOT NULL AND public_key IS NOT NULL AND sign_count IS NOT NULL)
    ),
    CONSTRAINT ck_user_mfa_methods_sign_count CHECK (sign_count IS NULL OR sign_count >= 0),
    CONSTRAINT ck_user_mfa_methods_status CHECK (status IN ('PENDING', 'ACTIVE', 'REVOKED')),
    CONSTRAINT ck_user_mfa_methods_revoked_state CHECK ((status = 'REVOKED') = (revoked_at IS NOT NULL)),
    CONSTRAINT ck_user_mfa_methods_row_version CHECK (row_version >= 1)
);

CREATE TABLE mfa_recovery_codes (
    mfa_recovery_code_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    code_batch_id uuid NOT NULL,
    code_hash bytea NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    used_at timestamptz,
    revoked_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_mfa_recovery_codes_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT uq_mfa_recovery_codes_hash UNIQUE (code_hash),
    CONSTRAINT ck_mfa_recovery_codes_hash CHECK (octet_length(code_hash) = 32),
    CONSTRAINT ck_mfa_recovery_codes_status CHECK (status IN ('ACTIVE', 'USED', 'REVOKED')),
    CONSTRAINT ck_mfa_recovery_codes_state CHECK (
        (status = 'ACTIVE' AND used_at IS NULL AND revoked_at IS NULL)
        OR (status = 'USED' AND used_at IS NOT NULL AND revoked_at IS NULL)
        OR (status = 'REVOKED' AND used_at IS NULL AND revoked_at IS NOT NULL)
    ),
    CONSTRAINT ck_mfa_recovery_codes_row_version CHECK (row_version >= 1)
);

CREATE TABLE user_password_history (
    user_password_history_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    secret_hash text NOT NULL,
    password_changed_at timestamptz NOT NULL,
    created_by_user_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_user_password_history_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_user_password_history_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE SET NULL
);

CREATE TABLE user_sessions (
    user_session_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    device_id varchar(256),
    ip_address inet,
    user_agent text,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at timestamptz,
    revoke_reason text,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_user_sessions_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_user_sessions_status CHECK (status IN ('ACTIVE', 'REVOKED', 'EXPIRED')),
    CONSTRAINT ck_user_sessions_expiry CHECK (expires_at > created_at),
    CONSTRAINT ck_user_sessions_revoked_state CHECK ((status = 'REVOKED') = (revoked_at IS NOT NULL)),
    CONSTRAINT ck_user_sessions_row_version CHECK (row_version >= 1)
);

CREATE TABLE session_refresh_tokens (
    refresh_token_id uuid PRIMARY KEY,
    user_session_id uuid NOT NULL,
    token_family_id uuid NOT NULL,
    parent_refresh_token_id uuid,
    rotation_number integer NOT NULL,
    token_hash bytea NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    used_at timestamptz,
    revoked_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_session_refresh_tokens_session FOREIGN KEY (user_session_id)
        REFERENCES user_sessions(user_session_id) ON DELETE RESTRICT,
    CONSTRAINT fk_session_refresh_tokens_parent FOREIGN KEY (parent_refresh_token_id)
        REFERENCES session_refresh_tokens(refresh_token_id) ON DELETE RESTRICT,
    CONSTRAINT uq_session_refresh_tokens_hash UNIQUE (token_hash),
    CONSTRAINT uq_session_refresh_tokens_rotation UNIQUE (token_family_id, rotation_number),
    CONSTRAINT ck_session_refresh_tokens_rotation CHECK (rotation_number >= 1),
    CONSTRAINT ck_session_refresh_tokens_hash CHECK (octet_length(token_hash) = 32),
    CONSTRAINT ck_session_refresh_tokens_status CHECK (status IN ('ACTIVE', 'USED', 'REVOKED', 'REUSED', 'EXPIRED')),
    CONSTRAINT ck_session_refresh_tokens_expiry CHECK (expires_at > issued_at),
    CONSTRAINT ck_session_refresh_tokens_state CHECK (
        (status = 'ACTIVE' AND used_at IS NULL AND revoked_at IS NULL)
        OR (status = 'USED' AND used_at IS NOT NULL AND revoked_at IS NULL)
        OR (status IN ('REVOKED', 'REUSED') AND revoked_at IS NOT NULL)
        OR (status = 'EXPIRED')
    ),
    CONSTRAINT ck_session_refresh_tokens_row_version CHECK (row_version >= 1)
);

CREATE TABLE login_attempts (
    login_attempt_id uuid NOT NULL,
    username_normalized varchar(128) NOT NULL,
    user_id uuid,
    result varchar(32) NOT NULL,
    failure_code varchar(64),
    ip_address inet,
    user_agent text,
    request_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_login_attempts PRIMARY KEY (created_at, login_attempt_id),
    CONSTRAINT ck_login_attempts_username CHECK (btrim(username_normalized) <> ''),
    CONSTRAINT ck_login_attempts_result CHECK (result IN ('SUCCESS', 'FAILURE', 'LOCKED')),
    CONSTRAINT ck_login_attempts_failure CHECK ((result = 'SUCCESS' AND failure_code IS NULL) OR result <> 'SUCCESS')
) PARTITION BY RANGE (created_at);

CREATE TABLE principal_security_versions (
    user_id uuid PRIMARY KEY,
    organization_membership_version bigint NOT NULL DEFAULT 1,
    delegation_version bigint NOT NULL DEFAULT 1,
    direct_grant_version bigint NOT NULL DEFAULT 1,
    share_version bigint NOT NULL DEFAULT 1,
    global_authorization_version bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_principal_security_versions_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_principal_security_versions_values CHECK (
        organization_membership_version >= 1 AND delegation_version >= 1
        AND direct_grant_version >= 1 AND share_version >= 1
        AND global_authorization_version >= 1
    )
);

CREATE TABLE user_offboarding_cases (
    user_offboarding_case_id uuid PRIMARY KEY,
    departing_user_id uuid NOT NULL,
    receiver_user_id uuid,
    target_space_id uuid,
    target_folder_id uuid,
    disposition varchar(32) NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'DRAFT',
    created_by_user_id uuid NOT NULL,
    approved_by_user_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    approved_at timestamptz,
    completed_at timestamptz,
    failure_code varchar(64),
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_user_offboarding_departing_user FOREIGN KEY (departing_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_user_offboarding_receiver_user FOREIGN KEY (receiver_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_user_offboarding_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_user_offboarding_approved_by FOREIGN KEY (approved_by_user_id)
        REFERENCES users(user_id) ON DELETE SET NULL,
    CONSTRAINT ck_user_offboarding_distinct_users CHECK (receiver_user_id IS NULL OR receiver_user_id <> departing_user_id),
    CONSTRAINT ck_user_offboarding_disposition CHECK (disposition IN ('TRANSFER', 'ARCHIVE', 'MIXED', 'RETAIN_FROZEN')),
    CONSTRAINT ck_user_offboarding_status CHECK (status IN ('DRAFT', 'APPROVED', 'PROCESSING', 'COMPLETED', 'CANCELLED', 'FAILED')),
    CONSTRAINT ck_user_offboarding_approval CHECK (
        (status IN ('APPROVED', 'PROCESSING', 'COMPLETED') AND approved_at IS NOT NULL)
        OR status IN ('DRAFT', 'CANCELLED', 'FAILED')
    ),
    CONSTRAINT ck_user_offboarding_completion CHECK ((status = 'COMPLETED') = (completed_at IS NOT NULL)),
    CONSTRAINT ck_user_offboarding_row_version CHECK (row_version >= 1)
);

-- ============================================================================
-- 2. 组织、成员与空间
-- ============================================================================

CREATE TABLE organizations (
    organization_id uuid PRIMARY KEY,
    parent_organization_id uuid,
    name varchar(256) NOT NULL,
    normalized_name varchar(256) NOT NULL,
    code varchar(128),
    normalized_code varchar(128),
    type_label varchar(64),
    sort_order integer NOT NULL DEFAULT 0,
    path_cache text,
    depth integer NOT NULL DEFAULT 0,
    tree_version bigint NOT NULL DEFAULT 1,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_organizations_parent FOREIGN KEY (parent_organization_id)
        REFERENCES organizations(organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_organizations_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_organizations_parent CHECK (parent_organization_id IS NULL OR parent_organization_id <> organization_id),
    CONSTRAINT ck_organizations_name CHECK (btrim(normalized_name) <> ''),
    CONSTRAINT ck_organizations_depth CHECK (depth >= 0),
    CONSTRAINT ck_organizations_tree_version CHECK (tree_version >= 1),
    CONSTRAINT ck_organizations_status CHECK (status IN ('ACTIVE', 'DISABLED', 'ARCHIVED', 'DELETED')),
    CONSTRAINT ck_organizations_deleted_state CHECK ((status = 'DELETED') = (deleted_at IS NOT NULL)),
    CONSTRAINT ck_organizations_row_version CHECK (row_version >= 1)
);

CREATE TABLE organization_closure (
    ancestor_organization_id uuid NOT NULL,
    descendant_organization_id uuid NOT NULL,
    depth integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_organization_closure PRIMARY KEY (ancestor_organization_id, descendant_organization_id),
    CONSTRAINT fk_organization_closure_ancestor FOREIGN KEY (ancestor_organization_id)
        REFERENCES organizations(organization_id) ON DELETE CASCADE,
    CONSTRAINT fk_organization_closure_descendant FOREIGN KEY (descendant_organization_id)
        REFERENCES organizations(organization_id) ON DELETE CASCADE,
    CONSTRAINT ck_organization_closure_depth CHECK (
        depth >= 0 AND ((depth = 0) = (ancestor_organization_id = descendant_organization_id))
    )
);

CREATE TABLE user_organizations (
    user_organization_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    membership_type varchar(32) NOT NULL,
    job_title varchar(128),
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    effective_from timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    effective_until timestamptz,
    created_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_user_organizations_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_user_organizations_organization FOREIGN KEY (organization_id)
        REFERENCES organizations(organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_user_organizations_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_user_organizations_type CHECK (membership_type IN ('PRIMARY', 'MEMBER')),
    CONSTRAINT ck_user_organizations_status CHECK (status IN ('ACTIVE', 'INACTIVE')),
    CONSTRAINT ck_user_organizations_period CHECK (effective_until IS NULL OR effective_until > effective_from),
    CONSTRAINT ck_user_organizations_row_version CHECK (row_version >= 1)
);

CREATE TABLE organization_security_versions (
    organization_id uuid PRIMARY KEY,
    membership_version bigint NOT NULL DEFAULT 1,
    delegation_version bigint NOT NULL DEFAULT 1,
    grant_version bigint NOT NULL DEFAULT 1,
    share_version bigint NOT NULL DEFAULT 1,
    subtree_security_epoch bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_organization_security_versions_org FOREIGN KEY (organization_id)
        REFERENCES organizations(organization_id) ON DELETE RESTRICT,
    CONSTRAINT ck_organization_security_versions_values CHECK (
        membership_version >= 1 AND delegation_version >= 1 AND grant_version >= 1
        AND share_version >= 1 AND subtree_security_epoch >= 1
    )
);

CREATE TABLE spaces (
    space_id uuid PRIMARY KEY,
    space_type varchar(32) NOT NULL,
    name varchar(256) NOT NULL,
    normalized_name varchar(256) NOT NULL,
    owner_user_id uuid,
    organization_id uuid,
    root_folder_id uuid,
    quota_bytes bigint NOT NULL,
    used_bytes bigint NOT NULL DEFAULT 0,
    reserved_bytes bigint NOT NULL DEFAULT 0,
    acl_version bigint NOT NULL DEFAULT 1,
    security_epoch bigint NOT NULL DEFAULT 1,
    config_schema_version integer NOT NULL DEFAULT 1,
    config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_spaces_owner_user FOREIGN KEY (owner_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_spaces_organization FOREIGN KEY (organization_id)
        REFERENCES organizations(organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_spaces_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_spaces_type CHECK (space_type IN ('PERSONAL', 'ORGANIZATION', 'PUBLIC')),
    CONSTRAINT ck_spaces_owner CHECK (
        (space_type = 'PERSONAL' AND owner_user_id IS NOT NULL AND organization_id IS NULL)
        OR (space_type = 'ORGANIZATION' AND owner_user_id IS NULL AND organization_id IS NOT NULL)
        OR (space_type = 'PUBLIC' AND owner_user_id IS NULL AND organization_id IS NULL)
    ),
    CONSTRAINT ck_spaces_quota CHECK (
        quota_bytes >= 0 AND used_bytes >= 0 AND reserved_bytes >= 0
        AND used_bytes <= quota_bytes AND reserved_bytes <= quota_bytes - used_bytes
    ),
    CONSTRAINT ck_spaces_versions CHECK (acl_version >= 1 AND security_epoch >= 1 AND config_schema_version >= 1),
    CONSTRAINT ck_spaces_config_object CHECK (jsonb_typeof(config_json) = 'object'),
    CONSTRAINT ck_spaces_status CHECK (status IN ('ACTIVE', 'FROZEN', 'ARCHIVED', 'DELETED')),
    CONSTRAINT ck_spaces_deleted_state CHECK ((status = 'DELETED') = (deleted_at IS NOT NULL)),
    CONSTRAINT ck_spaces_row_version CHECK (row_version >= 1)
);

CREATE TABLE quota_reservations (
    quota_reservation_id uuid PRIMARY KEY,
    space_id uuid NOT NULL,
    user_id uuid NOT NULL,
    reserved_bytes bigint NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    consumed_at timestamptz,
    released_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_quota_reservations_space FOREIGN KEY (space_id)
        REFERENCES spaces(space_id) ON DELETE RESTRICT,
    CONSTRAINT fk_quota_reservations_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_quota_reservations_bytes CHECK (reserved_bytes > 0),
    CONSTRAINT ck_quota_reservations_status CHECK (status IN ('ACTIVE', 'CONSUMED', 'RELEASED', 'EXPIRED')),
    CONSTRAINT ck_quota_reservations_expiry CHECK (expires_at > created_at),
    CONSTRAINT ck_quota_reservations_state CHECK (
        (status = 'ACTIVE' AND consumed_at IS NULL AND released_at IS NULL)
        OR (status = 'CONSUMED' AND consumed_at IS NOT NULL AND released_at IS NULL)
        OR (status IN ('RELEASED', 'EXPIRED') AND consumed_at IS NULL AND released_at IS NOT NULL)
    ),
    CONSTRAINT ck_quota_reservations_row_version CHECK (row_version >= 1)
);

CREATE TABLE admin_delegations (
    admin_delegation_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    scope varchar(32) NOT NULL,
    can_delegate boolean NOT NULL DEFAULT false,
    parent_admin_delegation_id uuid,
    granted_by_user_id uuid NOT NULL,
    valid_from timestamptz NOT NULL,
    valid_until timestamptz,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at timestamptz,
    revoke_reason text,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_admin_delegations_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_admin_delegations_organization FOREIGN KEY (organization_id)
        REFERENCES organizations(organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_admin_delegations_parent FOREIGN KEY (parent_admin_delegation_id)
        REFERENCES admin_delegations(admin_delegation_id) ON DELETE RESTRICT,
    CONSTRAINT fk_admin_delegations_granted_by FOREIGN KEY (granted_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_admin_delegations_parent CHECK (parent_admin_delegation_id IS NULL OR parent_admin_delegation_id <> admin_delegation_id),
    CONSTRAINT ck_admin_delegations_scope CHECK (scope IN ('SELF', 'SUBTREE')),
    CONSTRAINT ck_admin_delegations_period CHECK (valid_until IS NULL OR valid_until > valid_from),
    CONSTRAINT ck_admin_delegations_status CHECK (status IN ('ACTIVE', 'REVOKED', 'EXPIRED', 'INVALIDATED')),
    CONSTRAINT ck_admin_delegations_revoked_state CHECK ((status = 'REVOKED') = (revoked_at IS NOT NULL)),
    CONSTRAINT ck_admin_delegations_row_version CHECK (row_version >= 1)
);

CREATE TABLE admin_delegation_capabilities (
    admin_delegation_id uuid NOT NULL,
    capability varchar(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_admin_delegation_capabilities PRIMARY KEY (admin_delegation_id, capability),
    CONSTRAINT fk_admin_delegation_capabilities_delegation FOREIGN KEY (admin_delegation_id)
        REFERENCES admin_delegations(admin_delegation_id) ON DELETE CASCADE,
    CONSTRAINT ck_admin_delegation_capabilities_code CHECK (capability IN (
        'MANAGE_SPACE_CONTENT', 'MANAGE_SPACE_PERMISSION', 'MANAGE_SPACE_MEMBERS',
        'MANAGE_SPACE_RECYCLE_BIN', 'FORCE_UNLOCK', 'VIEW_SPACE_AUDIT', 'DELEGATE_ADMIN'
    ))
);

CREATE TABLE organization_change_plans (
    organization_change_plan_id uuid PRIMARY KEY,
    plan_type varchar(32) NOT NULL,
    name varchar(256) NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'DRAFT',
    expected_tree_version bigint NOT NULL,
    created_by_user_id uuid NOT NULL,
    approved_by_user_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    validated_at timestamptz,
    approved_at timestamptz,
    started_at timestamptz,
    completed_at timestamptz,
    failure_code varchar(64),
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_organization_change_plans_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_organization_change_plans_approved_by FOREIGN KEY (approved_by_user_id)
        REFERENCES users(user_id) ON DELETE SET NULL,
    CONSTRAINT ck_organization_change_plans_type CHECK (plan_type IN ('MOVE', 'MERGE', 'SPLIT', 'BULK_RESTRUCTURE')),
    CONSTRAINT ck_organization_change_plans_status CHECK (status IN ('DRAFT', 'VALIDATED', 'APPROVED', 'EXECUTING', 'COMPLETED', 'CANCELLED', 'FAILED')),
    CONSTRAINT ck_organization_change_plans_tree_version CHECK (expected_tree_version >= 1),
    CONSTRAINT ck_organization_change_plans_completion CHECK ((status = 'COMPLETED') = (completed_at IS NOT NULL)),
    CONSTRAINT ck_organization_change_plans_row_version CHECK (row_version >= 1)
);

CREATE TABLE organization_change_operations (
    organization_change_operation_id uuid PRIMARY KEY,
    organization_change_plan_id uuid NOT NULL,
    sequence_number integer NOT NULL,
    operation_type varchar(32) NOT NULL,
    source_organization_id uuid,
    target_organization_id uuid,
    operation_schema_version integer NOT NULL,
    operation_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at timestamptz,
    failure_code varchar(64),
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_org_change_operations_plan FOREIGN KEY (organization_change_plan_id)
        REFERENCES organization_change_plans(organization_change_plan_id) ON DELETE CASCADE,
    CONSTRAINT fk_org_change_operations_source FOREIGN KEY (source_organization_id)
        REFERENCES organizations(organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_org_change_operations_target FOREIGN KEY (target_organization_id)
        REFERENCES organizations(organization_id) ON DELETE RESTRICT,
    CONSTRAINT uq_org_change_operations_sequence UNIQUE (organization_change_plan_id, sequence_number),
    CONSTRAINT ck_org_change_operations_sequence CHECK (sequence_number >= 1),
    CONSTRAINT ck_org_change_operations_type CHECK (operation_type IN ('MOVE_NODE', 'MERGE_NODE', 'CREATE_NODE', 'MOVE_MEMBER', 'MOVE_SPACE_CONTENT')),
    CONSTRAINT ck_org_change_operations_schema CHECK (operation_schema_version >= 1),
    CONSTRAINT ck_org_change_operations_json CHECK (jsonb_typeof(operation_json) = 'object'),
    CONSTRAINT ck_org_change_operations_status CHECK (status IN ('PENDING', 'SUCCESS', 'FAILED', 'SKIPPED')),
    CONSTRAINT ck_org_change_operations_completion CHECK (
        (status IN ('SUCCESS', 'FAILED', 'SKIPPED') AND completed_at IS NOT NULL)
        OR (status = 'PENDING' AND completed_at IS NULL)
    ),
    CONSTRAINT ck_org_change_operations_row_version CHECK (row_version >= 1)
);

-- ============================================================================
-- 3. 统一命名空间、文件、存储、上传与锁
-- ============================================================================

CREATE TABLE namespace_entries (
    namespace_entry_id uuid PRIMARY KEY,
    space_id uuid NOT NULL,
    parent_folder_id uuid,
    entry_type varchar(32) NOT NULL,
    name varchar(512) NOT NULL,
    normalized_name varchar(512) NOT NULL,
    path_cache text,
    depth integer NOT NULL DEFAULT 0,
    lifecycle_status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_namespace_entries_space FOREIGN KEY (space_id)
        REFERENCES spaces(space_id) ON DELETE RESTRICT,
    CONSTRAINT fk_namespace_entries_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_namespace_entries_type CHECK (entry_type IN ('FOLDER', 'DOCUMENT', 'SHARED_ENTRY')),
    CONSTRAINT ck_namespace_entries_name CHECK (
        btrim(name) <> '' AND btrim(normalized_name) <> ''
        AND name NOT IN ('.', '..') AND normalized_name NOT IN ('.', '..')
        AND name !~ E'[\\/\\\\]' AND normalized_name !~ E'[\\/\\\\]'
        AND name !~ '[[:cntrl:]]' AND normalized_name !~ '[[:cntrl:]]'
    ),
    CONSTRAINT ck_namespace_entries_depth CHECK (depth >= 0),
    CONSTRAINT ck_namespace_entries_lifecycle CHECK (lifecycle_status IN ('ACTIVE', 'TRASHED', 'ARCHIVED', 'PURGING', 'PURGED')),
    CONSTRAINT ck_namespace_entries_deleted_state CHECK (
        (lifecycle_status IN ('PURGING', 'PURGED')) = (deleted_at IS NOT NULL)
    ),
    CONSTRAINT ck_namespace_entries_row_version CHECK (row_version >= 1)
);

CREATE TABLE folders (
    folder_id uuid PRIMARY KEY,
    inheritance_mode varchar(32) NOT NULL DEFAULT 'INHERIT',
    acl_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_folders_namespace_entry FOREIGN KEY (folder_id)
        REFERENCES namespace_entries(namespace_entry_id) ON DELETE RESTRICT,
    CONSTRAINT ck_folders_inheritance CHECK (inheritance_mode IN ('INHERIT', 'BREAK')),
    CONSTRAINT ck_folders_versions CHECK (acl_version >= 1 AND row_version >= 1)
);

ALTER TABLE namespace_entries
    ADD CONSTRAINT fk_namespace_entries_parent_folder
    FOREIGN KEY (parent_folder_id) REFERENCES folders(folder_id)
    ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE documents (
    document_id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL,
    current_version_id uuid,
    availability_status varchar(32) NOT NULL DEFAULT 'PENDING_SCAN',
    extension_normalized varchar(64),
    inheritance_mode varchar(32) NOT NULL DEFAULT 'INHERIT',
    acl_version bigint NOT NULL DEFAULT 1,
    classification varchar(64),
    metadata_schema_version integer NOT NULL DEFAULT 1,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_documents_namespace_entry FOREIGN KEY (document_id)
        REFERENCES namespace_entries(namespace_entry_id) ON DELETE RESTRICT,
    CONSTRAINT fk_documents_owner_user FOREIGN KEY (owner_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_documents_availability CHECK (availability_status IN ('AVAILABLE', 'PENDING_SCAN', 'QUARANTINED', 'BLOCKED')),
    CONSTRAINT ck_documents_inheritance CHECK (inheritance_mode IN ('INHERIT', 'BREAK')),
    CONSTRAINT ck_documents_versions CHECK (acl_version >= 1 AND metadata_schema_version >= 1 AND row_version >= 1),
    CONSTRAINT ck_documents_metadata CHECK (jsonb_typeof(metadata_json) = 'object')
);

CREATE TABLE storage_objects (
    storage_object_id uuid PRIMARY KEY,
    provider varchar(64) NOT NULL,
    bucket varchar(256) NOT NULL,
    object_key varchar(1024) NOT NULL,
    provider_version_id varchar(512),
    size_bytes bigint NOT NULL,
    sha256 bytea NOT NULL,
    etag varchar(256),
    storage_class varchar(64) NOT NULL,
    encryption_key_ref varchar(512),
    scan_status varchar(32) NOT NULL DEFAULT 'PENDING',
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    verified_at timestamptz,
    deleted_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT uq_storage_objects_location UNIQUE (provider, bucket, object_key),
    CONSTRAINT ck_storage_objects_location CHECK (btrim(provider) <> '' AND btrim(bucket) <> '' AND btrim(object_key) <> ''),
    CONSTRAINT ck_storage_objects_size CHECK (size_bytes >= 0),
    CONSTRAINT ck_storage_objects_sha256 CHECK (octet_length(sha256) = 32),
    CONSTRAINT ck_storage_objects_scan_status CHECK (scan_status IN ('PENDING', 'CLEAN', 'INFECTED', 'FAILED')),
    CONSTRAINT ck_storage_objects_status CHECK (status IN ('ACTIVE', 'ORPHAN', 'PENDING_DELETE', 'DELETED')),
    CONSTRAINT ck_storage_objects_deleted_state CHECK ((status = 'DELETED') = (deleted_at IS NOT NULL)),
    CONSTRAINT ck_storage_objects_row_version CHECK (row_version >= 1)
);

ALTER TABLE users
    ADD CONSTRAINT fk_users_avatar_storage_object
    FOREIGN KEY (avatar_storage_object_id) REFERENCES storage_objects(storage_object_id)
    ON DELETE SET NULL;

CREATE TABLE document_versions (
    document_version_id uuid PRIMARY KEY,
    document_id uuid NOT NULL,
    version_number bigint NOT NULL,
    storage_object_id uuid NOT NULL,
    size_bytes bigint NOT NULL,
    sha256 bytea NOT NULL,
    mime_type varchar(256) NOT NULL,
    change_note text,
    source_type varchar(32) NOT NULL,
    restored_from_version_id uuid,
    created_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_document_versions_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE RESTRICT,
    CONSTRAINT fk_document_versions_storage_object FOREIGN KEY (storage_object_id)
        REFERENCES storage_objects(storage_object_id) ON DELETE RESTRICT,
    CONSTRAINT fk_document_versions_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT uq_document_versions_number UNIQUE (document_id, version_number),
    CONSTRAINT uq_document_versions_document_id UNIQUE (document_id, document_version_id),
    CONSTRAINT ck_document_versions_number CHECK (version_number >= 1),
    CONSTRAINT ck_document_versions_size CHECK (size_bytes >= 0),
    CONSTRAINT ck_document_versions_sha256 CHECK (octet_length(sha256) = 32),
    CONSTRAINT ck_document_versions_mime CHECK (btrim(mime_type) <> ''),
    CONSTRAINT ck_document_versions_source CHECK (source_type IN ('WEB', 'WEBDAV', 'MIGRATION', 'AGENT', 'RESTORE')),
    CONSTRAINT fk_document_versions_restore_source FOREIGN KEY (document_id, restored_from_version_id)
        REFERENCES document_versions(document_id, document_version_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED
);

ALTER TABLE documents
    ADD CONSTRAINT fk_documents_current_version
    FOREIGN KEY (document_id, current_version_id)
    REFERENCES document_versions(document_id, document_version_id)
    ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE spaces
    ADD CONSTRAINT fk_spaces_root_folder
    FOREIGN KEY (root_folder_id) REFERENCES folders(folder_id)
    ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE storage_scan_results (
    storage_scan_result_id uuid PRIMARY KEY,
    storage_object_id uuid NOT NULL,
    scanner_name varchar(128) NOT NULL,
    scanner_version varchar(128) NOT NULL,
    signature_version varchar(128),
    result varchar(32) NOT NULL,
    threat_name varchar(512),
    failure_code varchar(64),
    started_at timestamptz NOT NULL,
    completed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_storage_scan_results_object FOREIGN KEY (storage_object_id)
        REFERENCES storage_objects(storage_object_id) ON DELETE RESTRICT,
    CONSTRAINT ck_storage_scan_results_result CHECK (result IN ('CLEAN', 'INFECTED', 'FAILED')),
    CONSTRAINT ck_storage_scan_results_period CHECK (completed_at >= started_at),
    CONSTRAINT ck_storage_scan_results_payload CHECK (
        (result = 'INFECTED' AND threat_name IS NOT NULL)
        OR (result = 'FAILED' AND failure_code IS NOT NULL)
        OR result = 'CLEAN'
    )
);

CREATE TABLE upload_sessions (
    upload_session_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    space_id uuid NOT NULL,
    folder_id uuid NOT NULL,
    quota_reservation_id uuid NOT NULL,
    target_document_id uuid,
    upload_intent varchar(32) NOT NULL,
    file_name varchar(512) NOT NULL,
    normalized_name varchar(512) NOT NULL,
    declared_size_bytes bigint NOT NULL,
    declared_sha256 bytea,
    declared_mime_type varchar(256),
    provider_upload_id varchar(512),
    temporary_object_key varchar(1024) NOT NULL,
    part_size_bytes bigint NOT NULL,
    expected_part_count integer NOT NULL,
    expected_current_version_id uuid,
    expected_lock_fencing_token bigint,
    lock_token_hash bytea,
    status varchar(32) NOT NULL DEFAULT 'INITIATED',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at timestamptz,
    failure_code varchar(64),
    result_document_id uuid,
    result_version_id uuid,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_upload_sessions_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_upload_sessions_space FOREIGN KEY (space_id)
        REFERENCES spaces(space_id) ON DELETE RESTRICT,
    CONSTRAINT fk_upload_sessions_folder FOREIGN KEY (folder_id)
        REFERENCES folders(folder_id) ON DELETE RESTRICT,
    CONSTRAINT fk_upload_sessions_quota FOREIGN KEY (quota_reservation_id)
        REFERENCES quota_reservations(quota_reservation_id) ON DELETE RESTRICT,
    CONSTRAINT fk_upload_sessions_target_document FOREIGN KEY (target_document_id)
        REFERENCES documents(document_id) ON DELETE RESTRICT,
    CONSTRAINT fk_upload_sessions_expected_version FOREIGN KEY (target_document_id, expected_current_version_id)
        REFERENCES document_versions(document_id, document_version_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_upload_sessions_result_document FOREIGN KEY (result_document_id)
        REFERENCES documents(document_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_upload_sessions_result_version FOREIGN KEY (result_document_id, result_version_id)
        REFERENCES document_versions(document_id, document_version_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT uq_upload_sessions_quota UNIQUE (quota_reservation_id),
    CONSTRAINT uq_upload_sessions_temporary_key UNIQUE (temporary_object_key),
    CONSTRAINT ck_upload_sessions_intent CHECK (upload_intent IN ('CREATE', 'NEW_VERSION')),
    CONSTRAINT ck_upload_sessions_target CHECK (
        (upload_intent = 'CREATE' AND target_document_id IS NULL AND expected_current_version_id IS NULL)
        OR (upload_intent = 'NEW_VERSION' AND target_document_id IS NOT NULL)
    ),
    CONSTRAINT ck_upload_sessions_file_name CHECK (btrim(file_name) <> '' AND btrim(normalized_name) <> ''),
    CONSTRAINT ck_upload_sessions_size CHECK (declared_size_bytes >= 0 AND part_size_bytes > 0 AND expected_part_count > 0),
    CONSTRAINT ck_upload_sessions_declared_hash CHECK (declared_sha256 IS NULL OR octet_length(declared_sha256) = 32),
    CONSTRAINT ck_upload_sessions_lock_hash CHECK (lock_token_hash IS NULL OR octet_length(lock_token_hash) = 32),
    CONSTRAINT ck_upload_sessions_fencing CHECK (expected_lock_fencing_token IS NULL OR expected_lock_fencing_token >= 1),
    CONSTRAINT ck_upload_sessions_status CHECK (status IN ('INITIATED', 'UPLOADING', 'COMPLETING', 'COMPLETED', 'ABORTED', 'EXPIRED', 'FAILED')),
    CONSTRAINT ck_upload_sessions_expiry CHECK (expires_at > created_at),
    CONSTRAINT ck_upload_sessions_completion CHECK (
        (status = 'COMPLETED' AND completed_at IS NOT NULL AND result_document_id IS NOT NULL AND result_version_id IS NOT NULL)
        OR (status <> 'COMPLETED' AND completed_at IS NULL AND result_document_id IS NULL AND result_version_id IS NULL)
    ),
    CONSTRAINT ck_upload_sessions_row_version CHECK (row_version >= 1)
);

CREATE TABLE upload_parts (
    upload_session_id uuid NOT NULL,
    part_number integer NOT NULL,
    etag varchar(256) NOT NULL,
    size_bytes bigint NOT NULL,
    checksum bytea,
    status varchar(32) NOT NULL DEFAULT 'UPLOADED',
    uploaded_at timestamptz NOT NULL,
    verified_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT pk_upload_parts PRIMARY KEY (upload_session_id, part_number),
    CONSTRAINT fk_upload_parts_session FOREIGN KEY (upload_session_id)
        REFERENCES upload_sessions(upload_session_id) ON DELETE CASCADE,
    CONSTRAINT ck_upload_parts_number CHECK (part_number >= 1),
    CONSTRAINT ck_upload_parts_size CHECK (size_bytes > 0),
    CONSTRAINT ck_upload_parts_status CHECK (status IN ('UPLOADED', 'VERIFIED')),
    CONSTRAINT ck_upload_parts_verified CHECK ((status = 'VERIFIED') = (verified_at IS NOT NULL)),
    CONSTRAINT ck_upload_parts_row_version CHECK (row_version >= 1)
);

CREATE TABLE document_lock_counters (
    document_id uuid PRIMARY KEY,
    last_fencing_token bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_document_lock_counters_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE CASCADE,
    CONSTRAINT ck_document_lock_counters_token CHECK (last_fencing_token >= 0)
);

CREATE TABLE document_locks (
    document_lock_id uuid PRIMARY KEY,
    document_id uuid NOT NULL,
    user_id uuid NOT NULL,
    token_hash bytea NOT NULL,
    fencing_token bigint NOT NULL,
    source varchar(32) NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    acquired_at timestamptz NOT NULL,
    heartbeat_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    released_at timestamptz,
    released_by_user_id uuid,
    release_reason text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_document_locks_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE RESTRICT,
    CONSTRAINT fk_document_locks_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_document_locks_released_by FOREIGN KEY (released_by_user_id)
        REFERENCES users(user_id) ON DELETE SET NULL,
    CONSTRAINT uq_document_locks_token_hash UNIQUE (token_hash),
    CONSTRAINT uq_document_locks_fencing UNIQUE (document_id, fencing_token),
    CONSTRAINT ck_document_locks_hash CHECK (octet_length(token_hash) = 32),
    CONSTRAINT ck_document_locks_fencing CHECK (fencing_token >= 1),
    CONSTRAINT ck_document_locks_source CHECK (source IN ('WEB', 'WEBDAV', 'OFFICE', 'AGENT')),
    CONSTRAINT ck_document_locks_status CHECK (status IN ('ACTIVE', 'RELEASED', 'EXPIRED', 'FORCED')),
    CONSTRAINT ck_document_locks_period CHECK (heartbeat_at >= acquired_at AND expires_at > acquired_at),
    CONSTRAINT ck_document_locks_release CHECK (
        (status = 'ACTIVE' AND released_at IS NULL AND released_by_user_id IS NULL)
        OR (status IN ('RELEASED', 'EXPIRED', 'FORCED') AND released_at IS NOT NULL)
    ),
    CONSTRAINT ck_document_locks_force_reason CHECK (
        status <> 'FORCED' OR (release_reason IS NOT NULL AND btrim(release_reason) <> '')
    ),
    CONSTRAINT ck_document_locks_row_version CHECK (row_version >= 1)
);

ALTER TABLE user_offboarding_cases
    ADD CONSTRAINT fk_user_offboarding_target_space
    FOREIGN KEY (target_space_id) REFERENCES spaces(space_id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_user_offboarding_target_folder
    FOREIGN KEY (target_folder_id) REFERENCES folders(folder_id) ON DELETE RESTRICT;

-- ============================================================================
-- 4. 权限、共享、标签与生命周期
-- ============================================================================

CREATE TABLE permission_grants (
    permission_grant_id uuid PRIMARY KEY,
    subject_user_id uuid,
    subject_organization_id uuid,
    space_id uuid,
    folder_id uuid,
    document_id uuid,
    inherit_to_descendants boolean NOT NULL DEFAULT false,
    grant_source varchar(32) NOT NULL,
    valid_from timestamptz NOT NULL,
    valid_until timestamptz,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    granted_by_user_id uuid NOT NULL,
    grant_reason text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at timestamptz,
    revoked_by_user_id uuid,
    revoke_reason text,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_permission_grants_subject_user FOREIGN KEY (subject_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_permission_grants_subject_org FOREIGN KEY (subject_organization_id)
        REFERENCES organizations(organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_permission_grants_space FOREIGN KEY (space_id)
        REFERENCES spaces(space_id) ON DELETE RESTRICT,
    CONSTRAINT fk_permission_grants_folder FOREIGN KEY (folder_id)
        REFERENCES folders(folder_id) ON DELETE RESTRICT,
    CONSTRAINT fk_permission_grants_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE RESTRICT,
    CONSTRAINT fk_permission_grants_granted_by FOREIGN KEY (granted_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_permission_grants_revoked_by FOREIGN KEY (revoked_by_user_id)
        REFERENCES users(user_id) ON DELETE SET NULL,
    CONSTRAINT ck_permission_grants_subject_xor CHECK (num_nonnulls(subject_user_id, subject_organization_id) = 1),
    CONSTRAINT ck_permission_grants_resource_xor CHECK (num_nonnulls(space_id, folder_id, document_id) = 1),
    CONSTRAINT ck_permission_grants_inheritance CHECK (document_id IS NULL OR NOT inherit_to_descendants),
    CONSTRAINT ck_permission_grants_source CHECK (grant_source IN ('MANUAL', 'TEMPLATE', 'MIGRATION', 'SYSTEM')),
    CONSTRAINT ck_permission_grants_period CHECK (valid_until IS NULL OR valid_until > valid_from),
    CONSTRAINT ck_permission_grants_status CHECK (status IN ('ACTIVE', 'REVOKED', 'EXPIRED')),
    CONSTRAINT ck_permission_grants_revoked_state CHECK (
        (status = 'REVOKED' AND revoked_at IS NOT NULL)
        OR (status <> 'REVOKED' AND revoked_at IS NULL AND revoked_by_user_id IS NULL)
    ),
    CONSTRAINT ck_permission_grants_row_version CHECK (row_version >= 1)
);

CREATE TABLE permission_grant_actions (
    permission_grant_id uuid NOT NULL,
    action varchar(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_permission_grant_actions PRIMARY KEY (permission_grant_id, action),
    CONSTRAINT fk_permission_grant_actions_grant FOREIGN KEY (permission_grant_id)
        REFERENCES permission_grants(permission_grant_id) ON DELETE CASCADE,
    CONSTRAINT ck_permission_grant_actions_action CHECK (action IN (
        'LIST', 'READ_METADATA', 'PREVIEW', 'DOWNLOAD', 'UPLOAD', 'CREATE_FOLDER',
        'WRITE_CONTENT', 'RENAME', 'MOVE', 'DELETE', 'RESTORE', 'PURGE', 'SHARE',
        'LOCK', 'MANAGE_VERSION', 'MANAGE_PERMISSION'
    ))
);

CREATE TABLE shares (
    share_id uuid PRIMARY KEY,
    source_document_id uuid,
    source_folder_id uuid,
    creator_user_id uuid NOT NULL,
    target_kind varchar(32) NOT NULL,
    target_user_id uuid,
    target_organization_id uuid,
    target_space_id uuid,
    token_hash bytea,
    password_hash text,
    allow_reshare boolean NOT NULL DEFAULT false,
    valid_from timestamptz NOT NULL,
    valid_until timestamptz,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at timestamptz,
    revoked_by_user_id uuid,
    revoke_reason text,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_shares_source_document FOREIGN KEY (source_document_id)
        REFERENCES documents(document_id) ON DELETE RESTRICT,
    CONSTRAINT fk_shares_source_folder FOREIGN KEY (source_folder_id)
        REFERENCES folders(folder_id) ON DELETE RESTRICT,
    CONSTRAINT fk_shares_creator FOREIGN KEY (creator_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_shares_target_user FOREIGN KEY (target_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_shares_target_org FOREIGN KEY (target_organization_id)
        REFERENCES organizations(organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_shares_target_space FOREIGN KEY (target_space_id)
        REFERENCES spaces(space_id) ON DELETE RESTRICT,
    CONSTRAINT fk_shares_revoked_by FOREIGN KEY (revoked_by_user_id)
        REFERENCES users(user_id) ON DELETE SET NULL,
    CONSTRAINT uq_shares_token_hash UNIQUE (token_hash),
    CONSTRAINT ck_shares_source_xor CHECK (num_nonnulls(source_document_id, source_folder_id) = 1),
    CONSTRAINT ck_shares_target_kind CHECK (target_kind IN ('USER', 'ORGANIZATION', 'SPACE', 'LINK')),
    CONSTRAINT ck_shares_target CHECK (
        (target_kind = 'USER' AND target_user_id IS NOT NULL AND target_organization_id IS NULL AND target_space_id IS NULL AND token_hash IS NULL)
        OR (target_kind = 'ORGANIZATION' AND target_user_id IS NULL AND target_organization_id IS NOT NULL AND target_space_id IS NULL AND token_hash IS NULL)
        OR (target_kind = 'SPACE' AND target_user_id IS NULL AND target_organization_id IS NULL AND target_space_id IS NOT NULL AND token_hash IS NULL)
        OR (target_kind = 'LINK' AND target_user_id IS NULL AND target_organization_id IS NULL AND target_space_id IS NULL AND token_hash IS NOT NULL)
    ),
    CONSTRAINT ck_shares_token_hash CHECK (token_hash IS NULL OR octet_length(token_hash) = 32),
    CONSTRAINT ck_shares_period CHECK (valid_until IS NULL OR valid_until > valid_from),
    CONSTRAINT ck_shares_status CHECK (status IN ('ACTIVE', 'EXPIRED', 'REVOKED', 'SUSPENDED', 'SOURCE_UNAVAILABLE')),
    CONSTRAINT ck_shares_revoked_state CHECK (
        (status = 'REVOKED' AND revoked_at IS NOT NULL)
        OR (status <> 'REVOKED' AND revoked_at IS NULL AND revoked_by_user_id IS NULL)
    ),
    CONSTRAINT ck_shares_row_version CHECK (row_version >= 1)
);

CREATE TABLE share_actions (
    share_id uuid NOT NULL,
    action varchar(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_share_actions PRIMARY KEY (share_id, action),
    CONSTRAINT fk_share_actions_share FOREIGN KEY (share_id)
        REFERENCES shares(share_id) ON DELETE CASCADE,
    CONSTRAINT ck_share_actions_action CHECK (action IN ('READ_METADATA', 'PREVIEW', 'DOWNLOAD', 'WRITE_CONTENT'))
);

CREATE TABLE shared_entries (
    shared_entry_id uuid PRIMARY KEY,
    share_id uuid NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    removed_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_shared_entries_namespace_entry FOREIGN KEY (shared_entry_id)
        REFERENCES namespace_entries(namespace_entry_id) ON DELETE RESTRICT,
    CONSTRAINT fk_shared_entries_share FOREIGN KEY (share_id)
        REFERENCES shares(share_id) ON DELETE RESTRICT,
    CONSTRAINT fk_shared_entries_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT uq_shared_entries_share UNIQUE (share_id),
    CONSTRAINT ck_shared_entries_status CHECK (status IN ('ACTIVE', 'UNAVAILABLE', 'REMOVED')),
    CONSTRAINT ck_shared_entries_removed CHECK ((status = 'REMOVED') = (removed_at IS NOT NULL)),
    CONSTRAINT ck_shared_entries_row_version CHECK (row_version >= 1)
);

CREATE TABLE tags (
    tag_id uuid PRIMARY KEY,
    name varchar(128) NOT NULL,
    normalized_name varchar(128) NOT NULL,
    scope_kind varchar(32) NOT NULL,
    scope_space_id uuid,
    created_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_tags_scope_space FOREIGN KEY (scope_space_id)
        REFERENCES spaces(space_id) ON DELETE RESTRICT,
    CONSTRAINT fk_tags_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_tags_name CHECK (btrim(normalized_name) <> ''),
    CONSTRAINT ck_tags_scope_kind CHECK (scope_kind IN ('GLOBAL', 'SPACE')),
    CONSTRAINT ck_tags_scope CHECK (
        (scope_kind = 'GLOBAL' AND scope_space_id IS NULL)
        OR (scope_kind = 'SPACE' AND scope_space_id IS NOT NULL)
    ),
    CONSTRAINT ck_tags_row_version CHECK (row_version >= 1)
);

CREATE TABLE document_tags (
    document_tag_id uuid PRIMARY KEY,
    document_id uuid NOT NULL,
    tag_id uuid NOT NULL,
    source varchar(32) NOT NULL,
    source_reference varchar(256) NOT NULL,
    confidence numeric(5,4),
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_by_user_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    removed_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_document_tags_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE CASCADE,
    CONSTRAINT fk_document_tags_tag FOREIGN KEY (tag_id)
        REFERENCES tags(tag_id) ON DELETE RESTRICT,
    CONSTRAINT fk_document_tags_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_document_tags_source CHECK (source IN ('USER', 'AI', 'SYSTEM')),
    CONSTRAINT ck_document_tags_source_user CHECK ((source = 'USER' AND created_by_user_id IS NOT NULL) OR source <> 'USER'),
    CONSTRAINT ck_document_tags_confidence CHECK (
        (source = 'AI' AND confidence IS NOT NULL AND confidence >= 0 AND confidence <= 1)
        OR (source <> 'AI' AND confidence IS NULL)
    ),
    CONSTRAINT ck_document_tags_status CHECK (status IN ('ACTIVE', 'REMOVED')),
    CONSTRAINT ck_document_tags_removed CHECK ((status = 'REMOVED') = (removed_at IS NOT NULL)),
    CONSTRAINT ck_document_tags_row_version CHECK (row_version >= 1)
);

CREATE TABLE favorites (
    user_id uuid NOT NULL,
    namespace_entry_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_favorites PRIMARY KEY (user_id, namespace_entry_id),
    CONSTRAINT fk_favorites_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE CASCADE,
    CONSTRAINT fk_favorites_entry FOREIGN KEY (namespace_entry_id)
        REFERENCES namespace_entries(namespace_entry_id) ON DELETE CASCADE
);

CREATE TABLE recent_documents (
    user_id uuid NOT NULL,
    document_id uuid NOT NULL,
    last_action varchar(32) NOT NULL,
    last_accessed_at timestamptz NOT NULL,
    access_count bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_recent_documents PRIMARY KEY (user_id, document_id),
    CONSTRAINT fk_recent_documents_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE CASCADE,
    CONSTRAINT fk_recent_documents_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE CASCADE,
    CONSTRAINT ck_recent_documents_action CHECK (btrim(last_action) <> ''),
    CONSTRAINT ck_recent_documents_count CHECK (access_count >= 0)
);

CREATE TABLE recycle_items (
    recycle_item_id uuid PRIMARY KEY,
    namespace_entry_id uuid NOT NULL,
    original_space_id uuid NOT NULL,
    original_parent_folder_id uuid,
    original_name varchar(512) NOT NULL,
    deleted_by_user_id uuid NOT NULL,
    deleted_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    restored_to_folder_id uuid,
    restored_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_recycle_items_entry FOREIGN KEY (namespace_entry_id)
        REFERENCES namespace_entries(namespace_entry_id) ON DELETE RESTRICT,
    CONSTRAINT fk_recycle_items_original_space FOREIGN KEY (original_space_id)
        REFERENCES spaces(space_id) ON DELETE RESTRICT,
    CONSTRAINT fk_recycle_items_deleted_by FOREIGN KEY (deleted_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_recycle_items_restored_folder FOREIGN KEY (restored_to_folder_id)
        REFERENCES folders(folder_id) ON DELETE RESTRICT,
    CONSTRAINT ck_recycle_items_name CHECK (btrim(original_name) <> ''),
    CONSTRAINT ck_recycle_items_expiry CHECK (expires_at > deleted_at),
    CONSTRAINT ck_recycle_items_status CHECK (status IN ('ACTIVE', 'RESTORED', 'PURGING', 'PURGED')),
    CONSTRAINT ck_recycle_items_restore CHECK (
        (status = 'RESTORED' AND restored_at IS NOT NULL AND restored_to_folder_id IS NOT NULL)
        OR (status <> 'RESTORED' AND restored_at IS NULL AND restored_to_folder_id IS NULL)
    ),
    CONSTRAINT ck_recycle_items_row_version CHECK (row_version >= 1)
);

CREATE TABLE retention_policies (
    retention_policy_id uuid PRIMARY KEY,
    name varchar(256) NOT NULL,
    normalized_name varchar(256) NOT NULL,
    recycle_days integer,
    archive_after_days integer,
    cold_after_days integer,
    purge_after_days integer,
    version_retention_days integer,
    min_versions integer,
    allow_user_override boolean NOT NULL DEFAULT false,
    priority integer NOT NULL DEFAULT 0,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_retention_policies_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT uq_retention_policies_name UNIQUE (normalized_name),
    CONSTRAINT ck_retention_policies_name CHECK (btrim(normalized_name) <> ''),
    CONSTRAINT ck_retention_policies_values CHECK (
        (recycle_days IS NULL OR recycle_days >= 0)
        AND (archive_after_days IS NULL OR archive_after_days >= 0)
        AND (cold_after_days IS NULL OR cold_after_days >= 0)
        AND (purge_after_days IS NULL OR purge_after_days >= 0)
        AND (version_retention_days IS NULL OR version_retention_days >= 0)
        AND (min_versions IS NULL OR min_versions >= 0)
    ),
    CONSTRAINT ck_retention_policies_order CHECK (
        (archive_after_days IS NULL OR cold_after_days IS NULL OR archive_after_days <= cold_after_days)
        AND (cold_after_days IS NULL OR purge_after_days IS NULL OR cold_after_days <= purge_after_days)
        AND (archive_after_days IS NULL OR purge_after_days IS NULL OR archive_after_days <= purge_after_days)
    ),
    CONSTRAINT ck_retention_policies_status CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT ck_retention_policies_row_version CHECK (row_version >= 1)
);

CREATE TABLE retention_policy_targets (
    retention_policy_target_id uuid PRIMARY KEY,
    retention_policy_id uuid NOT NULL,
    target_kind varchar(32) NOT NULL,
    space_id uuid,
    folder_id uuid,
    tag_id uuid,
    mime_pattern varchar(256),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_retention_targets_policy FOREIGN KEY (retention_policy_id)
        REFERENCES retention_policies(retention_policy_id) ON DELETE CASCADE,
    CONSTRAINT fk_retention_targets_space FOREIGN KEY (space_id)
        REFERENCES spaces(space_id) ON DELETE RESTRICT,
    CONSTRAINT fk_retention_targets_folder FOREIGN KEY (folder_id)
        REFERENCES folders(folder_id) ON DELETE RESTRICT,
    CONSTRAINT fk_retention_targets_tag FOREIGN KEY (tag_id)
        REFERENCES tags(tag_id) ON DELETE RESTRICT,
    CONSTRAINT ck_retention_targets_kind CHECK (target_kind IN ('SPACE', 'FOLDER', 'TAG', 'MIME')),
    CONSTRAINT ck_retention_targets_value CHECK (
        num_nonnulls(space_id, folder_id, tag_id, mime_pattern) = 1
        AND ((target_kind = 'SPACE' AND space_id IS NOT NULL)
          OR (target_kind = 'FOLDER' AND folder_id IS NOT NULL)
          OR (target_kind = 'TAG' AND tag_id IS NOT NULL)
          OR (target_kind = 'MIME' AND mime_pattern IS NOT NULL AND btrim(mime_pattern) <> ''))
    )
);

CREATE TABLE legal_holds (
    legal_hold_id uuid PRIMARY KEY,
    document_id uuid NOT NULL,
    document_version_id uuid,
    case_reference varchar(256) NOT NULL,
    reason text NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    placed_by_user_id uuid NOT NULL,
    placed_at timestamptz NOT NULL,
    released_by_user_id uuid,
    released_at timestamptz,
    release_reason text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_legal_holds_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE RESTRICT,
    CONSTRAINT fk_legal_holds_version FOREIGN KEY (document_id, document_version_id)
        REFERENCES document_versions(document_id, document_version_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_legal_holds_placed_by FOREIGN KEY (placed_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_legal_holds_released_by FOREIGN KEY (released_by_user_id)
        REFERENCES users(user_id) ON DELETE SET NULL,
    CONSTRAINT ck_legal_holds_case CHECK (btrim(case_reference) <> '' AND btrim(reason) <> ''),
    CONSTRAINT ck_legal_holds_status CHECK (status IN ('ACTIVE', 'RELEASED')),
    CONSTRAINT ck_legal_holds_release CHECK (
        (status = 'ACTIVE' AND released_by_user_id IS NULL AND released_at IS NULL AND release_reason IS NULL)
        OR (status = 'RELEASED' AND released_at IS NOT NULL
            AND release_reason IS NOT NULL AND btrim(release_reason) <> '')
    ),
    CONSTRAINT ck_legal_holds_row_version CHECK (row_version >= 1)
);

-- ============================================================================
-- 5. 审计、幂等、事务外盒与后台任务
-- ============================================================================

CREATE TABLE audit_events (
    audit_event_id uuid NOT NULL,
    event_type varchar(128) NOT NULL,
    risk_level varchar(16) NOT NULL,
    actor_type varchar(32) NOT NULL,
    actor_id uuid,
    actor_display_name varchar(256),
    actor_employee_no varchar(128),
    effective_role varchar(32),
    admin_delegation_id uuid,
    share_id uuid,
    resource_type varchar(32),
    resource_id uuid,
    resource_name varchar(512),
    space_id uuid,
    organization_id uuid,
    document_id uuid,
    document_version_id uuid,
    action varchar(128) NOT NULL,
    result varchar(32) NOT NULL,
    failure_code varchar(64),
    source_channel varchar(32) NOT NULL,
    ip_address inet,
    user_agent text,
    request_id uuid NOT NULL,
    trace_id varchar(128),
    correlation_id uuid,
    reason text,
    metadata_schema_version integer NOT NULL DEFAULT 1,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    hash_schema_version integer,
    chain_id varchar(128),
    sequence_number bigint,
    previous_hash bytea,
    event_hash bytea,
    partition_date date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_audit_events PRIMARY KEY (partition_date, audit_event_id),
    CONSTRAINT uq_audit_events_chain_sequence UNIQUE (partition_date, chain_id, sequence_number),
    CONSTRAINT ck_audit_events_event_type CHECK (btrim(event_type) <> '' AND btrim(action) <> ''),
    CONSTRAINT ck_audit_events_risk CHECK (risk_level IN ('NORMAL', 'HIGH', 'CRITICAL')),
    CONSTRAINT ck_audit_events_actor CHECK (actor_type IN ('USER', 'SYSTEM', 'AGENT', 'MIGRATION', 'SERVICE')),
    CONSTRAINT ck_audit_events_result CHECK (result IN ('SUCCESS', 'FAILURE', 'DENIED')),
    CONSTRAINT ck_audit_events_source CHECK (source_channel IN ('WEB', 'API', 'WEBDAV', 'AGENT', 'MIGRATION', 'SYSTEM')),
    CONSTRAINT ck_audit_events_failure CHECK ((result = 'SUCCESS' AND failure_code IS NULL) OR result <> 'SUCCESS'),
    CONSTRAINT ck_audit_events_metadata CHECK (metadata_schema_version >= 1 AND jsonb_typeof(metadata_json) = 'object'),
    CONSTRAINT ck_audit_events_partition_date CHECK (partition_date = (created_at AT TIME ZONE 'UTC')::date),
    CONSTRAINT ck_audit_events_chain_all_or_none CHECK (
        num_nonnulls(hash_schema_version, chain_id, sequence_number, previous_hash, event_hash) IN (0, 5)
    ),
    CONSTRAINT ck_audit_events_chain_required CHECK (
        (risk_level = 'NORMAL' AND hash_schema_version IS NULL)
        OR (risk_level IN ('HIGH', 'CRITICAL') AND hash_schema_version IS NOT NULL)
    ),
    CONSTRAINT ck_audit_events_hash_schema CHECK (hash_schema_version IS NULL OR hash_schema_version >= 1),
    CONSTRAINT ck_audit_events_sequence CHECK (sequence_number IS NULL OR sequence_number >= 1),
    CONSTRAINT ck_audit_events_previous_hash CHECK (previous_hash IS NULL OR octet_length(previous_hash) = 32),
    CONSTRAINT ck_audit_events_event_hash CHECK (event_hash IS NULL OR octet_length(event_hash) = 32)
) PARTITION BY RANGE (partition_date);

CREATE TABLE audit_chain_heads (
    chain_id varchar(128) NOT NULL,
    partition_date date NOT NULL,
    last_sequence_number bigint NOT NULL,
    last_event_id uuid NOT NULL,
    last_hash bytea NOT NULL,
    batch_root bytea,
    anchor_location varchar(1024),
    status varchar(32) NOT NULL DEFAULT 'ACTIVE',
    verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT pk_audit_chain_heads PRIMARY KEY (chain_id, partition_date),
    CONSTRAINT ck_audit_chain_heads_sequence CHECK (last_sequence_number >= 0),
    CONSTRAINT ck_audit_chain_heads_hash CHECK (octet_length(last_hash) = 32),
    CONSTRAINT ck_audit_chain_heads_batch_root CHECK (batch_root IS NULL OR octet_length(batch_root) = 32),
    CONSTRAINT ck_audit_chain_heads_status CHECK (status IN ('ACTIVE', 'SEALED', 'INVALID')),
    CONSTRAINT ck_audit_chain_heads_row_version CHECK (row_version >= 1)
);

CREATE TABLE idempotency_records (
    idempotency_record_id uuid PRIMARY KEY,
    principal_kind varchar(32) NOT NULL,
    user_id uuid,
    service_principal varchar(256),
    operation varchar(128) NOT NULL,
    idempotency_key varchar(256) NOT NULL,
    request_hash bytea NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'IN_PROGRESS',
    response_status_code integer,
    response_schema_version integer,
    response_json jsonb,
    result_resource_type varchar(64),
    result_resource_id uuid,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at timestamptz,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_idempotency_records_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_idempotency_records_principal_kind CHECK (principal_kind IN ('USER', 'SERVICE', 'SYSTEM')),
    CONSTRAINT ck_idempotency_records_principal CHECK (
        (principal_kind = 'USER' AND user_id IS NOT NULL AND service_principal IS NULL)
        OR (principal_kind IN ('SERVICE', 'SYSTEM') AND user_id IS NULL AND service_principal IS NOT NULL AND btrim(service_principal) <> '')
    ),
    CONSTRAINT ck_idempotency_records_names CHECK (btrim(operation) <> '' AND btrim(idempotency_key) <> ''),
    CONSTRAINT ck_idempotency_records_request_hash CHECK (octet_length(request_hash) = 32),
    CONSTRAINT ck_idempotency_records_status CHECK (status IN ('IN_PROGRESS', 'COMPLETED', 'FAILED')),
    CONSTRAINT ck_idempotency_records_response_schema CHECK (response_schema_version IS NULL OR response_schema_version >= 1),
    CONSTRAINT ck_idempotency_records_response_pair CHECK ((response_schema_version IS NULL) = (response_json IS NULL)),
    CONSTRAINT ck_idempotency_records_completion CHECK (
        (status IN ('COMPLETED', 'FAILED') AND completed_at IS NOT NULL AND response_status_code IS NOT NULL)
        OR (status = 'IN_PROGRESS' AND completed_at IS NULL AND response_status_code IS NULL)
    ),
    CONSTRAINT ck_idempotency_records_expiry CHECK (expires_at > created_at),
    CONSTRAINT ck_idempotency_records_row_version CHECK (row_version >= 1)
);

CREATE TABLE outbox_events (
    outbox_event_id uuid PRIMARY KEY,
    aggregate_type varchar(64) NOT NULL,
    aggregate_id uuid NOT NULL,
    aggregate_version bigint NOT NULL,
    event_type varchar(128) NOT NULL,
    event_schema_version integer NOT NULL,
    payload_json jsonb NOT NULL,
    deduplication_key varchar(256) NOT NULL,
    correlation_id uuid,
    causation_id uuid,
    priority integer NOT NULL DEFAULT 0,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    attempt_count integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL,
    available_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_by varchar(256),
    locked_at timestamptz,
    lease_until timestamptz,
    next_retry_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at timestamptz,
    last_error_code varchar(64),
    last_error_summary text,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT uq_outbox_events_deduplication UNIQUE (deduplication_key),
    CONSTRAINT ck_outbox_events_names CHECK (btrim(aggregate_type) <> '' AND btrim(event_type) <> '' AND btrim(deduplication_key) <> ''),
    CONSTRAINT ck_outbox_events_versions CHECK (aggregate_version >= 1 AND event_schema_version >= 1),
    CONSTRAINT ck_outbox_events_payload CHECK (jsonb_typeof(payload_json) = 'object'),
    CONSTRAINT ck_outbox_events_status CHECK (status IN ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED', 'DEAD')),
    CONSTRAINT ck_outbox_events_attempts CHECK (attempt_count >= 0 AND max_attempts > 0 AND attempt_count <= max_attempts),
    CONSTRAINT ck_outbox_events_lease CHECK (
        (status = 'PROCESSING' AND locked_by IS NOT NULL AND locked_at IS NOT NULL AND lease_until IS NOT NULL AND lease_until > locked_at)
        OR (status <> 'PROCESSING')
    ),
    CONSTRAINT ck_outbox_events_published CHECK ((status = 'PUBLISHED') = (published_at IS NOT NULL)),
    CONSTRAINT ck_outbox_events_row_version CHECK (row_version >= 1)
);

CREATE TABLE background_jobs (
    background_job_id uuid PRIMARY KEY,
    job_type varchar(128) NOT NULL,
    target_document_id uuid,
    target_document_version_id uuid,
    target_storage_object_id uuid,
    payload_schema_version integer NOT NULL,
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    deduplication_key varchar(256) NOT NULL,
    priority integer NOT NULL DEFAULT 0,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    attempt_count integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL,
    available_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_by varchar(256),
    locked_at timestamptz,
    lease_until timestamptz,
    heartbeat_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at timestamptz,
    completed_at timestamptz,
    last_error_code varchar(64),
    last_error_summary text,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_background_jobs_document FOREIGN KEY (target_document_id)
        REFERENCES documents(document_id) ON DELETE RESTRICT,
    CONSTRAINT fk_background_jobs_version FOREIGN KEY (target_document_id, target_document_version_id)
        REFERENCES document_versions(document_id, document_version_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_background_jobs_storage_object FOREIGN KEY (target_storage_object_id)
        REFERENCES storage_objects(storage_object_id) ON DELETE RESTRICT,
    CONSTRAINT ck_background_jobs_name CHECK (btrim(job_type) <> '' AND btrim(deduplication_key) <> ''),
    CONSTRAINT ck_background_jobs_target_version CHECK (
        target_document_version_id IS NULL OR target_document_id IS NOT NULL
    ),
    CONSTRAINT ck_background_jobs_payload CHECK (payload_schema_version >= 1 AND jsonb_typeof(payload_json) = 'object'),
    CONSTRAINT ck_background_jobs_status CHECK (status IN ('PENDING', 'PROCESSING', 'SUCCESS', 'FAILED', 'DEAD', 'CANCELLED', 'SKIPPED')),
    CONSTRAINT ck_background_jobs_attempts CHECK (attempt_count >= 0 AND max_attempts > 0 AND attempt_count <= max_attempts),
    CONSTRAINT ck_background_jobs_lease CHECK (
        (status = 'PROCESSING' AND locked_by IS NOT NULL AND locked_at IS NOT NULL AND lease_until IS NOT NULL AND lease_until > locked_at)
        OR status <> 'PROCESSING'
    ),
    CONSTRAINT ck_background_jobs_completion CHECK (
        (status IN ('SUCCESS', 'DEAD', 'CANCELLED', 'SKIPPED') AND completed_at IS NOT NULL)
        OR status IN ('PENDING', 'PROCESSING', 'FAILED')
    ),
    CONSTRAINT ck_background_jobs_row_version CHECK (row_version >= 1)
);

-- ============================================================================
-- 6. 搜索、预览、AI 与 Agent
-- ============================================================================

CREATE TABLE document_index_states (
    document_id uuid PRIMARY KEY,
    indexed_version_id uuid,
    indexed_acl_version bigint,
    indexed_space_security_epoch bigint,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    indexed_at timestamptz,
    last_error_code varchar(64),
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_document_index_states_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE CASCADE,
    CONSTRAINT fk_document_index_states_version FOREIGN KEY (document_id, indexed_version_id)
        REFERENCES document_versions(document_id, document_version_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT ck_document_index_states_versions CHECK (
        (indexed_acl_version IS NULL OR indexed_acl_version >= 1)
        AND (indexed_space_security_epoch IS NULL OR indexed_space_security_epoch >= 1)
    ),
    CONSTRAINT ck_document_index_states_status CHECK (status IN ('PENDING', 'CURRENT', 'STALE', 'FAILED')),
    CONSTRAINT ck_document_index_states_current CHECK (
        status <> 'CURRENT' OR (indexed_version_id IS NOT NULL AND indexed_acl_version IS NOT NULL AND indexed_space_security_epoch IS NOT NULL AND indexed_at IS NOT NULL)
    ),
    CONSTRAINT ck_document_index_states_row_version CHECK (row_version >= 1)
);

CREATE TABLE preview_artifacts (
    preview_artifact_id uuid PRIMARY KEY,
    document_id uuid NOT NULL,
    document_version_id uuid NOT NULL,
    preview_type varchar(32) NOT NULL,
    renderer_name varchar(128) NOT NULL,
    renderer_version varchar(128) NOT NULL,
    output_storage_object_id uuid,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at timestamptz,
    failure_code varchar(64),
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_preview_artifacts_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE CASCADE,
    CONSTRAINT fk_preview_artifacts_version FOREIGN KEY (document_id, document_version_id)
        REFERENCES document_versions(document_id, document_version_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_preview_artifacts_output FOREIGN KEY (output_storage_object_id)
        REFERENCES storage_objects(storage_object_id) ON DELETE RESTRICT,
    CONSTRAINT uq_preview_artifacts_renderer UNIQUE (document_version_id, preview_type, renderer_name, renderer_version),
    CONSTRAINT ck_preview_artifacts_type CHECK (preview_type IN ('PDF', 'THUMBNAIL', 'TEXT', 'OFFICE')),
    CONSTRAINT ck_preview_artifacts_status CHECK (status IN ('PENDING', 'PROCESSING', 'SUCCESS', 'FAILED', 'SKIPPED')),
    CONSTRAINT ck_preview_artifacts_completion CHECK (
        (status IN ('SUCCESS', 'FAILED', 'SKIPPED') AND completed_at IS NOT NULL)
        OR status IN ('PENDING', 'PROCESSING')
    ),
    CONSTRAINT ck_preview_artifacts_output CHECK (status <> 'SUCCESS' OR output_storage_object_id IS NOT NULL),
    CONSTRAINT ck_preview_artifacts_row_version CHECK (row_version >= 1)
);

CREATE TABLE document_extractions (
    document_extraction_id uuid PRIMARY KEY,
    document_id uuid NOT NULL,
    document_version_id uuid NOT NULL,
    parser_name varchar(128) NOT NULL,
    parser_version varchar(128) NOT NULL,
    extraction_schema_version integer NOT NULL,
    extracted_text_storage_object_id uuid,
    summary text,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at timestamptz,
    failure_code varchar(64),
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_document_extractions_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE CASCADE,
    CONSTRAINT fk_document_extractions_version FOREIGN KEY (document_id, document_version_id)
        REFERENCES document_versions(document_id, document_version_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_document_extractions_text_object FOREIGN KEY (extracted_text_storage_object_id)
        REFERENCES storage_objects(storage_object_id) ON DELETE RESTRICT,
    CONSTRAINT uq_document_extractions_parser UNIQUE (document_version_id, parser_name, parser_version, extraction_schema_version),
    CONSTRAINT ck_document_extractions_schema CHECK (extraction_schema_version >= 1),
    CONSTRAINT ck_document_extractions_metadata CHECK (jsonb_typeof(metadata_json) = 'object'),
    CONSTRAINT ck_document_extractions_status CHECK (status IN ('PENDING', 'PROCESSING', 'SUCCESS', 'FAILED', 'SKIPPED')),
    CONSTRAINT ck_document_extractions_completion CHECK (
        (status IN ('SUCCESS', 'FAILED', 'SKIPPED') AND processed_at IS NOT NULL)
        OR status IN ('PENDING', 'PROCESSING')
    ),
    CONSTRAINT ck_document_extractions_row_version CHECK (row_version >= 1)
);

CREATE TABLE document_chunks (
    document_chunk_id uuid PRIMARY KEY,
    document_extraction_id uuid NOT NULL,
    chunk_index integer NOT NULL,
    content text NOT NULL,
    page_number integer,
    locator_schema_version integer NOT NULL,
    locator_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_document_chunks_extraction FOREIGN KEY (document_extraction_id)
        REFERENCES document_extractions(document_extraction_id) ON DELETE CASCADE,
    CONSTRAINT uq_document_chunks_index UNIQUE (document_extraction_id, chunk_index),
    CONSTRAINT ck_document_chunks_index CHECK (chunk_index >= 0),
    CONSTRAINT ck_document_chunks_content CHECK (content <> ''),
    CONSTRAINT ck_document_chunks_page CHECK (page_number IS NULL OR page_number >= 0),
    CONSTRAINT ck_document_chunks_locator CHECK (locator_schema_version >= 1 AND jsonb_typeof(locator_json) = 'object')
);

-- pgvector 为可选能力。DDL 使用动态 SQL，确保未安装扩展时核心 Schema 仍可导入。
DO $fw_vector$
DECLARE
    vector_schema name;
BEGIN
    SELECT n.nspname
      INTO vector_schema
      FROM pg_extension e
      JOIN pg_namespace n ON n.oid = e.extnamespace
     WHERE e.extname = 'vector';

    IF vector_schema IS NULL AND EXISTS (
        SELECT 1 FROM pg_available_extensions WHERE name = 'vector'
    ) THEN
        BEGIN
            EXECUTE 'CREATE SCHEMA IF NOT EXISTS file_workshop_extensions';
            EXECUTE 'CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA file_workshop_extensions';
        EXCEPTION
            WHEN insufficient_privilege THEN
                RAISE NOTICE 'pgvector 可用但当前账号无 CREATE EXTENSION 权限，已跳过 chunk_embeddings。';
            WHEN OTHERS THEN
                RAISE NOTICE 'pgvector 创建失败（%），已跳过 chunk_embeddings。', SQLERRM;
        END;

        SELECT n.nspname
          INTO vector_schema
          FROM pg_extension e
          JOIN pg_namespace n ON n.oid = e.extnamespace
         WHERE e.extname = 'vector';
    END IF;

    IF vector_schema IS NOT NULL THEN
        EXECUTE format($ddl$
            CREATE TABLE file_workshop.chunk_embeddings (
                chunk_embedding_id uuid PRIMARY KEY,
                document_chunk_id uuid NOT NULL,
                provider varchar(128) NOT NULL,
                model_name varchar(128) NOT NULL,
                model_version varchar(128) NOT NULL,
                dimensions integer NOT NULL,
                embedding %I.vector NOT NULL,
                created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
                CONSTRAINT fk_chunk_embeddings_chunk FOREIGN KEY (document_chunk_id)
                    REFERENCES file_workshop.document_chunks(document_chunk_id) ON DELETE CASCADE,
                CONSTRAINT uq_chunk_embeddings_model UNIQUE (document_chunk_id, provider, model_name, model_version),
                CONSTRAINT ck_chunk_embeddings_dimensions CHECK (dimensions > 0),
                CONSTRAINT ck_chunk_embeddings_names CHECK (
                    btrim(provider) <> '' AND btrim(model_name) <> '' AND btrim(model_version) <> ''
                ),
                CONSTRAINT ck_chunk_embeddings_vector_dimensions CHECK (%I.vector_dims(embedding) = dimensions)
            )
        $ddl$, vector_schema, vector_schema);
        RAISE NOTICE 'pgvector 已启用，chunk_embeddings 已创建。';
    ELSE
        RAISE NOTICE '服务器未提供 pgvector，已跳过可选表 chunk_embeddings。';
    END IF;
END
$fw_vector$;

CREATE TABLE ai_tasks (
    ai_task_id uuid PRIMARY KEY,
    user_id uuid,
    task_type varchar(64) NOT NULL,
    document_id uuid,
    document_version_id uuid,
    provider varchar(128),
    model_name varchar(128),
    model_version varchar(128),
    input_schema_version integer NOT NULL,
    input_summary_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    output_schema_version integer,
    output_summary_json jsonb,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    request_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at timestamptz,
    completed_at timestamptz,
    failure_code varchar(64),
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_ai_tasks_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_ai_tasks_document FOREIGN KEY (document_id)
        REFERENCES documents(document_id) ON DELETE RESTRICT,
    CONSTRAINT fk_ai_tasks_version FOREIGN KEY (document_id, document_version_id)
        REFERENCES document_versions(document_id, document_version_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT ck_ai_tasks_name CHECK (btrim(task_type) <> ''),
    CONSTRAINT ck_ai_tasks_document_version CHECK (document_version_id IS NULL OR document_id IS NOT NULL),
    CONSTRAINT ck_ai_tasks_input CHECK (input_schema_version >= 1 AND jsonb_typeof(input_summary_json) = 'object'),
    CONSTRAINT ck_ai_tasks_output CHECK (
        (output_schema_version IS NULL AND output_summary_json IS NULL)
        OR (output_schema_version >= 1 AND output_summary_json IS NOT NULL AND jsonb_typeof(output_summary_json) = 'object')
    ),
    CONSTRAINT ck_ai_tasks_status CHECK (status IN ('PENDING', 'PROCESSING', 'SUCCESS', 'FAILED', 'CANCELLED', 'SKIPPED')),
    CONSTRAINT ck_ai_tasks_completion CHECK (
        (status IN ('SUCCESS', 'FAILED', 'CANCELLED', 'SKIPPED') AND completed_at IS NOT NULL)
        OR status IN ('PENDING', 'PROCESSING')
    ),
    CONSTRAINT ck_ai_tasks_row_version CHECK (row_version >= 1)
);

CREATE TABLE agent_confirmations (
    agent_confirmation_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    ai_task_id uuid,
    action_type varchar(128) NOT NULL,
    action_schema_version integer NOT NULL,
    action_summary_json jsonb NOT NULL,
    action_hash bytea NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    approved_at timestamptz,
    rejected_at timestamptz,
    consumed_at timestamptz,
    request_id uuid NOT NULL,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_agent_confirmations_user FOREIGN KEY (user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_agent_confirmations_ai_task FOREIGN KEY (ai_task_id)
        REFERENCES ai_tasks(ai_task_id) ON DELETE RESTRICT,
    CONSTRAINT ck_agent_confirmations_action CHECK (btrim(action_type) <> '' AND action_schema_version >= 1 AND jsonb_typeof(action_summary_json) = 'object'),
    CONSTRAINT ck_agent_confirmations_hash CHECK (octet_length(action_hash) = 32),
    CONSTRAINT ck_agent_confirmations_status CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'EXPIRED', 'CONSUMED')),
    CONSTRAINT ck_agent_confirmations_expiry CHECK (expires_at > created_at),
    CONSTRAINT ck_agent_confirmations_state CHECK (
        (status IN ('PENDING', 'EXPIRED') AND approved_at IS NULL AND rejected_at IS NULL AND consumed_at IS NULL)
        OR (status = 'APPROVED' AND approved_at IS NOT NULL AND rejected_at IS NULL AND consumed_at IS NULL)
        OR (status = 'REJECTED' AND approved_at IS NULL AND rejected_at IS NOT NULL AND consumed_at IS NULL)
        OR (status = 'CONSUMED' AND approved_at IS NOT NULL AND rejected_at IS NULL AND consumed_at IS NOT NULL)
    ),
    CONSTRAINT ck_agent_confirmations_row_version CHECK (row_version >= 1)
);

CREATE TABLE agent_tool_calls (
    agent_tool_call_id uuid NOT NULL,
    ai_task_id uuid NOT NULL,
    user_id uuid NOT NULL,
    tool_name varchar(128) NOT NULL,
    risk_level varchar(16) NOT NULL,
    target_resource_type varchar(32),
    target_resource_id uuid,
    arguments_schema_version integer NOT NULL,
    arguments_summary_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    authorization_result varchar(32) NOT NULL,
    agent_confirmation_id uuid,
    execution_result varchar(32) NOT NULL,
    duration_ms bigint,
    request_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_agent_tool_calls PRIMARY KEY (created_at, agent_tool_call_id),
    CONSTRAINT ck_agent_tool_calls_name CHECK (btrim(tool_name) <> ''),
    CONSTRAINT ck_agent_tool_calls_risk CHECK (risk_level IN ('NORMAL', 'HIGH', 'CRITICAL')),
    CONSTRAINT ck_agent_tool_calls_target CHECK ((target_resource_type IS NULL) = (target_resource_id IS NULL)),
    CONSTRAINT ck_agent_tool_calls_arguments CHECK (arguments_schema_version >= 1 AND jsonb_typeof(arguments_summary_json) = 'object'),
    CONSTRAINT ck_agent_tool_calls_authorization CHECK (authorization_result IN ('ALLOW', 'DENY')),
    CONSTRAINT ck_agent_tool_calls_confirmation CHECK (risk_level = 'NORMAL' OR agent_confirmation_id IS NOT NULL OR authorization_result = 'DENY'),
    CONSTRAINT ck_agent_tool_calls_execution CHECK (execution_result IN ('SUCCESS', 'FAILURE', 'SKIPPED')),
    CONSTRAINT ck_agent_tool_calls_duration CHECK (duration_ms IS NULL OR duration_ms >= 0)
) PARTITION BY RANGE (created_at);

-- ============================================================================
-- 7. 迁移、配置与历史
-- ============================================================================

CREATE TABLE migration_jobs (
    migration_job_id uuid PRIMARY KEY,
    name varchar(256) NOT NULL,
    source_type varchar(32) NOT NULL,
    source_secret_ref varchar(512) NOT NULL,
    target_space_id uuid NOT NULL,
    mode varchar(32) NOT NULL,
    mapping_schema_version integer NOT NULL,
    permission_mapping_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    checkpoint_schema_version integer NOT NULL,
    checkpoint_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at timestamptz,
    heartbeat_at timestamptz,
    completed_at timestamptz,
    failure_code varchar(64),
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_migration_jobs_space FOREIGN KEY (target_space_id)
        REFERENCES spaces(space_id) ON DELETE RESTRICT,
    CONSTRAINT fk_migration_jobs_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_migration_jobs_name CHECK (btrim(name) <> '' AND btrim(source_secret_ref) <> ''),
    CONSTRAINT ck_migration_jobs_source CHECK (source_type IN ('SMB', 'LOCAL')),
    CONSTRAINT ck_migration_jobs_mode CHECK (mode IN ('INITIAL', 'INCREMENTAL', 'CUTOVER')),
    CONSTRAINT ck_migration_jobs_mapping CHECK (mapping_schema_version >= 1 AND jsonb_typeof(permission_mapping_json) = 'object'),
    CONSTRAINT ck_migration_jobs_checkpoint CHECK (checkpoint_schema_version >= 1 AND jsonb_typeof(checkpoint_json) = 'object'),
    CONSTRAINT ck_migration_jobs_status CHECK (status IN ('PENDING', 'RUNNING', 'PAUSED', 'SUCCESS', 'FAILED', 'CANCELLED')),
    CONSTRAINT ck_migration_jobs_completion CHECK (
        (status IN ('SUCCESS', 'FAILED', 'CANCELLED') AND completed_at IS NOT NULL)
        OR status IN ('PENDING', 'RUNNING', 'PAUSED')
    ),
    CONSTRAINT ck_migration_jobs_row_version CHECK (row_version >= 1)
);

CREATE TABLE migration_items (
    migration_item_id uuid PRIMARY KEY,
    migration_job_id uuid NOT NULL,
    source_path text NOT NULL,
    normalized_source_path text NOT NULL,
    source_generation bigint NOT NULL,
    source_type varchar(32) NOT NULL,
    source_file_identity varchar(512),
    source_size_bytes bigint,
    source_modified_at timestamptz,
    source_sha256 bytea,
    target_namespace_entry_id uuid,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    attempt_count integer NOT NULL DEFAULT 0,
    error_code varchar(64),
    error_summary text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_migration_items_job FOREIGN KEY (migration_job_id)
        REFERENCES migration_jobs(migration_job_id) ON DELETE CASCADE,
    CONSTRAINT fk_migration_items_target FOREIGN KEY (target_namespace_entry_id)
        REFERENCES namespace_entries(namespace_entry_id) ON DELETE RESTRICT,
    CONSTRAINT uq_migration_items_source UNIQUE (migration_job_id, normalized_source_path, source_generation),
    CONSTRAINT ck_migration_items_path CHECK (source_path <> '' AND normalized_source_path <> ''),
    CONSTRAINT ck_migration_items_generation CHECK (source_generation >= 1),
    CONSTRAINT ck_migration_items_type CHECK (source_type IN ('FILE', 'FOLDER')),
    CONSTRAINT ck_migration_items_size CHECK (source_size_bytes IS NULL OR source_size_bytes >= 0),
    CONSTRAINT ck_migration_items_sha256 CHECK (source_sha256 IS NULL OR octet_length(source_sha256) = 32),
    CONSTRAINT ck_migration_items_status CHECK (status IN ('PENDING', 'COPIED', 'VERIFIED', 'FAILED', 'SKIPPED')),
    CONSTRAINT ck_migration_items_attempts CHECK (attempt_count >= 0),
    CONSTRAINT ck_migration_items_row_version CHECK (row_version >= 1)
);

CREATE TABLE system_settings (
    setting_key varchar(256) PRIMARY KEY,
    value_schema_version integer NOT NULL,
    value_json jsonb,
    secret_ref varchar(512),
    version bigint NOT NULL DEFAULT 1,
    created_by_user_id uuid NOT NULL,
    updated_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version bigint NOT NULL DEFAULT 1,
    CONSTRAINT fk_system_settings_created_by FOREIGN KEY (created_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_system_settings_updated_by FOREIGN KEY (updated_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT ck_system_settings_key CHECK (btrim(setting_key) <> ''),
    CONSTRAINT ck_system_settings_schema CHECK (value_schema_version >= 1),
    CONSTRAINT ck_system_settings_value CHECK (
        num_nonnulls(value_json, secret_ref) = 1
        AND (secret_ref IS NULL OR btrim(secret_ref) <> '')
    ),
    CONSTRAINT ck_system_settings_versions CHECK (version >= 1 AND row_version >= 1)
);

CREATE TABLE system_setting_revisions (
    system_setting_revision_id uuid PRIMARY KEY,
    setting_key varchar(256) NOT NULL,
    version bigint NOT NULL,
    value_schema_version integer NOT NULL,
    value_json jsonb,
    secret_ref varchar(512),
    changed_by_user_id uuid NOT NULL,
    change_reason text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_system_setting_revisions_setting FOREIGN KEY (setting_key)
        REFERENCES system_settings(setting_key) ON DELETE RESTRICT,
    CONSTRAINT fk_system_setting_revisions_changed_by FOREIGN KEY (changed_by_user_id)
        REFERENCES users(user_id) ON DELETE RESTRICT,
    CONSTRAINT uq_system_setting_revisions_version UNIQUE (setting_key, version),
    CONSTRAINT ck_system_setting_revisions_version CHECK (version >= 1 AND value_schema_version >= 1),
    CONSTRAINT ck_system_setting_revisions_value CHECK (
        num_nonnulls(value_json, secret_ref) = 1
        AND (secret_ref IS NULL OR btrim(secret_ref) <> '')
    ),
    CONSTRAINT ck_system_setting_revisions_reason CHECK (btrim(change_reason) <> '')
);

-- ============================================================================
-- 8. 跨表不变量与只追加保护
-- ============================================================================

CREATE FUNCTION validate_namespace_entry_context()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    parent_space_id uuid;
    parent_depth integer;
BEGIN
    IF NEW.parent_folder_id IS NULL THEN
        IF NEW.depth <> 0 THEN
            RAISE EXCEPTION '根目录项 depth 必须为 0: %', NEW.namespace_entry_id
                USING ERRCODE = '23514';
        END IF;
        RETURN NEW;
    END IF;

    SELECT ne.space_id, ne.depth
      INTO parent_space_id, parent_depth
      FROM folders f
      JOIN namespace_entries ne ON ne.namespace_entry_id = f.folder_id
     WHERE f.folder_id = NEW.parent_folder_id;

    IF NOT FOUND OR parent_space_id <> NEW.space_id THEN
        RAISE EXCEPTION '父文件夹不存在或不属于同一 Space: entry=%, parent=%',
            NEW.namespace_entry_id, NEW.parent_folder_id
            USING ERRCODE = '23514';
    END IF;

    IF NEW.depth <> parent_depth + 1 THEN
        RAISE EXCEPTION '目录项 depth 与父目录不一致: %', NEW.namespace_entry_id
            USING ERRCODE = '23514';
    END IF;

    IF EXISTS (
        WITH RECURSIVE ancestors(folder_id) AS (
            SELECT NEW.parent_folder_id
            UNION ALL
            SELECT ne.parent_folder_id
              FROM namespace_entries ne
              JOIN ancestors a ON ne.namespace_entry_id = a.folder_id
             WHERE ne.parent_folder_id IS NOT NULL
        )
        SELECT 1 FROM ancestors WHERE folder_id = NEW.namespace_entry_id
    ) THEN
        RAISE EXCEPTION '文件夹移动会形成环: %', NEW.namespace_entry_id
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END
$function$;

CREATE CONSTRAINT TRIGGER ct_namespace_entries_context
AFTER INSERT OR UPDATE ON namespace_entries
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_namespace_entry_context();

CREATE FUNCTION protect_namespace_entry_identity()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
BEGIN
    IF NEW.entry_type <> OLD.entry_type THEN
        RAISE EXCEPTION 'namespace_entries.entry_type 创建后不可改变: %', OLD.namespace_entry_id
            USING ERRCODE = '23514';
    END IF;

    IF EXISTS (SELECT 1 FROM spaces s WHERE s.root_folder_id = OLD.namespace_entry_id)
       AND (NEW.space_id, NEW.parent_folder_id, NEW.name, NEW.normalized_name, NEW.lifecycle_status)
           IS DISTINCT FROM
           (OLD.space_id, OLD.parent_folder_id, OLD.name, OLD.normalized_name, OLD.lifecycle_status) THEN
        RAISE EXCEPTION 'Space 根文件夹不可移动、重命名或删除: %', OLD.namespace_entry_id
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END
$function$;

CREATE TRIGGER tg_namespace_entries_protect_identity
BEFORE UPDATE ON namespace_entries
FOR EACH ROW EXECUTE FUNCTION protect_namespace_entry_identity();

CREATE FUNCTION validate_namespace_subtype()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    actual_type varchar(32);
    subtype_id uuid;
BEGIN
    subtype_id := CASE TG_TABLE_NAME
        WHEN 'folders' THEN NEW.folder_id
        WHEN 'documents' THEN NEW.document_id
        WHEN 'shared_entries' THEN NEW.shared_entry_id
    END;

    SELECT entry_type INTO actual_type
      FROM namespace_entries
     WHERE namespace_entry_id = subtype_id;

    IF actual_type IS DISTINCT FROM TG_ARGV[0] THEN
        RAISE EXCEPTION '命名空间子类型不匹配: entry=%, expected=%, actual=%',
            subtype_id, TG_ARGV[0], actual_type USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

CREATE CONSTRAINT TRIGGER ct_folders_entry_type
AFTER INSERT OR UPDATE ON folders
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_namespace_subtype('FOLDER');

CREATE CONSTRAINT TRIGGER ct_documents_entry_type
AFTER INSERT OR UPDATE ON documents
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_namespace_subtype('DOCUMENT');

CREATE CONSTRAINT TRIGGER ct_shared_entries_entry_type
AFTER INSERT OR UPDATE ON shared_entries
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_namespace_subtype('SHARED_ENTRY');

CREATE FUNCTION validate_space_root_folder()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    root_entry namespace_entries%ROWTYPE;
BEGIN
    IF NEW.root_folder_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT * INTO root_entry
      FROM namespace_entries
     WHERE namespace_entry_id = NEW.root_folder_id;

    IF NOT FOUND
       OR root_entry.entry_type <> 'FOLDER'
       OR root_entry.space_id <> NEW.space_id
       OR root_entry.parent_folder_id IS NOT NULL
       OR root_entry.depth <> 0 THEN
        RAISE EXCEPTION 'Space 根文件夹无效: space=%, folder=%', NEW.space_id, NEW.root_folder_id
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

CREATE CONSTRAINT TRIGGER ct_spaces_root_folder
AFTER INSERT OR UPDATE ON spaces
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_space_root_folder();

CREATE FUNCTION validate_upload_session_context()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    context_space_id uuid;
    reservation_space_id uuid;
    reservation_user_id uuid;
BEGIN
    SELECT ne.space_id INTO context_space_id
      FROM folders f
      JOIN namespace_entries ne ON ne.namespace_entry_id = f.folder_id
     WHERE f.folder_id = NEW.folder_id;
    IF context_space_id IS DISTINCT FROM NEW.space_id THEN
        RAISE EXCEPTION '上传目标文件夹不属于目标 Space: %', NEW.upload_session_id
            USING ERRCODE = '23514';
    END IF;

    IF NEW.target_document_id IS NOT NULL THEN
        SELECT ne.space_id INTO context_space_id
          FROM documents d
          JOIN namespace_entries ne ON ne.namespace_entry_id = d.document_id
         WHERE d.document_id = NEW.target_document_id;
        IF context_space_id IS DISTINCT FROM NEW.space_id THEN
            RAISE EXCEPTION '上传目标 Document 不属于目标 Space: %', NEW.upload_session_id
                USING ERRCODE = '23514';
        END IF;
    END IF;

    SELECT qr.space_id, qr.user_id
      INTO reservation_space_id, reservation_user_id
      FROM quota_reservations qr
     WHERE qr.quota_reservation_id = NEW.quota_reservation_id;
    IF reservation_space_id IS DISTINCT FROM NEW.space_id OR reservation_user_id IS DISTINCT FROM NEW.user_id THEN
        RAISE EXCEPTION '上传会话不能消费其他用户或 Space 的配额预留: %', NEW.upload_session_id
            USING ERRCODE = '23514';
    END IF;

    IF NEW.result_document_id IS NOT NULL THEN
        SELECT ne.space_id INTO context_space_id
          FROM documents d
          JOIN namespace_entries ne ON ne.namespace_entry_id = d.document_id
         WHERE d.document_id = NEW.result_document_id;
        IF context_space_id IS DISTINCT FROM NEW.space_id THEN
            RAISE EXCEPTION '上传结果 Document 不属于目标 Space: %', NEW.upload_session_id
                USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END
$function$;

CREATE CONSTRAINT TRIGGER ct_upload_sessions_context
AFTER INSERT OR UPDATE ON upload_sessions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_upload_session_context();

CREATE FUNCTION validate_shared_entry_context()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    entry_space_id uuid;
    share_target_kind varchar(32);
    share_target_space_id uuid;
BEGIN
    SELECT ne.space_id INTO entry_space_id
      FROM namespace_entries ne
     WHERE ne.namespace_entry_id = NEW.shared_entry_id;

    SELECT s.target_kind, s.target_space_id
      INTO share_target_kind, share_target_space_id
      FROM shares s
     WHERE s.share_id = NEW.share_id;

    IF share_target_kind = 'SPACE' AND entry_space_id IS DISTINCT FROM share_target_space_id THEN
        RAISE EXCEPTION '共享引用不属于 Share 的目标 Space: %', NEW.shared_entry_id
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

CREATE CONSTRAINT TRIGGER ct_shared_entries_context
AFTER INSERT OR UPDATE ON shared_entries
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_shared_entry_context();

CREATE FUNCTION validate_organization_closure(p_organization_id uuid)
RETURNS void
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    parent_id uuid;
BEGIN
    SELECT parent_organization_id INTO parent_id
      FROM organizations
     WHERE organization_id = p_organization_id;
    IF NOT FOUND THEN
        RETURN;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM organization_closure
         WHERE ancestor_organization_id = p_organization_id
           AND descendant_organization_id = p_organization_id
           AND depth = 0
    ) THEN
        RAISE EXCEPTION '组织闭包缺少自身行: %', p_organization_id USING ERRCODE = '23514';
    END IF;

    IF parent_id IS NULL THEN
        IF EXISTS (
            SELECT 1 FROM organization_closure
             WHERE descendant_organization_id = p_organization_id
               AND ancestor_organization_id <> p_organization_id
        ) THEN
            RAISE EXCEPTION '根组织存在非法祖先: %', p_organization_id USING ERRCODE = '23514';
        END IF;
        RETURN;
    END IF;

    IF EXISTS (
        SELECT 1 FROM organization_closure
         WHERE ancestor_organization_id = p_organization_id
           AND descendant_organization_id = parent_id
    ) THEN
        RAISE EXCEPTION '组织树形成环: %', p_organization_id USING ERRCODE = '23514';
    END IF;

    IF EXISTS (
        (SELECT ancestor_organization_id, depth + 1 AS depth
           FROM organization_closure
          WHERE descendant_organization_id = parent_id)
        EXCEPT
        (SELECT ancestor_organization_id, depth
           FROM organization_closure
          WHERE descendant_organization_id = p_organization_id
            AND ancestor_organization_id <> p_organization_id)
    ) OR EXISTS (
        (SELECT ancestor_organization_id, depth
           FROM organization_closure
          WHERE descendant_organization_id = p_organization_id
            AND ancestor_organization_id <> p_organization_id)
        EXCEPT
        (SELECT ancestor_organization_id, depth + 1 AS depth
           FROM organization_closure
          WHERE descendant_organization_id = parent_id)
    ) THEN
        RAISE EXCEPTION '组织邻接关系与闭包关系不一致: %', p_organization_id USING ERRCODE = '23514';
    END IF;
END
$function$;

CREATE FUNCTION validate_organization_closure_trigger()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
BEGIN
    IF TG_TABLE_NAME = 'organizations' THEN
        PERFORM validate_organization_closure(NEW.organization_id);
    ELSE
        IF TG_OP <> 'DELETE' THEN
            PERFORM validate_organization_closure(NEW.descendant_organization_id);
        END IF;
        IF TG_OP = 'DELETE' THEN
            PERFORM validate_organization_closure(OLD.descendant_organization_id);
        ELSIF TG_OP = 'UPDATE'
              AND OLD.descendant_organization_id <> NEW.descendant_organization_id THEN
            PERFORM validate_organization_closure(OLD.descendant_organization_id);
        END IF;
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END
$function$;

CREATE CONSTRAINT TRIGGER ct_organizations_closure
AFTER INSERT OR UPDATE ON organizations
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_organization_closure_trigger();

CREATE CONSTRAINT TRIGGER ct_organization_closure_consistency
AFTER INSERT OR UPDATE OR DELETE ON organization_closure
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_organization_closure_trigger();

CREATE FUNCTION validate_admin_delegation(p_admin_delegation_id uuid)
RETURNS void
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    child admin_delegations%ROWTYPE;
    parent admin_delegations%ROWTYPE;
BEGIN
    SELECT * INTO child FROM admin_delegations WHERE admin_delegation_id = p_admin_delegation_id;
    IF NOT FOUND OR child.parent_admin_delegation_id IS NULL THEN
        RETURN;
    END IF;

    SELECT * INTO parent FROM admin_delegations WHERE admin_delegation_id = child.parent_admin_delegation_id;
    IF NOT FOUND OR NOT parent.can_delegate THEN
        RAISE EXCEPTION '父管理委派不存在或不允许继续委派: %', p_admin_delegation_id USING ERRCODE = '23514';
    END IF;

    IF child.valid_from < parent.valid_from
       OR (parent.valid_until IS NOT NULL AND (child.valid_until IS NULL OR child.valid_until > parent.valid_until)) THEN
        RAISE EXCEPTION '子管理委派有效期超出父委派: %', p_admin_delegation_id USING ERRCODE = '23514';
    END IF;

    IF parent.scope = 'SELF' THEN
        IF child.organization_id <> parent.organization_id OR child.scope <> 'SELF' THEN
            RAISE EXCEPTION '子管理委派组织范围超出 SELF 父委派: %', p_admin_delegation_id USING ERRCODE = '23514';
        END IF;
    ELSIF NOT EXISTS (
        SELECT 1 FROM organization_closure
         WHERE ancestor_organization_id = parent.organization_id
           AND descendant_organization_id = child.organization_id
    ) THEN
        RAISE EXCEPTION '子管理委派组织不在父委派子树内: %', p_admin_delegation_id USING ERRCODE = '23514';
    END IF;

    IF EXISTS (
        SELECT capability FROM admin_delegation_capabilities WHERE admin_delegation_id = child.admin_delegation_id
        EXCEPT
        SELECT capability FROM admin_delegation_capabilities WHERE admin_delegation_id = parent.admin_delegation_id
    ) THEN
        RAISE EXCEPTION '子管理委派能力超出父委派: %', p_admin_delegation_id USING ERRCODE = '23514';
    END IF;
END
$function$;

CREATE FUNCTION validate_admin_delegation_trigger()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    delegation_id uuid;
    child_id uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        delegation_id := OLD.admin_delegation_id;
    ELSE
        delegation_id := NEW.admin_delegation_id;
    END IF;

    PERFORM validate_admin_delegation(delegation_id);
    FOR child_id IN
        SELECT admin_delegation_id FROM admin_delegations
         WHERE parent_admin_delegation_id = delegation_id
    LOOP
        PERFORM validate_admin_delegation(child_id);
    END LOOP;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END
$function$;

CREATE CONSTRAINT TRIGGER ct_admin_delegations_scope
AFTER INSERT OR UPDATE ON admin_delegations
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_admin_delegation_trigger();

CREATE CONSTRAINT TRIGGER ct_admin_delegation_capability_subset
AFTER INSERT OR UPDATE OR DELETE ON admin_delegation_capabilities
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_admin_delegation_trigger();

CREATE FUNCTION validate_permission_grant_actions()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    grant_id uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        grant_id := OLD.permission_grant_id;
    ELSE
        grant_id := NEW.permission_grant_id;
    END IF;
    IF EXISTS (SELECT 1 FROM permission_grants WHERE permission_grant_id = grant_id)
       AND NOT EXISTS (SELECT 1 FROM permission_grant_actions WHERE permission_grant_id = grant_id) THEN
        RAISE EXCEPTION '权限授权必须至少包含一个动作: %', grant_id USING ERRCODE = '23514';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END
$function$;

CREATE CONSTRAINT TRIGGER ct_permission_grants_nonempty_actions
AFTER INSERT OR UPDATE ON permission_grants
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_permission_grant_actions();

CREATE CONSTRAINT TRIGGER ct_permission_grant_actions_nonempty
AFTER DELETE ON permission_grant_actions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_permission_grant_actions();

CREATE FUNCTION validate_share_actions()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    target_share_id uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        target_share_id := OLD.share_id;
    ELSE
        target_share_id := NEW.share_id;
    END IF;
    IF EXISTS (SELECT 1 FROM shares WHERE share_id = target_share_id)
       AND NOT EXISTS (SELECT 1 FROM share_actions WHERE share_id = target_share_id) THEN
        RAISE EXCEPTION '共享必须至少包含一个动作: %', target_share_id USING ERRCODE = '23514';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END
$function$;

CREATE CONSTRAINT TRIGGER ct_shares_nonempty_actions
AFTER INSERT OR UPDATE ON shares
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_share_actions();

CREATE CONSTRAINT TRIGGER ct_share_actions_nonempty
AFTER DELETE ON share_actions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_share_actions();

CREATE FUNCTION reject_history_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $function$
BEGIN
    RAISE EXCEPTION '% 是只追加表，不允许 %', TG_TABLE_NAME, TG_OP USING ERRCODE = '55000';
END
$function$;

CREATE TRIGGER tg_user_password_history_append_only
BEFORE UPDATE OR DELETE ON user_password_history
FOR EACH ROW EXECUTE FUNCTION reject_history_mutation();

CREATE TRIGGER tg_document_versions_append_only
BEFORE UPDATE OR DELETE ON document_versions
FOR EACH ROW EXECUTE FUNCTION reject_history_mutation();

CREATE TRIGGER tg_storage_scan_results_append_only
BEFORE UPDATE OR DELETE ON storage_scan_results
FOR EACH ROW EXECUTE FUNCTION reject_history_mutation();

CREATE TRIGGER tg_audit_events_append_only
BEFORE UPDATE OR DELETE ON audit_events
FOR EACH ROW EXECUTE FUNCTION reject_history_mutation();

CREATE TRIGGER tg_agent_tool_calls_append_only
BEFORE UPDATE OR DELETE ON agent_tool_calls
FOR EACH ROW EXECUTE FUNCTION reject_history_mutation();

CREATE TRIGGER tg_system_setting_revisions_append_only
BEFORE UPDATE OR DELETE ON system_setting_revisions
FOR EACH ROW EXECUTE FUNCTION reject_history_mutation();

-- ============================================================================
-- 9. 唯一索引与查询索引
-- ============================================================================

CREATE UNIQUE INDEX uq_users_username_normalized ON users (username_normalized);
CREATE UNIQUE INDEX uq_users_employee_no_normalized ON users (employee_no_normalized)
    WHERE employee_no_normalized IS NOT NULL;
CREATE UNIQUE INDEX uq_users_email_normalized ON users (email_normalized)
    WHERE email_normalized IS NOT NULL;
CREATE INDEX ix_users_created_by ON users (created_by_user_id) WHERE created_by_user_id IS NOT NULL;
CREATE INDEX ix_users_avatar_object ON users (avatar_storage_object_id) WHERE avatar_storage_object_id IS NOT NULL;

CREATE UNIQUE INDEX uq_user_credentials_active_password ON user_credentials (user_id)
    WHERE credential_type = 'PASSWORD' AND status = 'ACTIVE';
CREATE UNIQUE INDEX uq_user_credentials_active_external
    ON user_credentials (credential_type, COALESCE(provider, ''), identifier_normalized)
    WHERE credential_type IN ('LDAP', 'OIDC') AND status = 'ACTIVE';
CREATE UNIQUE INDEX uq_user_credentials_active_app_password
    ON user_credentials (user_id, identifier_normalized)
    WHERE credential_type = 'APP_PASSWORD' AND status = 'ACTIVE';
CREATE INDEX ix_user_credentials_user_status ON user_credentials (user_id, status);
CREATE INDEX ix_user_mfa_methods_user_status ON user_mfa_methods (user_id, status);
CREATE INDEX ix_mfa_recovery_codes_user_batch_status ON mfa_recovery_codes (user_id, code_batch_id, status);
CREATE INDEX ix_user_password_history_user_changed ON user_password_history (user_id, password_changed_at DESC);
CREATE INDEX ix_user_sessions_user_status_expiry ON user_sessions (user_id, status, expires_at);
CREATE INDEX ix_refresh_tokens_session_status ON session_refresh_tokens (user_session_id, status, expires_at);
CREATE INDEX ix_refresh_tokens_parent ON session_refresh_tokens (parent_refresh_token_id) WHERE parent_refresh_token_id IS NOT NULL;
CREATE INDEX ix_login_attempts_username_created ON login_attempts (username_normalized, created_at DESC);
CREATE INDEX ix_login_attempts_user_created ON login_attempts (user_id, created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX ix_login_attempts_ip_created ON login_attempts (ip_address, created_at DESC) WHERE ip_address IS NOT NULL;
CREATE UNIQUE INDEX uq_user_offboarding_open_case ON user_offboarding_cases (departing_user_id)
    WHERE status IN ('DRAFT', 'APPROVED', 'PROCESSING');

CREATE UNIQUE INDEX uq_organizations_root_name ON organizations (normalized_name)
    WHERE parent_organization_id IS NULL AND status <> 'DELETED';
CREATE UNIQUE INDEX uq_organizations_child_name ON organizations (parent_organization_id, normalized_name)
    WHERE parent_organization_id IS NOT NULL AND status <> 'DELETED';
CREATE UNIQUE INDEX uq_organizations_active_code ON organizations (normalized_code)
    WHERE normalized_code IS NOT NULL AND status <> 'DELETED';
CREATE INDEX ix_organizations_parent_sort ON organizations (parent_organization_id, sort_order, organization_id);
CREATE INDEX ix_organization_closure_descendant ON organization_closure (descendant_organization_id, depth, ancestor_organization_id);
CREATE INDEX ix_organization_closure_ancestor ON organization_closure (ancestor_organization_id, depth, descendant_organization_id);
CREATE UNIQUE INDEX uq_user_organizations_active_membership ON user_organizations (user_id, organization_id)
    WHERE status = 'ACTIVE';
CREATE UNIQUE INDEX uq_user_organizations_active_primary ON user_organizations (user_id)
    WHERE status = 'ACTIVE' AND membership_type = 'PRIMARY';
CREATE INDEX ix_user_organizations_user_period ON user_organizations (user_id, status, effective_from, effective_until);
CREATE INDEX ix_user_organizations_org_members ON user_organizations (organization_id, status, user_id);
CREATE UNIQUE INDEX uq_spaces_personal_owner ON spaces (owner_user_id)
    WHERE space_type = 'PERSONAL' AND status <> 'DELETED';
CREATE UNIQUE INDEX uq_spaces_organization ON spaces (organization_id)
    WHERE space_type = 'ORGANIZATION' AND status <> 'DELETED';
CREATE INDEX ix_spaces_created_by ON spaces (created_by_user_id);
CREATE INDEX ix_quota_reservations_expiry ON quota_reservations (status, expires_at)
    WHERE status = 'ACTIVE';
CREATE INDEX ix_quota_reservations_space_user ON quota_reservations (space_id, user_id, status);
CREATE INDEX ix_admin_delegations_user_scope ON admin_delegations (user_id, status, valid_until, organization_id);
CREATE INDEX ix_admin_delegations_org_status ON admin_delegations (organization_id, status);
CREATE INDEX ix_admin_delegations_parent ON admin_delegations (parent_admin_delegation_id)
    WHERE parent_admin_delegation_id IS NOT NULL;
CREATE INDEX ix_org_change_plans_created_by ON organization_change_plans (created_by_user_id, created_at DESC);
CREATE INDEX ix_org_change_operations_source ON organization_change_operations (source_organization_id)
    WHERE source_organization_id IS NOT NULL;
CREATE INDEX ix_org_change_operations_target ON organization_change_operations (target_organization_id)
    WHERE target_organization_id IS NOT NULL;

CREATE UNIQUE INDEX uq_namespace_entries_child_name
    ON namespace_entries (space_id, parent_folder_id, normalized_name)
    WHERE parent_folder_id IS NOT NULL AND lifecycle_status IN ('ACTIVE', 'ARCHIVED');
CREATE UNIQUE INDEX uq_namespace_entries_root_name
    ON namespace_entries (space_id, normalized_name)
    WHERE parent_folder_id IS NULL AND lifecycle_status IN ('ACTIVE', 'ARCHIVED');
CREATE INDEX ix_namespace_entries_directory
    ON namespace_entries (space_id, parent_folder_id, lifecycle_status, normalized_name, namespace_entry_id);
CREATE INDEX ix_namespace_entries_recent
    ON namespace_entries (space_id, updated_at DESC, namespace_entry_id);
CREATE INDEX ix_namespace_entries_parent ON namespace_entries (parent_folder_id) WHERE parent_folder_id IS NOT NULL;
CREATE INDEX ix_namespace_entries_created_by ON namespace_entries (created_by_user_id);
CREATE INDEX ix_documents_owner ON documents (owner_user_id);
CREATE INDEX ix_document_versions_document_desc ON document_versions (document_id, version_number DESC);
CREATE INDEX ix_document_versions_storage_object ON document_versions (storage_object_id);
CREATE INDEX ix_document_versions_created_by ON document_versions (created_by_user_id);
CREATE INDEX ix_storage_objects_dedup ON storage_objects (sha256, size_bytes, scan_status, status);
CREATE INDEX ix_storage_scan_results_object_created ON storage_scan_results (storage_object_id, created_at DESC);
CREATE INDEX ix_upload_sessions_user_recovery ON upload_sessions (user_id, status, expires_at);
CREATE INDEX ix_upload_sessions_space_status ON upload_sessions (space_id, status);
CREATE INDEX ix_upload_sessions_folder ON upload_sessions (folder_id);
CREATE INDEX ix_upload_sessions_target_document ON upload_sessions (target_document_id) WHERE target_document_id IS NOT NULL;
CREATE UNIQUE INDEX uq_document_locks_active ON document_locks (document_id) WHERE status = 'ACTIVE';
CREATE INDEX ix_document_locks_user_status ON document_locks (user_id, status);

CREATE INDEX ix_permission_grants_subject_user ON permission_grants (subject_user_id, status, valid_until)
    WHERE subject_user_id IS NOT NULL;
CREATE INDEX ix_permission_grants_subject_org ON permission_grants (subject_organization_id, status, valid_until)
    WHERE subject_organization_id IS NOT NULL;
CREATE INDEX ix_permission_grants_space ON permission_grants (space_id, status, valid_until)
    WHERE space_id IS NOT NULL;
CREATE INDEX ix_permission_grants_folder ON permission_grants (folder_id, status, valid_until)
    WHERE folder_id IS NOT NULL;
CREATE INDEX ix_permission_grants_document ON permission_grants (document_id, status, valid_until)
    WHERE document_id IS NOT NULL;
CREATE INDEX ix_shares_source_document ON shares (source_document_id, status) WHERE source_document_id IS NOT NULL;
CREATE INDEX ix_shares_source_folder ON shares (source_folder_id, status) WHERE source_folder_id IS NOT NULL;
CREATE INDEX ix_shares_target_user ON shares (target_user_id, status, valid_until) WHERE target_user_id IS NOT NULL;
CREATE INDEX ix_shares_target_org ON shares (target_organization_id, status, valid_until) WHERE target_organization_id IS NOT NULL;
CREATE INDEX ix_shares_target_space ON shares (target_space_id, status, valid_until) WHERE target_space_id IS NOT NULL;
CREATE INDEX ix_shares_creator ON shares (creator_user_id, created_at DESC);
CREATE UNIQUE INDEX uq_tags_global_name ON tags (normalized_name) WHERE scope_kind = 'GLOBAL';
CREATE UNIQUE INDEX uq_tags_space_name ON tags (scope_space_id, normalized_name) WHERE scope_kind = 'SPACE';
CREATE UNIQUE INDEX uq_document_tags_active_source
    ON document_tags (document_id, tag_id, source, source_reference) WHERE status = 'ACTIVE';
CREATE INDEX ix_document_tags_tag_status ON document_tags (tag_id, status, document_id);
CREATE INDEX ix_recent_documents_user_accessed ON recent_documents (user_id, last_accessed_at DESC, document_id);
CREATE UNIQUE INDEX uq_recycle_items_active_entry ON recycle_items (namespace_entry_id) WHERE status = 'ACTIVE';
CREATE INDEX ix_recycle_items_cleanup ON recycle_items (status, expires_at);
CREATE UNIQUE INDEX uq_retention_targets_space ON retention_policy_targets (retention_policy_id, space_id)
    WHERE target_kind = 'SPACE';
CREATE UNIQUE INDEX uq_retention_targets_folder ON retention_policy_targets (retention_policy_id, folder_id)
    WHERE target_kind = 'FOLDER';
CREATE UNIQUE INDEX uq_retention_targets_tag ON retention_policy_targets (retention_policy_id, tag_id)
    WHERE target_kind = 'TAG';
CREATE UNIQUE INDEX uq_retention_targets_mime ON retention_policy_targets (retention_policy_id, mime_pattern)
    WHERE target_kind = 'MIME';
CREATE INDEX ix_legal_holds_document_status ON legal_holds (document_id, status);
CREATE INDEX ix_legal_holds_version_status ON legal_holds (document_version_id, status)
    WHERE document_version_id IS NOT NULL;

CREATE INDEX ix_audit_events_created ON audit_events (created_at DESC, audit_event_id);
CREATE INDEX ix_audit_events_actor ON audit_events (actor_id, created_at DESC) WHERE actor_id IS NOT NULL;
CREATE INDEX ix_audit_events_resource ON audit_events (resource_id, created_at DESC) WHERE resource_id IS NOT NULL;
CREATE INDEX ix_audit_events_type ON audit_events (event_type, created_at DESC);
CREATE INDEX ix_audit_events_request ON audit_events (request_id);
CREATE INDEX ix_audit_events_chain ON audit_events (chain_id, sequence_number) WHERE chain_id IS NOT NULL;
CREATE UNIQUE INDEX uq_idempotency_user_key
    ON idempotency_records (user_id, operation, idempotency_key) WHERE principal_kind = 'USER';
CREATE UNIQUE INDEX uq_idempotency_service_key
    ON idempotency_records (principal_kind, service_principal, operation, idempotency_key)
    WHERE principal_kind IN ('SERVICE', 'SYSTEM');
CREATE INDEX ix_idempotency_expiry ON idempotency_records (expires_at);
CREATE INDEX ix_outbox_events_claim
    ON outbox_events (status, available_at, lease_until, priority DESC, outbox_event_id)
    WHERE status IN ('PENDING', 'PROCESSING', 'FAILED');
CREATE INDEX ix_outbox_events_aggregate ON outbox_events (aggregate_type, aggregate_id, aggregate_version);
CREATE UNIQUE INDEX uq_background_jobs_active_dedup
    ON background_jobs (job_type, deduplication_key)
    WHERE status IN ('PENDING', 'PROCESSING', 'FAILED');
CREATE INDEX ix_background_jobs_claim
    ON background_jobs (status, available_at, lease_until, priority DESC, background_job_id)
    WHERE status IN ('PENDING', 'PROCESSING', 'FAILED');
CREATE INDEX ix_background_jobs_document ON background_jobs (target_document_id) WHERE target_document_id IS NOT NULL;
CREATE INDEX ix_background_jobs_storage ON background_jobs (target_storage_object_id) WHERE target_storage_object_id IS NOT NULL;

CREATE INDEX ix_preview_artifacts_document ON preview_artifacts (document_id, status);
CREATE INDEX ix_preview_artifacts_output ON preview_artifacts (output_storage_object_id)
    WHERE output_storage_object_id IS NOT NULL;
CREATE INDEX ix_document_extractions_document ON document_extractions (document_id, status);
CREATE INDEX ix_document_extractions_text_object ON document_extractions (extracted_text_storage_object_id)
    WHERE extracted_text_storage_object_id IS NOT NULL;
CREATE INDEX ix_document_chunks_extraction ON document_chunks (document_extraction_id, chunk_index);
CREATE INDEX ix_ai_tasks_user_created ON ai_tasks (user_id, created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX ix_ai_tasks_document ON ai_tasks (document_id, status) WHERE document_id IS NOT NULL;
CREATE INDEX ix_agent_confirmations_user_status ON agent_confirmations (user_id, status, expires_at);
CREATE INDEX ix_agent_tool_calls_task_created ON agent_tool_calls (ai_task_id, created_at DESC);
CREATE INDEX ix_agent_tool_calls_user_created ON agent_tool_calls (user_id, created_at DESC);
CREATE INDEX ix_agent_tool_calls_request ON agent_tool_calls (request_id);
CREATE INDEX ix_migration_jobs_space_status ON migration_jobs (target_space_id, status);
CREATE INDEX ix_migration_items_job_status ON migration_items (migration_job_id, status, migration_item_id);
CREATE INDEX ix_migration_items_target ON migration_items (target_namespace_entry_id)
    WHERE target_namespace_entry_id IS NOT NULL;

-- 为尚未被上面业务索引覆盖的外键自动补齐普通索引。名称由表、列和约束名稳定生成。
DO $fw_fk_indexes$
DECLARE
    foreign_key record;
    index_name text;
BEGIN
    FOR foreign_key IN
        SELECT c.conrelid,
               c.conname,
               cls.relname AS table_name,
               c.conkey,
               (
                   SELECT string_agg(quote_ident(a.attname), ', ' ORDER BY keys.ordinality)
                     FROM unnest(c.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
                     JOIN pg_attribute a
                       ON a.attrelid = c.conrelid AND a.attnum = keys.attnum
               ) AS column_list,
               (
                   SELECT string_agg(a.attname, '_' ORDER BY keys.ordinality)
                     FROM unnest(c.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
                     JOIN pg_attribute a
                       ON a.attrelid = c.conrelid AND a.attnum = keys.attnum
               ) AS column_name_part
          FROM pg_constraint c
          JOIN pg_class cls ON cls.oid = c.conrelid
          JOIN pg_namespace n ON n.oid = cls.relnamespace
         WHERE c.contype = 'f'
           AND n.nspname = 'file_workshop'
           AND NOT EXISTS (
               SELECT 1
                 FROM pg_index i
                WHERE i.indrelid = c.conrelid
                  AND i.indisvalid
                  AND i.indpred IS NULL
                  AND i.indnkeyatts >= cardinality(c.conkey)
                  AND NOT EXISTS (
                      SELECT 1
                        FROM generate_subscripts(c.conkey, 1) AS positions(position)
                       WHERE i.indkey[positions.position - 1] IS DISTINCT FROM c.conkey[positions.position]
                  )
           )
         ORDER BY cls.relname, c.conname
    LOOP
        index_name := left(
            'ixfk_' || foreign_key.table_name || '_' || foreign_key.column_name_part,
            54
        ) || '_' || substr(md5(foreign_key.conname), 1, 8);
        EXECUTE format(
            'CREATE INDEX %I ON %s (%s)',
            index_name,
            foreign_key.conrelid::regclass,
            foreign_key.column_list
        );
    END LOOP;
END
$fw_fk_indexes$;

-- ============================================================================
-- 10. 初始月分区
-- ============================================================================

DO $fw_partitions$
DECLARE
    month_offset integer;
    timestamp_start timestamptz;
    timestamp_end timestamptz;
    date_start date;
    date_end date;
    partition_suffix text;
BEGIN
    FOR month_offset IN -1..3 LOOP
        timestamp_start := date_trunc('month', CURRENT_TIMESTAMP) + make_interval(months => month_offset);
        timestamp_end := timestamp_start + interval '1 month';
        date_start := timestamp_start::date;
        date_end := timestamp_end::date;
        partition_suffix := to_char(timestamp_start, 'YYYYMM');

        EXECUTE format(
            'CREATE TABLE file_workshop.%I PARTITION OF file_workshop.login_attempts FOR VALUES FROM (%L) TO (%L)',
            'login_attempts_p' || partition_suffix, timestamp_start, timestamp_end
        );
        EXECUTE format(
            'CREATE TABLE file_workshop.%I PARTITION OF file_workshop.audit_events FOR VALUES FROM (%L) TO (%L)',
            'audit_events_p' || partition_suffix, date_start, date_end
        );
        EXECUTE format(
            'CREATE TABLE file_workshop.%I PARTITION OF file_workshop.agent_tool_calls FOR VALUES FROM (%L) TO (%L)',
            'agent_tool_calls_p' || partition_suffix, timestamp_start, timestamp_end
        );
    END LOOP;

    CREATE TABLE file_workshop.login_attempts_default
        PARTITION OF file_workshop.login_attempts DEFAULT;
    CREATE TABLE file_workshop.audit_events_default
        PARTITION OF file_workshop.audit_events DEFAULT;
    CREATE TABLE file_workshop.agent_tool_calls_default
        PARTITION OF file_workshop.agent_tool_calls DEFAULT;
END
$fw_partitions$;

-- ============================================================================
-- 11. 导入后只读验证结果
-- ============================================================================

WITH expected(table_name, optional) AS (
    VALUES
      ('users', false), ('user_credentials', false), ('user_mfa_methods', false),
      ('mfa_recovery_codes', false), ('user_password_history', false), ('user_sessions', false),
      ('session_refresh_tokens', false), ('login_attempts', false), ('principal_security_versions', false),
      ('user_offboarding_cases', false), ('organizations', false), ('organization_closure', false),
      ('user_organizations', false), ('organization_security_versions', false), ('spaces', false),
      ('quota_reservations', false), ('admin_delegations', false), ('admin_delegation_capabilities', false),
      ('organization_change_plans', false), ('organization_change_operations', false),
      ('namespace_entries', false), ('folders', false), ('documents', false),
      ('document_versions', false), ('storage_objects', false), ('storage_scan_results', false),
      ('upload_sessions', false), ('upload_parts', false), ('document_lock_counters', false),
      ('document_locks', false), ('permission_grants', false), ('permission_grant_actions', false),
      ('shares', false), ('share_actions', false), ('shared_entries', false),
      ('tags', false), ('document_tags', false), ('favorites', false), ('recent_documents', false),
      ('recycle_items', false), ('retention_policies', false), ('retention_policy_targets', false),
      ('legal_holds', false), ('audit_events', false), ('audit_chain_heads', false),
      ('idempotency_records', false), ('outbox_events', false), ('background_jobs', false),
      ('document_index_states', false), ('preview_artifacts', false), ('document_extractions', false),
      ('document_chunks', false), ('chunk_embeddings', true), ('ai_tasks', false),
      ('agent_confirmations', false), ('agent_tool_calls', false), ('migration_jobs', false),
      ('migration_items', false), ('system_settings', false), ('system_setting_revisions', false)
), actual AS (
    SELECT c.relname AS table_name
      FROM pg_class c
      JOIN pg_namespace n ON n.oid = c.relnamespace
     WHERE n.nspname = 'file_workshop'
       AND c.relkind IN ('r', 'p')
)
SELECT
    current_database() AS database_name,
    current_setting('server_version') AS postgresql_version,
    (SELECT count(*) FROM expected) AS designed_table_count,
    (SELECT count(*) FROM expected e JOIN actual a USING (table_name)) AS created_table_count,
    (SELECT count(*) FROM expected e LEFT JOIN actual a USING (table_name)
      WHERE NOT e.optional AND a.table_name IS NULL) AS missing_required_table_count,
    EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') AS pgvector_enabled,
    to_regclass('file_workshop.chunk_embeddings') IS NOT NULL AS chunk_embeddings_created;

WITH expected(table_name, optional) AS (
    VALUES
      ('users', false), ('user_credentials', false), ('user_mfa_methods', false),
      ('mfa_recovery_codes', false), ('user_password_history', false), ('user_sessions', false),
      ('session_refresh_tokens', false), ('login_attempts', false), ('principal_security_versions', false),
      ('user_offboarding_cases', false), ('organizations', false), ('organization_closure', false),
      ('user_organizations', false), ('organization_security_versions', false), ('spaces', false),
      ('quota_reservations', false), ('admin_delegations', false), ('admin_delegation_capabilities', false),
      ('organization_change_plans', false), ('organization_change_operations', false),
      ('namespace_entries', false), ('folders', false), ('documents', false),
      ('document_versions', false), ('storage_objects', false), ('storage_scan_results', false),
      ('upload_sessions', false), ('upload_parts', false), ('document_lock_counters', false),
      ('document_locks', false), ('permission_grants', false), ('permission_grant_actions', false),
      ('shares', false), ('share_actions', false), ('shared_entries', false),
      ('tags', false), ('document_tags', false), ('favorites', false), ('recent_documents', false),
      ('recycle_items', false), ('retention_policies', false), ('retention_policy_targets', false),
      ('legal_holds', false), ('audit_events', false), ('audit_chain_heads', false),
      ('idempotency_records', false), ('outbox_events', false), ('background_jobs', false),
      ('document_index_states', false), ('preview_artifacts', false), ('document_extractions', false),
      ('document_chunks', false), ('chunk_embeddings', true), ('ai_tasks', false),
      ('agent_confirmations', false), ('agent_tool_calls', false), ('migration_jobs', false),
      ('migration_items', false), ('system_settings', false), ('system_setting_revisions', false)
)
SELECT e.table_name AS missing_table,
       CASE WHEN e.optional THEN 'OPTIONAL_PGVECTOR' ELSE 'REQUIRED' END AS requirement
  FROM expected e
 WHERE to_regclass(format('file_workshop.%I', e.table_name)) IS NULL
 ORDER BY e.optional, e.table_name;

SELECT parent.relname AS partitioned_table,
       count(child.oid) AS child_partition_count
  FROM pg_class parent
  JOIN pg_namespace n ON n.oid = parent.relnamespace
  JOIN pg_inherits i ON i.inhparent = parent.oid
  JOIN pg_class child ON child.oid = i.inhrelid
 WHERE n.nspname = 'file_workshop'
   AND parent.relname IN ('login_attempts', 'audit_events', 'agent_tool_calls')
 GROUP BY parent.relname
 ORDER BY parent.relname;

SELECT count(*) AS unvalidated_constraint_count
  FROM pg_constraint c
  JOIN pg_namespace n ON n.oid = c.connamespace
 WHERE n.nspname = 'file_workshop'
   AND NOT c.convalidated;

-- +goose StatementEnd

-- +goose Down
-- 首版基线回滚会删除整个业务 Schema，只能用于空库/测试库演练。
-- +goose StatementBegin
DROP SCHEMA IF EXISTS file_workshop CASCADE;
-- +goose StatementEnd
