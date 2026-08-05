package domain

import (
	"errors"
	"unicode/utf8"

	passwordvalidator "github.com/go-passwd/validator"
)

const (
	PasswordMinimumLength = 12
	PasswordMaximumLength = 128
)

var (
	ErrPasswordTooShort    = errors.New("password is too short")
	ErrPasswordTooLong     = errors.New("password is too long")
	ErrPasswordMatchesUser = errors.New("password matches username")
	ErrPasswordCommon      = errors.New("password is commonly used")
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
