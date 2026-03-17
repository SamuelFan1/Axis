package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/domain/zone"
	mysqldriver "github.com/go-sql-driver/mysql"
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
    region_uuid VARCHAR(36) NOT NULL,
    name VARCHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY uk_region_name (region_uuid, name),
    KEY idx_region_uuid (region_uuid)
)`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create zones table: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE zones ADD COLUMN IF NOT EXISTS region_uuid VARCHAR(36) NOT NULL DEFAULT ''`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("upgrade zones table: %w", err)
		}
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE zones DROP INDEX uk_name`); err != nil && !isMissingIndexError(err) {
		return fmt.Errorf("drop legacy zones.uk_name index: %w", err)
	}
	return nil
}

func (r *ZoneRepository) EnsureConstraints(ctx context.Context) error {
	if err := ensureUniqueCompositeIndex(ctx, r.db, "zones", "uk_region_name", "region_uuid, name"); err != nil {
		return err
	}
	if err := ensureIndex(ctx, r.db, "zones", "idx_region_uuid", "region_uuid"); err != nil {
		return err
	}
	if err := ensureForeignKey(ctx, r.db, "zones", "fk_zones_region_uuid", "region_uuid", "regions", "uuid"); err != nil {
		return err
	}
	return nil
}

func (r *ZoneRepository) Create(ctx context.Context, regionUUID string, name string) (zone.Zone, error) {
	regionUUID = strings.TrimSpace(regionUUID)
	name = strings.TrimSpace(strings.ToUpper(name))
	if regionUUID == "" {
		return zone.Zone{}, fmt.Errorf("zone region_uuid is required")
	}
	if name == "" {
		return zone.Zone{}, fmt.Errorf("zone name is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return zone.Zone{}, fmt.Errorf("begin create zone tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var existing zone.Zone
	err = tx.QueryRowContext(ctx,
		`SELECT uuid, region_uuid, name
		 FROM zones
		 WHERE region_uuid = ? AND name = ?
		 LIMIT 1`,
		regionUUID,
		name,
	).Scan(&existing.UUID, &existing.RegionUUID, &existing.Name)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return zone.Zone{}, fmt.Errorf("commit existing zone: %w", err)
		}
		return existing, nil
	}
	if err != sql.ErrNoRows {
		return zone.Zone{}, fmt.Errorf("find existing zone: %w", err)
	}

	var legacy zone.Zone
	err = tx.QueryRowContext(ctx,
		`SELECT uuid, region_uuid, name
		 FROM zones
		 WHERE name = ? AND (region_uuid = '' OR region_uuid IS NULL)
		 LIMIT 1`,
		name,
	).Scan(&legacy.UUID, &legacy.RegionUUID, &legacy.Name)
	if err == nil {
		if _, err := tx.ExecContext(ctx, `UPDATE zones SET region_uuid = ? WHERE uuid = ?`, regionUUID, legacy.UUID); err != nil {
			return zone.Zone{}, fmt.Errorf("promote legacy zone to scoped zone: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return zone.Zone{}, fmt.Errorf("commit promoted legacy zone: %w", err)
		}
		return zone.Zone{UUID: legacy.UUID, RegionUUID: regionUUID, Name: legacy.Name}, nil
	}
	if err != sql.ErrNoRows {
		return zone.Zone{}, fmt.Errorf("find legacy zone: %w", err)
	}

	zoneUUID := uuid.NewString()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO zones (uuid, region_uuid, name) VALUES (?, ?, ?)`,
		zoneUUID,
		regionUUID,
		name,
	); err != nil {
		return zone.Zone{}, fmt.Errorf("create zone: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return zone.Zone{}, fmt.Errorf("commit create zone: %w", err)
	}
	return zone.Zone{UUID: zoneUUID, RegionUUID: regionUUID, Name: name}, nil
}

