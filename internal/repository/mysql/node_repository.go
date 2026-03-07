package mysql

import (
	"context"
	"database/sql"
	"fmt"

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
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    last_seen_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY uk_management_address (management_address),
    KEY idx_region_status (region, status),
    KEY idx_last_seen_at (last_seen_at)
)`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create managed_nodes table: %w", err)
	}
	return nil
}

func (r *NodeRepository) FindByManagementAddress(ctx context.Context, managementAddress string) (*node.Node, error) {
	const query = `
SELECT
    uuid,
    hostname,
    management_address,
    region,
    status,
    created_at,
    updated_at,
    last_seen_at
FROM managed_nodes
WHERE management_address = ?
LIMIT 1`

	var item node.Node
	err := r.db.QueryRowContext(ctx, query, managementAddress).Scan(
		&item.UUID,
		&item.Hostname,
		&item.ManagementAddress,
		&item.Region,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastSeenAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find managed node by management address: %w", err)
	}

	return &item, nil
}

func (r *NodeRepository) FindByUUID(ctx context.Context, uuid string) (*node.Node, error) {
	const query = `
SELECT
    uuid,
    hostname,
    management_address,
    region,
    status,
    created_at,
    updated_at,
    last_seen_at
FROM managed_nodes
WHERE uuid = ?
LIMIT 1`

	var item node.Node
	err := r.db.QueryRowContext(ctx, query, uuid).Scan(
		&item.UUID,
		&item.Hostname,
		&item.ManagementAddress,
		&item.Region,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastSeenAt,
	)
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
    uuid, hostname, management_address, region, status, created_at, updated_at, last_seen_at
) VALUES (
    ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6)
)
ON DUPLICATE KEY UPDATE
    hostname = VALUES(hostname),
    management_address = VALUES(management_address),
    region = VALUES(region),
    status = VALUES(status),
    updated_at = CURRENT_TIMESTAMP(6),
    last_seen_at = CURRENT_TIMESTAMP(6)`

	if _, err := r.db.ExecContext(
		ctx,
		query,
		item.UUID,
		item.Hostname,
		item.ManagementAddress,
		item.Region,
		item.Status,
	); err != nil {
		return fmt.Errorf("upsert managed node: %w", err)
	}
	return nil
}

func (r *NodeRepository) List(ctx context.Context) ([]node.Node, error) {
	const query = `
SELECT
    uuid,
    hostname,
    management_address,
    region,
    status,
    created_at,
    updated_at,
    last_seen_at
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
		if err := rows.Scan(
			&item.UUID,
			&item.Hostname,
			&item.ManagementAddress,
			&item.Region,
			&item.Status,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastSeenAt,
		); err != nil {
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
