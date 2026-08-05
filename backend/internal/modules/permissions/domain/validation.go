package domain

import (
	"slices"
	"strings"
	"time"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 50
	MaxPageSize     = 200
)

func NormalizePage(page, pageSize int) (int, int, error) {
	if page < 1 {
		return 0, 0, &ValidationError{Field: "page"}
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		return 0, 0, &ValidationError{Field: "pageSize"}
	}
	return page, pageSize, nil
}

func ValidateDelegation(value NewAdminDelegation) error {
	if value.UserID.String() == "00000000-0000-0000-0000-000000000000" {
		return &ValidationError{Field: "userId"}
	}
	if value.OrganizationID.String() == "00000000-0000-0000-0000-000000000000" {
		return &ValidationError{Field: "organizationId"}
	}
	if value.Scope != ScopeSelf && value.Scope != ScopeSubtree {
		return &ValidationError{Field: "scope"}
	}
	if value.ValidUntil != nil && !value.ValidUntil.After(value.ValidFrom) {
		return &ValidationError{Field: "validUntil"}
	}
	if len(value.Capabilities) == 0 {
		return &ValidationError{Field: "capabilities"}
	}
	seen := map[string]struct{}{}
	for _, capability := range value.Capabilities {
		if _, ok := Capabilities[capability]; !ok {
			return &ValidationError{Field: "capabilities"}
		}
		if _, ok := seen[capability]; ok {
			return &ValidationError{Field: "capabilities"}
		}
		seen[capability] = struct{}{}
	}
	if value.CanDelegate && !slices.Contains(value.Capabilities, CapabilityDelegateAdmin) {
		return &ValidationError{Field: "canDelegate"}
	}
	return nil
}

func ValidateGrant(value NewPermissionGrant) error {
	if (value.SubjectUserID == nil) == (value.SubjectOrganizationID == nil) {
		return &ValidationError{Field: "subject"}
	}
	resourceCount := 0
	if value.SpaceID != nil {
		resourceCount++
	}
	if value.FolderID != nil {
		resourceCount++
	}
	if value.DocumentID != nil {
		resourceCount++
	}
	if resourceCount != 1 {
		return &ValidationError{Field: "resource"}
	}
	if value.DocumentID != nil && value.InheritToDescendants {
		return &ValidationError{Field: "inheritToDescendants"}
	}
	if value.GrantSource != "MANUAL" && value.GrantSource != "TEMPLATE" && value.GrantSource != "MIGRATION" && value.GrantSource != "SYSTEM" {
		return &ValidationError{Field: "grantSource"}
	}
	if value.ValidUntil != nil && !value.ValidUntil.After(value.ValidFrom) {
		return &ValidationError{Field: "validUntil"}
	}
	return ValidateActions(value.Actions)
}

func ValidateActions(actions []string) error {
	if len(actions) == 0 {
		return &ValidationError{Field: "actions"}
	}
	seen := map[string]struct{}{}
	for _, action := range actions {
		if _, ok := Actions[action]; !ok {
			return &ValidationError{Field: "actions"}
		}
		if _, ok := seen[action]; ok {
			return &ValidationError{Field: "actions"}
		}
		seen[action] = struct{}{}
	}
	return nil
}

func TrimmedOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func Effective(status string, from time.Time, until *time.Time, now time.Time) bool {
	return status == StatusActive && !from.After(now) && (until == nil || until.After(now))
}
