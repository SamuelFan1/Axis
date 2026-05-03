package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/node"
	mysqldriver "github.com/go-sql-driver/mysql"
)

type NodeRepository struct {
	db          *sql.DB
	runtimeDB   *sql.DB
	metricsRepo *NodeMetricsRepository
}

func NewNodeRepository(db *sql.DB, runtimeDBs ...*sql.DB) *NodeRepository {
	runtimeDB := db
	if len(runtimeDBs) > 0 && runtimeDBs[0] != nil {
		runtimeDB = runtimeDBs[0]
	}
	return &NodeRepository{
		db:          db,
		runtimeDB:   runtimeDB,
		metricsRepo: NewNodeMetricsRepository(runtimeDB),
	}
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
	const healthDDL = `
CREATE TABLE IF NOT EXISTS node_health_by_region (
    observer_region VARCHAR(64) NOT NULL,
    node_uuid VARCHAR(36) NOT NULL,
    status VARCHAR(16) NOT NULL,
    status_source VARCHAR(32) NOT NULL DEFAULT 'self_report',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    last_seen_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    last_reported_at DATETIME(6) NULL,
    PRIMARY KEY (observer_region, node_uuid),
    KEY idx_node_health_uuid (node_uuid),
    KEY idx_node_health_status (observer_region, status),
    KEY idx_node_health_last_seen_at (observer_region, last_seen_at)
)`
	if _, err := r.runtimeDB.ExecContext(ctx, healthDDL); err != nil {
		return fmt.Errorf("create node_health_by_region table: %w", err)
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
	if _, err := r.runtimeDB.ExecContext(ctx, historyDDL); err != nil {
		return fmt.Errorf("create managed_nodes_history table: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS last_reported_at DATETIME(6) NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS internal_ip VARCHAR(64) DEFAULT ''`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS public_ip VARCHAR(64) DEFAULT ''`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS dns_label VARCHAR(64) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS dns_name VARCHAR(255) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS zone VARCHAR(16) NOT NULL DEFAULT ''`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS region_uuid VARCHAR(36) NULL DEFAULT NULL`,
		`ALTER TABLE managed_nodes ADD COLUMN IF NOT EXISTS zone_uuid VARCHAR(36) NULL DEFAULT NULL`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("upgrade managed_nodes table: %w", err)
		}
	}
	for _, stmt := range []string{
		`ALTER TABLE node_health_by_region ADD COLUMN IF NOT EXISTS status_source VARCHAR(32) NOT NULL DEFAULT 'self_report'`,
		`ALTER TABLE node_health_by_region ADD COLUMN IF NOT EXISTS created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)`,
		`ALTER TABLE node_health_by_region ADD COLUMN IF NOT EXISTS updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)`,
		`ALTER TABLE node_health_by_region ADD COLUMN IF NOT EXISTS last_seen_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)`,
		`ALTER TABLE node_health_by_region ADD COLUMN IF NOT EXISTS last_reported_at DATETIME(6) NULL`,
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
		if _, err := r.runtimeDB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("upgrade runtime node tables: %w", err)
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
	if err := ensureIndex(ctx, r.runtimeDB, "node_health_by_region", "idx_node_health_uuid", "node_uuid"); err != nil {
		return err
	}
	if err := ensureIndex(ctx, r.runtimeDB, "node_health_by_region", "idx_node_health_status", "observer_region, status"); err != nil {
		return err
	}
	if err := ensureIndex(ctx, r.runtimeDB, "node_health_by_region", "idx_node_health_last_seen_at", "observer_region, last_seen_at"); err != nil {
		return err
	}
	if err := r.metricsRepo.EnsureSchema(ctx); err != nil {
		return err
	}
	return nil
}

const selectNodeIdentityColumns = `
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
    created_at,
    updated_at`

func (r *NodeRepository) FindActiveByManagementAddress(ctx context.Context, managementAddress string) (*node.NodeIdentity, error) {
	const query = `SELECT` + selectNodeIdentityColumns + `
FROM managed_nodes
WHERE management_address = ?
LIMIT 1`

	var item node.NodeIdentity
	err := scanNodeIdentity(r.db.QueryRowContext(ctx, query, managementAddress), &item)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active managed node by management address: %w", err)
	}
	return &item, nil
}

func (r *NodeRepository) FindIdentityByUUID(ctx context.Context, uuid string) (*node.NodeIdentity, error) {
	const query = `SELECT` + selectNodeIdentityColumns + `
FROM managed_nodes
WHERE uuid = ?
LIMIT 1`

	var item node.NodeIdentity
	err := scanNodeIdentity(r.db.QueryRowContext(ctx, query, uuid), &item)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find managed node identity by uuid: %w", err)
	}
	return &item, nil
}

