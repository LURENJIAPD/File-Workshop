package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/modules/identity/security"
	"file-workshop/backend/internal/platform/config"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
	transactor Transactor
	hasher     domain.PasswordHasher
	tokens     AccessTokens
	limiter    LoginLimiter
	config     config.AuthConfig
	now        func() time.Time
	dummyHash  string
}

type LoginInput struct {
	Username string
	Password string
	Metadata domain.RequestMetadata
}

type AuthenticationResult struct {
	AccessToken     string
	AccessExpiresAt time.Time
	RefreshToken    string
	User            domain.User
	Session         domain.Session
}

func NewService(
	repository Repository,
	transactor Transactor,
	hasher domain.PasswordHasher,
	tokens AccessTokens,
	limiter LoginLimiter,
	cfg config.AuthConfig,
	now func() time.Time,
) (*Service, error) {
	dummyHash, err := hasher.Hash("file-workshop-dummy-password-comparison")
	if err != nil {
		return nil, fmt.Errorf("create dummy password hash: %w", err)
	}
	return &Service{
		repository: repository,
		transactor: transactor,
		hasher:     hasher,
		tokens:     tokens,
		limiter:    limiter,
		config:     cfg,
		now:        now,
		dummyHash:  dummyHash,
	}, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (AuthenticationResult, error) {
	now := s.now().UTC()
	usernameNormalized := domain.NormalizeUsername(input.Username)
	if usernameNormalized == "" {
		return AuthenticationResult{}, domain.ErrInvalidCredentials
	}
	ipKey := ""
	if input.Metadata.IPAddress != nil {
		ipKey = input.Metadata.IPAddress.String()
	}
	if allowed, retryAfter := s.limiter.Allow(ipKey, now); !allowed {
		return AuthenticationResult{}, &domain.RateLimitedError{RetryAfter: retryAfter}
	}

	failureState, err := s.repository.GetRecentFailureState(ctx, usernameNormalized, now.Add(-s.config.LoginFailureWindow))
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("read recent login failures: %w", err)
	}
	if retryAfter := s.lockRetryAfter(failureState, now); retryAfter > 0 {
		return AuthenticationResult{}, &domain.AccountLockedError{RetryAfter: retryAfter}
	}

	identity, findErr := s.repository.FindPasswordIdentity(ctx, usernameNormalized, now)
	encodedHash := s.dummyHash
	if findErr == nil {
		encodedHash = identity.SecretHash
	} else if !errors.Is(findErr, domain.ErrIdentityNotFound) {
		return AuthenticationResult{}, fmt.Errorf("find password identity: %w", findErr)
	}
	passwordMatches, compareErr := s.hasher.Compare(input.Password, encodedHash)
	if compareErr != nil {
		return AuthenticationResult{}, fmt.Errorf("compare password hash: %w", compareErr)
	}
	if findErr != nil || !passwordMatches || identity.User.Status != domain.UserStatusActive {
		failureCode := "INVALID_CREDENTIALS"
		var userID *uuid.UUID
		if findErr == nil {
			userID = &identity.User.ID
			if identity.User.Status != domain.UserStatusActive {
				failureCode = "ACCOUNT_" + identity.User.Status
			}
		}
		if err := s.recordAttempt(ctx, usernameNormalized, userID, "FAILURE", &failureCode, input.Metadata, now); err != nil {
			return AuthenticationResult{}, fmt.Errorf("record failed login: %w", err)
		}
		if identity.User.Status == "LOCKED" || failureState.Count+1 >= int64(s.config.LoginFailureLimit) {
			return AuthenticationResult{}, &domain.AccountLockedError{RetryAfter: s.config.LoginLockDuration}
		}
		return AuthenticationResult{}, domain.ErrInvalidCredentials
	}

	sessionID, err := uuid.NewV7()
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("generate session ID: %w", err)
	}
	refreshTokenID, err := uuid.NewV7()
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("generate refresh token ID: %w", err)
	}
	familyID, err := uuid.NewV7()
	if err != nil {
		return AuthenticationResult{}, fmt.Errorf("generate token family ID: %w", err)
	}
	rawRefreshToken, refreshHash, err := security.NewRefreshToken()
	if err != nil {
		return AuthenticationResult{}, err
	}
	accessToken, accessExpiresAt, err := s.tokens.Issue(identity.User.ID, sessionID, now)
	if err != nil {
		return AuthenticationResult{}, err
	}
	sessionExpiresAt := now.Add(s.config.RefreshTokenTTL)
	var session domain.Session
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		created, err := repository.CreateSession(ctx, domain.NewSession{
			ID:        sessionID,
			UserID:    identity.User.ID,
			Metadata:  input.Metadata,
			ExpiresAt: sessionExpiresAt,
			CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create user session: %w", err)
		}
		session = created
		if err := repository.CreateRefreshToken(ctx, domain.NewRefreshToken{
			ID:             refreshTokenID,
			SessionID:      sessionID,
			FamilyID:       familyID,
			RotationNumber: 1,
			Hash:           refreshHash,
			IssuedAt:       now,
			ExpiresAt:      sessionExpiresAt,
		}); err != nil {
			return fmt.Errorf("create refresh token: %w", err)
		}
		if err := repository.TouchLoginIdentity(ctx, identity.User.ID, identity.CredentialID, now); err != nil {
			return fmt.Errorf("update successful login timestamps: %w", err)
		}
		return s.recordAttemptWithRepository(ctx, repository, usernameNormalized, &identity.User.ID, "SUCCESS", nil, input.Metadata, now)
	})
	if err != nil {
		return AuthenticationResult{}, err
	}
	return AuthenticationResult{
		AccessToken:     accessToken,
		AccessExpiresAt: accessExpiresAt,
		RefreshToken:    rawRefreshToken,
		User:            identity.User,
		Session:         session,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (AuthenticationResult, error) {
	if rawRefreshToken == "" {
		return AuthenticationResult{}, domain.ErrAuthentication
	}
	now := s.now().UTC()
	tokenHash := security.HashRefreshToken(rawRefreshToken)
	var result AuthenticationResult
	var outcomeErr error
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetRefreshTokenForUpdate(ctx, tokenHash)
		if errors.Is(err, domain.ErrIdentityNotFound) {
			outcomeErr = domain.ErrAuthentication
			return nil
		}
		if err != nil {
			return fmt.Errorf("read refresh token: %w", err)
		}
		if current.RefreshTokenStatus == domain.RefreshStatusUsed {
			if err := repository.MarkRefreshTokenReused(ctx, current.RefreshTokenID, now); err != nil {
				return fmt.Errorf("mark reused refresh token: %w", err)
			}
			if err := repository.RevokeSession(ctx, current.Session.ID, now, "REFRESH_TOKEN_REUSE"); err != nil {
				return fmt.Errorf("revoke reused session: %w", err)
			}
			if err := repository.RevokeActiveRefreshTokens(ctx, current.Session.ID, now); err != nil {
				return fmt.Errorf("revoke token family: %w", err)
			}
			outcomeErr = domain.ErrTokenReused
			return nil
		}
		if current.RefreshTokenStatus != domain.RefreshStatusActive ||
			!current.RefreshTokenExpiresAt.After(now) ||
			current.Session.Status != domain.SessionStatusActive ||
			!current.Session.ExpiresAt.After(now) ||
			current.User.Status != domain.UserStatusActive {
			outcomeErr = domain.ErrAuthentication
			return nil
		}
		updated, err := repository.MarkRefreshTokenUsed(ctx, current.RefreshTokenID, now)
		if err != nil {
			return fmt.Errorf("consume refresh token: %w", err)
		}
		if !updated {
			return fmt.Errorf("consume refresh token: concurrent state change")
		}
		newTokenID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("generate rotated token ID: %w", err)
		}
		rawToken, hash, err := security.NewRefreshToken()
		if err != nil {
			return err
		}
		if err := repository.CreateRefreshToken(ctx, domain.NewRefreshToken{
			ID:             newTokenID,
			SessionID:      current.Session.ID,
			FamilyID:       current.FamilyID,
			ParentID:       &current.RefreshTokenID,
			RotationNumber: current.RotationNumber + 1,
			Hash:           hash,
			IssuedAt:       now,
			ExpiresAt:      current.Session.ExpiresAt,
		}); err != nil {
			return fmt.Errorf("create rotated refresh token: %w", err)
		}
		accessToken, accessExpiresAt, err := s.tokens.Issue(current.User.ID, current.Session.ID, now)
		if err != nil {
			return err
		}
		result = AuthenticationResult{
			AccessToken:     accessToken,
			AccessExpiresAt: accessExpiresAt,
			RefreshToken:    rawToken,
			User:            current.User,
			Session:         current.Session,
		}
		return nil
	})
	if err != nil {
		return AuthenticationResult{}, err
	}
	if outcomeErr != nil {
		return AuthenticationResult{}, outcomeErr
	}
	return result, nil
}

