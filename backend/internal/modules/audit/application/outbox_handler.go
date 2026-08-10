package application

import (
	"context"
	"encoding/json"
	"strings"

	"file-workshop/backend/internal/modules/audit/domain"
	backgrounddomain "file-workshop/backend/internal/modules/background/domain"

	"github.com/google/uuid"
)

func (s *Service) EventTypes() []string {
	return []string{
		"USER_CREATED", "USER_UPDATED", "USER_ROLE_CHANGED", "USER_ENABLED", "USER_DISABLED", "AUTH_ACCOUNT_LOCKED", "AUTH_PASSWORD_CHANGED", "AUTH_SESSION_REVOKED",
		"ORGANIZATION_CREATED", "ORGANIZATION_UPDATED", "ORGANIZATION_MOVED", "ORGANIZATION_ARCHIVED", "ORGANIZATION_MEMBER_ADDED", "ORGANIZATION_MEMBER_REMOVED",
		"ADMIN_DELEGATION_CREATED", "ADMIN_DELEGATION_INVALIDATED", "ADMIN_DELEGATION_REVOKED", "PERMISSION_GRANT_CREATED", "PERMISSION_GRANT_UPDATED", "PERMISSION_GRANT_REVOKED", "PERMISSION_INHERITANCE_BROKEN", "PERMISSION_INHERITANCE_RESTORED",
		"FOLDER_CREATED", "DOCUMENT_CREATED", "ENTRY_RENAMED", "ENTRY_MOVED",
	}
}

func (s *Service) HandleOutboxEvent(ctx context.Context, event backgrounddomain.OutboxEvent) error {
	auditEvent, err := s.auditEventFromOutbox(event)
	if err != nil {
		return backgrounddomain.PermanentError("AUDIT_EVENT_INVALID", err.Error())
	}
	if err = s.repository.InsertEvent(ctx, auditEvent); err != nil {
		return backgrounddomain.RetryableError("AUDIT_EVENT_WRITE_FAILED", err.Error())
	}
	return nil
}

func (s *Service) auditEventFromOutbox(event backgrounddomain.OutboxEvent) (domain.Event, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return domain.Event{}, err
	}
	now := event.CreatedAt.UTC()
	if now.IsZero() {
		now = s.now().UTC()
	}
	metadata, err := auditMetadata(event)
	if err != nil {
		return domain.Event{}, err
	}
	requestID := event.ID
	if event.CorrelationID != nil && *event.CorrelationID != uuid.Nil {
		requestID = *event.CorrelationID
	}
	resourceType := strings.TrimSpace(event.AggregateType)
	resourceID := event.AggregateID
	result := domain.Event{
		ID:                    id,
		EventType:             event.EventType,
		RiskLevel:             riskLevel(event.EventType),
		ActorType:             domain.ActorTypeSystem,
		ResourceType:          stringPtr(resourceType),
		ResourceID:            &resourceID,
		Action:                event.EventType,
		Result:                domain.ResultSuccess,
		SourceChannel:         domain.SourceSystem,
		RequestID:             requestID,
		CorrelationID:         event.CorrelationID,
		MetadataSchemaVersion: 1,
		MetadataJSON:          metadata,
		PartitionDate:         dateOnly(now),
		CreatedAt:             now,
	}
	payload := map[string]any{}
	_ = json.Unmarshal(event.PayloadJSON, &payload)
	if actorID := uuidFromPayload(payload, "actorUserId"); actorID != nil {
		result.ActorType = domain.ActorTypeUser
		result.ActorID = actorID
	}
	if reason := stringFromPayload(payload, "reason"); reason != nil {
		result.Reason = reason
	}
	if value := uuidFromPayload(payload, "spaceId"); value != nil {
		result.SpaceID = value
	}
	if value := uuidFromPayload(payload, "organizationId"); value != nil {
		result.OrganizationID = value
	}
	if value := uuidFromPayload(payload, "documentId"); value != nil {
		result.DocumentID = value
	}
	if value := uuidFromPayload(payload, "documentVersionId"); value != nil {
		result.DocumentVersionID = value
	}
	return result, nil
}

func auditMetadata(event backgrounddomain.OutboxEvent) ([]byte, error) {
	payload := json.RawMessage(event.PayloadJSON)
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	return json.Marshal(map[string]any{
		"outboxEventId":      event.ID.String(),
		"aggregateType":      event.AggregateType,
		"aggregateId":        event.AggregateID.String(),
		"aggregateVersion":   event.AggregateVersion,
		"eventSchemaVersion": event.EventSchemaVersion,
		"deduplicationKey":   event.DeduplicationKey,
		"sourcePayload":      payload,
		"requestIdFallback":  event.CorrelationID == nil,
		"mappedBy":           "audit.outbox_handler",
	})
}

func riskLevel(eventType string) string {
	switch eventType {
	case "USER_ROLE_CHANGED", "AUTH_PASSWORD_CHANGED",
		"ADMIN_DELEGATION_CREATED", "ADMIN_DELEGATION_INVALIDATED", "ADMIN_DELEGATION_REVOKED",
		"PERMISSION_GRANT_CREATED", "PERMISSION_GRANT_UPDATED", "PERMISSION_GRANT_REVOKED",
		"PERMISSION_INHERITANCE_BROKEN", "PERMISSION_INHERITANCE_RESTORED":
		return domain.RiskHigh
	case "AUTH_ACCOUNT_LOCKED":
		return domain.RiskCritical
	default:
		return domain.RiskNormal
	}
}

func uuidFromPayload(payload map[string]any, key string) *uuid.UUID {
	value, ok := payload[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func stringFromPayload(payload map[string]any, key string) *string {
	value, ok := payload[key].(string)
	if !ok {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

var _ interface {
	EventTypes() []string
	HandleOutboxEvent(context.Context, backgrounddomain.OutboxEvent) error
} = (*Service)(nil)
