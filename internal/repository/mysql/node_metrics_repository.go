package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type NodeMetricsRepository struct {
	db *sql.DB
}

func NewNodeMetricsRepository(db *sql.DB) *NodeMetricsRepository {
	return &NodeMetricsRepository{db: db}
}

func (r *NodeMetricsRepository) EnsureSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS managed_node_metrics_ext (
    node_uuid VARCHAR(36) PRIMARY KEY,
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
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    KEY idx_metrics_updated_at (updated_at)
)`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create managed_node_metrics_ext table: %w", err)
	}
	return nil
}

func (r *NodeMetricsRepository) Upsert(ctx context.Context, item node.Node) error {
	const query = `
INSERT INTO managed_node_metrics_ext (
    node_uuid,
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
    monitoring_snapshot
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
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
    updated_at = CURRENT_TIMESTAMP(6)`
	if _, err := r.db.ExecContext(
		ctx,
		query,
		item.UUID,
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
	); err != nil {
		return fmt.Errorf("upsert managed node metrics: %w", err)
	}
	return nil
}

func (r *NodeMetricsRepository) DeleteByNodeUUID(ctx context.Context, nodeUUID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM managed_node_metrics_ext WHERE node_uuid = ?`, strings.TrimSpace(nodeUUID)); err != nil {
		return fmt.Errorf("delete managed node metrics: %w", err)
	}
	return nil
}

func (r *NodeMetricsRepository) DeleteByRegionUUID(ctx context.Context, regionUUID string) error {
	if _, err := r.db.ExecContext(
		ctx,
		`DELETE m FROM managed_node_metrics_ext m
		  INNER JOIN managed_nodes n ON n.uuid = m.node_uuid
		  WHERE n.region_uuid = ?`,
		regionUUID,
	); err != nil {
		return fmt.Errorf("delete managed node metrics by region: %w", err)
	}
	return nil
}

func (r *NodeMetricsRepository) DeleteByZoneUUID(ctx context.Context, zoneUUID string) error {
	if _, err := r.db.ExecContext(
		ctx,
		`DELETE m FROM managed_node_metrics_ext m
		  INNER JOIN managed_nodes n ON n.uuid = m.node_uuid
		  WHERE n.zone_uuid = ?`,
		zoneUUID,
	); err != nil {
		return fmt.Errorf("delete managed node metrics by zone: %w", err)
	}
	return nil
}

func (r *NodeMetricsRepository) LoadByNodeUUIDs(ctx context.Context, nodeUUIDs []string) (map[string]node.Node, error) {
	result := make(map[string]node.Node)
	if len(nodeUUIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, 0, len(nodeUUIDs))
	args := make([]interface{}, 0, len(nodeUUIDs))
	for _, nodeUUID := range nodeUUIDs {
		nodeUUID = strings.TrimSpace(nodeUUID)
		if nodeUUID == "" {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, nodeUUID)
	}
	if len(placeholders) == 0 {
		return result, nil
	}
	query := fmt.Sprintf(`SELECT
    node_uuid,
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
    monitoring_snapshot
FROM managed_node_metrics_ext
WHERE node_uuid IN (%s)`, strings.Join(placeholders, ","))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list managed node metrics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item node.Node
		var diskDetailsRaw []byte
		var monitoringSnapshotRaw []byte
		if err := rows.Scan(
			&item.UUID,
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
			&monitoringSnapshotRaw,
		); err != nil {
			return nil, fmt.Errorf("scan managed node metrics: %w", err)
		}
		if len(diskDetailsRaw) > 0 {
			_ = json.Unmarshal(diskDetailsRaw, &item.DiskDetails)
		}
		if len(monitoringSnapshotRaw) > 0 {
			item.MonitoringSnapshot = append(item.MonitoringSnapshot[:0], monitoringSnapshotRaw...)
		}
		result[item.UUID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed node metrics: %w", err)
	}
	return result, nil
}

func marshalDiskDetails(details []node.DiskDetail) interface{} {
	if len(details) == 0 {
		return nil
	}
	b, err := json.Marshal(details)
	if err != nil {
		return nil
	}
	return b
}

func marshalRawJSON(raw json.RawMessage) interface{} {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}
