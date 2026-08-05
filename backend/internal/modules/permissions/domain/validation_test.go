package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateDelegationRequiresDelegateCapability(t *testing.T) {
	value := NewAdminDelegation{UserID: uuid.New(), OrganizationID: uuid.New(), Scope: ScopeSubtree, CanDelegate: true, Capabilities: []string{CapabilityManageContent}, ValidFrom: time.Now()}
	if err := ValidateDelegation(value); err == nil {
		t.Fatal("delegation with canDelegate but without DELEGATE_ADMIN was accepted")
	}
	value.Capabilities = append(value.Capabilities, CapabilityDelegateAdmin)
	if err := ValidateDelegation(value); err != nil {
		t.Fatalf("valid delegation rejected: %v", err)
	}
}

func TestValidateGrantRejectsDocumentInheritanceAndDuplicateActions(t *testing.T) {
	userID, documentID := uuid.New(), uuid.New()
	value := NewPermissionGrant{SubjectUserID: &userID, DocumentID: &documentID, Actions: []string{"DOWNLOAD"}, InheritToDescendants: true, GrantSource: "MANUAL", ValidFrom: time.Now()}
	if err := ValidateGrant(value); err == nil {
		t.Fatal("document descendant inheritance was accepted")
	}
	value.InheritToDescendants = false
	value.Actions = []string{"DOWNLOAD", "DOWNLOAD"}
	if err := ValidateGrant(value); err == nil {
		t.Fatal("duplicate actions were accepted")
	}
}

func TestNormalizePageRejectsAliasesByBoundary(t *testing.T) {
	if _, _, err := NormalizePage(1, 201); err == nil {
		t.Fatal("pageSize over 200 was accepted")
	}
	if page, pageSize, err := NormalizePage(1, 50); err != nil || page != 1 || pageSize != 50 {
		t.Fatalf("valid pagination rejected: %d %d %v", page, pageSize, err)
	}
}
