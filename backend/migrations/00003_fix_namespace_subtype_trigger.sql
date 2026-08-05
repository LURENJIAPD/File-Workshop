-- +goose Up
SET search_path TO file_workshop, public;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_namespace_subtype()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = file_workshop, pg_temp
AS $function$
DECLARE
    actual_type varchar(32);
    subtype_id uuid;
BEGIN
    IF TG_TABLE_NAME = 'folders' THEN
        subtype_id := NEW.folder_id;
    ELSIF TG_TABLE_NAME = 'documents' THEN
        subtype_id := NEW.document_id;
    ELSIF TG_TABLE_NAME = 'shared_entries' THEN
        subtype_id := NEW.shared_entry_id;
    ELSE
        RAISE EXCEPTION '不支持的命名空间子类型表: %', TG_TABLE_NAME USING ERRCODE = '23514';
    END IF;

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
-- +goose StatementEnd

-- +goose Down
SET search_path TO file_workshop, public;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_namespace_subtype()
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
-- +goose StatementEnd
