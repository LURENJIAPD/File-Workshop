package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNormalizeUsesUnicodeCompatibilityAndCaseFolding(t *testing.T) {
	if got := Normalize("  ＦＡＣＴＯＲＹ  "); got != "factory" {
		t.Fatalf("Normalize() = %q", got)
	}
}

func TestValidateJSONObjectRejectsNonObjectAndTrailingJSON(t *testing.T) {
	for _, value := range []json.RawMessage{
		json.RawMessage(`[]`),
		json.RawMessage(`null`),
		json.RawMessage(`{"valid":true} {"trailing":true}`),
	} {
		if !errors.Is(ValidateJSONObject(value), ErrInvalidInput) {
			t.Fatalf("ValidateJSONObject(%s) must reject invalid object", value)
		}
	}
	if err := ValidateJSONObject(json.RawMessage(`{"valid":true}`)); err != nil {
		t.Fatalf("valid object rejected: %v", err)
	}
}

func TestNormalizePageRejectsInvalidValues(t *testing.T) {
	for _, test := range []struct{ page, pageSize int }{{-1, 50}, {1, -1}, {1, 201}} {
		_, _, err := NormalizePage(test.page, test.pageSize)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizePage(%d, %d) error = %v", test.page, test.pageSize, err)
		}
	}
}

func TestValidateEnums(t *testing.T) {
	if err := ValidateOrganizationStatus(OrganizationStatusActive); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(ValidateSpaceStatus("UNKNOWN"), ErrInvalidInput) {
		t.Fatal("unknown space status must be rejected")
	}
}
