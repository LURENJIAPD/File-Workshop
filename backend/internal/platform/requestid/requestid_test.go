package requestid

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestResolvePreservesValidRequestID(t *testing.T) {
	const requestID = "upstream-Request_123.abc"
	if actual := Resolve("  " + requestID + "  "); actual != requestID {
		t.Fatalf("Resolve() = %q, want %q", actual, requestID)
	}
}

func TestResolveReplacesInvalidRequestIDWithUUIDv7(t *testing.T) {
	actual := Resolve("invalid request id with spaces")
	id, err := uuid.Parse(actual)
	if err != nil {
		t.Fatalf("Resolve() returned invalid UUID: %q", actual)
	}
	if id.Version() != 7 {
		t.Fatalf("Resolve() UUID version = %d, want 7", id.Version())
	}
}

func TestContextRoundTrip(t *testing.T) {
	ctx := WithContext(context.Background(), "request-123")
	if actual := FromContext(ctx); actual != "request-123" {
		t.Fatalf("FromContext() = %q", actual)
	}
}
