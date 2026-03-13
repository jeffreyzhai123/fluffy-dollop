package testutil

import "context"

// NoopHealthServer is a no-op implementation for tests
type NoopHealthServer struct{}

func (n *NoopHealthServer) Start(ctx context.Context) error {
	<-ctx.Done() // block until cancelled, then return cleanly
	return nil
}

// NewNoopHealthServer creates a noop health server for tests
func NewNoopHealthServer() *NoopHealthServer {
	return &NoopHealthServer{}
}
