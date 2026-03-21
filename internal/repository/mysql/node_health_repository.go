package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/domain/node"
)

func (r *NodeRepository) UpsertHealth(ctx context.Context, item node.NodeHealth) error {
	item.ObserverRegion = strings.TrimSpace(strings.ToLower(item.ObserverRegion))
	if item.ObserverRegion == "" {
		return fmt.Errorf("observer region is required")
	}
	if strings.TrimSpace(item.NodeUUID) == "" {
		return fmt.Errorf("node uuid is required")
	}
	if strings.TrimSpace(item.Status) == "" {
		item.Status = node.StatusDown
	}
	if strings.TrimSpace(item.StatusSource) == "" {
		item.StatusSource = "self_report"
	}

	const query = `
INSERT INTO node_health_by_region (
    observer_region,
    node_uuid,
    status,
    status_source,
    created_at,
    updated_at,
    last_seen_at,
    last_reported_at
) VALUES (
    ?, ?, ?, ?, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), ?
)
ON DUPLICATE KEY UPDATE
    status = VALUES(status),
    status_source = VALUES(status_source),
    updated_at = CURRENT_TIMESTAMP(6),
    last_seen_at = CURRENT_TIMESTAMP(6),
    last_reported_at = VALUES(last_reported_at)`
	if _, err := r.db.ExecContext(
		ctx,
		query,
		item.ObserverRegion,
		item.NodeUUID,
		item.Status,
		item.StatusSource,
		nullTime(item.LastReportedAt),
	); err != nil {
		return fmt.Errorf("upsert node health by region: %w", err)
	}

	return r.metricsRepo.Upsert(ctx, node.Node{
		UUID:               item.NodeUUID,
		CPUCores:           item.CPUCores,
		CPUUsagePercent:    item.CPUUsagePercent,
		MemoryTotalGB:      item.MemoryTotalGB,
		MemoryUsedGB:       item.MemoryUsedGB,
		MemoryUsagePercent: item.MemoryUsagePercent,
		SwapTotalGB:        item.SwapTotalGB,
		SwapUsedGB:         item.SwapUsedGB,
		SwapUsagePercent:   item.SwapUsagePercent,
		DiskUsagePercent:   item.DiskUsagePercent,
		DiskDetails:        item.DiskDetails,
		MonitoringSnapshot: item.MonitoringSnapshot,
	})
}

func (r *NodeRepository) FindLatestHealthByNodeUUID(ctx context.Context, nodeUUID string) (*node.NodeHealth, error) {
	identity, err := r.FindIdentityByUUID(ctx, nodeUUID)
	if err != nil {
		return nil, err
	}
	if identity == nil {
		return nil, nil
	}
	return r.findHealthByObserverRegion(ctx, identity.UUID, identity.Region)
}

func (r *NodeRepository) GetMonitoringSnapshot(ctx context.Context, nodeUUID string) (json.RawMessage, error) {
	health, err := r.FindLatestHealthByNodeUUID(ctx, nodeUUID)
	if err != nil {
		return nil, err
	}
	if health == nil {
		return nil, fmt.Errorf("node not found")
	}
	return append(json.RawMessage(nil), health.MonitoringSnapshot...), nil
}

