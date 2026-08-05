package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordPolicy(t *testing.T) {
	policy := NewPasswordPolicy()
	tests := []struct {
		name     string
		password string
		username string
		wantErr  error
	}{
		{name: "valid passphrase", password: "maple-river-workshop-2026", username: "root"},
		{name: "short", password: "short", username: "root", wantErr: ErrPasswordTooShort},
		{name: "long", password: strings.Repeat("界", 129), username: "root", wantErr: ErrPasswordTooLong},
		{name: "same as normalized username", password: "ＲＯＯＴ-account", username: "root-account", wantErr: ErrPasswordMatchesUser},
		{name: "common password", password: "password1234", username: "root", wantErr: ErrPasswordCommon},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := policy.Validate(test.password, test.username)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}
