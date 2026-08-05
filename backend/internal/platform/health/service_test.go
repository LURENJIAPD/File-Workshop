package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceLivenessDoesNotCheckDependencies(t *testing.T) {
	called := false
	service, err := NewService("file-workshop-server", []Dependency{
		{
			Name:     "postgresql",
			Required: true,
			Enabled:  true,
			Timeout:  time.Second,
			Check: func(context.Context) error {
				called = true
				return nil
			},
		},
	}, func() time.Time { return time.Date(2026, 8, 5, 8, 0, 0, 0, time.FixedZone("CST", 8*60*60)) })
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	report := service.Liveness()
	if called {
		t.Fatal("liveness must not call external dependency checks")
	}
	if report.Status != StatusOK || len(report.Checks) != 0 {
		t.Fatalf("unexpected liveness report: %#v", report)
	}
	if report.Timestamp.Location() != time.UTC {
		t.Fatalf("liveness timestamp must be UTC: %v", report.Timestamp)
	}
}

func TestServiceReadinessCombinesRequiredOptionalAndDisabledDependencies(t *testing.T) {
	tests := []struct {
		name           string
		postgresErr    error
		redisErr       error
		expectedStatus Status
	}{
		{name: "all available", expectedStatus: StatusOK},
		{name: "optional Redis unavailable", redisErr: errors.New("redis unavailable"), expectedStatus: StatusDegraded},
		{name: "required PostgreSQL unavailable", postgresErr: errors.New("postgres unavailable"), expectedStatus: StatusUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewService("file-workshop-server", []Dependency{
				{Name: "postgresql", Required: true, Enabled: true, Timeout: time.Second, Check: func(context.Context) error { return test.postgresErr }},
				{Name: "redis", Required: false, Enabled: true, Timeout: time.Second, Check: func(context.Context) error { return test.redisErr }},
				{Name: "minio", Required: false, Enabled: false},
			}, time.Now)
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			report := service.Readiness(context.Background())
			if report.Status != test.expectedStatus {
				t.Fatalf("status = %q, want %q", report.Status, test.expectedStatus)
			}
			if report.Checks["minio"].Status != ComponentDisabled {
				t.Fatalf("MinIO status = %q, want disabled", report.Checks["minio"].Status)
			}
		})
	}
}

func TestNewServiceRejectsInvalidDependencies(t *testing.T) {
	_, err := NewService("file-workshop-server", []Dependency{
		{Name: "postgresql", Enabled: true, Timeout: time.Second, Check: func(context.Context) error { return nil }},
		{Name: "postgresql", Enabled: false},
	}, time.Now)
	if err == nil {
		t.Fatal("expected duplicate dependency error")
	}
}
