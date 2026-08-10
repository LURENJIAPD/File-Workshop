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

	JobStatusPending    = "PENDING"
	JobStatusProcessing = "PROCESSING"
	JobStatusSuccess    = "SUCCESS"
	JobStatusFailed     = "FAILED"
	JobStatusDead       = "DEAD"
	JobStatusCancelled  = "CANCELLED"
	JobStatusSkipped    = "SKIPPED"

	SystemRoleAdmin = "SYSTEM_ADMIN"

	DefaultPage     = 1
	DefaultPageSize = 50
	MaxPageSize     = 200
	MaxBatchSize    = 50
)

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

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

type AdministrationSummary struct {
	OutboxEvents   []OutboxStatusCount
	BackgroundJobs []OutboxStatusCount
}

type BackgroundJob struct {
	ID                      uuid.UUID
	JobType                 string
	TargetDocumentID        *uuid.UUID
	TargetDocumentVersionID *uuid.UUID
	TargetStorageObjectID   *uuid.UUID
	PayloadSchemaVersion    int32
	PayloadJSON             json.RawMessage
	DeduplicationKey        string
	Priority                int32
	Status                  string
	AttemptCount            int32
	MaxAttempts             int32
	AvailableAt             time.Time
	LockedBy                *string
	LockedAt                *time.Time
	LeaseUntil              *time.Time
	HeartbeatAt             *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
	StartedAt               *time.Time
	CompletedAt             *time.Time
	LastErrorCode           *string
	LastErrorSummary        *string
	RowVersion              int64
}

type OutboxListFilter struct {
	Status    *string
	EventType *string
	Page      int
	PageSize  int
}

type OutboxListResult struct {
	Items    []OutboxEvent
	Page     int
	PageSize int
	Total    int64
}

type JobListFilter struct {
	Status   *string
	JobType  *string
	Page     int
	PageSize int
}

type JobListResult struct {
	Items    []BackgroundJob
	Page     int
	PageSize int
	Total    int64
}

type BatchJobItem struct {
	ID         uuid.UUID
	RowVersion int64
}

type BatchJobOperationResultItem struct {
	ID           uuid.UUID
	Success      bool
	Job          *BackgroundJob
	ErrorCode    *string
	ErrorMessage *string
}

type BatchJobOperationResult struct {
	Items     []BatchJobOperationResultItem
	Succeeded int
	Failed    int
}

type EnqueueJobInput struct {
	ID                      uuid.UUID
	JobType                 string
	TargetDocumentID        *uuid.UUID
	TargetDocumentVersionID *uuid.UUID
	TargetStorageObjectID   *uuid.UUID
	PayloadSchemaVersion    int32
	PayloadJSON             json.RawMessage
	DeduplicationKey        string
	Priority                int32
	MaxAttempts             int32
	AvailableAt             time.Time
}
