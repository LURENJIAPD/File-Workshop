package domain

import (
	"errors"
	"strings"
)

const (
	ErrorCodeHandlerFailed = "HANDLER_FAILED"
	ErrorCodePermanent     = "PERMANENT_FAILURE"
)

var ErrNoHandlers = errors.New("no outbox handlers registered")
var ErrForbidden = errors.New("background operation is forbidden")
var ErrInvalidInput = errors.New("background input is invalid")
var ErrNotFound = errors.New("background item not found")
var ErrConflict = errors.New("background item conflict")

type ValidationError struct {
	Field string
}

func (e *ValidationError) Error() string {
	if strings.TrimSpace(e.Field) == "" {
		return ErrInvalidInput.Error()
	}
	return ErrInvalidInput.Error() + ": " + e.Field
}

func (e *ValidationError) Unwrap() error {
	return ErrInvalidInput
}

type ProcessingError struct {
	Code      string
	Summary   string
	Retryable bool
}

func (e *ProcessingError) Error() string {
	if strings.TrimSpace(e.Summary) != "" {
		return e.Summary
	}
	if strings.TrimSpace(e.Code) != "" {
		return e.Code
	}
	return ErrorCodeHandlerFailed
}

func RetryableError(code, summary string) error {
	return &ProcessingError{Code: normalizeCode(code, ErrorCodeHandlerFailed), Summary: sanitizeSummary(summary), Retryable: true}
}

func PermanentError(code, summary string) error {
	return &ProcessingError{Code: normalizeCode(code, ErrorCodePermanent), Summary: sanitizeSummary(summary), Retryable: false}
}

func normalizeCode(value, fallback string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" || len(value) > 64 {
		return fallback
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return fallback
		}
	}
	return value
}

func sanitizeSummary(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		value = value[:512]
	}
	return value
}

func ClassifyError(err error) (code, summary string, retryable bool) {
	if err == nil {
		return "", "", false
	}
	var processing *ProcessingError
	if errors.As(err, &processing) {
		return normalizeCode(processing.Code, ErrorCodeHandlerFailed), sanitizeSummary(processing.Error()), processing.Retryable
	}
	return ErrorCodeHandlerFailed, sanitizeSummary(err.Error()), true
}

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

func ValidateOutboxStatus(value string) error {
	switch value {
	case OutboxStatusPending, OutboxStatusProcessing, OutboxStatusPublished, OutboxStatusFailed, OutboxStatusDead:
		return nil
	default:
		return &ValidationError{Field: "status"}
	}
}

func ValidateJobStatus(value string) error {
	switch value {
	case JobStatusPending, JobStatusProcessing, JobStatusSuccess, JobStatusFailed, JobStatusDead, JobStatusCancelled, JobStatusSkipped:
		return nil
	default:
		return &ValidationError{Field: "status"}
	}
}
