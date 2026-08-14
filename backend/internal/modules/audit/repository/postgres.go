package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/audit/domain"
	"file-workshop/backend/internal/platform/database/dbgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgreSQL struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewPostgreSQL(pool *pgxpool.Pool) *PostgreSQL {
	return &PostgreSQL{pool: pool, queries: dbgen.New(pool)}
}

func (r *PostgreSQL) ListEvents(ctx context.Context, filter domain.EventListFilter) (domain.EventListResult, error) {
	params := eventFilterParams(filter)
	total, err := r.queries.CountAuditEvents(ctx, &dbgen.CountAuditEventsParams{
		DateFrom:     params.DateFrom,
		DateTo:       params.DateTo,
		EventType:    params.EventType,
		RiskLevel:    params.RiskLevel,
		ActorType:    params.ActorType,
		ActorID:      params.ActorID,
		ResourceType: params.ResourceType,
		ResourceID:   params.ResourceID,
		Result:       params.Result,
		RequestID:    params.RequestID,
	})
	if err != nil {
		return domain.EventListResult{}, err
	}
	rows, err := r.queries.ListAuditEvents(ctx, params)
	if err != nil {
		return domain.EventListResult{}, err
	}
	items := make([]domain.Event, 0, len(rows))
	for _, row := range rows {
		event, err := eventFromListRow(row)
		if err != nil {
			return domain.EventListResult{}, err
		}
		items = append(items, event)
	}
	return domain.EventListResult{Items: items, Page: filter.Page, PageSize: filter.PageSize, Total: total}, nil
}

func (r *PostgreSQL) GetEvent(ctx context.Context, id uuid.UUID, partitionDate time.Time) (domain.Event, error) {
	row, err := r.queries.GetAuditEvent(ctx, &dbgen.GetAuditEventParams{Column1: pgDate(partitionDate), Column2: pgUUID(id)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Event{}, domain.ErrNotFound
		}
		return domain.Event{}, err
	}
	return eventFromGetRow(row)
}

func (r *PostgreSQL) GetSummary(ctx context.Context, filter domain.SummaryFilter) (domain.Summary, error) {
	dateFrom := pgDate(filter.DateFrom)
	dateTo := pgDate(filter.DateTo)
	total, err := r.queries.CountAuditEvents(ctx, &dbgen.CountAuditEventsParams{DateFrom: dateFrom, DateTo: dateTo})
	if err != nil {
		return domain.Summary{}, err
	}
	riskRows, err := r.queries.CountAuditEventsByRiskLevel(ctx, &dbgen.CountAuditEventsByRiskLevelParams{DateFrom: dateFrom, DateTo: dateTo})
	if err != nil {
		return domain.Summary{}, err
	}
	resultRows, err := r.queries.CountAuditEventsByResult(ctx, &dbgen.CountAuditEventsByResultParams{DateFrom: dateFrom, DateTo: dateTo})
	if err != nil {
		return domain.Summary{}, err
	}
	actorRows, err := r.queries.CountAuditEventsByActorType(ctx, &dbgen.CountAuditEventsByActorTypeParams{DateFrom: dateFrom, DateTo: dateTo})
	if err != nil {
		return domain.Summary{}, err
	}
	chainRows, err := r.queries.CountAuditChainHeadsByStatus(ctx, &dbgen.CountAuditChainHeadsByStatusParams{DateFrom: dateFrom, DateTo: dateTo})
	if err != nil {
		return domain.Summary{}, err
	}
	return domain.Summary{
		DateFrom:          filter.DateFrom,
		DateTo:            filter.DateTo,
		TotalEvents:       total,
		RiskLevelCounts:   riskLevelCounts(riskRows),
		ResultCounts:      resultCounts(resultRows),
		ActorTypeCounts:   actorTypeCounts(actorRows),
		ChainStatusCounts: chainStatusCounts(chainRows),
	}, nil
}

