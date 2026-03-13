package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/routing"
)

type RoutingSnapshotRepository struct {
	db *sql.DB
}

func NewRoutingSnapshotRepository(db *sql.DB) *RoutingSnapshotRepository {
	return &RoutingSnapshotRepository{db: db}
}

func (r *RoutingSnapshotRepository) EnsureSchema(ctx context.Context) error {
	const manifestDDL = `
CREATE TABLE IF NOT EXISTS routing_snapshot_manifests (
    version VARCHAR(64) PRIMARY KEY,
    payload JSON NOT NULL,
    generated_at DATETIME(6) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    KEY idx_generated_at (generated_at)
)`
	if _, err := r.db.ExecContext(ctx, manifestDDL); err != nil {
		return fmt.Errorf("create routing_snapshot_manifests table: %w", err)
	}

	const bundleDDL = `
CREATE TABLE IF NOT EXISTS routing_snapshot_bundles (
    version VARCHAR(64) NOT NULL,
    region_name VARCHAR(64) NOT NULL,
    payload JSON NOT NULL,
    generated_at DATETIME(6) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (version, region_name),
    KEY idx_generated_at (generated_at)
)`
	if _, err := r.db.ExecContext(ctx, bundleDDL); err != nil {
		return fmt.Errorf("create routing_snapshot_bundles table: %w", err)
	}

	return nil
}

func (r *RoutingSnapshotRepository) SaveManifest(ctx context.Context, manifest routing.Manifest) error {
	payload, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal routing manifest: %w", err)
	}

	const query = `
INSERT INTO routing_snapshot_manifests (
    version,
    payload,
    generated_at,
    expires_at
) VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    payload = VALUES(payload),
    generated_at = VALUES(generated_at),
    expires_at = VALUES(expires_at)`

	if _, err := r.db.ExecContext(
		ctx,
		query,
		manifest.Version,
		payload,
		manifest.GeneratedAt,
		manifest.ExpiresAt,
	); err != nil {
		return fmt.Errorf("save routing manifest: %w", err)
	}
	return nil
}

func (r *RoutingSnapshotRepository) SaveBundles(ctx context.Context, bundles []routing.Bundle) error {
	if len(bundles) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin routing bundle tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const query = `
INSERT INTO routing_snapshot_bundles (
    version,
    region_name,
    payload,
    generated_at,
    expires_at
) VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    payload = VALUES(payload),
    generated_at = VALUES(generated_at),
    expires_at = VALUES(expires_at)`

	for _, bundle := range bundles {
		payload, err := json.Marshal(bundle)
		if err != nil {
			return fmt.Errorf("marshal routing bundle: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			query,
			bundle.Version,
			bundle.Region,
			payload,
			bundle.GeneratedAt,
			bundle.ExpiresAt,
		); err != nil {
			return fmt.Errorf("save routing bundle: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit routing bundle tx: %w", err)
	}
	return nil
}

func (r *RoutingSnapshotRepository) GetLatestManifest(ctx context.Context) (*routing.Manifest, error) {
	const query = `
SELECT payload
FROM routing_snapshot_manifests
ORDER BY generated_at DESC
LIMIT 1`

	var payload []byte
	if err := r.db.QueryRowContext(ctx, query).Scan(&payload); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest routing manifest: %w", err)
	}

	return unmarshalManifest(payload)
}

func (r *RoutingSnapshotRepository) GetManifestByVersion(ctx context.Context, version string) (*routing.Manifest, error) {
	const query = `
SELECT payload
FROM routing_snapshot_manifests
WHERE version = ?
LIMIT 1`

	var payload []byte
	if err := r.db.QueryRowContext(ctx, query, version).Scan(&payload); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get routing manifest by version: %w", err)
	}

	return unmarshalManifest(payload)
}

func (r *RoutingSnapshotRepository) ListBundlesByVersion(ctx context.Context, version string) ([]routing.Bundle, error) {
	const query = `
SELECT payload
FROM routing_snapshot_bundles
WHERE version = ?
ORDER BY region_name ASC`

	rows, err := r.db.QueryContext(ctx, query, version)
	if err != nil {
		return nil, fmt.Errorf("list routing bundles by version: %w", err)
	}
	defer rows.Close()

	var bundles []routing.Bundle
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan routing bundle payload: %w", err)
		}
		var bundle routing.Bundle
		if err := json.Unmarshal(payload, &bundle); err != nil {
			return nil, fmt.Errorf("unmarshal routing bundle: %w", err)
		}
		bundles = append(bundles, bundle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate routing bundles: %w", err)
	}
	return bundles, nil
}

func unmarshalManifest(payload []byte) (*routing.Manifest, error) {
	var manifest routing.Manifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal routing manifest: %w", err)
	}
	return &manifest, nil
}

func marshalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value
}
