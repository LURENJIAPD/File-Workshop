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
