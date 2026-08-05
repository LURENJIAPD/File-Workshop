package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/identity/application"
	"file-workshop/backend/internal/modules/identity/domain"
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

func (r *PostgreSQL) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin identity transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	transactionRepository := &PostgreSQL{pool: r.pool, queries: r.queries.WithTx(tx)}
	if err := operation(transactionRepository); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit identity transaction: %w", err)
	}
	return nil
}

func (r *PostgreSQL) FindPasswordIdentity(ctx context.Context, normalizedUsername string, now time.Time) (domain.PasswordIdentity, error) {
	row, err := r.queries.GetPasswordLoginIdentity(ctx, &dbgen.GetPasswordLoginIdentityParams{
		UsernameNormalized: normalizedUsername,
		ExpiresAt:          timestamptz(now),
	})
	if err != nil {
		return domain.PasswordIdentity{}, mapNotFound(err)
	}
	userID, err := googleUUID(row.UserID)
	if err != nil {
		return domain.PasswordIdentity{}, err
	}
	credentialID, err := googleUUID(row.UserCredentialID)
	if err != nil {
		return domain.PasswordIdentity{}, err
	}
	return domain.PasswordIdentity{
		User: domain.User{
			ID:          userID,
			Username:    row.Username,
			DisplayName: row.DisplayName,
			SystemRole:  row.SystemRole,
			Status:      row.UserStatus,
			Locale:      row.Locale,
			Timezone:    row.Timezone,
		},
		CredentialID: credentialID,
		SecretHash:   row.SecretHash.String,
	}, nil
}

func (r *PostgreSQL) GetRecentFailureState(ctx context.Context, normalizedUsername string, since time.Time) (domain.FailureState, error) {
	row, err := r.queries.GetRecentLoginFailureState(ctx, &dbgen.GetRecentLoginFailureStateParams{
		UsernameNormalized: normalizedUsername,
		CreatedAt:          timestamptz(since),
	})
	if err != nil {
		return domain.FailureState{}, err
	}
	return domain.FailureState{Count: row.FailureCount, LastFailureAt: optionalTime(row.LastFailureAt)}, nil
}

func (r *PostgreSQL) InsertLoginAttempt(ctx context.Context, attempt domain.LoginAttempt) error {
	return r.queries.InsertLoginAttempt(ctx, &dbgen.InsertLoginAttemptParams{
		LoginAttemptID:     pgUUID(attempt.ID),
		UsernameNormalized: attempt.UsernameNormalized,
		UserID:             optionalUUID(attempt.UserID),
		Result:             attempt.Result,
		FailureCode:        optionalText(attempt.FailureCode),
		IpAddress:          attempt.Metadata.IPAddress,
		UserAgent:          nullableText(attempt.Metadata.UserAgent),
		RequestID:          pgUUID(attempt.Metadata.RequestID),
		CreatedAt:          timestamptz(attempt.CreatedAt),
	})
}

func (r *PostgreSQL) CreateSession(ctx context.Context, session domain.NewSession) (domain.Session, error) {
	row, err := r.queries.CreateUserSession(ctx, &dbgen.CreateUserSessionParams{
		UserSessionID: pgUUID(session.ID),
		UserID:        pgUUID(session.UserID),
		DeviceID:      optionalString(session.Metadata.DeviceID),
		IpAddress:     session.Metadata.IPAddress,
		UserAgent:     nullableText(session.Metadata.UserAgent),
		ExpiresAt:     timestamptz(session.ExpiresAt),
		CreatedAt:     timestamptz(session.CreatedAt),
	})
	if err != nil {
		return domain.Session{}, err
	}
	return sessionFromValues(row.UserSessionID, row.UserID, row.DeviceID, row.Status, row.ExpiresAt, row.LastSeenAt, row.CreatedAt)
}

func (r *PostgreSQL) CreateRefreshToken(ctx context.Context, token domain.NewRefreshToken) error {
	_, err := r.queries.CreateSessionRefreshToken(ctx, &dbgen.CreateSessionRefreshTokenParams{
		RefreshTokenID:       pgUUID(token.ID),
		UserSessionID:        pgUUID(token.SessionID),
		TokenFamilyID:        pgUUID(token.FamilyID),
		ParentRefreshTokenID: optionalUUID(token.ParentID),
		RotationNumber:       token.RotationNumber,
		TokenHash:            token.Hash,
		IssuedAt:             timestamptz(token.IssuedAt),
		ExpiresAt:            timestamptz(token.ExpiresAt),
	})
	return err
}

func (r *PostgreSQL) TouchLoginIdentity(ctx context.Context, userID, credentialID uuid.UUID, now time.Time) error {
	if err := r.queries.TouchUserAfterLogin(ctx, &dbgen.TouchUserAfterLoginParams{UserID: pgUUID(userID), LastLoginAt: timestamptz(now)}); err != nil {
		return err
	}
	return r.queries.TouchCredentialAfterLogin(ctx, &dbgen.TouchCredentialAfterLoginParams{UserCredentialID: pgUUID(credentialID), LastUsedAt: timestamptz(now)})
}

