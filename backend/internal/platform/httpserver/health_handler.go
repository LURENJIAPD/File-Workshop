package httpserver

import (
	"context"
	"log/slog"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/platform/health"
	"file-workshop/backend/internal/platform/requestid"
)

type HealthHandler struct {
	service *health.Service
	logger  *slog.Logger
}

func NewHealthHandler(service *health.Service, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{service: service, logger: logger}
}

func (h *HealthHandler) GetLiveness(ctx context.Context, _ api.GetLivenessRequestObject) (api.GetLivenessResponseObject, error) {
	requestID := requestid.FromContext(ctx)
	response := healthResponse(h.service.Liveness(), requestID)
	return api.GetLiveness200JSONResponse{
		Body: response,
		Headers: api.GetLiveness200ResponseHeaders{
			XRequestID: &requestID,
		},
	}, nil
}

func (h *HealthHandler) GetReadiness(ctx context.Context, _ api.GetReadinessRequestObject) (api.GetReadinessResponseObject, error) {
	requestID := requestid.FromContext(ctx)
	report := h.service.Readiness(ctx)
	for component, result := range report.Checks {
		if result.Err == nil {
			continue
		}
		logMethod := h.logger.WarnContext
		if result.Required {
			logMethod = h.logger.ErrorContext
		}
		logMethod(
			ctx,
			"Health dependency check failed",
			"requestId", requestID,
			"component", component,
			"required", result.Required,
			"error", result.Err,
		)
	}

	response := healthResponse(report, requestID)
	if report.Status == health.StatusUnavailable {
		return api.GetReadiness503JSONResponse{
			Body: response,
			Headers: api.GetReadiness503ResponseHeaders{
				XRequestID: &requestID,
			},
		}, nil
	}
	return api.GetReadiness200JSONResponse{
		Body: response,
		Headers: api.GetReadiness200ResponseHeaders{
			XRequestID: &requestID,
		},
	}, nil
}

func healthResponse(report health.Report, requestID string) api.HealthResponse {
	checks := make(map[string]api.ComponentHealth, len(report.Checks))
	for name, result := range report.Checks {
		message := result.Message
		checks[name] = api.ComponentHealth{
			Status:    componentStatus(result.Status),
			LatencyMs: result.Latency.Milliseconds(),
			Message:   &message,
		}
	}
	return api.HealthResponse{
		Status:    api.HealthStatus(report.Status),
		Service:   report.Service,
		Timestamp: report.Timestamp,
		RequestId: requestID,
		Checks:    checks,
	}
}

func componentStatus(status health.ComponentStatus) api.ComponentStatus {
	switch status {
	case health.ComponentOK:
		return api.ComponentStatusOk
	case health.ComponentDisabled:
		return api.ComponentStatusDisabled
	default:
		return api.ComponentStatusUnavailable
	}
}
