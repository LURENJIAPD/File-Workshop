package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/modules/users/domain"

	"github.com/google/uuid"
)

const idempotencyTTL = 24 * time.Hour

type Service struct {
	repository Repository
	transactor Transactor
	hasher     identitydomain.PasswordHasher
	policy     *identitydomain.PasswordPolicy
	now        func() time.Time
}

type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
}

type CreateUserInput struct {
	Username       string
	Password       string
	EmployeeNo     *string
	DisplayName    string
	Email          *string
	Phone          *string
	SystemRole     *string
	Locale         *string
	Timezone       *string
	IdempotencyKey string
	RequestID      uuid.UUID
}

type ChangeStatusInput struct {
	RowVersion int64
	Reason     string
	RequestID  uuid.UUID
}

func NewService(repository Repository, transactor Transactor, hasher identitydomain.PasswordHasher, now func() time.Time) *Service {
	return &Service{repository: repository, transactor: transactor, hasher: hasher, policy: identitydomain.NewPasswordPolicy(), now: now}
}

func (s *Service) GetCurrent(ctx context.Context, actor Actor) (domain.User, error) {
	return s.repository.GetUser(ctx, actor.UserID)
}

func (s *Service) Get(ctx context.Context, actor Actor, userID uuid.UUID) (domain.User, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.User{}, err
	}
	return s.repository.GetUser(ctx, userID)
}

func (s *Service) List(ctx context.Context, actor Actor, filter domain.ListFilter) (domain.ListResult, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.ListResult{}, err
	}
	page, pageSize, err := domain.NormalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return domain.ListResult{}, err
	}
	filter.Page, filter.PageSize = page, pageSize
	if filter.Status != nil {
		if err := domain.ValidateStatus(*filter.Status); err != nil {
			return domain.ListResult{}, err
		}
	}
	if filter.SystemRole != nil {
		if err := domain.ValidateRole(*filter.SystemRole); err != nil {
			return domain.ListResult{}, err
		}
	}
	return s.repository.ListUsers(ctx, filter)
}

func (s *Service) Create(ctx context.Context, actor Actor, input CreateUserInput) (domain.User, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.User{}, err
	}
	prepared, requestHash, err := s.prepareNewUser(actor, input)
	if err != nil {
		return domain.User{}, err
	}
	passwordHash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash new user password: %w", err)
	}
	prepared.PasswordHash = passwordHash

	var result domain.User
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		idempotencyID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("generate idempotency ID: %w", err)
		}
		created, err := repository.TryCreateIdempotency(ctx, idempotencyID, actor.UserID, input.IdempotencyKey, requestHash, prepared.CreatedAt.Add(idempotencyTTL), prepared.CreatedAt)
		if err != nil {
			return fmt.Errorf("claim create user idempotency key: %w", err)
		}
		if !created {
			record, err := repository.GetCreateIdempotencyForUpdate(ctx, actor.UserID, input.IdempotencyKey)
			if err != nil {
				return fmt.Errorf("read create user idempotency record: %w", err)
			}
			if !bytes.Equal(record.RequestHash, requestHash) {
				return domain.ErrIdempotencyConflict
			}
			if record.Status != "COMPLETED" || record.ResultResourceID == nil {
				return domain.ErrConflict
			}
			result, err = repository.GetUser(ctx, *record.ResultResourceID)
			return err
		}

		result, err = repository.InsertUser(ctx, prepared)
		if err != nil {
			return err
		}
		if err := repository.InsertPasswordCredential(ctx, prepared); err != nil {
			return err
		}
		if err := repository.InsertSecurityVersions(ctx, prepared.ID, prepared.CreatedAt); err != nil {
			return err
		}
		if err := s.insertEvent(ctx, repository, result, actor.UserID, input.RequestID, "USER_CREATED", nil); err != nil {
			return err
		}
		return repository.CompleteCreateIdempotency(ctx, actor.UserID, input.IdempotencyKey, prepared.ID, prepared.CreatedAt)
	})
	if err != nil {
		return domain.User{}, err
	}
	return result, nil
}

func (s *Service) UpdateCurrent(ctx context.Context, actor Actor, changes domain.UserChanges, requestID uuid.UUID) (domain.User, error) {
	changes.EmployeeNo = nil
	changes.SystemRole = nil
	return s.update(ctx, actor, actor.UserID, changes, requestID, false)
}

func (s *Service) Update(ctx context.Context, actor Actor, userID uuid.UUID, changes domain.UserChanges, requestID uuid.UUID) (domain.User, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.User{}, err
	}
	return s.update(ctx, actor, userID, changes, requestID, true)
}