func (r *PostgreSQL) ListChainHeads(ctx context.Context, filter domain.IntegrityFilter) (domain.IntegrityResult, error) {
	status := nullableText(filter.Status)
	total, err := r.queries.CountAuditChainHeads(ctx, &dbgen.CountAuditChainHeadsParams{DateFrom: pgDate(filter.DateFrom), DateTo: pgDate(filter.DateTo), Status: status})
	if err != nil {
		return domain.IntegrityResult{}, err
	}
	rows, err := r.queries.ListAuditChainHeads(ctx, &dbgen.ListAuditChainHeadsParams{DateFrom: pgDate(filter.DateFrom), DateTo: pgDate(filter.DateTo), Status: status, PageOffset: int64((filter.Page - 1) * filter.PageSize), PageSize: int32(filter.PageSize)})
	if err != nil {
		return domain.IntegrityResult{}, err
	}
	items := make([]domain.ChainHead, 0, len(rows))
	for _, row := range rows {
		items = append(items, chainHead(row))
	}
	return domain.IntegrityResult{Items: items, Page: filter.Page, PageSize: filter.PageSize, Total: total}, nil
}

func (r *PostgreSQL) InsertEvent(ctx context.Context, event domain.Event) error {
	if len(event.MetadataJSON) == 0 {
		event.MetadataJSON = json.RawMessage("{}")
	}
	if !domain.RequiresChain(event.RiskLevel) {
		return r.queries.InsertAuditEvent(ctx, insertParams(event))
	}
	return r.insertChainedEvent(ctx, event)
}

func (r *PostgreSQL) VerifyChain(ctx context.Context, chainID string, partitionDate time.Time, now time.Time) (domain.VerificationResult, error) {
	rows, err := r.queries.ListAuditChainEventsForVerify(ctx, &dbgen.ListAuditChainEventsForVerifyParams{Column1: pgDate(partitionDate), ChainID: pgtype.Text{String: chainID, Valid: true}})
	if err != nil {
		return domain.VerificationResult{}, err
	}
	if len(rows) == 0 {
		return domain.VerificationResult{}, domain.ErrNotFound
	}
	previous := append([]byte(nil), domain.ZeroHash...)
	for index, row := range rows {
		event, err := eventFromVerifyRow(row)
		if err != nil {
			return domain.VerificationResult{}, err
		}
		if event.SequenceNumber == nil || *event.SequenceNumber != int64(index+1) {
			return r.markInvalid(ctx, chainID, partitionDate, now, len(rows), "sequence number mismatch")
		}
		if string(event.PreviousHash) != string(previous) {
			return r.markInvalid(ctx, chainID, partitionDate, now, len(rows), "previous hash mismatch")
		}
		expected, err := domain.ComputeHash(event)
		if err != nil {
			return domain.VerificationResult{}, err
		}
		if string(expected) != string(event.EventHash) {
			return r.markInvalid(ctx, chainID, partitionDate, now, len(rows), "event hash mismatch")
		}
		previous = expected
	}
	rowsAffected, err := r.queries.MarkAuditChainVerified(ctx, &dbgen.MarkAuditChainVerifiedParams{ChainID: chainID, Column2: pgDate(partitionDate), VerifiedAt: pgTimestamptz(now)})
	if err != nil {
		return domain.VerificationResult{}, err
	}
	if rowsAffected != 1 {
		return domain.VerificationResult{}, domain.ErrNotFound
	}
	return domain.VerificationResult{ChainID: chainID, PartitionDate: partitionDate, CheckedEvents: len(rows), Verified: true}, nil
}

