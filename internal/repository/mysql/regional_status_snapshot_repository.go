package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type RegionalNodeStatusSnapshotRepository struct {
	db *sql.DB
}

func NewRegionalNodeStatusSnapshotRepository(db *sql.DB) *RegionalNodeStatusSnapshotRepository {
	return &RegionalNodeStatusSnapshotRepository{db: db}
}

func (r *RegionalNodeStatusSnapshotRepository) EnsureSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS regional_node_status_snapshots (
    source_region VARCHAR(64) NOT NULL,
    node_uuid VARCHAR(36) NOT NULL,
    home_region VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL,
    status_reason VARCHAR(512) NOT NULL DEFAULT '',
    snapshot_version VARCHAR(64) NOT NULL,
    internal_ip VARCHAR(64) DEFAULT '',
    public_ip VARCHAR(64) DEFAULT '',
    cpu_cores INT NOT NULL DEFAULT 0,
    cpu_usage_percent DOUBLE NOT NULL DEFAULT 0,
    memory_total_gb DOUBLE NOT NULL DEFAULT 0,
    memory_used_gb DOUBLE NOT NULL DEFAULT 0,
    memory_usage_percent DOUBLE NOT NULL DEFAULT 0,
    swap_total_gb DOUBLE NOT NULL DEFAULT 0,
    swap_used_gb DOUBLE NOT NULL DEFAULT 0,
    swap_usage_percent DOUBLE NOT NULL DEFAULT 0,
    disk_usage_percent DOUBLE NOT NULL DEFAULT 0,
    disk_details JSON NULL,
    monitoring_snapshot JSON NULL,
    last_seen_at DATETIME(6) NULL,
    last_reported_at DATETIME(6) NULL,
    observed_at DATETIME(6) NOT NULL,
    payload JSON NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (source_region, node_uuid),
    KEY idx_home_region_status (home_region, status),
    KEY idx_observed_at (observed_at)
)`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create regional_node_status_snapshots table: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE regional_node_status_snapshots ADD COLUMN IF NOT EXISTS status_reason VARCHAR(512) NOT NULL DEFAULT '' AFTER status`); err != nil {
		return fmt.Errorf("upgrade regional_node_status_snapshots status_reason: %w", err)
	}
	return nil
}