func (r *NodeRepository) FindByUUID(ctx context.Context, uuid string) (*node.Node, error) {
	identity, err := r.FindIdentityByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if identity == nil {
		return nil, nil
	}
	item, err := r.buildNodeView(ctx, *identity)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *NodeRepository) UpsertIdentity(ctx context.Context, item node.NodeIdentity) error {
	// region/zone text columns remain the hot-path read model for scheduling and routing,
	// while region_uuid/zone_uuid act as relational anchors to static master data.
	const query = `
INSERT INTO managed_nodes (
    uuid, hostname, management_address, internal_ip, public_ip, region, region_uuid, zone, zone_uuid, status, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6)
)
ON DUPLICATE KEY UPDATE
    hostname = VALUES(hostname),
    management_address = VALUES(management_address),
    internal_ip = VALUES(internal_ip),
    public_ip = VALUES(public_ip),
    region = VALUES(region),
    region_uuid = VALUES(region_uuid),
    zone = VALUES(zone),
    zone_uuid = VALUES(zone_uuid),
    updated_at = CURRENT_TIMESTAMP(6)`

	if _, err := r.db.ExecContext(
		ctx,
		query,
		item.UUID,
		item.Hostname,
		item.ManagementAddress,
		item.InternalIP,
		item.PublicIP,
		item.Region,
		nullString(item.RegionUUID),
		item.Zone,
		nullString(item.ZoneUUID),
		item.Status,
	); err != nil {
		return fmt.Errorf("upsert managed node identity: %w", err)
	}
	return nil
}

