package domain

import (
	"slices"
	"strings"
	"time"
)

var shareActions = map[string]struct{}{
	ActionReadMetadata: {}, ActionPreview: {}, ActionDownload: {}, ActionWriteContent: {},
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
	if pageSize < 1 || pageSize > MaxPageSize {
		return 0, 0, &ValidationError{Field: "pageSize"}
	}
	return page, pageSize, nil
}

func NormalizeActions(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > len(shareActions) {
		return nil, &ValidationError{Field: "actions"}
	}
	unique := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := shareActions[value]; !ok {
			return nil, &ValidationError{Field: "actions"}
		}
		if _, ok := seen[value]; ok {
			return nil, &ValidationError{Field: "actions"}
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	if !slices.Contains(unique, ActionReadMetadata) {
		return nil, &ValidationError{Field: "actions"}
	}
	slices.Sort(unique)
	return unique, nil
}

func ValidateSourceType(value string) error {
	switch value {
	case ResourceDocument, ResourceFolder:
		return nil
	default:
		return &ValidationError{Field: "sourceType"}
	}
}

func ValidateTarget(input CreateInput) error {
	switch input.TargetKind {
	case TargetUser:
		if input.TargetUserID == nil || input.TargetOrganizationID != nil {
			return &ValidationError{Field: "targetUserId"}
		}
	case TargetOrganization:
		if input.TargetOrganizationID == nil || input.TargetUserID != nil {
			return &ValidationError{Field: "targetOrganizationId"}
		}
	case TargetLink:
		if input.TargetUserID != nil || input.TargetOrganizationID != nil {
			return &ValidationError{Field: "targetKind"}
		}
	case TargetSpace:
		return ErrTargetUnsupported
	default:
		return &ValidationError{Field: "targetKind"}
	}
	return nil
}

func ValidatePeriod(validUntil *time.Time, now time.Time) error {
	if validUntil != nil && !validUntil.After(now) {
		return &ValidationError{Field: "validUntil"}
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
