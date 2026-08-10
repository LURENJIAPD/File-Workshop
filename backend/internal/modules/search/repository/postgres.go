package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/search/domain"
	"file-workshop/backend/internal/platform/database/dbgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgreSQL struct {
	queries *dbgen.Queries
}

func NewPostgreSQL(pool *pgxpool.Pool) *PostgreSQL {
	return &PostgreSQL{queries: dbgen.New(pool)}
}

func (r *PostgreSQL) SearchEntries(ctx context.Context, filter domain.Filter) ([]domain.Result, error) {
	rows, err := r.queries.SearchDirectoryEntries(ctx, &dbgen.SearchDirectoryEntriesParams{
		SpaceID:         optionalUUID(filter.SpaceID),
		EntryType:       optionalText(filter.EntryType),
		Query:           optionalText(filter.Query),
		Extension:       optionalText(filter.Extension),
		Classification:  optionalText(filter.Classification),
		CreatedByUserID: optionalUUID(filter.CreatedByUserID),
		UpdatedFrom:     optionalTime(filter.UpdatedFrom),
		UpdatedTo:       optionalTime(filter.UpdatedTo),
		MetadataKey:     optionalText(filter.MetadataKey),
		MetadataValue:   optionalText(filter.MetadataValue),
		PageOffset:      pageOffset(filter.Page, filter.PageSize),
		PageSize:        int32(filter.PageSize),
	})
	if err != nil {
		return nil, err
	}
	results := make([]domain.Result, 0, len(rows))
	for _, row := range rows {
		entry, err := entryFromSearch(row)
		if err != nil {
			return nil, err
		}
		results = append(results, domain.Result{Entry: entry, IndexStatus: optionalString(row.IndexStatus), Source: domain.SourcePostgresMetadata})
	}
	return results, nil
}

func entryFromSearch(row *dbgen.SearchDirectoryEntriesRow) (domain.Entry, error) {
	return entry(row.NamespaceEntryID, row.SpaceID, row.ParentFolderID, row.EntryType, row.Name, row.NormalizedName, row.PathCache, row.Depth, row.LifecycleStatus, row.CreatedByUserID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.RowVersion, row.IsRoot, row.FolderInheritanceMode, row.FolderAclVersion, row.OwnerUserID, row.CurrentVersionID, row.AvailabilityStatus, row.ExtensionNormalized, row.DocumentInheritanceMode, row.DocumentAclVersion, row.Classification, row.MetadataSchemaVersion, row.MetadataJson)
}

func entry(idValue, spaceIDValue, parentIDValue pgtype.UUID, entryType, name, normalized string, path pgtype.Text, depth int32, lifecycle string, creatorValue pgtype.UUID, createdAt, updatedAt pgtype.Timestamptz, deletedAt pgtype.Timestamptz, rowVersion int64, isRoot bool, folderMode pgtype.Text, folderACL pgtype.Int8, ownerValue, currentVersionValue pgtype.UUID, availability, extension, documentMode pgtype.Text, documentACL pgtype.Int8, classification pgtype.Text, metadataVersion pgtype.Int4, metadata json.RawMessage) (domain.Entry, error) {
	id, err := googleUUID(idValue)
	if err != nil {
		return domain.Entry{}, err
	}
	creator, err := googleUUID(creatorValue)
	if err != nil {
		return domain.Entry{}, err
	}
	result := domain.Entry{ID: id, SpaceID: uuidValue(spaceIDValue), ParentFolderID: optionalGoogleUUID(parentIDValue), EntryType: entryType, Name: name, NormalizedName: normalized, PathCache: optionalString(path), Depth: depth, LifecycleStatus: lifecycle, CreatedByUserID: creator, CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(), DeletedAt: optionalTimestamptz(deletedAt), RowVersion: rowVersion, IsRoot: isRoot}
	if entryType == domain.EntryTypeFolder {
		result.InheritanceMode = optionalString(folderMode)
		if folderACL.Valid {
			value := folderACL.Int64
			result.ACLVersion = &value
		}
	}
	if entryType == domain.EntryTypeDocument {
		result.OwnerUserID = optionalGoogleUUID(ownerValue)
		result.CurrentVersionID = optionalGoogleUUID(currentVersionValue)
		result.AvailabilityStatus = optionalString(availability)
		result.ExtensionNormalized = optionalString(extension)
		result.InheritanceMode = optionalString(documentMode)
		if documentACL.Valid {
			value := documentACL.Int64
			result.ACLVersion = &value
		}
		result.Classification = optionalString(classification)
		if metadataVersion.Valid {
			value := metadataVersion.Int32
			result.MetadataSchemaVersion = &value
		}
		if len(metadata) > 0 {
			result.MetadataJSON = append([]byte(nil), metadata...)
		}
	}
	return result, nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func uuidValue(value pgtype.UUID) uuid.UUID {
	return uuid.UUID(value.Bytes)
}

func googleUUID(value pgtype.UUID) (uuid.UUID, error) {
	if !value.Valid {
		return uuid.Nil, fmt.Errorf("uuid is null")
	}
	return uuid.UUID(value.Bytes), nil
}

func optionalGoogleUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func optionalUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgUUID(*value)
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func optionalTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func optionalTimestamptz(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func pageOffset(page, pageSize int) int64 {
	return int64(page-1) * int64(pageSize)
}
