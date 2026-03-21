package mysql

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/SamuelFan1/Axis/internal/domain/node"
)

func TestNodeRepositoryUpsertIdentityIncludesStatusOnInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	defer db.Close()

	repo := NewNodeRepository(db)
	item := node.NodeIdentity{
		UUID:              "11111111-1111-1111-1111-111111111111",
		Hostname:          "node-1",
		ManagementAddress: "10.0.0.1:9090",
		InternalIP:        "10.0.0.1",
		PublicIP:          "1.1.1.1",
		Region:            "asia",
		RegionUUID:        "region-asia",
		Zone:              "SG",
		ZoneUUID:          "zone-sg",
		Status:            node.StatusUp,
	}

	mock.ExpectExec(regexp.QuoteMeta(`
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
    updated_at = CURRENT_TIMESTAMP(6)`)).
		WithArgs(
			item.UUID,
			item.Hostname,
			item.ManagementAddress,
			item.InternalIP,
			item.PublicIP,
			item.Region,
			item.RegionUUID,
			item.Zone,
			item.ZoneUUID,
			item.Status,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpsertIdentity(context.Background(), item); err != nil {
		t.Fatalf("UpsertIdentity returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestNodeRepositoryUpsertHealthBindsLastReportedAt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	defer db.Close()

	repo := NewNodeRepository(db)
	reportedAt := time.Now().UTC()
	item := node.NodeHealth{
		ObserverRegion: "asia",
		NodeUUID:       "11111111-1111-1111-1111-111111111111",
		Status:         node.StatusUp,
		StatusSource:   "self_report",
		LastReportedAt: reportedAt,
	}

	mock.ExpectExec(regexp.QuoteMeta(`
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
    last_reported_at = VALUES(last_reported_at)`)).
		WithArgs(item.ObserverRegion, item.NodeUUID, item.Status, item.StatusSource, reportedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO managed_node_metrics_ext (
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
    updated_at = CURRENT_TIMESTAMP(6)`)).
		WithArgs(
			item.NodeUUID,
			0,
			0.0,
			0.0,
			0.0,
			0.0,
			0.0,
			0.0,
			0.0,
			0.0,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpsertHealth(context.Background(), item); err != nil {
		t.Fatalf("UpsertHealth returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
