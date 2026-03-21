package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type AggregatedNodeRepository struct {
	db *sql.DB
}

func NewAggregatedNodeRepository(db *sql.DB) *AggregatedNodeRepository {
	return &AggregatedNodeRepository{db: db}
}

func (r *AggregatedNodeRepository) EnsureSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS aggregated_node_status (
    node_uuid VARCHAR(36) PRIMARY KEY,
    hostname VARCHAR(255) NOT NULL,
    management_address VARCHAR(255) NOT NULL,
    internal_ip VARCHAR(64) DEFAULT '',
    public_ip VARCHAR(64) DEFAULT '',
    dns_label VARCHAR(64) NULL DEFAULT NULL,
    dns_name VARCHAR(255) NULL DEFAULT NULL,
    region VARCHAR(64) NOT NULL,
    region_uuid VARCHAR(36) NULL DEFAULT NULL,
    zone VARCHAR(16) NOT NULL DEFAULT '',
    zone_uuid VARCHAR(36) NULL DEFAULT NULL,
    status VARCHAR(16) NOT NULL,
    home_region VARCHAR(64) NOT NULL,
    status_source_region VARCHAR(64) NOT NULL,
    observed_at DATETIME(6) NULL,
    stale TINYINT(1) NOT NULL DEFAULT 0,
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
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    last_seen_at DATETIME(6) NULL,
    last_reported_at DATETIME(6) NULL,
    KEY idx_region_status (region, status),
    KEY idx_region_zone_status (region, zone, status),
    KEY idx_home_region_status (home_region, status),
    KEY idx_updated_at (updated_at)
)`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create aggregated_node_status table: %w", err)
	}
	return nil
}

func (r *AggregatedNodeRepository) ReplaceAll(ctx context.Context, items []node.AggregatedNodeStatus) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin aggregated node replace tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM aggregated_node_status`); err != nil {
		return fmt.Errorf("clear aggregated_node_status: %w", err)
	}
	if len(items) == 0 {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit empty aggregated node replace tx: %w", err)
		}
		return nil
	}

	const query = `
INSERT INTO aggregated_node_status (
    node_uuid, hostname, management_address, internal_ip, public_ip, dns_label, dns_name,
    region, region_uuid, zone, zone_uuid, status, home_region, status_source_region,
    observed_at, stale, cpu_cores, cpu_usage_percent, memory_total_gb, memory_used_gb,
    memory_usage_percent, swap_total_gb, swap_used_gb, swap_usage_percent, disk_usage_percent,
    disk_details, monitoring_snapshot, created_at, updated_at, last_seen_at, last_reported_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), ?, ?)`
	for _, item := range items {
		if _, err := tx.ExecContext(
			ctx,
			query,
			item.UUID,
			item.Hostname,
			item.ManagementAddress,
			item.InternalIP,
			item.PublicIP,
			nullString(item.DNSLabel),
			nullString(item.DNSName),
			item.Region,
			nullString(item.RegionUUID),
			item.Zone,
			nullString(item.ZoneUUID),
			item.Status,
			item.HomeRegion,
			item.StatusSourceRegion,
			nullTime(item.ObservedAt),
			item.Stale,
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
		); err != nil {
			return fmt.Errorf("insert aggregated_node_status: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit aggregated node replace tx: %w", err)
	}
	return nil
}

func (r *AggregatedNodeRepository) List(ctx context.Context) ([]node.Node, error) {
	rows, err := r.db.QueryContext(ctx, aggregatedNodeSelect+` ORDER BY region ASC, zone ASC, hostname ASC, node_uuid ASC`)
	if err != nil {
		return nil, fmt.Errorf("list aggregated nodes: %w", err)
	}
	defer rows.Close()
	return scanAggregatedNodeRows(rows)
}

func (r *AggregatedNodeRepository) FindByUUID(ctx context.Context, uuid string) (*node.Node, error) {
	row := r.db.QueryRowContext(ctx, aggregatedNodeSelect+` WHERE node_uuid = ? LIMIT 1`, strings.TrimSpace(uuid))
	item, err := scanAggregatedNodeRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find aggregated node by uuid: %w", err)
	}
	return item, nil
}