func (s *Service) update(ctx context.Context, actor Actor, userID uuid.UUID, changes domain.UserChanges, requestID uuid.UUID, allowAdminFields bool) (domain.User, error) {
	if changes.RowVersion < 1 {
		return domain.User{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if changes.DisplayName == nil && changes.Email == nil && changes.Phone == nil && changes.Locale == nil && changes.Timezone == nil && (!allowAdminFields || (changes.EmployeeNo == nil && changes.SystemRole == nil)) {
		return domain.User{}, &domain.ValidationError{Field: "body"}
	}
	var result domain.User
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		if allowAdminFields && changes.SystemRole != nil {
			if err := repository.LockSystemAdminMutation(ctx); err != nil {
				return err
			}
		}
		current, err := repository.GetUserForUpdate(ctx, userID)
		if err != nil {
			return err
		}
		if current.RowVersion != changes.RowVersion {
			return domain.ErrVersionConflict
		}
		if current.Status == domain.UserStatusDeleted {
			return domain.ErrInvalidState
		}
		updated, err := applyChanges(current, changes, allowAdminFields)
		if err != nil {
			return err
		}
		roleChanged := updated.SystemRole != current.SystemRole
		if roleChanged && current.SystemRole == domain.SystemRoleAdmin && current.Status == domain.UserStatusActive {
			if err := ensureAnotherActiveAdmin(ctx, repository); err != nil {
				return err
			}
		}
		now := s.now().UTC()
		result, err = repository.UpdateUser(ctx, updated, changes.RowVersion, now)
		if err != nil {
			return err
		}
		if roleChanged {
			if err := repository.IncrementGlobalAuthorizationVersion(ctx, userID, now); err != nil {
				return err
			}
			if err := s.insertEvent(ctx, repository, result, actor.UserID, requestID, "USER_ROLE_CHANGED", map[string]any{"previousRole": current.SystemRole}); err != nil {
				return err
			}
		}
		return s.insertEvent(ctx, repository, result, actor.UserID, requestID, "USER_UPDATED", nil)
	})
	return result, err
}

func (s *Service) ChangeStatus(ctx context.Context, actor Actor, userID uuid.UUID, target string, input ChangeStatusInput) (domain.User, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.User{}, err
	}
	if input.RowVersion < 1 || strings.TrimSpace(input.Reason) == "" || len([]rune(input.Reason)) > 512 {
		return domain.User{}, &domain.ValidationError{Field: "reason"}
	}
	if err := domain.ValidateStatus(target); err != nil {
		return domain.User{}, err
	}
	var result domain.User
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		if err := repository.LockSystemAdminMutation(ctx); err != nil {
			return err
		}
		current, err := repository.GetUserForUpdate(ctx, userID)
		if err != nil {
			return err
		}
		if current.RowVersion != input.RowVersion {
			return domain.ErrVersionConflict
		}
		if !domain.CanTransition(current.Status, target) {
			return domain.ErrInvalidState
		}
		if current.Status == target {
			result = current
			return nil
		}
		if current.SystemRole == domain.SystemRoleAdmin && current.Status == domain.UserStatusActive && target != domain.UserStatusActive {
			if err := ensureAnotherActiveAdmin(ctx, repository); err != nil {
				return err
			}
		}
		now := s.now().UTC()
		var deletedAt *time.Time
		if target == domain.UserStatusDeleted {
			deletedAt = &now
		}
		result, err = repository.SetUserStatus(ctx, userID, target, deletedAt, input.RowVersion, now)
		if err != nil {
			return err
		}
		if err := repository.IncrementGlobalAuthorizationVersion(ctx, userID, now); err != nil {
			return err
		}
		if target != domain.UserStatusActive {
			if err := repository.RevokeUserRefreshTokens(ctx, userID, now); err != nil {
				return err
			}
			if err := repository.RevokeUserSessions(ctx, userID, now, "USER_"+target); err != nil {
				return err
			}
		}
		if target == domain.UserStatusDeleted {
			if err := repository.RevokeUserCredentials(ctx, userID, now, strings.TrimSpace(input.Reason)); err != nil {
				return err
			}
		}
		return s.insertEvent(ctx, repository, result, actor.UserID, input.RequestID, statusEvent(target), map[string]any{"previousStatus": current.Status, "reason": strings.TrimSpace(input.Reason)})
	})
	return result, err
}

