package application

import (
	"context"
	"errors"
	"testing"

	"file-workshop/backend/internal/modules/users/domain"

	"github.com/google/uuid"
)

func TestApplyChangesClearsOptionalFields(t *testing.T) {
	t.Parallel()
	employeeNo, email, phone := "E-100", "user@example.com", "13800000000"
	current := domain.User{ID: uuid.New(), DisplayName: "Old", EmployeeNo: &employeeNo, Email: &email, Phone: &phone, SystemRole: domain.SystemRoleUser, Status: domain.UserStatusActive, Locale: "zh-CN", Timezone: "Asia/Shanghai"}
	empty := ""
	displayName := "New"
	updated, err := applyChanges(current, domain.UserChanges{EmployeeNo: &empty, Email: &empty, Phone: &empty, DisplayName: &displayName}, true)
	if err != nil {
		t.Fatalf("applyChanges() error = %v", err)
	}
	if updated.EmployeeNo != nil || updated.Email != nil || updated.Phone != nil || updated.DisplayName != "New" {
		t.Fatalf("applyChanges() did not clear optional fields: %#v", updated)
	}
}

func TestRequireAdmin(t *testing.T) {
	t.Parallel()
	if err := requireAdmin(Actor{Role: domain.SystemRoleAdmin}); err != nil {
		t.Fatalf("requireAdmin(admin) error = %v", err)
	}
	if err := requireAdmin(Actor{Role: domain.SystemRoleUser}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("requireAdmin(user) error = %v", err)
	}
}

func TestStatusEventUsesFrozenCatalog(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		domain.UserStatusActive:   "USER_ENABLED",
		domain.UserStatusDisabled: "USER_DISABLED",
		domain.UserStatusLocked:   "AUTH_ACCOUNT_LOCKED",
		domain.UserStatusDeleted:  "USER_UPDATED",
	}
	for status, expected := range tests {
		if actual := statusEvent(status); actual != expected {
			t.Fatalf("statusEvent(%q) = %q, want %q", status, actual, expected)
		}
	}
}

func TestEnsureAnotherActiveAdmin(t *testing.T) {
	t.Parallel()
	if err := ensureAnotherActiveAdmin(context.Background(), adminCountStub{count: 1}); !errors.Is(err, domain.ErrLastSystemAdmin) {
		t.Fatalf("ensureAnotherActiveAdmin(last) error = %v", err)
	}
	if err := ensureAnotherActiveAdmin(context.Background(), adminCountStub{count: 2}); err != nil {
		t.Fatalf("ensureAnotherActiveAdmin(two) error = %v", err)
	}
	expected := errors.New("database unavailable")
	if err := ensureAnotherActiveAdmin(context.Background(), adminCountStub{err: expected}); !errors.Is(err, expected) {
		t.Fatalf("ensureAnotherActiveAdmin(error) = %v", err)
	}
}

type adminCountStub struct {
	count int
	err   error
}

func (stub adminCountStub) CountActiveSystemAdminsForUpdate(context.Context) (int, error) {
	return stub.count, stub.err
}
