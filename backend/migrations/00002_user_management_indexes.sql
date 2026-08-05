-- +goose Up
SET search_path = file_workshop, public;

CREATE INDEX ix_users_created_desc
    ON users (created_at DESC, user_id DESC);
CREATE INDEX ix_users_status_role_created
    ON users (status, system_role, created_at DESC, user_id DESC);

-- +goose Down
SET search_path = file_workshop, public;

DROP INDEX IF EXISTS ix_users_status_role_created;
DROP INDEX IF EXISTS ix_users_created_desc;
