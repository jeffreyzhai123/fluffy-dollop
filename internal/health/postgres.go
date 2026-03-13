package health

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

type PostgresChecker struct {
	pool   *pgxpool.Pool
	logger *observability.Logger // might add Checker logging attempts later
}

func NewPostgresChecker(pool *pgxpool.Pool, logger *observability.Logger) *PostgresChecker {
	return &PostgresChecker{
		pool:   pool,
		logger: logger,
	}
}

func (p *PostgresChecker) Check(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping failed: %w", err)
	}
	return nil
}

func (p *PostgresChecker) Name() string {
	return "postgres"
}
