package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	OutboxStatusPending    = "PENDING"
	OutboxStatusProcessing = "PROCESSING"
	OutboxStatusPublished  = "PUBLISHED"
	OutboxStatusFailed     = "FAILED"
	OutboxStatusDead       = "DEAD"
)

type OutboxEvent struct {
	ID                 uuid.UUID
	AggregateType      string
	AggregateID        uuid.UUID
	AggregateVersion   int64
	EventType          string
	EventSchemaVersion int32
	PayloadJSON        json.RawMessage
	DeduplicationKey   string
	CorrelationID      *uuid.UUID
	CausationID        *uuid.UUID
	Priority           int32
	Status             string
	AttemptCount       int32
	MaxAttempts        int32
	AvailableAt        time.Time
	LockedBy           *string
	LockedAt           *time.Time
	LeaseUntil         *time.Time
	NextRetryAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	PublishedAt        *time.Time
	LastErrorCode      *string
	LastErrorSummary   *string
	RowVersion         int64
}

type OutboxStatusCount struct {
	Status string
	Count  int64
}
