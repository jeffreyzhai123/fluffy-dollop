package health

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

type Aggregator struct {
	checkers []HealthChecker
	timeout  time.Duration
	logger   *observability.Logger
}

type AggregatedHealth struct {
	Status  string        `json:"status"`
	Checks  []CheckResult `json:"checks"`
	Overall bool          `json:"overall"`
}

// NewAggregator creates a new health check aggregator
func NewAggregator(timeout time.Duration, logger *observability.Logger) *Aggregator {
	return &Aggregator{
		checkers: make([]HealthChecker, 0),
		timeout:  timeout,
		logger:   logger,
	}
}

// Register adds a health checker to the aggregator
func (a *Aggregator) Register(checker HealthChecker) {
	a.checkers = append(a.checkers, checker)
	a.logger.Debug("Registered health checker",
		"checker", checker.Name(),
		"total_checkers", len(a.checkers),
	)
}

// CheckAll runs all registered health checks
func (a *Aggregator) CheckAll(ctx context.Context) AggregatedHealth {
	checkCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	results := make([]CheckResult, len(a.checkers))

	g, gCtx := errgroup.WithContext(checkCtx)

	for i, checker := range a.checkers {
		g.Go(func() error {
			result := a.runCheckSafe(gCtx, checker)
			results[i] = result

			if result.Healthy {
				a.logger.Debug("health check passed", "checker", result.Name)
			} else {
				a.logger.Warn("health check failed", "checker", result.Name, "message", result.Message)
			}

			return nil
		})
	}

	g.Wait()

	overallHealthy := checkOverallHealth(results)
	status := "healthy"
	if !overallHealthy {
		status = "unhealthy"
	}

	aggregated := AggregatedHealth{
		Status:  status,
		Checks:  results,
		Overall: overallHealthy,
	}

	if overallHealthy {
		a.logger.Debug("overall health check passed",
			"total_checks", len(results),
		)
	} else {
		a.logger.Warn("overall health check failed",
			"total_checks", len(results),
			"failed_checks", countFailedChecks(results),
		)
	}

	return aggregated
}

// runCheckSafe executes a health check and recovers from any panics
func (a *Aggregator) runCheckSafe(ctx context.Context, checker HealthChecker) CheckResult {
	start := time.Now()
	name := checker.Name()

	// Buffered channel prevents goroutine leak
	resultChan := make(chan CheckResult, 1)

	go func() {
		// Panic recovery
		defer func() {
			if r := recover(); r != nil {
				a.logger.Error("health check panicked",
					"checker", name,
					"panic", r,
				)
				resultChan <- CheckResult{
					Name:     name,
					Healthy:  false,
					Message:  fmt.Sprintf("health check panicked: %v", r),
					Duration: time.Since(start),
				}
			}
		}()

		// Run the actual check
		err := checker.Check(ctx)

		result := CheckResult{
			Name:     name,
			Healthy:  err == nil,
			Duration: time.Since(start),
		}

		if err != nil {
			result.Message = err.Error()
		}

		// Non-blocking send
		select {
		case resultChan <- result:
		case <-ctx.Done():
			// Context cancelled, don't block
		}
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		return result
	case <-ctx.Done():
		a.logger.Warn("health check timeout",
			"checker", name,
			"timeout", a.timeout,
		)
		return CheckResult{
			Name:     name,
			Healthy:  false,
			Message:  "health check timeout",
			Duration: time.Since(start),
		}
	}
}

// This always returns healthy as long as the process is running
func (a *Aggregator) CheckLiveness(ctx context.Context) AggregatedHealth {
	return AggregatedHealth{
		Status:  "healthy",
		Checks:  []CheckResult{},
		Overall: true,
	}
}

// CheckReadiness runs all checks to determine if service is ready to serve traffic
func (a *Aggregator) CheckReadiness(ctx context.Context) AggregatedHealth {
	return a.CheckAll(ctx)
}

func checkOverallHealth(results []CheckResult) bool {
	for _, r := range results {
		if !r.Healthy {
			return false
		}
	}
	return true
}

// countFailedChecks counts how many checks failed
func countFailedChecks(results []CheckResult) int {
	count := 0
	for _, r := range results {
		if !r.Healthy {
			count++
		}
	}
	return count
}
