package application

import (
	"context"
	"testing"

	"file-workshop/backend/internal/modules/files/domain"
	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"

	"github.com/google/uuid"
)

type denyAuthorizer struct{}

func (denyAuthorizer) EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error) {
	return permissiondomain.PermissionEvaluation{Allowed: false}, nil
}

func TestRequirePermissionDeniesWhenAuthorizationDenies(t *testing.T) {
	service := NewService(nil, nil, denyAuthorizer{}, nil)
	err := service.requirePermission(context.Background(), domain.Actor{UserID: uuid.New()}, permissiondomain.ResourceSpace, uuid.New(), "LIST")
	if err != domain.ErrForbidden {
		t.Fatalf("err = %v, want forbidden", err)
	}
}
