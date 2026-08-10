package domain

import "strings"

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

func ValidateLockSource(value string) error {
	switch value {
	case LockSourceWeb, LockSourceWebDAV, LockSourceOffice, LockSourceAgent:
		return nil
	default:
		return &ValidationError{Field: "source"}
	}
}

func ValidateChangeNote(value *string) error {
	return validateOptionalText("changeNote", value, 2000)
}

func ValidateReason(value *string) error {
	return validateOptionalText("reason", value, 512)
}

func validateOptionalText(field string, value *string, max int) error {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" || len(trimmed) > max {
		return &ValidationError{Field: field}
	}
	*value = trimmed
	return nil
}