func (r *NodeRepository) UpdateHeartbeat(ctx context.Context, item node.Node) error {
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
	if err := r.metricsRepo.Upsert(ctx, item); err != nil {
		return err
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

	identity, err := r.FindActiveByManagementAddress(ctx, managementAddress)
	if err != nil {
		return fmt.Errorf("find active managed node by management address: %w", err)
	}
	if identity == nil {
		return nil
	}

	var health *node.NodeHealth
	if identity.Region != "" {
		health, err = r.findHealthByObserverRegion(ctx, identity.UUID, identity.Region)
		if err != nil {
			return err
		}
	}
	metricsByNode, err := r.metricsRepo.LoadByNodeUUIDs(ctx, []string{identity.UUID})
	if err != nil {
		return err
	}
	metrics := metricsByNode[identity.UUID]

	tx, err := r.runtimeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin archive managed node tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	status := node.StatusDown
	lastSeenAt := identity.UpdatedAt
	var lastReportedAt time.Time
	if health != nil {
		status = health.Status
		lastSeenAt = health.LastSeenAt
		lastReportedAt = health.LastReportedAt
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO managed_nodes_history (
		     uuid, hostname, management_address, internal_ip, public_ip, dns_label, dns_name,
		     region, region_uuid, zone, zone_uuid, status, cpu_cores, cpu_usage_percent,
		     memory_total_gb, memory_used_gb, memory_usage_percent, swap_total_gb, swap_used_gb,
		     swap_usage_percent, disk_usage_percent, disk_details, monitoring_snapshot,
		     created_at, updated_at, last_seen_at, last_reported_at, archived_at,
		     archive_reason, replaced_by_uuid
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(6), ?, NULLIF(?, ''))`,
		identity.UUID,
		identity.Hostname,
		identity.ManagementAddress,
		identity.InternalIP,
		identity.PublicIP,
		nullString(identity.DNSLabel),
		nullString(identity.DNSName),
		identity.Region,
		nullString(identity.RegionUUID),
		identity.Zone,
		nullString(identity.ZoneUUID),
		status,
		metrics.CPUCores,
		metrics.CPUUsagePercent,
		metrics.MemoryTotalGB,
		metrics.MemoryUsedGB,
		metrics.MemoryUsagePercent,
		metrics.SwapTotalGB,
		metrics.SwapUsedGB,
		metrics.SwapUsagePercent,
		metrics.DiskUsagePercent,
		marshalDiskDetails(metrics.DiskDetails),
		marshalRawJSON(metrics.MonitoringSnapshot),
		identity.CreatedAt,
		identity.UpdatedAt,
		nullTime(lastSeenAt),
		nullTime(lastReportedAt),
		reason,
		replacedByUUID,
	); err != nil {
		return fmt.Errorf("archive managed node by management address: %w", err)
	}

	if err := r.deleteRuntimeByNodeUUIDsTx(ctx, tx, []string{identity.UUID}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit archive managed node tx: %w", err)
	}
	tx = nil

	if _, err := r.db.ExecContext(ctx, `DELETE FROM managed_nodes WHERE management_address = ?`, managementAddress); err != nil {
		return fmt.Errorf("delete managed node by management address: %w", err)
	}
	return nil
}

func (r *NodeRepository) List(ctx context.Context) ([]node.Node, error) {
	const query = `SELECT` + selectNodeIdentityColumns + `
FROM managed_nodes
ORDER BY region ASC, zone ASC, hostname ASC, uuid ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list managed nodes: %w", err)
	}
	defer rows.Close()

	var identities []node.NodeIdentity
	for rows.Next() {
		var item node.NodeIdentity
		if err := scanNodeIdentity(rows, &item); err != nil {
			return nil, fmt.Errorf("scan managed node: %w", err)
		}
		identities = append(identities, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed nodes: %w", err)
	}
	return r.buildNodeViews(ctx, identities)
}

func (r *NodeRepository) DeleteByUUID(ctx context.Context, uuid string) (bool, error) {
	if err := r.deleteRuntimeByNodeUUIDs(ctx, []string{uuid}); err != nil {
		return false, err
	}
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

func (r *NodeRepository) DeleteByRegionUUID(ctx context.Context, regionUUID string) (int64, error) {
	uuids, err := r.listIdentityUUIDsByField(ctx, "region_uuid", regionUUID)
	if err != nil {
		return 0, err
	}
	if err := r.deleteRuntimeByNodeUUIDs(ctx, uuids); err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM managed_nodes WHERE region_uuid = ?`, regionUUID)
	if err != nil {
		return 0, fmt.Errorf("delete managed nodes by region uuid: %w", err)
	}
	return result.RowsAffected()
}

func (r *NodeRepository) DeleteByZoneUUID(ctx context.Context, zoneUUID string) (int64, error) {
	uuids, err := r.listIdentityUUIDsByField(ctx, "zone_uuid", zoneUUID)
	if err != nil {
		return 0, err
	}
	if err := r.deleteRuntimeByNodeUUIDs(ctx, uuids); err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM managed_nodes WHERE zone_uuid = ?`, zoneUUID)
	if err != nil {
		return 0, fmt.Errorf("delete managed nodes by zone uuid: %w", err)
	}
	return result.RowsAffected()
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
	nodes, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make(map[string]*node.RegionSummary)
	for _, item := range nodes {
		summary := summaries[item.Region]
		if summary == nil {
			summary = &node.RegionSummary{Region: item.Region}
			summaries[item.Region] = summary
		}
		summary.Total++
		if strings.EqualFold(strings.TrimSpace(item.Status), node.StatusUp) {
			summary.UpCount++
		} else {
			summary.DownCount++
		}
	}
	return sortRegionSummaries(summaries), nil
}

func (r *NodeRepository) ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error) {
	nodes, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make(map[string]*node.RegionZoneSummary)
	for _, item := range nodes {
		key := item.Region + "\x00" + item.Zone
		summary := summaries[key]
		if summary == nil {
			summary = &node.RegionZoneSummary{Region: item.Region, Zone: item.Zone}
			summaries[key] = summary
		}
		summary.Total++
		if strings.EqualFold(strings.TrimSpace(item.Status), node.StatusUp) {
			summary.UpCount++
		} else {
			summary.DownCount++
		}
	}
	return sortRegionZoneSummaries(summaries), nil
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
	return nil
}

func (r *NodeRepository) attachMetrics(ctx context.Context, item *node.Node) error {
	if item == nil || item.UUID == "" {
		return nil
	}
	metricsByNode, err := r.metricsRepo.LoadByNodeUUIDs(ctx, []string{item.UUID})
	if err != nil {
		return err
	}
	if metrics, ok := metricsByNode[item.UUID]; ok {
		applyNodeMetrics(item, metrics)
	}
	return nil
}

func (r *NodeRepository) attachMetricsBatch(ctx context.Context, items []node.Node) error {
	if len(items) == 0 {
		return nil
	}
	uuids := make([]string, 0, len(items))
	for _, item := range items {
		if item.UUID != "" {
			uuids = append(uuids, item.UUID)
		}
	}
	metricsByNode, err := r.metricsRepo.LoadByNodeUUIDs(ctx, uuids)
	if err != nil {
		return err
	}
	for i := range items {
		if metrics, ok := metricsByNode[items[i].UUID]; ok {
			applyNodeMetrics(&items[i], metrics)
		}
	}
	return nil
}

func applyNodeMetrics(target *node.Node, metrics node.Node) {
	target.CPUCores = metrics.CPUCores
	target.CPUUsagePercent = metrics.CPUUsagePercent
	target.MemoryTotalGB = metrics.MemoryTotalGB
	target.MemoryUsedGB = metrics.MemoryUsedGB
	target.MemoryUsagePercent = metrics.MemoryUsagePercent
	target.SwapTotalGB = metrics.SwapTotalGB
	target.SwapUsedGB = metrics.SwapUsedGB
	target.SwapUsagePercent = metrics.SwapUsagePercent
	target.DiskUsagePercent = metrics.DiskUsagePercent
	target.DiskDetails = metrics.DiskDetails
	target.MonitoringSnapshot = metrics.MonitoringSnapshot
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

func (r *NodeRepository) listIdentityUUIDsByField(ctx context.Context, fieldName string, value string) ([]string, error) {
	query := fmt.Sprintf(`SELECT uuid FROM managed_nodes WHERE %s = ? ORDER BY uuid ASC`, fieldName)
	rows, err := r.db.QueryContext(ctx, query, strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("list managed node uuids by %s: %w", fieldName, err)
	}
	defer rows.Close()
	var uuids []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, fmt.Errorf("scan managed node uuid by %s: %w", fieldName, err)
		}
		uuids = append(uuids, uuid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed node uuids by %s: %w", fieldName, err)
	}
	return uuids, nil
}

func (r *NodeRepository) deleteRuntimeByNodeUUIDs(ctx context.Context, nodeUUIDs []string) error {
	tx, err := r.runtimeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin runtime cleanup tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.deleteRuntimeByNodeUUIDsTx(ctx, tx, nodeUUIDs); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit runtime cleanup tx: %w", err)
	}
	return nil
}

