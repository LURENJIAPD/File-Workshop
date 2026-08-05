-- name: LockOrganizationTreeMutation :exec
SELECT pg_advisory_xact_lock(hashtextextended('file_workshop:organization_tree', 0));

-- name: GetOrganization :one
SELECT *
FROM organizations
WHERE organization_id = $1;

-- name: GetOrganizationForUpdate :one
SELECT *
FROM organizations
WHERE organization_id = $1
FOR UPDATE;

-- name: CountOrganizations :one
SELECT count(*)::bigint
FROM organizations
WHERE (sqlc.narg('parent_organization_id')::uuid IS NULL OR parent_organization_id = sqlc.narg('parent_organization_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'));

-- name: ListOrganizations :many
SELECT *
FROM organizations
WHERE (sqlc.narg('parent_organization_id')::uuid IS NULL OR parent_organization_id = sqlc.narg('parent_organization_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY sort_order, normalized_name, organization_id
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: InsertOrganization :one
INSERT INTO organizations (
    organization_id, parent_organization_id, name, normalized_name, code,
    normalized_code, type_label, sort_order, depth, created_by_user_id,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
RETURNING *;

-- name: InsertOrganizationClosure :exec
INSERT INTO organization_closure (
    ancestor_organization_id, descendant_organization_id, depth, created_at
)
SELECT ancestor_organization_id, sqlc.arg('organization_id')::uuid, depth + 1, sqlc.arg('created_at')::timestamptz
FROM organization_closure
WHERE descendant_organization_id = sqlc.narg('parent_organization_id')::uuid
UNION ALL
SELECT sqlc.arg('organization_id')::uuid, sqlc.arg('organization_id')::uuid, 0, sqlc.arg('created_at')::timestamptz;

-- name: InsertOrganizationSecurityVersions :exec
INSERT INTO organization_security_versions (organization_id, updated_at)
VALUES ($1, $2);

-- name: InsertSpace :one
INSERT INTO spaces (
    space_id, space_type, name, normalized_name, owner_user_id, organization_id,
    quota_bytes, config_schema_version, config_json, created_by_user_id,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
RETURNING *;

-- name: UpdateOrganization :one
UPDATE organizations
SET name = $2,
    normalized_name = $3,
    code = $4,
    normalized_code = $5,
    type_label = $6,
    sort_order = $7,
    updated_at = $8,
    row_version = row_version + 1
WHERE organization_id = $1
  AND row_version = $9
  AND status <> 'DELETED'
RETURNING *;

-- name: OrganizationWouldCreateCycle :one
SELECT EXISTS (
    SELECT 1
    FROM organization_closure
    WHERE ancestor_organization_id = $1
      AND descendant_organization_id = $2
)::boolean;

-- name: DeleteOrganizationExternalClosureLinks :exec
DELETE FROM organization_closure AS link
WHERE link.descendant_organization_id IN (
    SELECT subtree.descendant_organization_id
    FROM organization_closure AS subtree
    WHERE subtree.ancestor_organization_id = $1
)
AND link.ancestor_organization_id IN (
    SELECT ancestors.ancestor_organization_id
    FROM organization_closure AS ancestors
    WHERE ancestors.descendant_organization_id = $1
      AND ancestors.ancestor_organization_id <> $1
);

-- name: InsertMovedOrganizationClosureLinks :exec
INSERT INTO organization_closure (
    ancestor_organization_id, descendant_organization_id, depth, created_at
)
SELECT parent_path.ancestor_organization_id,
       subtree.descendant_organization_id,
       parent_path.depth + subtree.depth + 1,
       $3
FROM organization_closure AS parent_path
CROSS JOIN organization_closure AS subtree
WHERE parent_path.descendant_organization_id = $2
  AND subtree.ancestor_organization_id = $1;

-- name: UpdateMovedOrganizationSubtree :exec
UPDATE organizations
SET depth = depth + sqlc.arg('depth_delta')::integer,
    parent_organization_id = CASE
        WHEN organization_id = sqlc.arg('organization_id')::uuid THEN sqlc.narg('new_parent_organization_id')::uuid
        ELSE parent_organization_id
    END,
    path_cache = NULL,
    tree_version = tree_version + 1,
    updated_at = sqlc.arg('updated_at')::timestamptz,
    row_version = row_version + 1
WHERE organization_id IN (
    SELECT descendant_organization_id
    FROM organization_closure
    WHERE ancestor_organization_id = sqlc.arg('organization_id')::uuid
)
  AND (
    organization_id <> sqlc.arg('organization_id')::uuid
    OR row_version = sqlc.arg('expected_row_version')::bigint
  );

-- name: CountOrganizationRowVersion :one
SELECT count(*)::bigint
FROM organizations
WHERE organization_id = $1
  AND row_version = $2;

-- name: IncrementOrganizationSubtreeSecurityEpochs :exec
UPDATE organization_security_versions
SET subtree_security_epoch = subtree_security_epoch + 1,
    updated_at = $2
WHERE organization_id IN (
    SELECT ancestor_organization_id
    FROM organization_closure
    WHERE descendant_organization_id = $1
);

-- name: OrganizationDeletionBlocked :one
SELECT (
    EXISTS (
        SELECT 1 FROM organizations
        WHERE parent_organization_id = $1 AND status <> 'DELETED'
    )
    OR EXISTS (
        SELECT 1 FROM user_organizations
        WHERE organization_id = $1 AND status = 'ACTIVE'
    )
    OR EXISTS (
        SELECT 1 FROM spaces
        WHERE organization_id = $1 AND status <> 'DELETED'
    )
    OR EXISTS (
        SELECT 1 FROM admin_delegations
        WHERE organization_id = $1 AND status = 'ACTIVE'
          AND (valid_until IS NULL OR valid_until > $2)
    )
    OR EXISTS (
        SELECT 1
        FROM migration_jobs AS job
        JOIN spaces AS space ON space.space_id = job.target_space_id
        WHERE space.organization_id = $1
          AND job.status IN ('PENDING', 'RUNNING', 'PAUSED')
    )
    OR EXISTS (
        SELECT 1
        FROM legal_holds AS hold
        JOIN documents AS document ON document.document_id = hold.document_id
        JOIN namespace_entries AS entry ON entry.namespace_entry_id = document.document_id
        JOIN spaces AS space ON space.space_id = entry.space_id
        WHERE space.organization_id = $1 AND hold.status = 'ACTIVE'
    )
)::boolean;

-- name: SetOrganizationStatus :one
UPDATE organizations
SET status = sqlc.arg('status')::varchar(32),
    deleted_at = CASE WHEN sqlc.arg('status')::varchar(32) = 'DELETED' THEN sqlc.arg('updated_at')::timestamptz ELSE NULL END,
    updated_at = sqlc.arg('updated_at')::timestamptz,
    row_version = row_version + 1
WHERE organization_id = sqlc.arg('organization_id')::uuid
  AND row_version = sqlc.arg('row_version')::bigint
  AND status <> 'DELETED'
RETURNING *;

-- name: GetMembership :one
SELECT *
FROM user_organizations
WHERE user_organization_id = $1;

-- name: GetMembershipForUpdate :one
SELECT *
FROM user_organizations
WHERE user_organization_id = $1
  AND organization_id = $2
FOR UPDATE;

-- name: CountMemberships :one
SELECT count(*)::bigint
FROM user_organizations
WHERE (sqlc.narg('organization_id')::uuid IS NULL OR organization_id = sqlc.narg('organization_id'))
  AND (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('effective_at')::timestamptz IS NULL OR (
      effective_from <= sqlc.narg('effective_at')
      AND (effective_until IS NULL OR effective_until > sqlc.narg('effective_at'))
  ));

-- name: ListMemberships :many
SELECT *
FROM user_organizations
WHERE (sqlc.narg('organization_id')::uuid IS NULL OR organization_id = sqlc.narg('organization_id'))
  AND (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('effective_at')::timestamptz IS NULL OR (
      effective_from <= sqlc.narg('effective_at')
      AND (effective_until IS NULL OR effective_until > sqlc.narg('effective_at'))
  ))
ORDER BY effective_from DESC, user_organization_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: UserCanJoinOrganization :one
SELECT EXISTS (
    SELECT 1 FROM users
    WHERE user_id = $1 AND status = 'ACTIVE'
) AND EXISTS (
    SELECT 1 FROM organizations
    WHERE organization_id = $2 AND status = 'ACTIVE'
) AS allowed;

-- name: InsertMembership :one
INSERT INTO user_organizations (
    user_organization_id, user_id, organization_id, membership_type, job_title,
    effective_from, effective_until, created_by_user_id, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
RETURNING *;

-- name: UpdateMembership :one
UPDATE user_organizations
SET membership_type = $3,
    job_title = $4,
    status = $5,
    effective_until = $6,
    updated_at = $7,
    row_version = row_version + 1
WHERE user_organization_id = $1
  AND organization_id = $2
  AND row_version = $8
RETURNING *;

-- name: DeactivateMembership :one
UPDATE user_organizations
SET status = 'INACTIVE',
    effective_until = COALESCE(effective_until, $3),
    updated_at = $3,
    row_version = row_version + 1
WHERE user_organization_id = $1
  AND organization_id = $2
  AND row_version = $4
RETURNING *;

-- name: IncrementOrganizationMembershipVersion :exec
UPDATE organization_security_versions
SET membership_version = membership_version + 1,
    subtree_security_epoch = subtree_security_epoch + 1,
    updated_at = $2
WHERE organization_id = $1;

-- name: IncrementUserOrganizationMembershipVersion :exec
UPDATE principal_security_versions
SET organization_membership_version = organization_membership_version + 1,
    updated_at = $2
WHERE user_id = $1;

-- name: GetSpace :one
SELECT *
FROM spaces
WHERE space_id = $1;

-- name: GetSpaceForUpdate :one
SELECT *
FROM spaces
WHERE space_id = $1
FOR UPDATE;

-- name: GetPersonalSpaceByUser :one
SELECT *
FROM spaces
WHERE owner_user_id = $1
  AND space_type = 'PERSONAL'
  AND status <> 'DELETED';

-- name: GetOrganizationSpace :one
SELECT *
FROM spaces
WHERE organization_id = $1
  AND space_type = 'ORGANIZATION'
  AND status <> 'DELETED';

-- name: CountSpaces :one
SELECT count(*)::bigint
FROM spaces
WHERE (sqlc.narg('space_type')::text IS NULL OR space_type = sqlc.narg('space_type'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('organization_id')::uuid IS NULL OR organization_id = sqlc.narg('organization_id'))
  AND (sqlc.narg('owner_user_id')::uuid IS NULL OR owner_user_id = sqlc.narg('owner_user_id'));

-- name: ListSpaces :many
SELECT *
FROM spaces
WHERE (sqlc.narg('space_type')::text IS NULL OR space_type = sqlc.narg('space_type'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('organization_id')::uuid IS NULL OR organization_id = sqlc.narg('organization_id'))
  AND (sqlc.narg('owner_user_id')::uuid IS NULL OR owner_user_id = sqlc.narg('owner_user_id'))
ORDER BY created_at DESC, space_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: UserExistsAndIsActive :one
SELECT EXISTS (
    SELECT 1 FROM users WHERE user_id = $1 AND status = 'ACTIVE'
)::boolean;

-- name: UpdateSpace :one
UPDATE spaces
SET name = $2,
    normalized_name = $3,
    quota_bytes = $4,
    config_schema_version = $5,
    config_json = $6,
    updated_at = $7,
    row_version = row_version + 1
WHERE space_id = $1
  AND row_version = $8
  AND status <> 'DELETED'
  AND used_bytes + reserved_bytes <= $4
RETURNING *;

-- name: SpaceDeletionBlocked :one
SELECT (
    EXISTS (SELECT 1 FROM namespace_entries AS entry WHERE entry.space_id = $1)
    OR EXISTS (SELECT 1 FROM quota_reservations AS reservation WHERE reservation.space_id = $1 AND reservation.status = 'ACTIVE')
    OR EXISTS (SELECT 1 FROM migration_jobs AS job WHERE job.target_space_id = $1 AND job.status IN ('PENDING', 'RUNNING', 'PAUSED'))
    OR EXISTS (SELECT 1 FROM retention_policy_targets AS target WHERE target.space_id = $1)
)::boolean;

-- name: SetSpaceStatus :one
UPDATE spaces
SET status = sqlc.arg('status')::varchar(32),
    deleted_at = CASE WHEN sqlc.arg('status')::varchar(32) = 'DELETED' THEN sqlc.arg('updated_at')::timestamptz ELSE NULL END,
    security_epoch = security_epoch + 1,
    updated_at = sqlc.arg('updated_at')::timestamptz,
    row_version = row_version + 1
WHERE space_id = sqlc.arg('space_id')::uuid
  AND row_version = sqlc.arg('row_version')::bigint
  AND status <> 'DELETED'
RETURNING *;

-- name: ReserveSpaceQuota :one
UPDATE spaces
SET reserved_bytes = reserved_bytes + $2,
    updated_at = $3,
    row_version = row_version + 1
WHERE space_id = $1
  AND status = 'ACTIVE'
  AND used_bytes + reserved_bytes + $2 <= quota_bytes
RETURNING *;

-- name: InsertQuotaReservation :one
INSERT INTO quota_reservations (
    quota_reservation_id, space_id, user_id, reserved_bytes, expires_at,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING *;

-- name: GetQuotaReservationForUpdate :one
SELECT *
FROM quota_reservations
WHERE quota_reservation_id = $1
FOR UPDATE;

-- name: ConsumeSpaceQuotaReservation :execrows
UPDATE spaces
SET reserved_bytes = reserved_bytes - $2,
    used_bytes = used_bytes + $3,
    updated_at = $4,
    row_version = row_version + 1
WHERE space_id = $1
  AND reserved_bytes >= $2
  AND used_bytes + reserved_bytes - $2 + $3 <= quota_bytes;

-- name: ReleaseSpaceQuotaReservation :execrows
UPDATE spaces
SET reserved_bytes = reserved_bytes - $2,
    updated_at = $3,
    row_version = row_version + 1
WHERE space_id = $1
  AND reserved_bytes >= $2;

-- name: MarkQuotaReservationConsumed :one
UPDATE quota_reservations
SET status = 'CONSUMED',
    consumed_at = $2,
    updated_at = $2,
    row_version = row_version + 1
WHERE quota_reservation_id = $1
  AND status = 'ACTIVE'
RETURNING *;

-- name: MarkQuotaReservationReleased :one
UPDATE quota_reservations
SET status = $2,
    released_at = $3,
    updated_at = $3,
    row_version = row_version + 1
WHERE quota_reservation_id = $1
  AND status = 'ACTIVE'
RETURNING *;

-- name: GetOrganizationChangePlan :one
SELECT *
FROM organization_change_plans
WHERE organization_change_plan_id = $1;

-- name: GetOrganizationChangePlanForUpdate :one
SELECT *
FROM organization_change_plans
WHERE organization_change_plan_id = $1
FOR UPDATE;

-- name: CountOrganizationChangePlans :one
SELECT count(*)::bigint
FROM organization_change_plans
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'));

-- name: ListOrganizationChangePlans :many
SELECT *
FROM organization_change_plans
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC, organization_change_plan_id DESC
LIMIT sqlc.arg('page_size')::integer OFFSET sqlc.arg('page_offset')::bigint;

-- name: InsertOrganizationChangePlan :one
INSERT INTO organization_change_plans (
    organization_change_plan_id, plan_type, name, expected_tree_version,
    created_by_user_id, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING *;

-- name: ListOrganizationChangeOperations :many
SELECT *
FROM organization_change_operations
WHERE organization_change_plan_id = $1
ORDER BY sequence_number, organization_change_operation_id;

-- name: InsertOrganizationChangeOperation :one
INSERT INTO organization_change_operations (
    organization_change_operation_id, organization_change_plan_id, sequence_number,
    operation_type, source_organization_id, target_organization_id,
    operation_schema_version, operation_json, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
RETURNING *;

-- name: TouchDraftOrganizationChangePlan :one
UPDATE organization_change_plans
SET updated_at = $2,
    row_version = row_version + 1
WHERE organization_change_plan_id = $1
  AND status = 'DRAFT'
RETURNING *;

-- name: SetOrganizationChangePlanStatus :one
UPDATE organization_change_plans
SET status = sqlc.arg('status')::varchar(32),
    approved_by_user_id = CASE WHEN sqlc.arg('status')::varchar(32) = 'APPROVED' THEN sqlc.narg('approved_by_user_id')::uuid ELSE approved_by_user_id END,
    validated_at = CASE WHEN sqlc.arg('status')::varchar(32) = 'VALIDATED' THEN sqlc.arg('updated_at')::timestamptz ELSE validated_at END,
    approved_at = CASE WHEN sqlc.arg('status')::varchar(32) = 'APPROVED' THEN sqlc.arg('updated_at')::timestamptz ELSE approved_at END,
    started_at = CASE WHEN sqlc.arg('status')::varchar(32) = 'EXECUTING' THEN sqlc.arg('updated_at')::timestamptz ELSE started_at END,
    completed_at = CASE WHEN sqlc.arg('status')::varchar(32) = 'COMPLETED' THEN sqlc.arg('updated_at')::timestamptz ELSE completed_at END,
    failure_code = sqlc.narg('failure_code')::varchar(64),
    updated_at = sqlc.arg('updated_at')::timestamptz,
    row_version = row_version + 1
WHERE organization_change_plan_id = sqlc.arg('organization_change_plan_id')::uuid
  AND row_version = sqlc.arg('row_version')::bigint
RETURNING *;

-- name: MarkOrganizationChangeOperation :one
UPDATE organization_change_operations
SET status = $2,
    completed_at = $3,
    failure_code = $4,
    updated_at = $3,
    row_version = row_version + 1
WHERE organization_change_operation_id = $1
  AND status = 'PENDING'
RETURNING *;

-- name: TryCreateOrganizationIdempotencyRecord :execrows
INSERT INTO idempotency_records (
    idempotency_record_id, principal_kind, user_id, operation, idempotency_key,
    request_hash, status, expires_at, created_at, updated_at
) VALUES ($1, 'USER', $2, $3, $4, $5, 'IN_PROGRESS', $6, $7, $7)
ON CONFLICT DO NOTHING;

-- name: GetOrganizationIdempotencyRecordForUpdate :one
SELECT request_hash, status, result_resource_id
FROM idempotency_records
WHERE principal_kind = 'USER'
  AND user_id = $1
  AND operation = $2
  AND idempotency_key = $3
FOR UPDATE;

-- name: CompleteOrganizationIdempotencyRecord :exec
UPDATE idempotency_records
SET status = 'COMPLETED',
    response_status_code = 201,
    response_schema_version = 1,
    response_json = jsonb_build_object('resourceId', $4::uuid),
    result_resource_type = $5,
    result_resource_id = $4,
    completed_at = $6,
    updated_at = $6,
    row_version = row_version + 1
WHERE user_id = $1
  AND operation = $2
  AND idempotency_key = $3
  AND status = 'IN_PROGRESS';

-- name: InsertOrganizationOutboxEvent :exec
INSERT INTO outbox_events (
    outbox_event_id, aggregate_type, aggregate_id, aggregate_version, event_type,
    event_schema_version, payload_json, deduplication_key, correlation_id,
    max_attempts, available_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, 1, $6, $7, $8, 10, $9, $9, $9);
