package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/organizations/domain"

	"github.com/google/uuid"
)

func insertOrganizationEvent(ctx context.Context, repository Repository, organization domain.Organization, actorID, requestID uuid.UUID, eventType string, extra map[string]any, now time.Time) error {
	payload := map[string]any{
		"organizationId": organization.ID.String(),
		"actorUserId":    actorID.String(),
		"status":         organization.Status,
		"treeVersion":    organization.TreeVersion,
		"rowVersion":     organization.RowVersion,
	}
	for key, value := range extra {
		payload[key] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode organization event: %w", err)
	}
	eventID, err := newUUID("organization event")
	if err != nil {
		return err
	}
	deduplicationKey := fmt.Sprintf("organization:%s:%d:%s", organization.ID, organization.RowVersion, eventType)
	return repository.InsertEvent(ctx, domain.Event{ID: eventID, AggregateType: "ORGANIZATION", AggregateID: organization.ID, AggregateVersion: organization.RowVersion, Type: eventType, Payload: encoded, DeduplicationKey: deduplicationKey, CorrelationID: requestID, CreatedAt: now})
}
