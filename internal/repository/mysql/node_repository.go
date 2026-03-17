package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/node"
	mysqldriver "github.com/go-sql-driver/mysql"
)

type NodeRepository struct {
	db *sql.DB
}

func NewNodeRepository(db *sql.DB) *NodeRepository {
	return &NodeRepository{db: db}
}

func (r *NodeRepository) EnsureSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS managed_nodes (
    uuid VARCHAR(36) PRIMARY KEY,
    hostname VARCHAR(255) NOT NULL,
    management_address VARCHAR(255) NOT NULL,
    internal_ip VARCHAR(64) DEFAULT '',
    public_ip VARCHAR(64) DEFAULT '',
    dns_label VARCHAR(64) NULL DEFAULT NULL,
    dns_name VARCHAR(255) NULL DEFAULT NULL,
    region VARCHAR(64) NOT NULL,
    zone VARCHAR(16) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL,
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
    last_seen_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    last_reported_at DATETIME(6) NULL,
    UNIQUE KEY uk_management_address (management_address),
    UNIQUE KEY uk_dns_label (dns_label),
    UNIQUE KEY uk_dns_name (dns_name),
    KEY idx_region_status (region, status),
    KEY idx_region_zone_status (region, zone, status),
    KEY idx_last_seen_at (last_seen_at)
)`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create managed_nodes table: %w", err)
	}
	const historyDDL = `
CREATE TABLE IF NOT EXISTS managed_nodes_history (
    history_id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    uuid VARCHAR(36) NOT NULL,
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
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    last_seen_at DATETIME(6) NOT NULL,
    last_reported_at DATETIME(6) NULL,
    archived_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    archive_reason VARCHAR(64) NOT NULL,
    replaced_by_uuid VARCHAR(36) NULL DEFAULT NULL,
    KEY idx_history_management_address (management_address),
    KEY idx_history_uuid (uuid),
    KEY idx_history_archived_at (archived_at)
)`
	if _, err := r.db.ExecContext(ctx, historyDDL); err != nil {
		return fmt.Errorf("create managed_nodes_history table: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS cpu_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS memory_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS disk_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS last_reported_at DATETIME(6) NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS internal_ip VARCHAR(64) DEFAULT ''`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS public_ip VARCHAR(64) DEFAULT ''`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS dns_label VARCHAR(64) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS dns_name VARCHAR(255) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS cpu_cores INT NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS memory_total_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS memory_used_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS swap_total_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS swap_used_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS swap_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS disk_details JSON NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS monitoring_snapshot JSON NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS zone VARCHAR(16) NOT NULL DEFAULT ''`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS region_uuid VARCHAR(36) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS zone_uuid VARCHAR(36) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS internal_ip VARCHAR(64) DEFAULT ''`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS public_ip VARCHAR(64) DEFAULT ''`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS dns_label VARCHAR(64) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS dns_name VARCHAR(255) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS region_uuid VARCHAR(36) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS zone VARCHAR(16) NOT NULL DEFAULT ''`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS zone_uuid VARCHAR(36) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS cpu_cores INT NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS cpu_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS memory_total_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS memory_used_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS memory_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS swap_total_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS swap_used_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS swap_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS disk_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS disk_details JSON NULL`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS monitoring_snapshot JSON NULL`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS archived_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS archive_reason VARCHAR(64) NOT NULL DEFAULT 'unknown'`,
		`ALTER TABLE managed_nodes_history ADD COLUMN IF NOT EXISTS replaced_by_uuid VARCHAR(36) NULL DEFAULT NULL`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("upgrade managed_nodes table: %w", err)
		}
	}
	if err := ensureUniqueIndex(ctx, r.db, "managed_nodes", "uk_dns_label", "dns_label"); err != nil {
		return err
	}
	if err := ensureUniqueIndex(ctx, r.db, "managed_nodes", "uk_dns_name", "dns_name"); err != nil {
		return err
	}
	if err := ensureIndex(ctx, r.db, "managed_nodes", "idx_region_zone_status", "region, zone, status"); err != nil {
		return err
	}
	if err := ensureIndex(ctx, r.db, "managed_nodes", "idx_region_uuid", "region_uuid"); err != nil {
		return err
	}
	if err := ensureIndex(ctx, r.db, "managed_nodes", "idx_zone_uuid", "zone_uuid"); err != nil {
		return err
	}
	return nil
}

const selectNodeColumns = `
    uuid,
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
    last_reported_at`

func (r *NodeRepository) FindByManagementAddress(ctx context.Context, managementAddress string) (*node.Node, error) {
	return r.FindActiveByManagementAddress(ctx, managementAddress)
}

