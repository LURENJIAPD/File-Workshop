package domain

import "testing"

func TestValidateEntryNameRejectsUnsafeNames(t *testing.T) {
	for _, value := range []string{"", ".", "..", "a/b", "a\\b", "bad\nname"} {
		t.Run(value, func(t *testing.T) {
			if err := ValidateEntryName(value); err == nil {
				t.Fatalf("ValidateEntryName(%q) succeeded, want error", value)
			}
		})
	}
}

func TestExtensionNormalized(t *testing.T) {
	value := ExtensionNormalized(" Drawing.PDF ")
	if value == nil || *value != "pdf" {
		t.Fatalf("extension = %#v, want pdf", value)
	}
	if got := ExtensionNormalized(".gitignore"); got != nil {
		t.Fatalf("extension = %#v, want nil", got)
	}
}