func (r *PostgreSQL) insertChainedEvent(ctx context.Context, event domain.Event) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)
	chainID := fmt.Sprintf("fw-audit:%s:%s", event.PartitionDate.UTC().Format("20060102"), safeChainSegment(event.EventType))
	event.ChainID = &chainID
	version := domain.HashSchemaVersion
	event.HashSchemaVersion = &version
	head, err := queries.GetAuditChainHeadForUpdate(ctx, &dbgen.GetAuditChainHeadForUpdateParams{ChainID: chainID, Column2: pgDate(event.PartitionDate)})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	sequence := int64(1)
	previous := append([]byte(nil), domain.ZeroHash...)
	if err == nil {
		if head.Status != domain.ChainStatusActive {
			return domain.ErrConflict
		}
		sequence = head.LastSequenceNumber + 1
		previous = append([]byte(nil), head.LastHash...)
	}
	event.SequenceNumber = &sequence
	event.PreviousHash = previous
	hash, err := domain.ComputeHash(event)
	if err != nil {
		return err
	}
	event.EventHash = hash
	if err = queries.InsertAuditEvent(ctx, insertParams(event)); err != nil {
		return err
	}
	if sequence == 1 {
		err = queries.InsertAuditChainHead(ctx, &dbgen.InsertAuditChainHeadParams{ChainID: chainID, Column2: pgDate(event.PartitionDate), LastSequenceNumber: sequence, Column4: pgUUID(event.ID), LastHash: hash, CreatedAt: pgTimestamptz(event.CreatedAt)})
	} else {
		var rowsAffected int64
		rowsAffected, err = queries.UpdateAuditChainHead(ctx, &dbgen.UpdateAuditChainHeadParams{ChainID: chainID, Column2: pgDate(event.PartitionDate), LastSequenceNumber: sequence, Column4: pgUUID(event.ID), LastHash: hash, UpdatedAt: pgTimestamptz(event.CreatedAt)})
		if err == nil && rowsAffected != 1 {
			err = domain.ErrConflict
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgreSQL) markInvalid(ctx context.Context, chainID string, partitionDate, now time.Time, checked int, reason string) (domain.VerificationResult, error) {
	rowsAffected, err := r.queries.MarkAuditChainInvalid(ctx, &dbgen.MarkAuditChainInvalidParams{ChainID: chainID, Column2: pgDate(partitionDate), VerifiedAt: pgTimestamptz(now)})
	if err != nil {
		return domain.VerificationResult{}, err
	}
	if rowsAffected != 1 {
		return domain.VerificationResult{}, domain.ErrNotFound
	}
	return domain.VerificationResult{ChainID: chainID, PartitionDate: partitionDate, CheckedEvents: checked, Verified: false, FailureReason: &reason}, nil
}

func eventFilterParams(filter domain.EventListFilter) *dbgen.ListAuditEventsParams {
	return &dbgen.ListAuditEventsParams{
		DateFrom:     pgDate(filter.DateFrom),
		DateTo:       pgDate(filter.DateTo),
		EventType:    nullableText(filter.EventType),
		RiskLevel:    nullableText(filter.RiskLevel),
		ActorType:    nullableText(filter.ActorType),
		ActorID:      optionalUUID(filter.ActorID),
		ResourceType: nullableText(filter.ResourceType),
		ResourceID:   optionalUUID(filter.ResourceID),
		Result:       nullableText(filter.Result),
		RequestID:    optionalUUID(filter.RequestID),
		PageOffset:   int64((filter.Page - 1) * filter.PageSize),
		PageSize:     int32(filter.PageSize),
	}
}

func riskLevelCounts(rows []*dbgen.CountAuditEventsByRiskLevelRow) []domain.CountByValue {
	result := make([]domain.CountByValue, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.CountByValue{Value: row.RiskLevel, Count: row.EventCount})
	}
	return result
}

func resultCounts(rows []*dbgen.CountAuditEventsByResultRow) []domain.CountByValue {
	result := make([]domain.CountByValue, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.CountByValue{Value: row.Result, Count: row.EventCount})
	}
	return result
}

func actorTypeCounts(rows []*dbgen.CountAuditEventsByActorTypeRow) []domain.CountByValue {
	result := make([]domain.CountByValue, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.CountByValue{Value: row.ActorType, Count: row.EventCount})
	}
	return result
}

func chainStatusCounts(rows []*dbgen.CountAuditChainHeadsByStatusRow) []domain.CountByValue {
	result := make([]domain.CountByValue, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.CountByValue{Value: row.Status, Count: row.ChainCount})
	}
	return result
}