func (r *ZoneRepository) List(ctx context.Context) ([]zone.ZoneListItem, error) {
	const query = `
SELECT
    z.uuid,
    z.region_uuid,
    COALESCE(r.name, '') AS region_name,
    z.name,
    COUNT(n.uuid) AS total,
    COALESCE(SUM(CASE WHEN n.status = 'up' THEN 1 ELSE 0 END), 0) AS up_count,
    COALESCE(SUM(CASE WHEN n.status = 'down' THEN 1 ELSE 0 END), 0) AS down_count
FROM zones z
LEFT JOIN regions r ON z.region_uuid = r.uuid
LEFT JOIN managed_nodes n ON z.uuid = n.zone_uuid
GROUP BY z.uuid, z.region_uuid, r.name, z.name
ORDER BY r.name ASC, z.name ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()

	var items []zone.ZoneListItem
	for rows.Next() {
		var item zone.ZoneListItem
		if err := rows.Scan(&item.UUID, &item.RegionUUID, &item.RegionName, &item.Name, &item.Total, &item.UpCount, &item.DownCount); err != nil {
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
		`SELECT uuid, region_uuid, name FROM zones WHERE uuid = ? LIMIT 1`,
		uuid,
	).Scan(&item.UUID, &item.RegionUUID, &item.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find zone by uuid: %w", err)
	}
	return &item, nil
}

func (r *ZoneRepository) FindByRegionUUIDAndName(ctx context.Context, regionUUID string, name string) (*zone.Zone, error) {
	regionUUID = strings.TrimSpace(regionUUID)
	name = strings.TrimSpace(strings.ToUpper(name))
	if regionUUID == "" || name == "" {
		return nil, nil
	}
	var item zone.Zone
	err := r.db.QueryRowContext(ctx,
		`SELECT uuid, region_uuid, name FROM zones WHERE region_uuid = ? AND name = ? LIMIT 1`,
		regionUUID,
		name,
	).Scan(&item.UUID, &item.RegionUUID, &item.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find zone by region and name: %w", err)
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

func (r *ZoneRepository) CountByRegionUUID(ctx context.Context, regionUUID string) (int, error) {
	var total int
	if err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(1) FROM zones WHERE region_uuid = ?`,
		regionUUID,
	).Scan(&total); err != nil {
		return 0, fmt.Errorf("count zones by region uuid: %w", err)
	}
	return total, nil
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
		`SELECT DISTINCT region_uuid, zone
		 FROM managed_nodes
		 WHERE (zone_uuid IS NULL OR zone_uuid = '')
		   AND zone != ''
		   AND region_uuid IS NOT NULL
		   AND region_uuid != ''`)
	if err != nil {
		return fmt.Errorf("list distinct zones: %w", err)
	}
	defer rows.Close()
	type zoneRegionPair struct {
		regionUUID string
		zoneName   string
	}
	var zonePairs []zoneRegionPair
	for rows.Next() {
		var regionUUID, name string
		if err := rows.Scan(&regionUUID, &name); err != nil {
			return fmt.Errorf("scan zone: %w", err)
		}
		zonePairs = append(zonePairs, zoneRegionPair{regionUUID: regionUUID, zoneName: name})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate zones: %w", err)
	}
	for _, pair := range zonePairs {
		z, err := r.FindByRegionUUIDAndName(ctx, pair.regionUUID, pair.zoneName)
		if err != nil {
			return err
		}
		if z == nil {
			continue
		}
		_, err = r.db.ExecContext(ctx,
			`UPDATE managed_nodes
			 SET zone_uuid = ?
			 WHERE region_uuid = ?
			   AND UPPER(TRIM(zone)) = ?
			   AND (zone_uuid IS NULL OR zone_uuid = '')`,
			z.UUID,
			pair.regionUUID,
			strings.ToUpper(strings.TrimSpace(pair.zoneName)),
		)
		if err != nil {
			return fmt.Errorf("update zone_uuid for %q/%q: %w", pair.regionUUID, pair.zoneName, err)
		}
	}
	return nil
}

func ensureUniqueCompositeIndex(ctx context.Context, db *sql.DB, tableName, indexName, columns string) error {
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

	stmt := fmt.Sprintf("ALTER TABLE %s ADD UNIQUE INDEX %s (%s)", tableName, indexName, columns)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		if isDuplicateKeyNameError(err) {
			return nil
		}
		return fmt.Errorf("create index %s: %w", indexName, err)
	}
	return nil
}

func isMissingIndexError(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1091
}

func ensureForeignKey(ctx context.Context, db *sql.DB, tableName, constraintName, columnName, refTable, refColumn string) error {
	var existingCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(1)
		 FROM information_schema.key_column_usage
		 WHERE table_schema = DATABASE()
		   AND table_name = ?
		   AND constraint_name = ?`,
		tableName,
		constraintName,
	).Scan(&existingCount); err != nil {
		return fmt.Errorf("check foreign key %s: %w", constraintName, err)
	}
	if existingCount > 0 {
		return nil
	}

	stmt := fmt.Sprintf(
		"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s)",
		tableName,
		constraintName,
		columnName,
		refTable,
		refColumn,
	)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("create foreign key %s: %w", constraintName, err)
	}
	return nil
}
