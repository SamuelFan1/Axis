package mysql

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestTryClaimHTTPSProbeReturnsFalseWhenAnotherAxisOwnsLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	repo := NewNodeRepository(db, db)
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO node_https_probe_state (observer_region, node_uuid, next_check_at)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE node_uuid = VALUES(node_uuid)`)).
		WithArgs("asia", "node-1", now).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE node_https_probe_state").
		WithArgs("axis-b", now.Add(time.Second), "asia", "node-1", now, now, "axis-b").
		WillReturnResult(sqlmock.NewResult(0, 0))

	claimed, err := repo.TryClaimHTTPSProbe(context.Background(), "asia", "node-1", "axis-b", now, time.Second)
	if err != nil {
		t.Fatalf("TryClaimHTTPSProbe returned error: %v", err)
	}
	if claimed {
		t.Fatal("expected competing Axis lease claim to be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