func insertParams(event domain.Event) *dbgen.InsertAuditEventParams {
	return &dbgen.InsertAuditEventParams{
		Column1:               pgUUID(event.ID),
		EventType:             event.EventType,
		RiskLevel:             event.RiskLevel,
		ActorType:             event.ActorType,
		Column5:               optionalUUID(event.ActorID),
		ActorDisplayName:      nullableText(event.ActorDisplayName),
		ActorEmployeeNo:       nullableText(event.ActorEmployeeNo),
		EffectiveRole:         nullableText(event.EffectiveRole),
		Column9:               optionalUUID(event.AdminDelegationID),
		Column10:              optionalUUID(event.ShareID),
		ResourceType:          nullableText(event.ResourceType),
		Column12:              optionalUUID(event.ResourceID),
		ResourceName:          nullableText(event.ResourceName),
		Column14:              optionalUUID(event.SpaceID),
		Column15:              optionalUUID(event.OrganizationID),
		Column16:              optionalUUID(event.DocumentID),
		Column17:              optionalUUID(event.DocumentVersionID),
		Action:                event.Action,
		Result:                event.Result,
		FailureCode:           nullableText(event.FailureCode),
		SourceChannel:         event.SourceChannel,
		Column22:              pgUUID(event.RequestID),
		TraceID:               nullableText(event.TraceID),
		Column24:              optionalUUID(event.CorrelationID),
		Reason:                nullableText(event.Reason),
		MetadataSchemaVersion: event.MetadataSchemaVersion,
		Column27:              event.MetadataJSON,
		HashSchemaVersion:     optionalInt32(event.HashSchemaVersion),
		ChainID:               nullableText(event.ChainID),
		SequenceNumber:        optionalInt64(event.SequenceNumber),
		PreviousHash:          event.PreviousHash,
		EventHash:             event.EventHash,
		PartitionDate:         pgDate(event.PartitionDate),
		CreatedAt:             pgTimestamptz(event.CreatedAt),
	}
}

func eventFromListRow(row *dbgen.ListAuditEventsRow) (domain.Event, error) {
	return eventFromValues(row.AuditEventID, row.EventType, row.RiskLevel, row.ActorType, row.ActorID, row.ActorDisplayName, row.ActorEmployeeNo, row.EffectiveRole, row.AdminDelegationID, row.ShareID, row.ResourceType, row.ResourceID, row.ResourceName, row.SpaceID, row.OrganizationID, row.DocumentID, row.DocumentVersionID, row.Action, row.Result, row.FailureCode, row.SourceChannel, row.IpAddress, row.UserAgent, row.RequestID, row.TraceID, row.CorrelationID, row.Reason, row.MetadataSchemaVersion, row.MetadataJson, row.HashSchemaVersion, row.ChainID, row.SequenceNumber, row.PreviousHash, row.EventHash, row.PartitionDate, row.CreatedAt)
}

func eventFromGetRow(row *dbgen.GetAuditEventRow) (domain.Event, error) {
	return eventFromValues(row.AuditEventID, row.EventType, row.RiskLevel, row.ActorType, row.ActorID, row.ActorDisplayName, row.ActorEmployeeNo, row.EffectiveRole, row.AdminDelegationID, row.ShareID, row.ResourceType, row.ResourceID, row.ResourceName, row.SpaceID, row.OrganizationID, row.DocumentID, row.DocumentVersionID, row.Action, row.Result, row.FailureCode, row.SourceChannel, row.IpAddress, row.UserAgent, row.RequestID, row.TraceID, row.CorrelationID, row.Reason, row.MetadataSchemaVersion, row.MetadataJson, row.HashSchemaVersion, row.ChainID, row.SequenceNumber, row.PreviousHash, row.EventHash, row.PartitionDate, row.CreatedAt)
}

func eventFromVerifyRow(row *dbgen.ListAuditChainEventsForVerifyRow) (domain.Event, error) {
	return eventFromValues(row.AuditEventID, row.EventType, row.RiskLevel, row.ActorType, row.ActorID, row.ActorDisplayName, row.ActorEmployeeNo, row.EffectiveRole, row.AdminDelegationID, row.ShareID, row.ResourceType, row.ResourceID, row.ResourceName, row.SpaceID, row.OrganizationID, row.DocumentID, row.DocumentVersionID, row.Action, row.Result, row.FailureCode, row.SourceChannel, nil, pgtype.Text{}, row.RequestID, row.TraceID, row.CorrelationID, row.Reason, row.MetadataSchemaVersion, row.MetadataJson, row.HashSchemaVersion, row.ChainID, row.SequenceNumber, row.PreviousHash, row.EventHash, row.PartitionDate, row.CreatedAt)
}

