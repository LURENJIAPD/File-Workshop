package application

import (
	"testing"

	"file-workshop/backend/internal/modules/permissions/domain"

	"github.com/google/uuid"
)

func TestGrantAppliesHonorsBreakInheritance(t *testing.T) {
	spaceID, parentID, targetID := uuid.New(), uuid.New(), uuid.New()
	grant := domain.PermissionGrant{SpaceID: &spaceID, InheritToDescendants: true}
	resource := domain.Resource{Type: domain.ResourceDocument, ID: targetID, SpaceID: spaceID, InheritanceMode: domain.InheritanceInherit, FolderAncestors: []domain.FolderAncestor{{ID: parentID, Distance: 1, InheritanceMode: domain.InheritanceBreak}}}
	if grantApplies(grant, resource) {
		t.Fatal("space grant crossed a BREAK folder")
	}

	folderGrant := domain.PermissionGrant{FolderID: &parentID, InheritToDescendants: true}
	if !grantApplies(folderGrant, resource) {
		t.Fatal("direct ACL on BREAK folder did not inherit to its child")
	}

	resource.InheritanceMode = domain.InheritanceBreak
	direct := domain.PermissionGrant{DocumentID: &targetID}
	if !grantApplies(direct, resource) {
		t.Fatal("direct grant was blocked by target BREAK mode")
	}
	if grantApplies(folderGrant, resource) {
		t.Fatal("ancestor grant crossed target BREAK mode")
	}
}
