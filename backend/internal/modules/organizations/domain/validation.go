package domain

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func Normalize(value string) string {
	return strings.ToLower(norm.NFKC.String(strings.TrimSpace(value)))
}

func NormalizeOptional(value string) (*string, *string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	normalized := Normalize(trimmed)
	return &trimmed, &normalized
}

func ValidateOrganizationName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > 256 {
		return &ValidationError{Field: "name"}
	}
	return nil
}

func ValidateOrganizationStatus(status string) error {
	switch status {
	case OrganizationStatusActive, OrganizationStatusDisabled, OrganizationStatusArchived, OrganizationStatusDeleted:
		return nil
	default:
		return &ValidationError{Field: "status"}
	}
}

func ValidateMembershipType(value string) error {
	if value != MembershipTypePrimary && value != MembershipTypeMember {
		return &ValidationError{Field: "membershipType"}
	}
	return nil
}

func ValidateMembershipStatus(value string) error {
	if value != MembershipStatusActive && value != MembershipStatusInactive {
		return &ValidationError{Field: "status"}
	}
	return nil
}

func ValidateSpaceType(value string) error {
	switch value {
	case SpaceTypePersonal, SpaceTypeOrganization, SpaceTypePublic:
		return nil
	default:
		return &ValidationError{Field: "spaceType"}
	}
}

func ValidateSpaceStatus(value string) error {
	switch value {
	case SpaceStatusActive, SpaceStatusFrozen, SpaceStatusArchived, SpaceStatusDeleted:
		return nil
	default:
		return &ValidationError{Field: "status"}
	}
}

func ValidatePlanType(value string) error {
	switch value {
	case PlanTypeMove, PlanTypeMerge, PlanTypeSplit, PlanTypeBulkRestructure:
		return nil
	default:
		return &ValidationError{Field: "planType"}
	}
}

func ValidatePlanStatus(value string) error {
	switch value {
	case PlanStatusDraft, PlanStatusValidated, PlanStatusApproved, PlanStatusExecuting, PlanStatusCompleted, PlanStatusCancelled, PlanStatusFailed:
		return nil
	default:
		return &ValidationError{Field: "status"}
	}
}

func ValidateOperationType(value string) error {
	switch value {
	case OperationTypeMoveNode, OperationTypeMergeNode, OperationTypeCreateNode, OperationTypeMoveMember, OperationTypeMoveSpaceContent:
		return nil
	default:
		return &ValidationError{Field: "operationType"}
	}
}

func ValidateJSONObject(value json.RawMessage) error {
	if len(value) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	var object map[string]any
	if err := decoder.Decode(&object); err != nil || object == nil {
		return &ValidationError{Field: "config"}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return &ValidationError{Field: "config"}
	}
	return nil
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
	return page, pageSize, nil
}
