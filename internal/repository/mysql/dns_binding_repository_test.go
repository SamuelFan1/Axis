package mysql

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestDNSBindingRepositoryList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	defer db.Close()

	repo := NewDNSBindingRepository(db)
	createdAt := time.Now().UTC().Add(-time.Hour)
	updatedAt := time.Now().UTC()

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip, created_at, updated_at
FROM dns_bindings
ORDER BY zone, record_prefix, sequence, node_uuid`)).
		WillReturnRows(sqlmock.NewRows([]string{
			"node_uuid", "dns_label", "dns_name", "zone", "record_prefix", "sequence", "last_public_ip", "created_at", "updated_at",
		}).
			AddRow("node-1", "dl-001", "dl-001.example.com", "example.com", "dl-", 1, "1.1.1.1", createdAt, updatedAt).
			AddRow("node-2", "dl-002", "dl-002.example.com", "example.com", "dl-", 2, "2.2.2.2", createdAt, updatedAt))

	bindings, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}
	if bindings[0].NodeUUID != "node-1" || bindings[0].DNSName != "dl-001.example.com" || bindings[0].Sequence != 1 {
		t.Fatalf("unexpected first binding: %+v", bindings[0])
	}
	if bindings[1].NodeUUID != "node-2" || bindings[1].LastPublicIP != "2.2.2.2" || !bindings[1].UpdatedAt.Equal(updatedAt) {
		t.Fatalf("unexpected second binding: %+v", bindings[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestDNSBindingRepositoryListReturnsQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	defer db.Close()

	repo := NewDNSBindingRepository(db)
	queryErr := errors.New("query failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip, created_at, updated_at
FROM dns_bindings
ORDER BY zone, record_prefix, sequence, node_uuid`)).
		WillReturnError(queryErr)

	bindings, err := repo.List(context.Background())
	if !errors.Is(err, queryErr) {
		t.Fatalf("expected query error, got bindings=%+v err=%v", bindings, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestDNSBindingRepositoryAllocateForNodeReturnsExistingBinding(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	defer db.Close()

	repo := NewDNSBindingRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO dns_binding_counters (zone, record_prefix, next_sequence)
		 VALUES (?, ?, 1)
		 ON DUPLICATE KEY UPDATE next_sequence = next_sequence`)).
		WithArgs("example.com", "dl-").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip, created_at, updated_at
FROM dns_bindings
WHERE node_uuid = ?
LIMIT 1`)).
		WithArgs("node-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"node_uuid", "dns_label", "dns_name", "zone", "record_prefix", "sequence", "last_public_ip", "created_at", "updated_at",
		}).AddRow("node-1", "dl-007", "dl-007.example.com", "example.com", "dl-", 7, "1.1.1.1", time.Now().UTC(), time.Now().UTC()))
	mock.ExpectCommit()

	binding, err := repo.AllocateForNode(context.Background(), "node-1", "example.com", "dl-")
	if err != nil {
		t.Fatalf("AllocateForNode returned error: %v", err)
	}
	if binding == nil {
		t.Fatal("expected binding, got nil")
	}
	if binding.DNSLabel != "dl-007" || binding.DNSName != "dl-007.example.com" {
		t.Fatalf("unexpected binding: %+v", binding)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestDNSBindingRepositoryAllocateForNodeSkipsDuplicateLabels(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	defer db.Close()

	repo := NewDNSBindingRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO dns_binding_counters (zone, record_prefix, next_sequence)
		 VALUES (?, ?, 1)
		 ON DUPLICATE KEY UPDATE next_sequence = next_sequence`)).
		WithArgs("example.com", "dl-").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip, created_at, updated_at
FROM dns_bindings
WHERE node_uuid = ?
LIMIT 1`)).
		WithArgs("node-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"node_uuid", "dns_label", "dns_name", "zone", "record_prefix", "sequence", "last_public_ip", "created_at", "updated_at",
		}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT next_sequence
		 FROM dns_binding_counters
		 WHERE zone = ? AND record_prefix = ?
		 FOR UPDATE`)).
		WithArgs("example.com", "dl-").
		WillReturnRows(sqlmock.NewRows([]string{"next_sequence"}).AddRow(7))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO dns_bindings (
			    node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip
			) VALUES (?, ?, ?, ?, ?, ?, '')`)).
		WithArgs("node-1", "dl-007", "dl-007.example.com", "example.com", "dl-", 7).
		WillReturnError(&mysqldriver.MySQLError{Number: 1062, Message: "duplicate label"})
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE dns_binding_counters
		 SET next_sequence = ?
		 WHERE zone = ? AND record_prefix = ?`)).
		WithArgs(8, "example.com", "dl-").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip, created_at, updated_at
FROM dns_bindings
WHERE node_uuid = ?
LIMIT 1`)).
		WithArgs("node-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"node_uuid", "dns_label", "dns_name", "zone", "record_prefix", "sequence", "last_public_ip", "created_at", "updated_at",
		}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT next_sequence
		 FROM dns_binding_counters
		 WHERE zone = ? AND record_prefix = ?
		 FOR UPDATE`)).
		WithArgs("example.com", "dl-").
		WillReturnRows(sqlmock.NewRows([]string{"next_sequence"}).AddRow(8))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO dns_bindings (
			    node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip
			) VALUES (?, ?, ?, ?, ?, ?, '')`)).
		WithArgs("node-1", "dl-008", "dl-008.example.com", "example.com", "dl-", 8).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE dns_binding_counters
		 SET next_sequence = ?
		 WHERE zone = ? AND record_prefix = ?`)).
		WithArgs(9, "example.com", "dl-").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	binding, err := repo.AllocateForNode(context.Background(), "node-1", "example.com", "dl-")
	if err != nil {
		t.Fatalf("AllocateForNode returned error: %v", err)
	}
	if binding == nil {
		t.Fatal("expected binding, got nil")
	}
	if binding.Sequence != 8 || binding.DNSLabel != "dl-008" {
		t.Fatalf("expected dl-008 after duplicate skip, got %+v", binding)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestDNSBindingRepositorySeedFromManagedNodes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	defer db.Close()

	repo := NewDNSBindingRepository(db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT uuid, dns_label, dns_name, public_ip
		 FROM managed_nodes
		 WHERE dns_label IS NOT NULL
		   AND dns_name IS NOT NULL`)).
		WillReturnRows(sqlmock.NewRows([]string{"uuid", "dns_label", "dns_name", "public_ip"}).
			AddRow("node-1", "dl-021", "dl-021.example.com", "1.1.1.1").
			AddRow("node-2", "bad-001", "bad-001.example.com", "2.2.2.2").
			AddRow("node-3", "dl-099", "dl-099.other.com", "3.3.3.3"))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT IGNORE INTO dns_bindings (
			    node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip
			) VALUES (?, ?, ?, ?, ?, ?, ?)`)).
		WithArgs("node-1", "dl-021", "dl-021.example.com", "example.com", "dl-", 21, "1.1.1.1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT MAX(sequence)
		 FROM dns_bindings
		 WHERE zone = ? AND record_prefix = ?`)).
		WithArgs("example.com", "dl-").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(21))

	result, err := repo.SeedFromManagedNodes(context.Background(), "example.com", "dl-")
	if err != nil {
		t.Fatalf("SeedFromManagedNodes returned error: %v", err)
	}
	if result.ManagedNodesMaxSequence != 21 {
		t.Fatalf("expected managed nodes max sequence 21, got %d", result.ManagedNodesMaxSequence)
	}
	if result.DNSBindingsMaxSequence != 21 {
		t.Fatalf("expected dns bindings max sequence 21, got %d", result.DNSBindingsMaxSequence)
	}
	if result.SeededCount != 1 {
		t.Fatalf("expected 1 seeded row, got %d", result.SeededCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestDNSBindingRepositoryEnsureCounterFloor(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	defer db.Close()

	repo := NewDNSBindingRepository(db)

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO dns_binding_counters (zone, record_prefix, next_sequence)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE next_sequence = GREATEST(next_sequence, ?)`)).
		WithArgs("example.com", "dl-", 22, 22).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.EnsureCounterFloor(context.Background(), "example.com", "dl-", 21); err != nil {
		t.Fatalf("EnsureCounterFloor returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
