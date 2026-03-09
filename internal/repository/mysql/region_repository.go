package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/domain/region"
	"github.com/google/uuid"
)

type RegionRepository struct {
	db *sql.DB
}

func NewRegionRepository(db *sql.DB) *RegionRepository {
	return &RegionRepository{db: db}
}

func (r *RegionRepository) EnsureSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS regions (
    uuid VARCHAR(36) PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY uk_name (name)
)`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create regions table: %w", err)
	}
	return nil
}

func (r *RegionRepository) Create(ctx context.Context, name string) (region.Region, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return region.Region{}, fmt.Errorf("region name is required")
	}
	regionUUID := uuid.NewString()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO regions (uuid, name) VALUES (?, ?)`,
		regionUUID,
		name,
	)
	if err != nil {
		return region.Region{}, fmt.Errorf("create region: %w", err)
	}
	return region.Region{UUID: regionUUID, Name: name}, nil
}

func (r *RegionRepository) List(ctx context.Context) ([]region.RegionListItem, error) {
	const query = `
SELECT
    r.uuid,
    r.name,
    COUNT(DISTINCT CASE WHEN n.zone IS NOT NULL AND n.zone != '' THEN n.zone END) AS zone_num
FROM regions r
LEFT JOIN managed_nodes n ON r.uuid = n.region_uuid
GROUP BY r.uuid, r.name
ORDER BY r.name ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list regions: %w", err)
	}
	defer rows.Close()

	var items []region.RegionListItem
	for rows.Next() {
		var item region.RegionListItem
		if err := rows.Scan(&item.UUID, &item.Name, &item.ZoneNum); err != nil {
			return nil, fmt.Errorf("scan region: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate regions: %w", err)
	}
	return items, nil
}

func (r *RegionRepository) FindByUUID(ctx context.Context, uuid string) (*region.Region, error) {
	var item region.Region
	err := r.db.QueryRowContext(ctx,
		`SELECT uuid, name FROM regions WHERE uuid = ? LIMIT 1`,
		uuid,
	).Scan(&item.UUID, &item.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find region by uuid: %w", err)
	}
	return &item, nil
}

func (r *RegionRepository) FindByName(ctx context.Context, name string) (*region.Region, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	var item region.Region
	err := r.db.QueryRowContext(ctx,
		`SELECT uuid, name FROM regions WHERE name = ? LIMIT 1`,
		name,
	).Scan(&item.UUID, &item.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find region by name: %w", err)
	}
	return &item, nil
}

func (r *RegionRepository) DeleteByUUID(ctx context.Context, uuid string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM regions WHERE uuid = ?`, uuid)
	if err != nil {
		return false, fmt.Errorf("delete region: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete region rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

func (r *RegionRepository) DeleteNodesByRegionUUID(ctx context.Context, regionUUID string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM managed_nodes WHERE region_uuid = ?`, regionUUID)
	if err != nil {
		return 0, fmt.Errorf("delete nodes by region: %w", err)
	}
	return result.RowsAffected()
}

func (r *RegionRepository) MigrateNodesRegionUUID(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT region FROM managed_nodes WHERE (region_uuid IS NULL OR region_uuid = '') AND region != ''`)
	if err != nil {
		return fmt.Errorf("list distinct regions: %w", err)
	}
	defer rows.Close()
	var regionNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan region: %w", err)
		}
		regionNames = append(regionNames, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate regions: %w", err)
	}
	for _, name := range regionNames {
		reg, err := r.FindByName(ctx, name)
		if err != nil {
			return err
		}
		if reg == nil {
			created, err := r.Create(ctx, name)
			if err != nil {
				return err
			}
			reg = &created
		}
		_, err = r.db.ExecContext(ctx, `UPDATE managed_nodes SET region_uuid = ? WHERE region = ? AND (region_uuid IS NULL OR region_uuid = '')`, reg.UUID, name)
		if err != nil {
			return fmt.Errorf("update region_uuid for %q: %w", name, err)
		}
	}
	return nil
}