func (r *PostgreSQL) GetRefreshTokenForUpdate(ctx context.Context, hash []byte) (domain.RefreshSession, error) {
	row, err := r.queries.GetRefreshTokenForUpdate(ctx, hash)
	if err != nil {
		return domain.RefreshSession{}, mapNotFound(err)
	}
	refreshTokenID, err := googleUUID(row.RefreshTokenID)
	if err != nil {
		return domain.RefreshSession{}, err
	}
	familyID, err := googleUUID(row.TokenFamilyID)
	if err != nil {
		return domain.RefreshSession{}, err
	}
	userID, err := googleUUID(row.UserID)
	if err != nil {
		return domain.RefreshSession{}, err
	}
	session, err := sessionFromValues(row.UserSessionID, row.UserID, row.DeviceID, row.SessionStatus, row.SessionExpiresAt, row.LastSeenAt, row.SessionCreatedAt)
	if err != nil {
		return domain.RefreshSession{}, err
	}
	return domain.RefreshSession{
		RefreshTokenID:        refreshTokenID,
		Session:               session,
		FamilyID:              familyID,
		RotationNumber:        row.RotationNumber,
		RefreshTokenStatus:    row.RefreshTokenStatus,
		RefreshTokenExpiresAt: row.RefreshTokenExpiresAt.Time,
		User: domain.User{
			ID:          userID,
			Username:    row.Username,
			DisplayName: row.DisplayName,
			SystemRole:  row.SystemRole,
			Status:      row.UserStatus,
			Locale:      row.Locale,
			Timezone:    row.Timezone,
		},
	}, nil
}

func (r *PostgreSQL) MarkRefreshTokenUsed(ctx context.Context, tokenID uuid.UUID, now time.Time) (bool, error) {
	rows, err := r.queries.MarkRefreshTokenUsed(ctx, &dbgen.MarkRefreshTokenUsedParams{RefreshTokenID: pgUUID(tokenID), UsedAt: timestamptz(now)})
	return rows == 1, err
}

func (r *PostgreSQL) MarkRefreshTokenReused(ctx context.Context, tokenID uuid.UUID, now time.Time) error {
	return r.queries.MarkRefreshTokenReused(ctx, &dbgen.MarkRefreshTokenReusedParams{RefreshTokenID: pgUUID(tokenID), RevokedAt: timestamptz(now)})
}

func (r *PostgreSQL) RevokeSession(ctx context.Context, sessionID uuid.UUID, now time.Time, reason string) error {
	return r.queries.RevokeUserSession(ctx, &dbgen.RevokeUserSessionParams{
		UserSessionID: pgUUID(sessionID),
		UpdatedAt:     timestamptz(now),
		RevokeReason:  pgtype.Text{String: reason, Valid: true},
	})
}

func (r *PostgreSQL) RevokeActiveRefreshTokens(ctx context.Context, sessionID uuid.UUID, now time.Time) error {
	return r.queries.RevokeActiveRefreshTokensForSession(ctx, &dbgen.RevokeActiveRefreshTokensForSessionParams{
		UserSessionID: pgUUID(sessionID),
		RevokedAt:     timestamptz(now),
	})
}

func (r *PostgreSQL) GetCurrentSession(ctx context.Context, sessionID uuid.UUID) (domain.Session, domain.User, error) {
	row, err := r.queries.GetCurrentSessionIdentity(ctx, pgUUID(sessionID))
	if err != nil {
		return domain.Session{}, domain.User{}, mapNotFound(err)
	}
	session, err := sessionFromValues(row.UserSessionID, row.UserID, row.DeviceID, row.SessionStatus, row.SessionExpiresAt, row.LastSeenAt, row.SessionCreatedAt)
	if err != nil {
		return domain.Session{}, domain.User{}, err
	}
	userID, err := googleUUID(row.UserID)
	if err != nil {
		return domain.Session{}, domain.User{}, err
	}
	return session, domain.User{
		ID:          userID,
		Username:    row.Username,
		DisplayName: row.DisplayName,
		SystemRole:  row.SystemRole,
		Status:      row.UserStatus,
		Locale:      row.Locale,
		Timezone:    row.Timezone,
	}, nil
}

func (r *PostgreSQL) FindSessionIDByRefreshHash(ctx context.Context, hash []byte) (uuid.UUID, error) {
	value, err := r.queries.GetSessionIDByRefreshTokenHash(ctx, hash)
	if err != nil {
		return uuid.Nil, mapNotFound(err)
	}
	return googleUUID(value)
}

func (r *PostgreSQL) TouchSession(ctx context.Context, sessionID uuid.UUID, now time.Time) error {
	return r.queries.TouchUserSession(ctx, &dbgen.TouchUserSessionParams{UserSessionID: pgUUID(sessionID), LastSeenAt: timestamptz(now)})
}

func sessionFromValues(id, userID pgtype.UUID, deviceID pgtype.Text, status string, expiresAt, lastSeenAt, createdAt pgtype.Timestamptz) (domain.Session, error) {
	sessionUUID, err := googleUUID(id)
	if err != nil {
		return domain.Session{}, err
	}
	userUUID, err := googleUUID(userID)
	if err != nil {
		return domain.Session{}, err
	}
	return domain.Session{
		ID:         sessionUUID,
		UserID:     userUUID,
		DeviceID:   optionalStringValue(deviceID),
		Status:     status,
		ExpiresAt:  expiresAt.Time,
		LastSeenAt: optionalTime(lastSeenAt),
		CreatedAt:  createdAt.Time,
	}, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrIdentityNotFound
	}
	return err
}

func pgUUID(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: true}
}

func optionalUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgUUID(*value)
}

func googleUUID(value pgtype.UUID) (uuid.UUID, error) {
	if !value.Valid {
		return uuid.Nil, fmt.Errorf("database UUID is null")
	}
	return uuid.UUID(value.Bytes), nil
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time.UTC()
	return &timestamp
}

func optionalString(value *string) pgtype.Text {
	if value == nil || *value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalStringValue(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func nullableText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}
