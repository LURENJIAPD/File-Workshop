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

func TestPasswordPolicyChecksFiveMostRecentHashes(t *testing.T) {
	policy := NewPasswordPolicy()
	hasher := historyHasher{matchingHash: "hash-5"}
	if err := policy.ValidateHistory("new password", []string{"hash-1", "hash-2", "hash-3", "hash-4", "hash-5", "hash-6"}, hasher); !errors.Is(err, ErrPasswordRecentlyUsed) {
		t.Fatalf("ValidateHistory() error = %v", err)
	}
	hasher.matchingHash = "hash-6"
	if err := policy.ValidateHistory("new password", []string{"hash-1", "hash-2", "hash-3", "hash-4", "hash-5", "hash-6"}, hasher); err != nil {
		t.Fatalf("ValidateHistory() checked more than five hashes: %v", err)
	}
}

type historyHasher struct {
	matchingHash string
}

func (historyHasher) Hash(string) (string, error) { return "", nil }
func (h historyHasher) Compare(_ string, encodedHash string) (bool, error) {
	return encodedHash == h.matchingHash, nil
}
