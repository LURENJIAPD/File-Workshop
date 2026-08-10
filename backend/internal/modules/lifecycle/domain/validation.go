package domain

import (
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

func ValidateReason(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > 512 {
		return &ValidationError{Field: "reason"}
	}
	return nil
}
