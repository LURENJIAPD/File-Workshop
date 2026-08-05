package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/users/application"
	"file-workshop/backend/internal/modules/users/domain"
	"file-workshop/backend/internal/platform/database/dbgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (r *PostgreSQL) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin user transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	transactionRepository := &PostgreSQL{pool: r.pool, queries: r.queries.WithTx(tx)}
	if err := operation(transactionRepository); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit user transaction: %w", mapDatabaseError(err))
	}
	return nil
}

func (r *PostgreSQL) GetUser(ctx context.Context, userID uuid.UUID) (domain.User, error) {
	row, err := r.queries.GetManagedUserByID(ctx, pgUUID(userID))
	if err != nil {
		return domain.User{}, mapNotFound(err)
	}
	return userFromValues(row.UserID, row.Username, row.UsernameNormalized, row.EmployeeNo, row.EmployeeNoNormalized, row.DisplayName, row.Email, row.EmailNormalized, row.Phone, row.SystemRole, row.Status, row.Locale, row.Timezone, row.LastLoginAt, row.CreatedByUserID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.RowVersion)
}

func (r *PostgreSQL) GetUserForUpdate(ctx context.Context, userID uuid.UUID) (domain.User, error) {
	row, err := r.queries.GetManagedUserForUpdate(ctx, pgUUID(userID))
	if err != nil {
		return domain.User{}, mapNotFound(err)
	}
	return userFromValues(row.UserID, row.Username, row.UsernameNormalized, row.EmployeeNo, row.EmployeeNoNormalized, row.DisplayName, row.Email, row.EmailNormalized, row.Phone, row.SystemRole, row.Status, row.Locale, row.Timezone, row.LastLoginAt, row.CreatedByUserID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.RowVersion)
}

func (r *PostgreSQL) ListUsers(ctx context.Context, filter domain.ListFilter) (domain.ListResult, error) {
	params := &dbgen.ListManagedUsersParams{Status: optionalFilter(filter.Status), SystemRole: optionalFilter(filter.SystemRole), PageOffset: int64(filter.Page-1) * int64(filter.PageSize), PageSize: int32(filter.PageSize)}
	rows, err := r.queries.ListManagedUsers(ctx, params)
	if err != nil {
		return domain.ListResult{}, mapDatabaseError(err)
	}
	items := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		user, err := userFromValues(row.UserID, row.Username, row.UsernameNormalized, row.EmployeeNo, row.EmployeeNoNormalized, row.DisplayName, row.Email, row.EmailNormalized, row.Phone, row.SystemRole, row.Status, row.Locale, row.Timezone, row.LastLoginAt, row.CreatedByUserID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.RowVersion)
		if err != nil {
			return domain.ListResult{}, err
		}
		items = append(items, user)
	}
	total, err := r.queries.CountManagedUsers(ctx, &dbgen.CountManagedUsersParams{Status: optionalFilter(filter.Status), SystemRole: optionalFilter(filter.SystemRole)})
	if err != nil {
		return domain.ListResult{}, mapDatabaseError(err)
	}
	return domain.ListResult{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *PostgreSQL) InsertUser(ctx context.Context, input domain.NewUser) (domain.User, error) {
	row, err := r.queries.InsertManagedUser(ctx, &dbgen.InsertManagedUserParams{UserID: pgUUID(input.ID), Username: input.Username, UsernameNormalized: input.UsernameNormalized, EmployeeNo: optionalText(input.EmployeeNo), EmployeeNoNormalized: optionalText(input.EmployeeNoNormalized), DisplayName: input.DisplayName, Email: optionalText(input.Email), EmailNormalized: optionalText(input.EmailNormalized), Phone: optionalText(input.Phone), SystemRole: input.SystemRole, Locale: input.Locale, Timezone: input.Timezone, CreatedByUserID: pgUUID(input.CreatedByUserID), CreatedAt: timestamptz(input.CreatedAt)})
	if err != nil {
		return domain.User{}, mapDatabaseError(err)
	}
	return userFromValues(row.UserID, row.Username, row.UsernameNormalized, row.EmployeeNo, row.EmployeeNoNormalized, row.DisplayName, row.Email, row.EmailNormalized, row.Phone, row.SystemRole, row.Status, row.Locale, row.Timezone, row.LastLoginAt, row.CreatedByUserID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.RowVersion)
}

