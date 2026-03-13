package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/observation"
)

type ObservationRepository struct {
	db *sql.DB
}

func NewObservationRepository(db *sql.DB) *ObservationRepository {
	return &ObservationRepository{db: db}
}

func (r *ObservationRepository) EnsureSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS routing_observations (
    source_colo VARCHAR(32) NOT NULL,
    target_node_uuid VARCHAR(36) NOT NULL,
    success_latency_sum_ms DOUBLE NOT NULL DEFAULT 0,
    success_count BIGINT NOT NULL DEFAULT 0,
    error_count BIGINT NOT NULL DEFAULT 0,
    sample_count BIGINT NOT NULL DEFAULT 0,
    last_observed_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (source_colo, target_node_uuid),
    KEY idx_target_node_uuid (target_node_uuid),
    KEY idx_last_observed_at (last_observed_at)
)`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create routing_observations table: %w", err)
	}
	return nil
}

func (r *ObservationRepository) UpsertMany(ctx context.Context, items []observation.RecordInput) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin observation tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const query = `
INSERT INTO routing_observations (
    source_colo,
    target_node_uuid,
    success_latency_sum_ms,
    success_count,
    error_count,
    sample_count,
    last_observed_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    success_latency_sum_ms = success_latency_sum_ms + VALUES(success_latency_sum_ms),
    success_count = success_count + VALUES(success_count),
    error_count = error_count + VALUES(error_count),
    sample_count = sample_count + VALUES(sample_count),
    last_observed_at = GREATEST(last_observed_at, VALUES(last_observed_at)),
    updated_at = CURRENT_TIMESTAMP(6)`

	for _, item := range items {
		sourceColo := strings.TrimSpace(strings.ToUpper(item.SourceColo))
		targetNodeUUID := strings.TrimSpace(item.TargetNodeUUID)
		if sourceColo == "" || targetNodeUUID == "" {
			continue
		}
		observedAt := item.ObservedAt
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		sampleCount := item.SampleCount
		if sampleCount <= 0 {
			sampleCount = 1
		}

		successCount := int64(0)
		errorCount := sampleCount
		successLatencySumMs := 0.0
		if item.Success {
			successCount = sampleCount
			errorCount = 0
			if item.LatencyMs > 0 {
				successLatencySumMs = item.LatencyMs * float64(sampleCount)
			}
		}

		if _, err := tx.ExecContext(
			ctx,
			query,
			sourceColo,
			targetNodeUUID,
			successLatencySumMs,
			successCount,
			errorCount,
			sampleCount,
			observedAt,
		); err != nil {
			return fmt.Errorf("upsert routing observation: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit observation tx: %w", err)
	}
	return nil
}

func (r *ObservationRepository) List(ctx context.Context) ([]observation.Aggregate, error) {
	const query = `
SELECT
    source_colo,
    target_node_uuid,
    success_latency_sum_ms,
    success_count,
    error_count,
    sample_count,
    last_observed_at,
    created_at,
    updated_at
FROM routing_observations
ORDER BY source_colo ASC, target_node_uuid ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list routing observations: %w", err)
	}
	defer rows.Close()

	var items []observation.Aggregate
	for rows.Next() {
		var item observation.Aggregate
		if err := rows.Scan(
			&item.SourceColo,
			&item.TargetNodeUUID,
			&item.SuccessLatencySumMs,
			&item.SuccessCount,
			&item.ErrorCount,
			&item.SampleCount,
			&item.LastObservedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan routing observation: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate routing observations: %w", err)
	}
	return items, nil
}
