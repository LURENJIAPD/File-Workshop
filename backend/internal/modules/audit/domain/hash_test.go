package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestComputeHashIsStableAndSensitive(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	requestID := uuid.New()
	resourceType := "USER"
	resourceID := uuid.New()
	chainID := "fw-audit:20260810:USER_ROLE_CHANGED"
	sequence := int64(1)
	event := Event{
		ID:                    id,
		EventType:             "USER_ROLE_CHANGED",
		RiskLevel:             RiskHigh,
		ActorType:             ActorTypeUser,
		ResourceType:          &resourceType,
		ResourceID:            &resourceID,
		Action:                "USER_ROLE_CHANGED",
		Result:                ResultSuccess,
		RequestID:             requestID,
		MetadataSchemaVersion: 1,
		MetadataJSON:          json.RawMessage(`{"rowVersion":3}`),
		ChainID:               &chainID,
		SequenceNumber:        &sequence,
		PreviousHash:          ZeroHash,
		PartitionDate:         time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		CreatedAt:             time.Date(2026, 8, 10, 9, 30, 0, 0, time.UTC),
	}
	first, err := ComputeHash(event)
	if err != nil {
		t.Fatalf("ComputeHash() error = %v", err)
	}
	second, err := ComputeHash(event)
	if err != nil {
		t.Fatalf("ComputeHash() second error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("ComputeHash() is not stable for identical event")
	}
	event.Result = ResultDenied
	tampered, err := ComputeHash(event)
	if err != nil {
		t.Fatalf("ComputeHash() tampered error = %v", err)
	}
	if string(first) == string(tampered) {
		t.Fatal("ComputeHash() did not change after event mutation")
	}
}
