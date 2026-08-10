package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

func NormalizePage(page, pageSize int) (int, int, error) {
	if page == 0 {
		page = DefaultPage
	}
	if pageSize == 0 {
		pageSize = DefaultPageSize
	}
	if page < 1 {
		return 0, 0, &ValidationError{Field: "page"}
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		return 0, 0, &ValidationError{Field: "pageSize"}
	}
	return page, pageSize, nil
}

func NormalizeFilter(input Filter) (Filter, error) {
	page, pageSize, err := NormalizePage(input.Page, input.PageSize)
	if err != nil {
		return Filter{}, err
	}
	input.Page, input.PageSize = page, pageSize
	input.Query = normalizeOptional(input.Query, "query", 128)
	input.Extension = normalizeOptional(input.Extension, "extension", 64)
	input.Classification = normalizeOptional(input.Classification, "classification", 64)
	input.MetadataKey = normalizeOptional(input.MetadataKey, "metadataKey", 128)
	input.MetadataValue = normalizeOptional(input.MetadataValue, "metadataValue", 256)
	if input.Query == invalidString || input.Extension == invalidString || input.Classification == invalidString || input.MetadataKey == invalidString || input.MetadataValue == invalidString {
		return Filter{}, ErrInvalidInput
	}
	if input.EntryType != nil {
		value := strings.TrimSpace(*input.EntryType)
		switch value {
		case EntryTypeFolder, EntryTypeDocument:
			input.EntryType = &value
		default:
			return Filter{}, &ValidationError{Field: "entryType"}
		}
	}
	if (input.MetadataKey == nil) != (input.MetadataValue == nil) {
		return Filter{}, &ValidationError{Field: "metadata"}
	}
	if input.UpdatedFrom != nil && input.UpdatedTo != nil && input.UpdatedFrom.After(*input.UpdatedTo) {
		return Filter{}, &ValidationError{Field: "updatedAt"}
	}
	if !hasAnyCondition(input) {
		return Filter{}, &ValidationError{Field: "query"}
	}
	input.UpdatedFrom = utcOptional(input.UpdatedFrom)
	input.UpdatedTo = utcOptional(input.UpdatedTo)
	return input, nil
}

var invalidString = new(string)

func normalizeOptional(value *string, field string, maxLength int) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" || len(trimmed) > maxLength || !utf8.ValidString(trimmed) {
		return invalidString
	}
	normalized := strings.ToLower(trimmed)
	if field == "metadataKey" {
		normalized = trimmed
	}
	return &normalized
}

func utcOptional(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func hasAnyCondition(value Filter) bool {
	return value.Query != nil || value.SpaceID != nil || value.EntryType != nil || value.Extension != nil ||
		value.Classification != nil || value.CreatedByUserID != nil || value.UpdatedFrom != nil ||
		value.UpdatedTo != nil || value.MetadataKey != nil
}

func NormalizeIndexRefreshInput(documentIDs []uuid.UUID, reason string) ([]uuid.UUID, string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 256 || len(documentIDs) < 1 || len(documentIDs) > MaxBatchSize {
		return nil, "", ErrInvalidInput
	}
	seen := make(map[uuid.UUID]struct{}, len(documentIDs))
	result := make([]uuid.UUID, 0, len(documentIDs))
	for _, documentID := range documentIDs {
		if documentID == uuid.Nil {
			return nil, "", ErrInvalidInput
		}
		if _, ok := seen[documentID]; ok {
			return nil, "", ErrInvalidInput
		}
		seen[documentID] = struct{}{}
		result = append(result, documentID)
	}
	return result, reason, nil
}
