package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	SystemRoleAdmin = "SYSTEM_ADMIN"

	DefaultPage     = 1
	DefaultPageSize = 50
	MaxPageSize     = 200

	RiskNormal   = "NORMAL"
	RiskHigh     = "HIGH"
	RiskCritical = "CRITICAL"

	ActorTypeUser      = "USER"
	ActorTypeSystem    = "SYSTEM"
	ActorTypeAgent     = "AGENT"
	ActorTypeMigration = "MIGRATION"
	ActorTypeService   = "SERVICE"

	ResultSuccess = "SUCCESS"
	ResultFailure = "FAILURE"
	ResultDenied  = "DENIED"

	SourceAPI    = "API"
	SourceSystem = "SYSTEM"

	ChainStatusActive  = "ACTIVE"
	ChainStatusSealed  = "SEALED"
	ChainStatusInvalid = "INVALID"

	HashSchemaVersion = int32(1)
)

var ZeroHash = make([]byte, sha256.Size)

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

type Event struct {
	ID                    uuid.UUID
	EventType             string
	RiskLevel             string
	ActorType             string
	ActorID               *uuid.UUID
	ActorDisplayName      *string
	ActorEmployeeNo       *string
	EffectiveRole         *string
	AdminDelegationID     *uuid.UUID
	ShareID               *uuid.UUID
	ResourceType          *string
	ResourceID            *uuid.UUID
	ResourceName          *string
	SpaceID               *uuid.UUID
	OrganizationID        *uuid.UUID
	DocumentID            *uuid.UUID
	DocumentVersionID     *uuid.UUID
	Action                string
	Result                string
	FailureCode           *string
	SourceChannel         string
	IPAddress             *string
	UserAgent             *string
	RequestID             uuid.UUID
	TraceID               *string
	CorrelationID         *uuid.UUID
	Reason                *string
	MetadataSchemaVersion int32
	MetadataJSON          json.RawMessage
	HashSchemaVersion     *int32
	ChainID               *string
	SequenceNumber        *int64
	PreviousHash          []byte
	EventHash             []byte
	PartitionDate         time.Time
	CreatedAt             time.Time
}

type ChainHead struct {
	ChainID            string
	PartitionDate      time.Time
	LastSequenceNumber int64
	LastEventID        uuid.UUID
	LastHash           []byte
	BatchRoot          []byte
	AnchorLocation     *string
	Status             string
	VerifiedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	RowVersion         int64
}

type EventListFilter struct {
	DateFrom     time.Time
	DateTo       time.Time
	EventType    *string
	RiskLevel    *string
	ActorType    *string
	ActorID      *uuid.UUID
	ResourceType *string
	ResourceID   *uuid.UUID
	Result       *string
	RequestID    *uuid.UUID
	Page         int
	PageSize     int
}

type EventListResult struct {
	Items    []Event
	Page     int
	PageSize int
	Total    int64
}

type IntegrityFilter struct {
	DateFrom time.Time
	DateTo   time.Time
	Status   *string
	Page     int
	PageSize int
}

type IntegrityResult struct {
	Items    []ChainHead
	Page     int
	PageSize int
	Total    int64
}

type CountByValue struct {
	Value string
	Count int64
}

type SummaryFilter struct {
	DateFrom time.Time
	DateTo   time.Time
}

type Summary struct {
	DateFrom          time.Time
	DateTo            time.Time
	TotalEvents       int64
	RiskLevelCounts   []CountByValue
	ResultCounts      []CountByValue
	ActorTypeCounts   []CountByValue
	ChainStatusCounts []CountByValue
}

type VerificationResult struct {
	ChainID       string
	PartitionDate time.Time
	CheckedEvents int
	Verified      bool
	FailureReason *string
}

func RequiresChain(riskLevel string) bool {
	return riskLevel == RiskHigh || riskLevel == RiskCritical
}

func ComputeHash(event Event) ([]byte, error) {
	payload := struct {
		Schema        int32           `json:"schema"`
		ID            string          `json:"id"`
		EventType     string          `json:"eventType"`
		RiskLevel     string          `json:"riskLevel"`
		ActorType     string          `json:"actorType"`
		ActorID       string          `json:"actorId,omitempty"`
		ResourceType  string          `json:"resourceType,omitempty"`
		ResourceID    string          `json:"resourceId,omitempty"`
		Action        string          `json:"action"`
		Result        string          `json:"result"`
		RequestID     string          `json:"requestId"`
		Metadata      json.RawMessage `json:"metadata"`
		ChainID       string          `json:"chainId"`
		Sequence      int64           `json:"sequenceNumber"`
		PreviousHash  string          `json:"previousHash"`
		PartitionDate string          `json:"partitionDate"`
		CreatedAt     string          `json:"createdAt"`
	}{
		Schema:        HashSchemaVersion,
		ID:            event.ID.String(),
		EventType:     event.EventType,
		RiskLevel:     event.RiskLevel,
		ActorType:     event.ActorType,
		Action:        event.Action,
		Result:        event.Result,
		RequestID:     event.RequestID.String(),
		Metadata:      event.MetadataJSON,
		PreviousHash:  hex.EncodeToString(event.PreviousHash),
		PartitionDate: event.PartitionDate.UTC().Format(time.DateOnly),
		CreatedAt:     event.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if event.ActorID != nil {
		payload.ActorID = event.ActorID.String()
	}
	if event.ResourceType != nil {
		payload.ResourceType = *event.ResourceType
	}
	if event.ResourceID != nil {
		payload.ResourceID = event.ResourceID.String()
	}
	if event.ChainID != nil {
		payload.ChainID = *event.ChainID
	}
	if event.SequenceNumber != nil {
		payload.Sequence = *event.SequenceNumber
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	return sum[:], nil
}
