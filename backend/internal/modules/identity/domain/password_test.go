package domain

import "testing"

func TestArgon2IDHasherRoundTrip(t *testing.T) {
	hasher := NewArgon2IDHasher()
	hash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	matched, err := hasher.Compare("correct horse battery staple", hash)
	if err != nil || !matched {
		t.Fatalf("Compare(correct) = %v, %v", matched, err)
	}
	matched, err = hasher.Compare("wrong password", hash)
	if err != nil || matched {
		t.Fatalf("Compare(wrong) = %v, %v", matched, err)
	}
}
