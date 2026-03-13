package bootstrap

import (
	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/registry"
	"github.com/redis/go-redis/v9"
)

// SetupRegistry creates a Registry backed by the given Redis client.
// Used by both the worker and the control plane binary.
func SetupRegistry(
	client *redis.Client,
	cfg *config.Config,
	logger *observability.Logger,
) *registry.Registry {
	return registry.NewRegistry(client, &cfg.Registry, logger)
}

// SetupControlPlane creates a ControlPlane using the given Registry.
// Only called by cmd/controlplane/main.go.
func SetupControlPlane(
	reg *registry.Registry,
	cfg *config.Config,
	logger *observability.Logger,
) *registry.ControlPlane {
	return registry.NewControlPlane(reg, &cfg.Registry, logger)
}
