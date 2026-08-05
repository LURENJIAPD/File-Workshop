package domain

import (
	"errors"
	"fmt"
	"unicode/utf8"

	passwordvalidator "github.com/go-passwd/validator"
)

const (
	PasswordMinimumLength = 12
	PasswordMaximumLength = 128
	PasswordHistoryLimit  = 5
)

var (
	ErrPasswordTooShort     = errors.New("password is too short")
	ErrPasswordTooLong      = errors.New("password is too long")
	ErrPasswordMatchesUser  = errors.New("password matches username")
	ErrPasswordCommon       = errors.New("password is commonly used")
	ErrPasswordRecentlyUsed = errors.New("password was recently used")
)

type PasswordPolicy struct {
	commonPasswordValidator *passwordvalidator.Validator
}

func NewPasswordPolicy() *PasswordPolicy {
	return &PasswordPolicy{
		commonPasswordValidator: passwordvalidator.New(passwordvalidator.CommonPassword(ErrPasswordCommon)),
	}
}

func (p *PasswordPolicy) Validate(password, username string) error {
	length := utf8.RuneCountInString(password)
	if length < PasswordMinimumLength {
		return ErrPasswordTooShort
	}
	if length > PasswordMaximumLength {
		return ErrPasswordTooLong
	}
	if NormalizeUsername(password) == NormalizeUsername(username) {
		return ErrPasswordMatchesUser
	}
	if err := p.commonPasswordValidator.Validate(password); err != nil {
		return err
	}
	return nil
}

func (p *PasswordPolicy) ValidateHistory(password string, recentHashes []string, hasher PasswordHasher) error {
	limit := min(len(recentHashes), PasswordHistoryLimit)
	for _, encodedHash := range recentHashes[:limit] {
		matches, err := hasher.Compare(password, encodedHash)
		if err != nil {
			return fmt.Errorf("compare password history: %w", err)
		}
		if matches {
			return ErrPasswordRecentlyUsed
		}
	}
	return nil
}
