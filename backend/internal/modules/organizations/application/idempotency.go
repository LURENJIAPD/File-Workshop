package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

func requestHash(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode idempotent request: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return hash[:], nil
}

func validateIdempotencyKey(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > 128 {
		return &domain.ValidationError{Field: "Idempotency-Key"}
	}
	return nil
}

func claimIdempotency(ctx context.Context, repository Repository, actorID uuid.UUID, operation, key string, hash []byte, now time.Time) (*uuid.UUID, error) {
	recordID, err := newUUID("idempotency")
	if err != nil {
		return nil, err
	}
	created, err := repository.TryCreateIdempotency(ctx, recordID, actorID, operation, key, hash, now.Add(idempotencyTTL), now)
	if err != nil {
		return nil, fmt.Errorf("claim %s idempotency key: %w", operation, err)
	}
	if created {
		return nil, nil
	}
	record, err := repository.GetIdempotencyForUpdate(ctx, actorID, operation, key)
	if err != nil {
		return nil, fmt.Errorf("read %s idempotency key: %w", operation, err)
	}
	if !bytes.Equal(record.RequestHash, hash) {
		return nil, domain.ErrIdempotencyConflict
	}
	if record.Status != "COMPLETED" || record.ResultResourceID == nil {
		return nil, domain.ErrConflict
	}
	return record.ResultResourceID, nil
}