func (r *PostgreSQL) InsertPasswordCredential(ctx context.Context, input domain.NewUser) error {
	return mapDatabaseError(r.queries.InsertManagedPasswordCredential(ctx, &dbgen.InsertManagedPasswordCredentialParams{UserCredentialID: pgUUID(input.CredentialID), UserID: pgUUID(input.ID), Identifier: input.Username, IdentifierNormalized: input.UsernameNormalized, SecretHash: pgtype.Text{String: input.PasswordHash, Valid: true}, CreatedAt: timestamptz(input.CreatedAt)}))
}

func (r *PostgreSQL) InsertSecurityVersions(ctx context.Context, userID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.InsertPrincipalSecurityVersions(ctx, &dbgen.InsertPrincipalSecurityVersionsParams{UserID: pgUUID(userID), UpdatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) UpdateUser(ctx context.Context, user domain.User, expectedVersion int64, now time.Time) (domain.User, error) {
	row, err := r.queries.UpdateManagedUser(ctx, &dbgen.UpdateManagedUserParams{UserID: pgUUID(user.ID), EmployeeNo: optionalText(user.EmployeeNo), EmployeeNoNormalized: optionalText(user.EmployeeNoNormalized), DisplayName: user.DisplayName, Email: optionalText(user.Email), EmailNormalized: optionalText(user.EmailNormalized), Phone: optionalText(user.Phone), SystemRole: user.SystemRole, Locale: user.Locale, Timezone: user.Timezone, UpdatedAt: timestamptz(now), RowVersion: expectedVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.User{}, mapDatabaseError(err)
	}
	return userFromValues(row.UserID, row.Username, row.UsernameNormalized, row.EmployeeNo, row.EmployeeNoNormalized, row.DisplayName, row.Email, row.EmailNormalized, row.Phone, row.SystemRole, row.Status, row.Locale, row.Timezone, row.LastLoginAt, row.CreatedByUserID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.RowVersion)
}

func (r *PostgreSQL) SetUserStatus(ctx context.Context, userID uuid.UUID, status string, deletedAt *time.Time, expectedVersion int64, now time.Time) (domain.User, error) {
	row, err := r.queries.SetManagedUserStatus(ctx, &dbgen.SetManagedUserStatusParams{UserID: pgUUID(userID), Status: status, DeletedAt: optionalTimeValue(deletedAt), UpdatedAt: timestamptz(now), RowVersion: expectedVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.User{}, mapDatabaseError(err)
	}
	return userFromValues(row.UserID, row.Username, row.UsernameNormalized, row.EmployeeNo, row.EmployeeNoNormalized, row.DisplayName, row.Email, row.EmailNormalized, row.Phone, row.SystemRole, row.Status, row.Locale, row.Timezone, row.LastLoginAt, row.CreatedByUserID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.RowVersion)
}

func (r *PostgreSQL) TouchUserForPasswordReset(ctx context.Context, userID uuid.UUID, expectedVersion int64, now time.Time) (int64, error) {
	version, err := r.queries.TouchManagedUserForPasswordReset(ctx, &dbgen.TouchManagedUserForPasswordResetParams{UserID: pgUUID(userID), UpdatedAt: timestamptz(now), RowVersion: expectedVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrVersionConflict
	}
	return version, mapDatabaseError(err)
}

func (r *PostgreSQL) UpdatePasswordCredential(ctx context.Context, userID uuid.UUID, passwordHash string, now time.Time) (bool, error) {
	rows, err := r.queries.UpdateManagedPasswordCredential(ctx, &dbgen.UpdateManagedPasswordCredentialParams{UserID: pgUUID(userID), SecretHash: pgtype.Text{String: passwordHash, Valid: true}, UpdatedAt: timestamptz(now)})
	return rows == 1, mapDatabaseError(err)
}

func (r *PostgreSQL) CountActiveSystemAdminsForUpdate(ctx context.Context) (int, error) {
	rows, err := r.queries.LockActiveSystemAdministrators(ctx)
	return len(rows), mapDatabaseError(err)
}

func (r *PostgreSQL) LockSystemAdminMutation(ctx context.Context) error {
	return mapDatabaseError(r.queries.LockSystemAdminMutation(ctx))
}

func (r *PostgreSQL) IncrementGlobalAuthorizationVersion(ctx context.Context, userID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.IncrementGlobalAuthorizationVersion(ctx, &dbgen.IncrementGlobalAuthorizationVersionParams{UserID: pgUUID(userID), UpdatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) RevokeUserSessions(ctx context.Context, userID uuid.UUID, now time.Time, reason string) error {
	return mapDatabaseError(r.queries.RevokeManagedUserSessions(ctx, &dbgen.RevokeManagedUserSessionsParams{UserID: pgUUID(userID), RevokedAt: timestamptz(now), RevokeReason: pgtype.Text{String: reason, Valid: true}}))
}

func (r *PostgreSQL) RevokeUserRefreshTokens(ctx context.Context, userID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.RevokeManagedUserRefreshTokens(ctx, &dbgen.RevokeManagedUserRefreshTokensParams{UserID: pgUUID(userID), RevokedAt: timestamptz(now)}))
}

func (r *PostgreSQL) RevokeUserCredentials(ctx context.Context, userID uuid.UUID, now time.Time, reason string) error {
	return mapDatabaseError(r.queries.RevokeManagedUserCredentials(ctx, &dbgen.RevokeManagedUserCredentialsParams{UserID: pgUUID(userID), RevokedAt: timestamptz(now), RevokeReason: pgtype.Text{String: reason, Valid: true}}))
}

func (r *PostgreSQL) ListUserSessions(ctx context.Context, userID uuid.UUID, page, pageSize int) (domain.SessionListResult, error) {
	rows, err := r.queries.ListManagedUserSessions(ctx, &dbgen.ListManagedUserSessionsParams{UserID: pgUUID(userID), PageSize: int32(pageSize), PageOffset: int64(page-1) * int64(pageSize)})
	if err != nil {
		return domain.SessionListResult{}, mapDatabaseError(err)
	}
	items := make([]domain.Session, 0, len(rows))
	for _, row := range rows {
		sessionID, err := googleUUID(row.UserSessionID)
		if err != nil {
			return domain.SessionListResult{}, err
		}
		rowUserID, err := googleUUID(row.UserID)
		if err != nil {
			return domain.SessionListResult{}, err
		}
		items = append(items, domain.Session{ID: sessionID, UserID: rowUserID, DeviceID: optionalString(row.DeviceID), IPAddress: row.IpAddress, UserAgent: optionalString(row.UserAgent), Status: row.Status, ExpiresAt: row.ExpiresAt.Time.UTC(), LastSeenAt: optionalTime(row.LastSeenAt), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), RevokedAt: optionalTime(row.RevokedAt), RevokeReason: optionalString(row.RevokeReason), RowVersion: row.RowVersion})
	}
	total, err := r.queries.CountManagedUserSessions(ctx, pgUUID(userID))
	if err != nil {
		return domain.SessionListResult{}, mapDatabaseError(err)
	}
	return domain.SessionListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r *PostgreSQL) OwnedSessionExists(ctx context.Context, userID, sessionID uuid.UUID) (bool, error) {
	_, err := r.queries.GetOwnedUserSession(ctx, &dbgen.GetOwnedUserSessionParams{UserSessionID: pgUUID(sessionID), UserID: pgUUID(userID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, mapDatabaseError(err)
}

func (r *PostgreSQL) RevokeOwnedSessionTokens(ctx context.Context, userID, sessionID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.RevokeOwnedUserSessionTokens(ctx, &dbgen.RevokeOwnedUserSessionTokensParams{UserSessionID: pgUUID(sessionID), UserID: pgUUID(userID), RevokedAt: timestamptz(now)}))
}

func (r *PostgreSQL) RevokeOwnedSession(ctx context.Context, userID, sessionID uuid.UUID, now time.Time) (bool, error) {
	rows, err := r.queries.RevokeOwnedUserSession(ctx, &dbgen.RevokeOwnedUserSessionParams{UserSessionID: pgUUID(sessionID), UserID: pgUUID(userID), RevokedAt: timestamptz(now)})
	return rows == 1, mapDatabaseError(err)
}

func (r *PostgreSQL) TryCreateIdempotency(ctx context.Context, recordID, actorID uuid.UUID, key string, hash []byte, expiresAt, now time.Time) (bool, error) {
	rows, err := r.queries.TryCreateUserIdempotencyRecord(ctx, &dbgen.TryCreateUserIdempotencyRecordParams{IdempotencyRecordID: pgUUID(recordID), UserID: pgUUID(actorID), IdempotencyKey: key, RequestHash: hash, ExpiresAt: timestamptz(expiresAt), CreatedAt: timestamptz(now)})
	return rows == 1, mapDatabaseError(err)
}

func (r *PostgreSQL) GetCreateIdempotencyForUpdate(ctx context.Context, actorID uuid.UUID, key string) (domain.IdempotencyRecord, error) {
	row, err := r.queries.GetUserIdempotencyRecordForUpdate(ctx, &dbgen.GetUserIdempotencyRecordForUpdateParams{UserID: pgUUID(actorID), IdempotencyKey: key})
	if err != nil {
		return domain.IdempotencyRecord{}, mapDatabaseError(err)
	}
	return domain.IdempotencyRecord{RequestHash: row.RequestHash, Status: row.Status, ResultResourceID: optionalGoogleUUID(row.ResultResourceID)}, nil
}

func (r *PostgreSQL) CompleteCreateIdempotency(ctx context.Context, actorID uuid.UUID, key string, userID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.CompleteUserIdempotencyRecord(ctx, &dbgen.CompleteUserIdempotencyRecordParams{UserID: pgUUID(actorID), IdempotencyKey: key, ResultResourceID: pgUUID(userID), CompletedAt: timestamptz(now)}))
}

func (r *PostgreSQL) InsertEvent(ctx context.Context, event domain.Event) error {
	return mapDatabaseError(r.queries.InsertUserOutboxEvent(ctx, &dbgen.InsertUserOutboxEventParams{OutboxEventID: pgUUID(event.ID), AggregateID: pgUUID(event.AggregateID), AggregateVersion: event.AggregateVersion, EventType: event.Type, PayloadJson: event.Payload, DeduplicationKey: event.DeduplicationKey, CorrelationID: pgUUID(event.CorrelationID), AvailableAt: timestamptz(event.CreatedAt)}))
}

func userFromValues(id pgtype.UUID, username, usernameNormalized string, employeeNo, employeeNoNormalized pgtype.Text, displayName string, email, emailNormalized, phone pgtype.Text, role, status, locale, timezone string, lastLogin pgtype.Timestamptz, createdBy pgtype.UUID, createdAt, updatedAt, deletedAt pgtype.Timestamptz, rowVersion int64) (domain.User, error) {
	userID, err := googleUUID(id)
	if err != nil {
		return domain.User{}, err
	}
	return domain.User{ID: userID, Username: username, UsernameNormalized: usernameNormalized, EmployeeNo: optionalString(employeeNo), EmployeeNoNormalized: optionalString(employeeNoNormalized), DisplayName: displayName, Email: optionalString(email), EmailNormalized: optionalString(emailNormalized), Phone: optionalString(phone), SystemRole: role, Status: status, Locale: locale, Timezone: timezone, LastLoginAt: optionalTime(lastLogin), CreatedByUserID: optionalGoogleUUID(createdBy), CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(), DeletedAt: optionalTime(deletedAt), RowVersion: rowVersion}, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrUserNotFound
	}
	return mapDatabaseError(err)
}

func mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return fmt.Errorf("%w: unique value", domain.ErrConflict)
	}
	return err
}

func pgUUID(value uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: value, Valid: true} }

func googleUUID(value pgtype.UUID) (uuid.UUID, error) {
	if !value.Valid {
		return uuid.Nil, fmt.Errorf("database UUID is null")
	}
	return uuid.UUID(value.Bytes), nil
}

func optionalGoogleUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	result := uuid.UUID(value.Bytes)
	return &result
}

func optionalText(value *string) pgtype.Text {
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

func optionalFilter(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func optionalTimeValue(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*value)
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
