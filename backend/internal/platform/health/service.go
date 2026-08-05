package health

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusOK          Status = "ok"
	StatusDegraded    Status = "degraded"
	StatusUnavailable Status = "unavailable"
)

type ComponentStatus string

const (
	ComponentOK          ComponentStatus = "ok"
	ComponentUnavailable ComponentStatus = "unavailable"
	ComponentDisabled    ComponentStatus = "disabled"
)

type CheckFunc func(context.Context) error

type Dependency struct {
	Name     string
	Required bool
	Enabled  bool
	Timeout  time.Duration
	Check    CheckFunc
}

type CheckResult struct {
	Status   ComponentStatus
	Latency  time.Duration
	Message  string
	Required bool
	Err      error
}

type Report struct {
	Status    Status
	Service   string
	Timestamp time.Time
	Checks    map[string]CheckResult
}

type Service struct {
	serviceName  string
	dependencies []Dependency
	now          func() time.Time
}

func NewService(serviceName string, dependencies []Dependency, now func() time.Time) (*Service, error) {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return nil, errors.New("health service name is required")
	}
	if now == nil {
		now = time.Now
	}

	seenNames := make(map[string]struct{}, len(dependencies))
	validatedDependencies := make([]Dependency, 0, len(dependencies))
	for _, dependency := range dependencies {
		dependency.Name = strings.TrimSpace(dependency.Name)
		if dependency.Name == "" {
			return nil, errors.New("health dependency name is required")
		}
		if _, exists := seenNames[dependency.Name]; exists {
			return nil, fmt.Errorf("health dependency %q is duplicated", dependency.Name)
		}
		seenNames[dependency.Name] = struct{}{}
		if dependency.Enabled {
			if dependency.Check == nil {
				return nil, fmt.Errorf("health dependency %q has no checker", dependency.Name)
			}
			if dependency.Timeout <= 0 {
				return nil, fmt.Errorf("health dependency %q must have a positive timeout", dependency.Name)
			}
		}
		validatedDependencies = append(validatedDependencies, dependency)
	}

	return &Service{
		serviceName:  serviceName,
		dependencies: validatedDependencies,
		now:          now,
	}, nil
}

func (s *Service) Liveness() Report {
	return Report{
		Status:    StatusOK,
		Service:   s.serviceName,
		Timestamp: s.now().UTC(),
		Checks:    map[string]CheckResult{},
	}
}

func (s *Service) Readiness(ctx context.Context) Report {
	report := Report{
		Status:    StatusOK,
		Service:   s.serviceName,
		Timestamp: s.now().UTC(),
		Checks:    make(map[string]CheckResult, len(s.dependencies)),
	}

	for _, dependency := range s.dependencies {
		if !dependency.Enabled {
			report.Checks[dependency.Name] = CheckResult{
				Status:   ComponentDisabled,
				Message:  "disabled by configuration",
				Required: dependency.Required,
			}
			continue
		}

		startedAt := time.Now()
		checkContext, cancel := context.WithTimeout(ctx, dependency.Timeout)
		err := dependency.Check(checkContext)
		cancel()
		elapsed := time.Since(startedAt)

		if err == nil {
			report.Checks[dependency.Name] = CheckResult{
				Status:   ComponentOK,
				Latency:  elapsed,
				Message:  "available",
				Required: dependency.Required,
			}
			continue
		}

		report.Checks[dependency.Name] = CheckResult{
			Status:   ComponentUnavailable,
			Latency:  elapsed,
			Message:  "unavailable",
			Required: dependency.Required,
			Err:      err,
		}
		if dependency.Required {
			report.Status = StatusUnavailable
		} else if report.Status == StatusOK {
			report.Status = StatusDegraded
		}
	}

	return report
}
