CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS log_metrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    service VARCHAR(255) NOT NULL,
    level VARCHAR(50) NOT NULL,
    count BIGINT NOT NULL DEFAULT 0,
    message TEXT NOT NULL,
    window_start TIMESTAMP NOT NULL,
    window_end TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(service, level, window_start, window_end)
);

CREATE INDEX idx_log_metrics_service ON log_metrics(service);
CREATE INDEX idx_log_metrics_level ON log_metrics(level);
CREATE INDEX idx_log_metrics_time ON log_metrics(window_start, window_end);