func (r *NodeRepository) MarkTimedOutNodesDown(ctx context.Context, localRegion string, timeoutSec int) (int, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	localRegion = strings.TrimSpace(strings.ToLower(localRegion))
	if localRegion == "" {
		return 0, nil
	}
	query := `UPDATE node_health_by_region
		 SET status = 'down',
		     status_source = 'timeout_monitor',
		     updated_at = CURRENT_TIMESTAMP(6)
		 WHERE observer_region = ?
		   AND status <> 'down'
		   AND COALESCE(last_reported_at, last_seen_at) < DATE_SUB(CURRENT_TIMESTAMP(6), INTERVAL ? SECOND)`
	result, err := r.db.ExecContext(ctx, query, localRegion, timeoutSec)
	if err != nil {
		return 0, fmt.Errorf("mark timed out node health down: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("mark timed out node health down rows affected: %w", err)
	}
	return int(rowsAffected), nil
}

func (r *NodeRepository) buildNodeViews(ctx context.Context, identities []node.NodeIdentity) ([]node.Node, error) {
	if len(identities) == 0 {
		return nil, nil
	}
	healthByNode, err := r.loadHomeHealthByNodeUUIDs(ctx, identities)
	if err != nil {
		return nil, err
	}
	uuids := make([]string, 0, len(identities))
	for _, item := range identities {
		if item.UUID != "" {
			uuids = append(uuids, item.UUID)
		}
	}
	metricsByNode, err := r.metricsRepo.LoadByNodeUUIDs(ctx, uuids)
	if err != nil {
		return nil, err
	}
	items := make([]node.Node, 0, len(identities))
	for _, identity := range identities {
		var health *node.NodeHealth
		if loaded, ok := healthByNode[identity.UUID]; ok {
			copied := loaded
			health = &copied
		}
		aggregate := identity.Aggregate(health)
		if metrics, ok := metricsByNode[identity.UUID]; ok {
			applyNodeMetrics(&aggregate, metrics)
		}
		items = append(items, aggregate)
	}
	return items, nil
}

func (r *NodeRepository) buildNodeView(ctx context.Context, identity node.NodeIdentity) (node.Node, error) {
	items, err := r.buildNodeViews(ctx, []node.NodeIdentity{identity})
	if err != nil {
		return node.Node{}, err
	}
	if len(items) == 0 {
		return node.Node{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (r *NodeRepository) loadHomeHealthByNodeUUIDs(ctx context.Context, identities []node.NodeIdentity) (map[string]node.NodeHealth, error) {
	result := make(map[string]node.NodeHealth)
	if len(identities) == 0 {
		return result, nil
	}
	placeholders := make([]string, 0, len(identities))
	args := make([]interface{}, 0, len(identities))
	expectedRegion := make(map[string]string, len(identities))
	for _, item := range identities {
		if item.UUID == "" {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, item.UUID)
		expectedRegion[item.UUID] = strings.TrimSpace(strings.ToLower(item.Region))
	}
	if len(placeholders) == 0 {
		return result, nil
	}
	query := fmt.Sprintf(`SELECT
    observer_region,
    node_uuid,
    status,
    status_source,
    created_at,
    updated_at,
    last_seen_at,
    last_reported_at
FROM node_health_by_region
WHERE node_uuid IN (%s)`, strings.Join(placeholders, ","))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list node health by uuid: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item node.NodeHealth
		var lastReportedAt sql.NullTime
		if err := rows.Scan(
			&item.ObserverRegion,
			&item.NodeUUID,
			&item.Status,
			&item.StatusSource,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastSeenAt,
			&lastReportedAt,
		); err != nil {
			return nil, fmt.Errorf("scan node health by uuid: %w", err)
		}
		if lastReportedAt.Valid {
			item.LastReportedAt = lastReportedAt.Time
		}
		if expectedRegion[item.NodeUUID] != strings.TrimSpace(strings.ToLower(item.ObserverRegion)) {
			continue
		}
		result[item.NodeUUID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate node health by uuid: %w", err)
	}
	return result, nil
}

func (r *NodeRepository) findHealthByObserverRegion(ctx context.Context, nodeUUID string, observerRegion string) (*node.NodeHealth, error) {
	observerRegion = strings.TrimSpace(strings.ToLower(observerRegion))
	if nodeUUID == "" || observerRegion == "" {
		return nil, nil
	}
	const query = `SELECT
    observer_region,
    node_uuid,
    status,
    status_source,
    created_at,
    updated_at,
    last_seen_at,
    last_reported_at
FROM node_health_by_region
WHERE observer_region = ?
  AND node_uuid = ?
LIMIT 1`
	var item node.NodeHealth
	var lastReportedAt sql.NullTime
	if err := r.db.QueryRowContext(ctx, query, observerRegion, nodeUUID).Scan(
		&item.ObserverRegion,
		&item.NodeUUID,
		&item.Status,
		&item.StatusSource,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastSeenAt,
		&lastReportedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find node health by observer region: %w", err)
	}
	if lastReportedAt.Valid {
		item.LastReportedAt = lastReportedAt.Time
	}
	metricsByNode, err := r.metricsRepo.LoadByNodeUUIDs(ctx, []string{nodeUUID})
	if err != nil {
		return nil, err
	}
	if metrics, ok := metricsByNode[nodeUUID]; ok {
		item.CPUCores = metrics.CPUCores
		item.CPUUsagePercent = metrics.CPUUsagePercent
		item.MemoryTotalGB = metrics.MemoryTotalGB
		item.MemoryUsedGB = metrics.MemoryUsedGB
		item.MemoryUsagePercent = metrics.MemoryUsagePercent
		item.SwapTotalGB = metrics.SwapTotalGB
		item.SwapUsedGB = metrics.SwapUsedGB
		item.SwapUsagePercent = metrics.SwapUsagePercent
		item.DiskUsagePercent = metrics.DiskUsagePercent
		item.DiskDetails = metrics.DiskDetails
		item.MonitoringSnapshot = append(item.MonitoringSnapshot[:0], metrics.MonitoringSnapshot...)
	}
	return &item, nil
}

func scanNodeIdentity(src scanner, item *node.NodeIdentity) error {
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
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return err
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