func eventFromValues(idValue pgtype.UUID, eventType, riskLevel, actorType string, actorID pgtype.UUID, actorDisplayName, actorEmployeeNo, effectiveRole pgtype.Text, adminDelegationID, shareID pgtype.UUID, resourceType pgtype.Text, resourceID pgtype.UUID, resourceName pgtype.Text, spaceID, organizationID, documentID, documentVersionID pgtype.UUID, action, result string, failureCode pgtype.Text, sourceChannel string, ipAddress any, userAgent pgtype.Text, requestIDValue pgtype.UUID, traceID pgtype.Text, correlationID pgtype.UUID, reason pgtype.Text, metadataSchemaVersion int32, metadataJSON []byte, hashSchemaVersion pgtype.Int4, chainID pgtype.Text, sequenceNumber pgtype.Int8, previousHash, eventHash []byte, partitionDate pgtype.Date, createdAt pgtype.Timestamptz) (domain.Event, error) {
	id, err := googleUUID(idValue)
	if err != nil {
		return domain.Event{}, err
	}
	requestID, err := googleUUID(requestIDValue)
	if err != nil {
		return domain.Event{}, err
	}
	return domain.Event{
		ID:                    id,
		EventType:             eventType,
		RiskLevel:             riskLevel,
		ActorType:             actorType,
		ActorID:               optionalGoogleUUID(actorID),
		ActorDisplayName:      optionalString(actorDisplayName),
		ActorEmployeeNo:       optionalString(actorEmployeeNo),
		EffectiveRole:         optionalString(effectiveRole),
		AdminDelegationID:     optionalGoogleUUID(adminDelegationID),
		ShareID:               optionalGoogleUUID(shareID),
		ResourceType:          optionalString(resourceType),
		ResourceID:            optionalGoogleUUID(resourceID),
		ResourceName:          optionalString(resourceName),
		SpaceID:               optionalGoogleUUID(spaceID),
		OrganizationID:        optionalGoogleUUID(organizationID),
		DocumentID:            optionalGoogleUUID(documentID),
		DocumentVersionID:     optionalGoogleUUID(documentVersionID),
		Action:                action,
		Result:                result,
		FailureCode:           optionalString(failureCode),
		SourceChannel:         sourceChannel,
		IPAddress:             optionalInterfaceString(ipAddress),
		UserAgent:             optionalString(userAgent),
		RequestID:             requestID,
		TraceID:               optionalString(traceID),
		CorrelationID:         optionalGoogleUUID(correlationID),
		Reason:                optionalString(reason),
		MetadataSchemaVersion: metadataSchemaVersion,
		MetadataJSON:          append([]byte(nil), metadataJSON...),
		HashSchemaVersion:     optionalInt4(hashSchemaVersion),
		ChainID:               optionalString(chainID),
		SequenceNumber:        optionalInt8(sequenceNumber),
		PreviousHash:          append([]byte(nil), previousHash...),
		EventHash:             append([]byte(nil), eventHash...),
		PartitionDate:         partitionDate.Time,
		CreatedAt:             createdAt.Time,
	}, nil
}

func chainHead(row *dbgen.AuditChainHead) domain.ChainHead {
	return domain.ChainHead{
		ChainID:            row.ChainID,
		PartitionDate:      row.PartitionDate.Time,
		LastSequenceNumber: row.LastSequenceNumber,
		LastEventID:        uuid.Must(googleUUID(row.LastEventID)),
		LastHash:           append([]byte(nil), row.LastHash...),
		BatchRoot:          append([]byte(nil), row.BatchRoot...),
		AnchorLocation:     optionalString(row.AnchorLocation),
		Status:             row.Status,
		VerifiedAt:         optionalTime(row.VerifiedAt),
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
		RowVersion:         row.RowVersion,
	}
}

func safeChainSegment(value string) string {
	return value
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func optionalUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgUUID(*id)
}

func googleUUID(value pgtype.UUID) (uuid.UUID, error) {
	if !value.Valid {
		return uuid.Nil, fmt.Errorf("uuid is null")
	}
	return uuid.UUID(value.Bytes), nil
}

func optionalGoogleUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func nullableText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func optionalInterfaceString(value any) *string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if typed == "" {
			return nil
		}
		return &typed
	case []byte:
		if len(typed) == 0 {
			return nil
		}
		result := string(typed)
		return &result
	default:
		result := fmt.Sprint(value)
		if result == "" || result == "<nil>" {
			return nil
		}
		return &result
	}
}

func optionalInt32(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func optionalInt4(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}

func optionalInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func optionalInt8(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func pgDate(value time.Time) pgtype.Date {
	return pgtype.Date{Time: value.UTC(), Valid: true}
}

func pgTimestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
