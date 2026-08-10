package domain

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
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

func ValidateEntryType(value string) error {
	switch value {
	case EntryTypeFolder, EntryTypeDocument:
		return nil
	default:
		return &ValidationError{Field: "entryType"}
	}
}

func ValidateLifecycleStatus(value string) error {
	switch value {
	case LifecycleActive, LifecycleTrashed, LifecycleArchived, LifecyclePurging, LifecyclePurged:
		return nil
	default:
		return &ValidationError{Field: "lifecycleStatus"}
	}
}

func NormalizeName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func ValidateEntryName(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "." || trimmed == ".." || len(trimmed) > 512 || !utf8.ValidString(trimmed) {
		return &ValidationError{Field: "name"}
	}
	for _, r := range trimmed {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return &ValidationError{Field: "name"}
		}
	}
	return nil
}

func ValidateClassification(value *string) error {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" || len(trimmed) > 64 {
		return &ValidationError{Field: "classification"}
	}
	return nil
}

func ValidateMetadata(value json.RawMessage) error {
	if len(value) == 0 {
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal(value, &object); err != nil {
		return &ValidationError{Field: "metadata"}
	}
	if object == nil {
		return &ValidationError{Field: "metadata"}
	}
	return nil
}

func ExtensionNormalized(name string) *string {
	trimmed := strings.TrimSpace(name)
	index := strings.LastIndex(trimmed, ".")
	if index <= 0 || index == len(trimmed)-1 {
		return nil
	}
	value := strings.ToLower(trimmed[index+1:])
	if value == "" || len(value) > 64 {
		return nil
	}
	return &value
}