func (r *RegionalNodeStatusSnapshotRepository) UpsertSnapshot(ctx context.Context, snapshot node.RegionalNodeStatusSnapshot) error {
	if len(snapshot.Nodes) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin regional snapshot tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const query = `
INSERT INTO regional_node_status_snapshots (
    source_region,
    node_uuid,
    home_region,
    status,
    status_reason,
    snapshot_version,
    internal_ip,
    public_ip,
    cpu_cores,
    cpu_usage_percent,
    memory_total_gb,
    memory_used_gb,
    memory_usage_percent,
    swap_total_gb,
    swap_used_gb,
    swap_usage_percent,
    disk_usage_percent,
    disk_details,
    monitoring_snapshot,
    last_seen_at,
    last_reported_at,
    observed_at,
    payload
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    home_region = VALUES(home_region),
    status = VALUES(status),
    status_reason = VALUES(status_reason),
    snapshot_version = VALUES(snapshot_version),
    internal_ip = VALUES(internal_ip),
    public_ip = VALUES(public_ip),
    cpu_cores = VALUES(cpu_cores),
    cpu_usage_percent = VALUES(cpu_usage_percent),
    memory_total_gb = VALUES(memory_total_gb),
    memory_used_gb = VALUES(memory_used_gb),
    memory_usage_percent = VALUES(memory_usage_percent),
    swap_total_gb = VALUES(swap_total_gb),
    swap_used_gb = VALUES(swap_used_gb),
    swap_usage_percent = VALUES(swap_usage_percent),
    disk_usage_percent = VALUES(disk_usage_percent),
    disk_details = VALUES(disk_details),
    monitoring_snapshot = VALUES(monitoring_snapshot),
    last_seen_at = VALUES(last_seen_at),
    last_reported_at = VALUES(last_reported_at),
    observed_at = VALUES(observed_at),
    payload = VALUES(payload),
    updated_at = CURRENT_TIMESTAMP(6)`

	for _, item := range snapshot.Nodes {
		sourceRegion := strings.TrimSpace(strings.ToLower(snapshot.SourceRegion))
		homeRegion := strings.TrimSpace(strings.ToLower(item.HomeRegion))
		if sourceRegion == "" || item.NodeUUID == "" || homeRegion == "" {
			continue
		}
		observedAt := item.ObservedAt
		if observedAt.IsZero() {
			observedAt = snapshot.ObservedAt
		}
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		payload := marshalRegionalSnapshotPayload(item)
		if _, err := tx.ExecContext(
			ctx,
			query,
			sourceRegion,
			item.NodeUUID,
			homeRegion,
			strings.TrimSpace(strings.ToLower(item.Status)),
			strings.TrimSpace(item.StatusReason),
			snapshot.SnapshotVersion,
			item.InternalIP,
			item.PublicIP,
			item.CPUCores,
			item.CPUUsagePercent,
			item.MemoryTotalGB,
			item.MemoryUsedGB,
			item.MemoryUsagePercent,
			item.SwapTotalGB,
			item.SwapUsedGB,
			item.SwapUsagePercent,
			item.DiskUsagePercent,
			marshalDiskDetails(item.DiskDetails),
			marshalRawJSON(item.MonitoringSnapshot),
			nullTime(item.LastSeenAt),
			nullTime(item.LastReportedAt),
			observedAt,
			payload,
		); err != nil {
			return fmt.Errorf("upsert regional node status snapshot: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit regional snapshot tx: %w", err)
	}
	return nil
}

func (r *RegionalNodeStatusSnapshotRepository) ListLatestByRegion(ctx context.Context, sourceRegion string) ([]node.RegionalNodeStatus, error) {
	const query = `
SELECT
    source_region,
    node_uuid,
    home_region,
    status,
    status_reason,
    internal_ip,
    public_ip,
    cpu_cores,
    cpu_usage_percent,
    memory_total_gb,
    memory_used_gb,
    memory_usage_percent,
    swap_total_gb,
    swap_used_gb,
    swap_usage_percent,
    disk_usage_percent,
    disk_details,
    monitoring_snapshot,
    last_seen_at,
    last_reported_at,
    observed_at
FROM regional_node_status_snapshots
WHERE source_region = ?
ORDER BY node_uuid ASC`
	return r.listSnapshots(ctx, query, strings.TrimSpace(strings.ToLower(sourceRegion)))
}

func (r *RegionalNodeStatusSnapshotRepository) ListLatest(ctx context.Context) ([]node.RegionalNodeStatus, error) {
	const query = `
SELECT
    source_region,
    node_uuid,
    home_region,
    status,
    status_reason,
    internal_ip,
    public_ip,
    cpu_cores,
    cpu_usage_percent,
    memory_total_gb,
    memory_used_gb,
    memory_usage_percent,
    swap_total_gb,
    swap_used_gb,
    swap_usage_percent,
    disk_usage_percent,
    disk_details,
    monitoring_snapshot,
    last_seen_at,
    last_reported_at,
    observed_at
FROM regional_node_status_snapshots
ORDER BY source_region ASC, node_uuid ASC`
	return r.listSnapshots(ctx, query)
}

func (r *RegionalNodeStatusSnapshotRepository) listSnapshots(ctx context.Context, query string, args ...interface{}) ([]node.RegionalNodeStatus, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list regional node status snapshots: %w", err)
	}
	defer rows.Close()

	var items []node.RegionalNodeStatus
	for rows.Next() {
		var item node.RegionalNodeStatus
		var diskDetailsRaw []byte
		var monitoringRaw []byte
		var lastSeenAt sql.NullTime
		var lastReportedAt sql.NullTime
		if err := rows.Scan(
			&item.SourceRegion,
			&item.NodeUUID,
			&item.HomeRegion,
			&item.Status,
			&item.StatusReason,
			&item.InternalIP,
			&item.PublicIP,
			&item.CPUCores,
			&item.CPUUsagePercent,
			&item.MemoryTotalGB,
			&item.MemoryUsedGB,
			&item.MemoryUsagePercent,
			&item.SwapTotalGB,
			&item.SwapUsedGB,
			&item.SwapUsagePercent,
			&item.DiskUsagePercent,
			&diskDetailsRaw,
			&monitoringRaw,
			&lastSeenAt,
			&lastReportedAt,
			&item.ObservedAt,
		); err != nil {
			return nil, fmt.Errorf("scan regional node status snapshot: %w", err)
		}
		if len(diskDetailsRaw) > 0 {
			_ = json.Unmarshal(diskDetailsRaw, &item.DiskDetails)
		}
		if len(monitoringRaw) > 0 {
			item.MonitoringSnapshot = append(item.MonitoringSnapshot[:0], monitoringRaw...)
		}
		if lastSeenAt.Valid {
			item.LastSeenAt = lastSeenAt.Time
		}
		if lastReportedAt.Valid {
			item.LastReportedAt = lastReportedAt.Time
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate regional node status snapshots: %w", err)
	}
	return items, nil
}

func marshalRegionalSnapshotPayload(item node.RegionalNodeStatus) interface{} {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil
	}
	return raw
}