func (r *NodeRepository) deleteRuntimeByNodeUUIDsTx(ctx context.Context, tx *sql.Tx, nodeUUIDs []string) error {
	if len(nodeUUIDs) == 0 {
		return nil
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
		return nil
	}
	deleteMetricsQuery := fmt.Sprintf(`DELETE FROM managed_node_metrics_ext WHERE node_uuid IN (%s)`, strings.Join(placeholders, ","))
	if _, err := tx.ExecContext(ctx, deleteMetricsQuery, args...); err != nil {
		return fmt.Errorf("delete managed node metrics by uuid: %w", err)
	}
	deleteHealthQuery := fmt.Sprintf(`DELETE FROM node_health_by_region WHERE node_uuid IN (%s)`, strings.Join(placeholders, ","))
	if _, err := tx.ExecContext(ctx, deleteHealthQuery, args...); err != nil {
		return fmt.Errorf("delete managed node health by uuid: %w", err)
	}
	return nil
}

func sortRegionSummaries(items map[string]*node.RegionSummary) []node.RegionSummary {
	regions := make([]string, 0, len(items))
	for region := range items {
		regions = append(regions, region)
	}
	sort.Strings(regions)
	result := make([]node.RegionSummary, 0, len(regions))
	for _, region := range regions {
		result = append(result, *items[region])
	}
	return result
}

func sortRegionZoneSummaries(items map[string]*node.RegionZoneSummary) []node.RegionZoneSummary {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]node.RegionZoneSummary, 0, len(keys))
	for _, key := range keys {
		result = append(result, *items[key])
	}
	return result
}
