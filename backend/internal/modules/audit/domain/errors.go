package domain

import (
	"errors"
	"strings"
)

var ErrForbidden = errors.New("audit operation is forbidden")
var ErrInvalidInput = errors.New("audit input is invalid")
var ErrNotFound = errors.New("audit item not found")
var ErrConflict = errors.New("audit state conflict")

type ValidationError struct {
	Field string
}

func (e *ValidationError) Error() string {
	if strings.TrimSpace(e.Field) == "" {
		return ErrInvalidInput.Error()
	}
	return ErrInvalidInput.Error() + ": " + e.Field
}

func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

func NormalizePage(page, pageSize int) (int, int, error) {
	if page == 0 {
		page = DefaultPage
	}
	if pageSize == 0 {
		pageSize = DefaultPageSize
	}
	if page < 1 {
		return 0, 0, &ValidationError{Field: "page"}
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		return 0, 0, &ValidationError{Field: "pageSize"}
	}
	return page, pageSize, nil
}

func ValidateRiskLevel(value string) error {
	switch value {
	case RiskNormal, RiskHigh, RiskCritical:
		return nil
	default:
		return &ValidationError{Field: "riskLevel"}
	}
}

func ValidateActorType(value string) error {
	switch value {
	case ActorTypeUser, ActorTypeSystem, ActorTypeAgent, ActorTypeMigration, ActorTypeService:
		return nil
	default:
		return &ValidationError{Field: "actorType"}
	}
}

func ValidateResult(value string) error {
	switch value {
	case ResultSuccess, ResultFailure, ResultDenied:
		return nil
	default:
		return &ValidationError{Field: "result"}
	}
}

func ValidateChainStatus(value string) error {
	switch value {
	case ChainStatusActive, ChainStatusSealed, ChainStatusInvalid:
		return nil
	default:
		return &ValidationError{Field: "status"}
	}
}
