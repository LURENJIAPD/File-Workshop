package domain

import "testing"

func TestNormalizeUsernameUsesNFKCAndUnicodeCaseFolding(t *testing.T) {
	if actual := NormalizeUsername("  ＲＯＯＴ  "); actual != "root" {
		t.Fatalf("NormalizeUsername() = %q, want root", actual)
	}
	if actual := NormalizeUsername("Straße"); actual != "strasse" {
		t.Fatalf("NormalizeUsername() = %q, want strasse", actual)
	}
}
