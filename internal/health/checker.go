package health

import (
	"context"
	"time"
)

// Checker defines the interface for health checkers
type HealthChecker interface {
	Check(ctx context.Context) error
	Name() string
}

type CheckResult struct {
	Name     string        `json:"name"`
	Healthy  bool          `json:"healthy"`
	Message  string        `json:"message"`
	Duration time.Duration `json:"duration"`
}