func (r *AggregatedNodeRepository) ListRegions(ctx context.Context) ([]node.RegionSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
    region,
    COUNT(*) AS total,
    SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) AS up_count,
    SUM(CASE WHEN status = 'down' THEN 1 ELSE 0 END) AS down_count
FROM aggregated_node_status
GROUP BY region
ORDER BY region ASC`)
	if err != nil {
		return nil, fmt.Errorf("list aggregated region summaries: %w", err)
	}
	defer rows.Close()
	var items []node.RegionSummary
	for rows.Next() {
		var item node.RegionSummary
		if err := rows.Scan(&item.Region, &item.Total, &item.UpCount, &item.DownCount); err != nil {
			return nil, fmt.Errorf("scan aggregated region summary: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate aggregated region summaries: %w", err)
	}
	return items, nil
}

func (r *AggregatedNodeRepository) ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
    region,
    zone,
    COUNT(*) AS total,
    SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) AS up_count,
    SUM(CASE WHEN status = 'down' THEN 1 ELSE 0 END) AS down_count
FROM aggregated_node_status
GROUP BY region, zone
ORDER BY region ASC, zone ASC`)
	if err != nil {
		return nil, fmt.Errorf("list aggregated region zone summaries: %w", err)
	}
	defer rows.Close()
	var items []node.RegionZoneSummary
	for rows.Next() {
		var item node.RegionZoneSummary
		if err := rows.Scan(&item.Region, &item.Zone, &item.Total, &item.UpCount, &item.DownCount); err != nil {
			return nil, fmt.Errorf("scan aggregated region zone summary: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate aggregated region zone summaries: %w", err)
	}
	return items, nil
}

func (r *AggregatedNodeRepository) GetMonitoringSnapshot(ctx context.Context, uuid string) (json.RawMessage, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT monitoring_snapshot FROM aggregated_node_status WHERE node_uuid = ? LIMIT 1`, strings.TrimSpace(uuid)).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("node not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get aggregated monitoring snapshot: %w", err)
	}
	return append(json.RawMessage(nil), raw...), nil
}

const aggregatedNodeSelect = `
SELECT
    node_uuid,
    hostname,
    management_address,
    internal_ip,
    public_ip,
    dns_label,
    dns_name,
    region,
    region_uuid,
    zone,
    zone_uuid,
    status,
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
    created_at,
    updated_at,
    last_seen_at,
    last_reported_at
FROM aggregated_node_status`

func scanAggregatedNodeRow(src scanner) (*node.Node, error) {
	var item node.Node
	var dnsLabel sql.NullString
	var dnsName sql.NullString
	var regionUUID sql.NullString
	var zoneUUID sql.NullString
	var diskDetailsRaw []byte
	var monitoringRaw []byte
	var lastSeenAt sql.NullTime
	var lastReportedAt sql.NullTime
	if err := src.Scan(
		&item.UUID,
		&item.Hostname,
		&item.ManagementAddress,
		&item.InternalIP,
		&item.PublicIP,
		&dnsLabel,
		&dnsName,
		&item.Region,
		&regionUUID,
		&item.Zone,
		&zoneUUID,
		&item.Status,
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
		&item.CreatedAt,
		&item.UpdatedAt,
		&lastSeenAt,
		&lastReportedAt,
	); err != nil {
		return nil, err
	}
	if dnsLabel.Valid {
		item.DNSLabel = dnsLabel.String
	}
	if dnsName.Valid {
		item.DNSName = dnsName.String
	}
	if regionUUID.Valid {
		item.RegionUUID = regionUUID.String
	}
	if zoneUUID.Valid {
		item.ZoneUUID = zoneUUID.String
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
	return &item, nil
}

func scanAggregatedNodeRows(rows *sql.Rows) ([]node.Node, error) {
	var items []node.Node
	for rows.Next() {
		item, err := scanAggregatedNodeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan aggregated node: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate aggregated nodes: %w", err)
	}
	return items, nil
}