func (s *Service) CurrentSession(ctx context.Context, rawAccessToken string) (domain.User, domain.Session, error) {
	now := s.now().UTC()
	principal, err := s.tokens.Parse(rawAccessToken, now)
	if err != nil {
		return domain.User{}, domain.Session{}, domain.ErrAuthentication
	}
	session, user, err := s.repository.GetCurrentSession(ctx, principal.SessionID)
	if errors.Is(err, domain.ErrIdentityNotFound) {
		return domain.User{}, domain.Session{}, domain.ErrAuthentication
	}
	if err != nil {
		return domain.User{}, domain.Session{}, fmt.Errorf("read current session: %w", err)
	}
	if user.ID != principal.UserID || user.Status != domain.UserStatusActive || session.Status != domain.SessionStatusActive || !session.ExpiresAt.After(now) {
		return domain.User{}, domain.Session{}, domain.ErrAuthentication
	}
	if err := s.repository.TouchSession(ctx, session.ID, now); err != nil {
		return domain.User{}, domain.Session{}, fmt.Errorf("touch current session: %w", err)
	}
	return user, session, nil
}

func (s *Service) Logout(ctx context.Context, rawAccessToken, rawRefreshToken string) error {
	now := s.now().UTC()
	var sessionID uuid.UUID
	if rawAccessToken != "" {
		if principal, err := s.tokens.Parse(rawAccessToken, now); err == nil {
			sessionID = principal.SessionID
		}
	}
	if sessionID == uuid.Nil && rawRefreshToken != "" {
		found, err := s.repository.FindSessionIDByRefreshHash(ctx, security.HashRefreshToken(rawRefreshToken))
		if err == nil {
			sessionID = found
		} else if !errors.Is(err, domain.ErrIdentityNotFound) {
			return fmt.Errorf("find logout session: %w", err)
		}
	}
	if sessionID == uuid.Nil {
		return nil
	}
	return s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		if err := repository.RevokeSession(ctx, sessionID, now, "USER_LOGOUT"); err != nil {
			return fmt.Errorf("revoke logout session: %w", err)
		}
		if err := repository.RevokeActiveRefreshTokens(ctx, sessionID, now); err != nil {
			return fmt.Errorf("revoke logout refresh tokens: %w", err)
		}
		return nil
	})
}

