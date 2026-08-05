package domain

import (
	"math"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

func NormalizeIdentifier(value string) string {
	return cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
}

func NormalizeOptional(value string) (*string, *string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	normalized := NormalizeIdentifier(trimmed)
	return &trimmed, &normalized
}

func PrepareEmail(value string) (*string, *string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil, nil
	}
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed.Address != trimmed {
		return nil, nil, &ValidationError{Field: "email"}
	}
	normalized := NormalizeIdentifier(trimmed)
	return &trimmed, &normalized, nil
}

func ValidateProfile(displayName, locale, timezone string, phone *string) error {
	if strings.TrimSpace(displayName) == "" || len([]rune(displayName)) > 128 {
		return &ValidationError{Field: "displayName"}
	}
	if strings.TrimSpace(locale) == "" || len(locale) > 16 {
		return &ValidationError{Field: "locale"}
	}
	if strings.TrimSpace(timezone) == "" || len(timezone) > 64 {
		return &ValidationError{Field: "timezone"}
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return &ValidationError{Field: "timezone"}
	}
	if phone != nil && (strings.TrimSpace(*phone) == "" || len([]rune(*phone)) > 64) {
		return &ValidationError{Field: "phone"}
	}
	return nil
}

func ValidateRole(role string) error {
	if role != SystemRoleUser && role != SystemRoleAdmin {
		return &ValidationError{Field: "systemRole"}
	}
	return nil
}

func ValidateStatus(status string) error {
	switch status {
	case UserStatusActive, UserStatusDisabled, UserStatusLocked, UserStatusDeleted:
		return nil
	default:
		return &ValidationError{Field: "status"}
	}
}

func CanTransition(current, target string) bool {
	if current == target {
		return true
	}
	if current == UserStatusDeleted {
		return false
	}
	switch target {
	case UserStatusActive, UserStatusDisabled, UserStatusLocked, UserStatusDeleted:
		return true
	default:
		return false
	}
}

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
	if pageSize < 1 || pageSize > MaximumPageSize {
		return 0, 0, &ValidationError{Field: "pageSize"}
	}
	if int64(page-1) > math.MaxInt64/int64(pageSize) {
		return 0, 0, &ValidationError{Field: "page"}
	}
	return page, pageSize, nil
}
