package domain

import (
	"errors"
	"testing"
)

func TestUserStateTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current string
		target  string
		allowed bool
	}{
		{name: "disable active", current: UserStatusActive, target: UserStatusDisabled, allowed: true},
		{name: "lock active", current: UserStatusActive, target: UserStatusLocked, allowed: true},
		{name: "enable disabled", current: UserStatusDisabled, target: UserStatusActive, allowed: true},
		{name: "delete locked", current: UserStatusLocked, target: UserStatusDeleted, allowed: true},
		{name: "repeat state", current: UserStatusDisabled, target: UserStatusDisabled, allowed: true},
		{name: "deleted is terminal", current: UserStatusDeleted, target: UserStatusActive, allowed: false},
		{name: "unknown target", current: UserStatusActive, target: "ARCHIVED", allowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if actual := CanTransition(test.current, test.target); actual != test.allowed {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", test.current, test.target, actual, test.allowed)
			}
		})
	}
}

func TestNormalizePage(t *testing.T) {
	t.Parallel()
	page, pageSize, err := NormalizePage(0, 0)
	if err != nil || page != 1 || pageSize != 50 {
		t.Fatalf("NormalizePage defaults = (%d, %d, %v)", page, pageSize, err)
	}
	for _, input := range [][2]int{{-1, 50}, {1, -1}, {1, 201}} {
		if _, _, err := NormalizePage(input[0], input[1]); !errors.Is(err, ErrInvalidUserInput) {
			t.Fatalf("NormalizePage(%d, %d) error = %v", input[0], input[1], err)
		}
	}
}

func TestPrepareEmailAndProfile(t *testing.T) {
	t.Parallel()
	email, normalized, err := PrepareEmail("User.Name@Example.COM")
	if err != nil || email == nil || normalized == nil || *normalized != "user.name@example.com" {
		t.Fatalf("PrepareEmail() = (%v, %v, %v)", email, normalized, err)
	}
	if _, _, err := PrepareEmail("Display Name <user@example.com>"); !errors.Is(err, ErrInvalidUserInput) {
		t.Fatalf("PrepareEmail(display form) error = %v", err)
	}
	phone := "13800000000"
	if err := ValidateProfile("测试用户", "zh-CN", "Asia/Shanghai", &phone); err != nil {
		t.Fatalf("ValidateProfile() error = %v", err)
	}
	if err := ValidateProfile("测试用户", "zh-CN", "Not/A-Timezone", &phone); !errors.Is(err, ErrInvalidUserInput) {
		t.Fatalf("ValidateProfile(invalid timezone) error = %v", err)
	}
}