func (s *Service) lockRetryAfter(state domain.FailureState, now time.Time) time.Duration {
	if state.Count < int64(s.config.LoginFailureLimit) || state.LastFailureAt == nil {
		return 0
	}
	retryAfter := s.config.LoginLockDuration - now.Sub(*state.LastFailureAt)
	if retryAfter < 0 {
		return 0
	}
	return retryAfter
}

func (s *Service) recordAttempt(
	ctx context.Context,
	usernameNormalized string,
	userID *uuid.UUID,
	result string,
	failureCode *string,
	metadata domain.RequestMetadata,
	now time.Time,
) error {
	return s.recordAttemptWithRepository(ctx, s.repository, usernameNormalized, userID, result, failureCode, metadata, now)
}

func (s *Service) recordAttemptWithRepository(
	ctx context.Context,
	repository Repository,
	usernameNormalized string,
	userID *uuid.UUID,
	result string,
	failureCode *string,
	metadata domain.RequestMetadata,
	now time.Time,
) error {
	attemptID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate login attempt ID: %w", err)
	}
	return repository.InsertLoginAttempt(ctx, domain.LoginAttempt{
		ID:                 attemptID,
		UsernameNormalized: usernameNormalized,
		UserID:             userID,
		Result:             result,
		FailureCode:        failureCode,
		Metadata:           metadata,
		CreatedAt:          now,
	})
}