func (s *Service) ResetPassword(ctx context.Context, actor Actor, userID uuid.UUID, password string, rowVersion int64, requestID uuid.UUID) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	current, err := s.repository.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if current.RowVersion != rowVersion {
		return domain.ErrVersionConflict
	}
	if current.Status == domain.UserStatusDeleted {
		return domain.ErrInvalidState
	}
	if err := s.policy.Validate(password, current.Username); err != nil {
		return &domain.ValidationError{Field: "password"}
	}
	passwordHash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("hash reset password: %w", err)
	}
	now := s.now().UTC()
	return s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		locked, err := repository.GetUserForUpdate(ctx, userID)
		if err != nil {
			return err
		}
		if locked.RowVersion != rowVersion {
			return domain.ErrVersionConflict
		}
		if locked.Status == domain.UserStatusDeleted {
			return domain.ErrInvalidState
		}
		newVersion, err := repository.TouchUserForPasswordReset(ctx, userID, rowVersion, now)
		if err != nil {
			return err
		}
		updated, err := repository.UpdatePasswordCredential(ctx, userID, passwordHash, now)
		if err != nil {
			return err
		}
		if !updated {
			return domain.ErrPasswordCredential
		}
		if err := repository.RevokeUserRefreshTokens(ctx, userID, now); err != nil {
			return err
		}
		if err := repository.RevokeUserSessions(ctx, userID, now, "PASSWORD_RESET"); err != nil {
			return err
		}
		locked.RowVersion = newVersion
		locked.UpdatedAt = now
		return s.insertEvent(ctx, repository, locked, actor.UserID, requestID, "AUTH_PASSWORD_CHANGED", nil)
	})
}

func (s *Service) ListSessions(ctx context.Context, actor Actor, page, pageSize int) (domain.SessionListResult, error) {
	page, pageSize, err := domain.NormalizePage(page, pageSize)
	if err != nil {
		return domain.SessionListResult{}, err
	}
	return s.repository.ListUserSessions(ctx, actor.UserID, page, pageSize)
}

func (s *Service) RevokeSession(ctx context.Context, actor Actor, sessionID, requestID uuid.UUID) error {
	now := s.now().UTC()
	return s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		exists, err := repository.OwnedSessionExists(ctx, actor.UserID, sessionID)
		if err != nil {
			return err
		}
		if !exists {
			return domain.ErrUserNotFound
		}
		if err := repository.RevokeOwnedSessionTokens(ctx, actor.UserID, sessionID, now); err != nil {
			return err
		}
		changed, err := repository.RevokeOwnedSession(ctx, actor.UserID, sessionID, now)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		user, err := repository.GetUser(ctx, actor.UserID)
		if err != nil {
			return err
		}
		return s.insertEvent(ctx, repository, user, actor.UserID, requestID, "AUTH_SESSION_REVOKED", map[string]any{"sessionId": sessionID.String()})
	})
}

