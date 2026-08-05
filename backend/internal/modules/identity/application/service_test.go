package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/platform/config"

	"github.com/google/uuid"
)

func TestLoginCreatesSessionRefreshTokenAndSuccessAttempt(t *testing.T) {
	now := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	userID := uuid.Must(uuid.NewV7())
	repository := &fakeRepository{
		identity: domain.PasswordIdentity{
			User:         domain.User{ID: userID, Username: "root", DisplayName: "Root", SystemRole: "SYSTEM_ADMIN", Status: "ACTIVE", Locale: "zh-CN", Timezone: "Asia/Shanghai"},
			CredentialID: uuid.Must(uuid.NewV7()), SecretHash: "stored-hash",
		},
	}
	service := newTestService(t, repository, now)
	result, err := service.Login(context.Background(), LoginInput{
		Username: "ＲＯＯＴ", Password: "correct",
		Metadata: domain.RequestMetadata{RequestID: uuid.Must(uuid.NewV7())},
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.User.ID != userID || result.AccessToken != "access-token" || result.RefreshToken == "" {
		t.Fatalf("unexpected login result: %#v", result)
	}
	if repository.createdRefresh.RotationNumber != 1 || len(repository.createdRefresh.Hash) != 32 {
		t.Fatalf("unexpected initial refresh token: %#v", repository.createdRefresh)
	}
	if len(repository.attempts) != 1 || repository.attempts[0].Result != "SUCCESS" || repository.attempts[0].UsernameNormalized != "root" {
		t.Fatalf("unexpected login attempts: %#v", repository.attempts)
	}
	if !repository.touchedLogin {
		t.Fatal("successful identity timestamps were not updated")
	}
}

func TestFifthFailedLoginLocksAccount(t *testing.T) {
	now := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	repository := &fakeRepository{
		identity: domain.PasswordIdentity{
			User:         domain.User{ID: uuid.Must(uuid.NewV7()), Status: "ACTIVE"},
			CredentialID: uuid.Must(uuid.NewV7()), SecretHash: "stored-hash",
		},
		failureState: domain.FailureState{Count: 4},
	}
	service := newTestService(t, repository, now)
	_, err := service.Login(context.Background(), LoginInput{
		Username: "root", Password: "wrong",
		Metadata: domain.RequestMetadata{RequestID: uuid.Must(uuid.NewV7())},
	})
	var locked *domain.AccountLockedError
	if !errors.As(err, &locked) || locked.RetryAfter != 15*time.Minute {
		t.Fatalf("Login() error = %v, want 15 minute lock", err)
	}
	if len(repository.attempts) != 1 || repository.attempts[0].Result != "FAILURE" {
		t.Fatalf("failed login was not recorded: %#v", repository.attempts)
	}
}

func TestRefreshTokenReuseRevokesSessionFamily(t *testing.T) {
	now := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	repository := &fakeRepository{refreshSession: domain.RefreshSession{
		RefreshTokenID:        uuid.Must(uuid.NewV7()),
		FamilyID:              uuid.Must(uuid.NewV7()),
		RefreshTokenStatus:    domain.RefreshStatusUsed,
		RefreshTokenExpiresAt: now.Add(time.Hour),
		Session:               domain.Session{ID: uuid.Must(uuid.NewV7()), Status: domain.SessionStatusActive, ExpiresAt: now.Add(time.Hour)},
		User:                  domain.User{ID: uuid.Must(uuid.NewV7()), Status: domain.UserStatusActive},
	}}
	service := newTestService(t, repository, now)
	_, err := service.Refresh(context.Background(), "previously-used-token")
	if !errors.Is(err, domain.ErrTokenReused) {
		t.Fatalf("Refresh() error = %v, want token reused", err)
	}
	if !repository.markedReused || !repository.revokedSession || !repository.revokedRefreshTokens {
		t.Fatalf("reuse response did not revoke full session family: %#v", repository)
	}
}

func newTestService(t *testing.T, repository *fakeRepository, now time.Time) *Service {
	t.Helper()
	service, err := NewService(
		repository,
		repository,
		fakeHasher{},
		fakeTokens{},
		allowAllLimiter{},
		config.AuthConfig{
			AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 7 * 24 * time.Hour,
			LoginFailureWindow: 15 * time.Minute, LoginFailureLimit: 5, LoginLockDuration: 15 * time.Minute,
		},
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

type fakeHasher struct{}

func (fakeHasher) Hash(string) (string, error) { return "dummy-hash", nil }
func (fakeHasher) Compare(password, encodedHash string) (bool, error) {
	return password == "correct" && encodedHash == "stored-hash", nil
}

type fakeTokens struct{}

func (fakeTokens) Issue(uuid.UUID, uuid.UUID, time.Time) (string, time.Time, error) {
	return "access-token", time.Date(2026, 8, 5, 8, 15, 0, 0, time.UTC), nil
}
func (fakeTokens) Parse(string, time.Time) (domain.AccessPrincipal, error) {
	return domain.AccessPrincipal{}, domain.ErrAuthentication
}

type allowAllLimiter struct{}

func (allowAllLimiter) Allow(string, time.Time) (bool, time.Duration) { return true, 0 }

type fakeRepository struct {
	identity             domain.PasswordIdentity
	identityErr          error
	failureState         domain.FailureState
	attempts             []domain.LoginAttempt
	createdRefresh       domain.NewRefreshToken
	refreshSession       domain.RefreshSession
	touchedLogin         bool
	markedReused         bool
	revokedSession       bool
	revokedRefreshTokens bool
}

func (r *fakeRepository) WithinTransaction(ctx context.Context, operation func(Repository) error) error {
	return operation(r)
}
func (r *fakeRepository) FindPasswordIdentity(context.Context, string, time.Time) (domain.PasswordIdentity, error) {
	return r.identity, r.identityErr
}
func (r *fakeRepository) GetRecentFailureState(context.Context, string, time.Time) (domain.FailureState, error) {
	return r.failureState, nil
}
func (r *fakeRepository) InsertLoginAttempt(_ context.Context, attempt domain.LoginAttempt) error {
	r.attempts = append(r.attempts, attempt)
	return nil
}
func (r *fakeRepository) CreateSession(_ context.Context, session domain.NewSession) (domain.Session, error) {
	return domain.Session{ID: session.ID, UserID: session.UserID, DeviceID: session.Metadata.DeviceID, Status: "ACTIVE", ExpiresAt: session.ExpiresAt, CreatedAt: session.CreatedAt}, nil
}
func (r *fakeRepository) CreateRefreshToken(_ context.Context, token domain.NewRefreshToken) error {
	r.createdRefresh = token
	return nil
}
func (r *fakeRepository) TouchLoginIdentity(context.Context, uuid.UUID, uuid.UUID, time.Time) error {
	r.touchedLogin = true
	return nil
}
func (r *fakeRepository) GetRefreshTokenForUpdate(context.Context, []byte) (domain.RefreshSession, error) {
	return r.refreshSession, nil
}
func (r *fakeRepository) MarkRefreshTokenUsed(context.Context, uuid.UUID, time.Time) (bool, error) {
	return true, nil
}
func (r *fakeRepository) MarkRefreshTokenReused(context.Context, uuid.UUID, time.Time) error {
	r.markedReused = true
	return nil
}
func (r *fakeRepository) RevokeSession(context.Context, uuid.UUID, time.Time, string) error {
	r.revokedSession = true
	return nil
}
func (r *fakeRepository) RevokeActiveRefreshTokens(context.Context, uuid.UUID, time.Time) error {
	r.revokedRefreshTokens = true
	return nil
}
func (r *fakeRepository) GetCurrentSession(context.Context, uuid.UUID) (domain.Session, domain.User, error) {
	return domain.Session{}, domain.User{}, domain.ErrIdentityNotFound
}
func (r *fakeRepository) FindSessionIDByRefreshHash(context.Context, []byte) (uuid.UUID, error) {
	return uuid.Nil, domain.ErrIdentityNotFound
}
func (r *fakeRepository) TouchSession(context.Context, uuid.UUID, time.Time) error { return nil }
