-- name: GetDatabaseHealth :one
SELECT current_database()::text AS database_name,
       current_user::text AS database_user,
       current_schema()::text AS current_schema,
       current_setting('server_version')::text AS server_version,
       current_setting('TimeZone')::text AS timezone;