func (s *Service) prepareNewUser(actor Actor, input CreateUserInput) (domain.NewUser, []byte, error) {
	username := strings.TrimSpace(input.Username)
	usernameNormalized := identitydomain.NormalizeUsername(username)
	if usernameNormalized == "" || len([]rune(username)) > 128 {
		return domain.NewUser{}, nil, &domain.ValidationError{Field: "username"}
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" || len(input.IdempotencyKey) > 128 {
		return domain.NewUser{}, nil, &domain.ValidationError{Field: "Idempotency-Key"}
	}
	if err := s.policy.Validate(input.Password, username); err != nil {
		return domain.NewUser{}, nil, &domain.ValidationError{Field: "password"}
	}
	employeeNo, employeeNormalized := optionalNormalized(input.EmployeeNo)
	email, emailNormalized, err := optionalEmail(input.Email)
	if err != nil {
		return domain.NewUser{}, nil, err
	}
	phone := optionalTrimmed(input.Phone)
	role := domain.SystemRoleUser
	if input.SystemRole != nil {
		role = string(*input.SystemRole)
	}
	locale := domain.DefaultLocale
	if input.Locale != nil {
		locale = strings.TrimSpace(*input.Locale)
	}
	timezone := domain.DefaultTimezone
	if input.Timezone != nil {
		timezone = strings.TrimSpace(*input.Timezone)
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if err := domain.ValidateRole(role); err != nil {
		return domain.NewUser{}, nil, err
	}
	if err := domain.ValidateProfile(displayName, locale, timezone, phone); err != nil {
		return domain.NewUser{}, nil, err
	}
	userID, err := uuid.NewV7()
	if err != nil {
		return domain.NewUser{}, nil, fmt.Errorf("generate user ID: %w", err)
	}
	credentialID, err := uuid.NewV7()
	if err != nil {
		return domain.NewUser{}, nil, fmt.Errorf("generate credential ID: %w", err)
	}
	now := s.now().UTC()
	prepared := domain.NewUser{ID: userID, CredentialID: credentialID, Username: username, UsernameNormalized: usernameNormalized, EmployeeNo: employeeNo, EmployeeNoNormalized: employeeNormalized, DisplayName: displayName, Email: email, EmailNormalized: emailNormalized, Phone: phone, SystemRole: role, Locale: locale, Timezone: timezone, CreatedByUserID: actor.UserID, CreatedAt: now}
	canonical, err := json.Marshal(struct {
		Username, Password, DisplayName, SystemRole, Locale, Timezone string
		EmployeeNo, Email, Phone                                      *string
	}{username, input.Password, displayName, role, locale, timezone, employeeNo, email, phone})
	if err != nil {
		return domain.NewUser{}, nil, fmt.Errorf("encode create user request hash: %w", err)
	}
	hash := sha256.Sum256(canonical)
	return prepared, hash[:], nil
}

func applyChanges(current domain.User, changes domain.UserChanges, allowAdminFields bool) (domain.User, error) {
	updated := current
	if changes.DisplayName != nil {
		updated.DisplayName = strings.TrimSpace(*changes.DisplayName)
	}
	if changes.Email != nil {
		value, normalized, err := domain.PrepareEmail(*changes.Email)
		if err != nil {
			return domain.User{}, err
		}
		updated.Email, updated.EmailNormalized = value, normalized
	}
	if changes.Phone != nil {
		updated.Phone = optionalTrimmed(changes.Phone)
	}
	if changes.Locale != nil {
		updated.Locale = strings.TrimSpace(*changes.Locale)
	}
	if changes.Timezone != nil {
		updated.Timezone = strings.TrimSpace(*changes.Timezone)
	}
	if allowAdminFields && changes.EmployeeNo != nil {
		updated.EmployeeNo, updated.EmployeeNoNormalized = domain.NormalizeOptional(*changes.EmployeeNo)
	}
	if allowAdminFields && changes.SystemRole != nil {
		updated.SystemRole = *changes.SystemRole
	}
	if err := domain.ValidateRole(updated.SystemRole); err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidateProfile(updated.DisplayName, updated.Locale, updated.Timezone, updated.Phone); err != nil {
		return domain.User{}, err
	}
	return updated, nil
}

func (s *Service) insertEvent(ctx context.Context, repository Repository, user domain.User, actorID, requestID uuid.UUID, eventType string, extra map[string]any) error {
	payload := map[string]any{"userId": user.ID.String(), "actorUserId": actorID.String(), "status": user.Status, "systemRole": user.SystemRole, "rowVersion": user.RowVersion}
	for key, value := range extra {
		payload[key] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode user event: %w", err)
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate user event ID: %w", err)
	}
	deduplicationKey := fmt.Sprintf("user:%s:%d:%s", user.ID, user.RowVersion, eventType)
	if sessionID, ok := extra["sessionId"].(string); ok {
		deduplicationKey += ":" + sessionID
	}
	return repository.InsertEvent(ctx, domain.Event{ID: eventID, AggregateID: user.ID, AggregateVersion: user.RowVersion, Type: eventType, Payload: encoded, DeduplicationKey: deduplicationKey, CorrelationID: requestID, CreatedAt: s.now().UTC()})
}

func requireAdmin(actor Actor) error {
	if actor.Role != domain.SystemRoleAdmin {
		return domain.ErrForbidden
	}
	return nil
}

type activeAdminLocker interface {
	CountActiveSystemAdminsForUpdate(context.Context) (int, error)
}

func ensureAnotherActiveAdmin(ctx context.Context, repository activeAdminLocker) error {
	count, err := repository.CountActiveSystemAdminsForUpdate(ctx)
	if err != nil {
		return err
	}
	if count <= 1 {
		return domain.ErrLastSystemAdmin
	}
	return nil
}

func optionalNormalized(value *string) (*string, *string) {
	if value == nil {
		return nil, nil
	}
	return domain.NormalizeOptional(*value)
}

func optionalEmail(value *string) (*string, *string, error) {
	if value == nil {
		return nil, nil, nil
	}
	return domain.PrepareEmail(*value)
}

func optionalTrimmed(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func statusEvent(status string) string {
	switch status {
	case domain.UserStatusActive:
		return "USER_ENABLED"
	case domain.UserStatusDisabled:
		return "USER_DISABLED"
	case domain.UserStatusLocked:
		return "AUTH_ACCOUNT_LOCKED"
	case domain.UserStatusDeleted:
		return "USER_UPDATED"
	default:
		panic("validated user status has no event mapping")
	}
}
