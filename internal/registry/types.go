package registry

import "time"

// WorkerInfo holds the registration data for a single worker process.
// Status is intentionally omitted — liveness is derived from LastSeen
// and the caller's dead threshold, not stored as explicit state.
type WorkerInfo struct {
	WorkerID  string
	LastSeen  time.Time
	StartedAt time.Time
}

// IsAlive returns true if the worker has heartbeated within threshold.
func (w *WorkerInfo) IsAlive(threshold time.Duration) bool {
	return time.Since(w.LastSeen) < threshold
}
