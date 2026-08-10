package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func ValidateIntent(value string) error {
	switch value {
	case IntentCreate, IntentNewVersion:
		return nil
	default:
		return &ValidationError{Field: "uploadIntent"}
	}
}

func ValidateFileName(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "." || trimmed == ".." || len(trimmed) > 512 || !utf8.ValidString(trimmed) {
		return &ValidationError{Field: "fileName"}
	}
	for _, r := range trimmed {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return &ValidationError{Field: "fileName"}
		}
	}
	return nil
}

func NormalizeName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func ValidatePartSize(value int64) error {
	if value < MinPartSizeBytes {
		return &ValidationError{Field: "partSizeBytes"}
	}
	return nil
}

func ValidateDeclaredSize(value int64) error {
	if value < 1 {
		return &ValidationError{Field: "declaredSizeBytes"}
	}
	return nil
}

func ValidateOptionalText(field string, value *string, max int) error {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" || len(trimmed) > max || !utf8.ValidString(trimmed) {
		return &ValidationError{Field: field}
	}
	*value = trimmed
	return nil
}

func ExpectedPartCount(sizeBytes, partSizeBytes int64) (int32, error) {
	if err := ValidateDeclaredSize(sizeBytes); err != nil {
		return 0, err
	}
	if err := ValidatePartSize(partSizeBytes); err != nil {
		return 0, err
	}
	count := (sizeBytes + partSizeBytes - 1) / partSizeBytes
	if count < 1 || count > MaxPartCount {
		return 0, &ValidationError{Field: "expectedPartCount"}
	}
	return int32(count), nil
}