func (r *NodeRepository) FindActiveByManagementAddress(ctx context.Context, managementAddress string) (*node.Node, error) {
	const query = `SELECT` + selectNodeColumns + `
FROM managed_nodes
WHERE management_address = ?
LIMIT 1`

	var item node.Node
	err := scanNode(r.db.QueryRowContext(ctx, query, managementAddress), &item)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active managed node by management address: %w", err)
	}

	return &item, nil
}

func (r *NodeRepository) FindByUUID(ctx context.Context, uuid string) (*node.Node, error) {
	const query = `SELECT` + selectNodeColumns + `
FROM managed_nodes
WHERE uuid = ?
LIMIT 1`

	var item node.Node
	err := scanNode(r.db.QueryRowContext(ctx, query, uuid), &item)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find managed node by uuid: %w", err)
	}

	return &item, nil
}

func (r *NodeRepository) Upsert(ctx context.Context, item node.Node) error {
	// region/zone text columns remain the hot-path read model for scheduling and routing,
	// while region_uuid/zone_uuid act as relational anchors to static master data.
	const query = `
INSERT INTO managed_nodes (
    uuid, hostname, management_address, region, region_uuid, zone, zone_uuid, status, cpu_usage_percent, memory_usage_percent, disk_usage_percent, created_at, updated_at, last_seen_at, last_reported_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), ?
)
ON DUPLICATE KEY UPDATE
    hostname = VALUES(hostname),
    management_address = VALUES(management_address),
    region = VALUES(region),
    region_uuid = VALUES(region_uuid),
    zone = VALUES(zone),
    zone_uuid = VALUES(zone_uuid),
    status = VALUES(status),
    cpu_usage_percent = VALUES(cpu_usage_percent),
    memory_usage_percent = VALUES(memory_usage_percent),
    disk_usage_percent = VALUES(disk_usage_percent),
    updated_at = CURRENT_TIMESTAMP(6),
    last_seen_at = CURRENT_TIMESTAMP(6),
    last_reported_at = VALUES(last_reported_at)`

	if _, err := r.db.ExecContext(
		ctx,
		query,
		item.UUID,
		item.Hostname,
		item.ManagementAddress,
		item.Region,
		nullString(item.RegionUUID),
		item.Zone,
		nullString(item.ZoneUUID),
		item.Status,
		item.CPUUsagePercent,
		item.MemoryUsagePercent,
		item.DiskUsagePercent,
		nullTime(item.LastReportedAt),
	); err != nil {
		return fmt.Errorf("upsert managed node: %w", err)
	}
	return nil
}

