package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/domain/zone"
	"github.com/google/uuid"
)

type ZoneRepository struct {
	db *sql.DB
}

func NewZoneRepository(db *sql.DB) *ZoneRepository {
	return &ZoneRepository{db: db}
}

func (r *ZoneRepository) EnsureSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS zones (
    uuid VARCHAR(36) PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY uk_name (name)
)`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create zones table: %w", err)
	}
	return nil
}

func (r *ZoneRepository) Create(ctx context.Context, name string) (zone.Zone, error) {
	name = strings.TrimSpace(strings.ToUpper(name))
	if name == "" {
		return zone.Zone{}, fmt.Errorf("zone name is required")
	}
	zoneUUID := uuid.NewString()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO zones (uuid, name) VALUES (?, ?)`,
		zoneUUID,
		name,
	)
	if err != nil {
		return zone.Zone{}, fmt.Errorf("create zone: %w", err)
	}
	return zone.Zone{UUID: zoneUUID, Name: name}, nil
}

func (r *ZoneRepository) List(ctx context.Context) ([]zone.ZoneListItem, error) {
	const query = `
SELECT
    z.uuid,
    z.name,
    COUNT(n.uuid) AS total,
    COALESCE(SUM(CASE WHEN n.status = 'up' THEN 1 ELSE 0 END), 0) AS up_count,
    COALESCE(SUM(CASE WHEN n.status = 'down' THEN 1 ELSE 0 END), 0) AS down_count
FROM zones z
LEFT JOIN managed_nodes n ON z.uuid = n.zone_uuid
GROUP BY z.uuid, z.name
ORDER BY z.name ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()

	var items []zone.ZoneListItem
	for rows.Next() {
		var item zone.ZoneListItem
		if err := rows.Scan(&item.UUID, &item.Name, &item.Total, &item.UpCount, &item.DownCount); err != nil {
			return nil, fmt.Errorf("scan zone: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate zones: %w", err)
	}
	return items, nil
}

func (r *ZoneRepository) FindByUUID(ctx context.Context, uuid string) (*zone.Zone, error) {
	var item zone.Zone
	err := r.db.QueryRowContext(ctx,
		`SELECT uuid, name FROM zones WHERE uuid = ? LIMIT 1`,
		uuid,
	).Scan(&item.UUID, &item.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find zone by uuid: %w", err)
	}
	return &item, nil
}

func (r *ZoneRepository) FindByName(ctx context.Context, name string) (*zone.Zone, error) {
	name = strings.TrimSpace(strings.ToUpper(name))
	if name == "" {
		return nil, nil
	}
	var item zone.Zone
	err := r.db.QueryRowContext(ctx,
		`SELECT uuid, name FROM zones WHERE name = ? LIMIT 1`,
		name,
	).Scan(&item.UUID, &item.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find zone by name: %w", err)
	}
	return &item, nil
}

func (r *ZoneRepository) DeleteByUUID(ctx context.Context, uuid string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM zones WHERE uuid = ?`, uuid)
	if err != nil {
		return false, fmt.Errorf("delete zone: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete zone rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

func (r *ZoneRepository) DeleteNodesByZoneUUID(ctx context.Context, zoneUUID string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM managed_nodes WHERE zone_uuid = ?`, zoneUUID)
	if err != nil {
		return 0, fmt.Errorf("delete nodes by zone: %w", err)
	}
	return result.RowsAffected()
}

func (r *ZoneRepository) MigrateNodesZoneUUID(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT zone FROM managed_nodes WHERE (zone_uuid IS NULL OR zone_uuid = '') AND zone != ''`)
	if err != nil {
		return fmt.Errorf("list distinct zones: %w", err)
	}
	defer rows.Close()
	var zoneNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan zone: %w", err)
		}
		zoneNames = append(zoneNames, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate zones: %w", err)
	}
	for _, name := range zoneNames {
		z, err := r.FindByName(ctx, name)
		if err != nil {
			return err
		}
		if z == nil {
			continue
		}
		_, err = r.db.ExecContext(ctx, `UPDATE managed_nodes SET zone_uuid = ? WHERE UPPER(TRIM(zone)) = ? AND (zone_uuid IS NULL OR zone_uuid = '')`, z.UUID, strings.ToUpper(strings.TrimSpace(name)))
		if err != nil {
			return fmt.Errorf("update zone_uuid for %q: %w", name, err)
		}
	}
	return nil
}
