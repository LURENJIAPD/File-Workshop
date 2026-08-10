package objectstorage

import (
	"context"
	"errors"
	"testing"
)

func TestDisabledClientReturnsDisabled(t *testing.T) {
	t.Parallel()
	client := NewDisabledClient()
	if !errors.Is(client.Check(context.Background()), ErrDisabled) {
		t.Fatal("Check() must return ErrDisabled")
	}
	if _, err := client.CreateMultipartUpload(context.Background(), CreateMultipartUploadInput{}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("CreateMultipartUpload() error = %v, want ErrDisabled", err)
	}
	if _, err := client.PresignGetObject(context.Background(), PresignGetObjectInput{}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("PresignGetObject() error = %v, want ErrDisabled", err)
	}
}

func TestValidatePresignedURL(t *testing.T) {
	t.Parallel()
	if err := ValidatePresignedURL("https://object.example.test/bucket/key?signature=1"); err != nil {
		t.Fatalf("valid URL rejected: %v", err)
	}
	if err := ValidatePresignedURL("file:///tmp/key"); err == nil {
		t.Fatal("file URL must be rejected")
	}
}