func (r *NodeRepository) UpdateHeartbeat(ctx context.Context, item node.Node) error {
	diskDetailsJSON := marshalDiskDetails(item.DiskDetails)
	monitoringSnapshotJSON := marshalRawJSON(item.MonitoringSnapshot)
	// dns_label/dns_name remain display mirrors on managed_nodes. The authoritative
	// DNS mapping lives in dns_bindings and is only mirrored back after successful
	// binding updates.
	const query = `
UPDATE managed_nodes
SET
    hostname = ?,
    management_address = ?,
    internal_ip = ?,
    public_ip = ?,
    region = ?,
    region_uuid = ?,
    zone = ?,
    zone_uuid = ?,
    status = ?,
    cpu_cores = ?,
    cpu_usage_percent = ?,
    memory_total_gb = ?,
    memory_used_gb = ?,
    memory_usage_percent = ?,
    swap_total_gb = ?,
    swap_used_gb = ?,
    swap_usage_percent = ?,
    disk_usage_percent = ?,
    disk_details = ?,
    monitoring_snapshot = ?,
    updated_at = CURRENT_TIMESTAMP(6),
    last_seen_at = CURRENT_TIMESTAMP(6),
    last_reported_at = CURRENT_TIMESTAMP(6)
WHERE uuid = ?`

	result, err := r.db.ExecContext(
		ctx,
		query,
		item.Hostname,
		item.ManagementAddress,
		item.InternalIP,
		item.PublicIP,
		item.Region,
		nullString(item.RegionUUID),
		item.Zone,
		nullString(item.ZoneUUID),
		item.Status,
		item.CPUCores,
		item.CPUUsagePercent,
		item.MemoryTotalGB,
		item.MemoryUsedGB,
		item.MemoryUsagePercent,
		item.SwapTotalGB,
		item.SwapUsedGB,
		item.SwapUsagePercent,
		item.DiskUsagePercent,
		diskDetailsJSON,
		monitoringSnapshotJSON,
		item.UUID,
	)
	if err != nil {
		return fmt.Errorf("update managed node heartbeat: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update managed node heartbeat rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *NodeRepository) SaveDNSBinding(ctx context.Context, uuid string, label string, name string) error {
	uuid = strings.TrimSpace(uuid)
	label = strings.TrimSpace(label)
	name = strings.TrimSpace(name)
	if uuid == "" {
		return fmt.Errorf("uuid is required")
	}
	if label == "" {
		return fmt.Errorf("dns label is required")
	}
	if name == "" {
		return fmt.Errorf("dns name is required")
	}

	result, err := r.db.ExecContext(
		ctx,
		`UPDATE managed_nodes
		 SET dns_label = ?, dns_name = ?, updated_at = CURRENT_TIMESTAMP(6)
		 WHERE uuid = ?`,
		label,
		name,
		uuid,
	)
	if err != nil {
		return fmt.Errorf("save managed node dns binding: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("save managed node dns binding rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *NodeRepository) ArchiveAndDeleteByManagementAddress(ctx context.Context, managementAddress string, replacedByUUID string, reason string) error {
	managementAddress = strings.TrimSpace(managementAddress)
	replacedByUUID = strings.TrimSpace(replacedByUUID)
	reason = strings.TrimSpace(reason)
	if managementAddress == "" {
		return fmt.Errorf("management address is required")
	}
	if reason == "" {
		reason = "replaced"
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin archive managed node tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	insertResult, err := tx.ExecContext(
		ctx,
		`INSERT INTO managed_nodes_history (
		     uuid, hostname, management_address, internal_ip, public_ip, dns_label, dns_name,
		     region, region_uuid, zone, zone_uuid, status, cpu_cores, cpu_usage_percent,
		     memory_total_gb, memory_used_gb, memory_usage_percent, swap_total_gb, swap_used_gb,
		     swap_usage_percent, disk_usage_percent, disk_details, monitoring_snapshot,
		     created_at, updated_at, last_seen_at, last_reported_at, archived_at,
		     archive_reason, replaced_by_uuid
		 )
		 SELECT
		     uuid, hostname, management_address, internal_ip, public_ip, dns_label, dns_name,
		     region, region_uuid, zone, zone_uuid, status, cpu_cores, cpu_usage_percent,
		     memory_total_gb, memory_used_gb, memory_usage_percent, swap_total_gb, swap_used_gb,
		     swap_usage_percent, disk_usage_percent, disk_details, monitoring_snapshot,
		     created_at, updated_at, last_seen_at, last_reported_at, CURRENT_TIMESTAMP(6),
		     ?, NULLIF(?, '')
		 FROM managed_nodes
		 WHERE management_address = ?`,
		reason,
		replacedByUUID,
		managementAddress,
	)
	if err != nil {
		return fmt.Errorf("archive managed node by management address: %w", err)
	}

	rowsAffected, err := insertResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("archive managed node rows affected: %w", err)
	}
	if rowsAffected == 0 {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit empty archive managed node tx: %w", err)
		}
		tx = nil
		return nil
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM managed_nodes WHERE management_address = ?`, managementAddress); err != nil {
		return fmt.Errorf("delete managed node by management address: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit archive managed node tx: %w", err)
	}
	tx = nil
	return nil
}

func (r *NodeRepository) List(ctx context.Context) ([]node.Node, error) {
	const query = `SELECT` + selectNodeColumns + `
FROM managed_nodes
ORDER BY region ASC, zone ASC, hostname ASC, uuid ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list managed nodes: %w", err)
	}
	defer rows.Close()

	var items []node.Node
	for rows.Next() {
		var item node.Node
		if err := scanNode(rows, &item); err != nil {
			return nil, fmt.Errorf("scan managed node: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed nodes: %w", err)
	}

	return items, nil
}

func (r *NodeRepository) DeleteByUUID(ctx context.Context, uuid string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM managed_nodes WHERE uuid = ?`, uuid)
	if err != nil {
		return false, fmt.Errorf("delete managed node: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete managed node rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

func (r *NodeRepository) UpdateStatus(ctx context.Context, uuid string, status string) (bool, error) {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE managed_nodes SET status = ?, updated_at = CURRENT_TIMESTAMP(6), last_seen_at = CURRENT_TIMESTAMP(6) WHERE uuid = ?`,
		status,
		uuid,
	)
	if err != nil {
		return false, fmt.Errorf("update managed node status: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update managed node status rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

func (r *NodeRepository) ListRegions(ctx context.Context) ([]node.RegionSummary, error) {
	const query = `
SELECT
    region,
    COUNT(*) AS total,
    SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) AS up_count,
    SUM(CASE WHEN status = 'down' THEN 1 ELSE 0 END) AS down_count
FROM managed_nodes
GROUP BY region
ORDER BY region ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list region summaries: %w", err)
	}
	defer rows.Close()

	var items []node.RegionSummary
	for rows.Next() {
		var item node.RegionSummary
		if err := rows.Scan(
			&item.Region,
			&item.Total,
			&item.UpCount,
			&item.DownCount,
		); err != nil {
			return nil, fmt.Errorf("scan region summary: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate region summaries: %w", err)
	}
	return items, nil
}

func (r *NodeRepository) ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error) {
	const query = `
SELECT
    region,
    zone,
    COUNT(*) AS total,
    SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) AS up_count,
    SUM(CASE WHEN status = 'down' THEN 1 ELSE 0 END) AS down_count
FROM managed_nodes
GROUP BY region, zone
ORDER BY region ASC, zone ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list region zone summaries: %w", err)
	}
	defer rows.Close()

	var items []node.RegionZoneSummary
	for rows.Next() {
		var item node.RegionZoneSummary
		if err := rows.Scan(
			&item.Region,
			&item.Zone,
			&item.Total,
			&item.UpCount,
			&item.DownCount,
		); err != nil {
			return nil, fmt.Errorf("scan region zone summary: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate region zone summaries: %w", err)
	}
	return items, nil
}

func (r *NodeRepository) MarkTimedOutNodesDown(ctx context.Context, localRegion string, timeoutSec int) (int, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	query := `UPDATE managed_nodes
		 SET status = 'down',
		     updated_at = CURRENT_TIMESTAMP(6)
		 WHERE status <> 'down'
		   AND COALESCE(last_reported_at, last_seen_at) < DATE_SUB(CURRENT_TIMESTAMP(6), INTERVAL ? SECOND)`
	args := []interface{}{timeoutSec}
	localRegion = strings.TrimSpace(strings.ToLower(localRegion))
	if localRegion != "" {
		query += ` AND region = ?`
		args = append(args, localRegion)
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("mark timed out nodes down: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("mark timed out nodes down rows affected: %w", err)
	}
	return int(rowsAffected), nil
}

func nullTime(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanNode(src scanner, item *node.Node) error {
	var lastReportedAt sql.NullTime
	var dnsLabel sql.NullString
	var dnsName sql.NullString
	var regionUUID sql.NullString
	var zoneUUID sql.NullString
	var diskDetailsRaw []byte
	var monitoringSnapshotRaw []byte
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
		&monitoringSnapshotRaw,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastSeenAt,
		&lastReportedAt,
	); err != nil {
		return err
	}
	if lastReportedAt.Valid {
		item.LastReportedAt = lastReportedAt.Time
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
	if len(monitoringSnapshotRaw) > 0 {
		item.MonitoringSnapshot = append(item.MonitoringSnapshot[:0], monitoringSnapshotRaw...)
	}
	return nil
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

func ensureUniqueIndex(ctx context.Context, db *sql.DB, tableName, indexName, columnName string) error {
	var existingCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(1)
		 FROM information_schema.statistics
		 WHERE table_schema = DATABASE()
		   AND table_name = ?
		   AND index_name = ?`,
		tableName,
		indexName,
	).Scan(&existingCount); err != nil {
		return fmt.Errorf("check index %s: %w", indexName, err)
	}
	if existingCount > 0 {
		return nil
	}

	stmt := fmt.Sprintf("ALTER TABLE %s ADD UNIQUE INDEX %s (%s)", tableName, indexName, columnName)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		if isDuplicateKeyNameError(err) {
			return nil
		}
		return fmt.Errorf("create index %s: %w", indexName, err)
	}
	return nil
}

func ensureIndex(ctx context.Context, db *sql.DB, tableName, indexName, columns string) error {
	var existingCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(1)
		 FROM information_schema.statistics
		 WHERE table_schema = DATABASE()
		   AND table_name = ?
		   AND index_name = ?`,
		tableName,
		indexName,
	).Scan(&existingCount); err != nil {
		return fmt.Errorf("check index %s: %w", indexName, err)
	}
	if existingCount > 0 {
		return nil
	}
	stmt := fmt.Sprintf("ALTER TABLE %s ADD INDEX %s (%s)", tableName, indexName, columns)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		if isDuplicateKeyNameError(err) {
			return nil
		}
		return fmt.Errorf("create index %s: %w", indexName, err)
	}
	return nil
}

func isDuplicateKeyNameError(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1061
}
