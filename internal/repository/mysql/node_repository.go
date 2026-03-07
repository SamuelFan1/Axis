package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/node"
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
    region VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL,
    cpu_usage_percent DOUBLE NOT NULL DEFAULT 0,
    memory_usage_percent DOUBLE NOT NULL DEFAULT 0,
    disk_usage_percent DOUBLE NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    last_seen_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    last_reported_at DATETIME(6) NULL,
    UNIQUE KEY uk_management_address (management_address),
    KEY idx_region_status (region, status),
    KEY idx_last_seen_at (last_seen_at)
)`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create managed_nodes table: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS cpu_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS memory_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS disk_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS last_reported_at DATETIME(6) NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS internal_ip VARCHAR(64) DEFAULT ''`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS public_ip VARCHAR(64) DEFAULT ''`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS cpu_cores INT NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS memory_total_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS memory_used_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS swap_total_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS swap_used_gb DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS swap_usage_percent DOUBLE NOT NULL DEFAULT 0`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS disk_details JSON NULL`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("upgrade managed_nodes table: %w", err)
		}
	}
	return nil
}

const selectNodeColumns = `
    uuid,
    hostname,
    management_address,
    internal_ip,
    public_ip,
    region,
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
    created_at,
    updated_at,
    last_seen_at,
    last_reported_at`

func (r *NodeRepository) FindByManagementAddress(ctx context.Context, managementAddress string) (*node.Node, error) {
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
		return nil, fmt.Errorf("find managed node by management address: %w", err)
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
	const query = `
INSERT INTO managed_nodes (
    uuid, hostname, management_address, region, status, cpu_usage_percent, memory_usage_percent, disk_usage_percent, created_at, updated_at, last_seen_at, last_reported_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), ?
)
ON DUPLICATE KEY UPDATE
    hostname = VALUES(hostname),
    management_address = VALUES(management_address),
    region = VALUES(region),
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
	const query = `
UPDATE managed_nodes
SET
    hostname = ?,
    management_address = ?,
    internal_ip = ?,
    public_ip = ?,
    region = ?,
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

func (r *NodeRepository) List(ctx context.Context) ([]node.Node, error) {
	const query = `SELECT` + selectNodeColumns + `
FROM managed_nodes
ORDER BY region ASC, hostname ASC, uuid ASC`

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

func (r *NodeRepository) MarkTimedOutNodesDown(ctx context.Context, timeoutSec int) (int, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE managed_nodes
		 SET status = 'down',
		     updated_at = CURRENT_TIMESTAMP(6)
		 WHERE status <> 'down'
		   AND COALESCE(last_reported_at, last_seen_at) < DATE_SUB(CURRENT_TIMESTAMP(6), INTERVAL ? SECOND)`,
		timeoutSec,
	)
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

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanNode(src scanner, item *node.Node) error {
	var lastReportedAt sql.NullTime
	var diskDetailsRaw []byte
	if err := src.Scan(
		&item.UUID,
		&item.Hostname,
		&item.ManagementAddress,
		&item.InternalIP,
		&item.PublicIP,
		&item.Region,
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
	if len(diskDetailsRaw) > 0 {
		_ = json.Unmarshal(diskDetailsRaw, &item.DiskDetails)
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
