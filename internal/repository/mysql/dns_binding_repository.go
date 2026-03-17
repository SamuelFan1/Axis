package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
	"github.com/SamuelFan1/Axis/internal/repository"
	mysqldriver "github.com/go-sql-driver/mysql"
)

type DNSBindingRepository struct {
	db *sql.DB
}

func NewDNSBindingRepository(db *sql.DB) *DNSBindingRepository {
	return &DNSBindingRepository{db: db}
}

func (r *DNSBindingRepository) EnsureSchema(ctx context.Context) error {
	const bindingsDDL = `
CREATE TABLE IF NOT EXISTS dns_bindings (
    node_uuid VARCHAR(36) PRIMARY KEY,
    dns_label VARCHAR(64) NOT NULL,
    dns_name VARCHAR(255) NOT NULL,
    zone VARCHAR(255) NOT NULL,
    record_prefix VARCHAR(32) NOT NULL,
    sequence INT NOT NULL,
    last_public_ip VARCHAR(64) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY uk_dns_bindings_label (dns_label),
    UNIQUE KEY uk_dns_bindings_name (dns_name),
    KEY idx_dns_bindings_sequence (zone, record_prefix, sequence)
)`
	if _, err := r.db.ExecContext(ctx, bindingsDDL); err != nil {
		return fmt.Errorf("create dns_bindings table: %w", err)
	}

	for _, stmt := range []string{
		`ALTER TABLE dns_bindings ADD COLUMN IF NOT EXISTS dns_label VARCHAR(64) NOT NULL`,
		`ALTER TABLE dns_bindings ADD COLUMN IF NOT EXISTS dns_name VARCHAR(255) NOT NULL`,
		`ALTER TABLE dns_bindings ADD COLUMN IF NOT EXISTS zone VARCHAR(255) NOT NULL`,
		`ALTER TABLE dns_bindings ADD COLUMN IF NOT EXISTS record_prefix VARCHAR(32) NOT NULL`,
		`ALTER TABLE dns_bindings ADD COLUMN IF NOT EXISTS sequence INT NOT NULL`,
		`ALTER TABLE dns_bindings ADD COLUMN IF NOT EXISTS last_public_ip VARCHAR(64) NOT NULL DEFAULT ''`,
		`ALTER TABLE dns_bindings ADD COLUMN IF NOT EXISTS created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)`,
		`ALTER TABLE dns_bindings ADD COLUMN IF NOT EXISTS updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("upgrade dns_bindings table: %w", err)
		}
	}
	if err := ensureUniqueIndex(ctx, r.db, "dns_bindings", "uk_dns_bindings_label", "dns_label"); err != nil {
		return err
	}
	if err := ensureUniqueIndex(ctx, r.db, "dns_bindings", "uk_dns_bindings_name", "dns_name"); err != nil {
		return err
	}
	if err := ensureIndex(ctx, r.db, "dns_bindings", "idx_dns_bindings_sequence", "zone, record_prefix, sequence"); err != nil {
		return err
	}

	const countersDDL = `
CREATE TABLE IF NOT EXISTS dns_binding_counters (
    zone VARCHAR(255) NOT NULL,
    record_prefix VARCHAR(32) NOT NULL,
    next_sequence INT NOT NULL,
    PRIMARY KEY (zone, record_prefix)
)`
	if _, err := r.db.ExecContext(ctx, countersDDL); err != nil {
		return fmt.Errorf("create dns_binding_counters table: %w", err)
	}

	for _, stmt := range []string{
		`ALTER TABLE dns_binding_counters ADD COLUMN IF NOT EXISTS next_sequence INT NOT NULL`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("upgrade dns_binding_counters table: %w", err)
		}
	}

	return nil
}

func (r *DNSBindingRepository) GetByNodeUUID(ctx context.Context, nodeUUID string) (*dnsbinding.Binding, error) {
	const query = `
SELECT node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip, created_at, updated_at
FROM dns_bindings
WHERE node_uuid = ?
LIMIT 1`

	var item dnsbinding.Binding
	if err := scanDNSBinding(r.db.QueryRowContext(ctx, query, strings.TrimSpace(nodeUUID)), &item); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find dns binding by node uuid: %w", err)
	}
	return &item, nil
}

func (r *DNSBindingRepository) GetByDNSLabel(ctx context.Context, label string) (*dnsbinding.Binding, error) {
	const query = `
SELECT node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip, created_at, updated_at
FROM dns_bindings
WHERE dns_label = ?
LIMIT 1`

	var item dnsbinding.Binding
	if err := scanDNSBinding(r.db.QueryRowContext(ctx, query, strings.TrimSpace(label)), &item); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find dns binding by label: %w", err)
	}
	return &item, nil
}

func (r *DNSBindingRepository) AllocateForNode(ctx context.Context, nodeUUID string, zone string, prefix string) (*dnsbinding.Binding, error) {
	nodeUUID = strings.TrimSpace(nodeUUID)
	zone = strings.Trim(strings.TrimSpace(zone), ".")
	prefix = strings.TrimSpace(prefix)

	if nodeUUID == "" {
		return nil, fmt.Errorf("node uuid is required")
	}
	if zone == "" {
		return nil, fmt.Errorf("dns zone is required")
	}
	if prefix == "" {
		return nil, fmt.Errorf("dns record prefix is required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin dns binding allocation tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := ensureCounterRowTx(ctx, tx, zone, prefix); err != nil {
		return nil, err
	}

	for {
		existing, err := getBindingByNodeUUIDTx(ctx, tx, nodeUUID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("commit existing dns binding allocation: %w", err)
			}
			tx = nil
			return existing, nil
		}

		nextSequence, err := getNextSequenceForUpdateTx(ctx, tx, zone, prefix)
		if err != nil {
			return nil, err
		}
		if nextSequence <= 0 {
			nextSequence = 1
		}

		label := platformdns.BuildDNSLabel(prefix, nextSequence)
		name := platformdns.BuildDNSName(label, zone)
		binding := dnsbinding.Binding{
			NodeUUID:     nodeUUID,
			DNSLabel:     label,
			DNSName:      name,
			Zone:         zone,
			RecordPrefix: prefix,
			Sequence:     nextSequence,
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO dns_bindings (
			    node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip
			) VALUES (?, ?, ?, ?, ?, ?, '')`,
			binding.NodeUUID,
			binding.DNSLabel,
			binding.DNSName,
			binding.Zone,
			binding.RecordPrefix,
			binding.Sequence,
		); err != nil {
			if isDuplicateEntryError(err) {
				if err := setNextSequenceTx(ctx, tx, zone, prefix, nextSequence+1); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("insert dns binding: %w", err)
		}

		if err := setNextSequenceTx(ctx, tx, zone, prefix, nextSequence+1); err != nil {
			return nil, err
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit dns binding allocation: %w", err)
		}
		tx = nil
		return &binding, nil
	}
}

func (r *DNSBindingRepository) UpdateLastPublicIP(ctx context.Context, nodeUUID string, publicIP string) error {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE dns_bindings
		 SET last_public_ip = ?, updated_at = CURRENT_TIMESTAMP(6)
		 WHERE node_uuid = ?`,
		strings.TrimSpace(publicIP),
		strings.TrimSpace(nodeUUID),
	)
	if err != nil {
		return fmt.Errorf("update dns binding last public ip: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update dns binding last public ip rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *DNSBindingRepository) SeedFromManagedNodes(ctx context.Context, zone string, prefix string) (repository.DNSBindingSeedResult, error) {
	zone = strings.Trim(strings.TrimSpace(zone), ".")
	prefix = strings.TrimSpace(prefix)

	result := repository.DNSBindingSeedResult{}
	if zone == "" || prefix == "" {
		return result, nil
	}

	rows, err := r.db.QueryContext(
		ctx,
		`SELECT uuid, dns_label, dns_name, public_ip
		 FROM managed_nodes
		 WHERE dns_label IS NOT NULL
		   AND dns_name IS NOT NULL`,
	)
	if err != nil {
		return result, fmt.Errorf("list managed node dns bindings for seed: %w", err)
	}
	defer rows.Close()

	expectedSuffix := "." + zone
	for rows.Next() {
		var nodeUUID string
		var dnsLabel sql.NullString
		var dnsName sql.NullString
		var publicIP sql.NullString
		if err := rows.Scan(&nodeUUID, &dnsLabel, &dnsName, &publicIP); err != nil {
			return result, fmt.Errorf("scan managed node dns binding for seed: %w", err)
		}

		label := strings.TrimSpace(dnsLabel.String)
		name := strings.TrimSpace(dnsName.String)
		if label == "" || name == "" {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(name), strings.ToLower(expectedSuffix)) {
			continue
		}

		sequence, ok := platformdns.ParseDNSSequence(prefix, label)
		if !ok {
			continue
		}
		if sequence > result.ManagedNodesMaxSequence {
			result.ManagedNodesMaxSequence = sequence
		}

		insertResult, err := r.db.ExecContext(
			ctx,
			`INSERT IGNORE INTO dns_bindings (
			    node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip
			) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			strings.TrimSpace(nodeUUID),
			label,
			name,
			zone,
			prefix,
			sequence,
			strings.TrimSpace(publicIP.String),
		)
		if err != nil {
			return result, fmt.Errorf("seed dns binding from managed_nodes: %w", err)
		}
		rowsAffected, err := insertResult.RowsAffected()
		if err == nil && rowsAffected > 0 {
			result.SeededCount += int(rowsAffected)
		}
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate managed node dns bindings for seed: %w", err)
	}

	maxSequence, err := maxSequenceFromDNSBindings(ctx, r.db, zone, prefix)
	if err != nil {
		return result, err
	}
	result.DNSBindingsMaxSequence = maxSequence
	return result, nil
}

func (r *DNSBindingRepository) EnsureCounterFloor(ctx context.Context, zone string, prefix string, floor int) error {
	zone = strings.Trim(strings.TrimSpace(zone), ".")
	prefix = strings.TrimSpace(prefix)
	if zone == "" || prefix == "" {
		return nil
	}

	if floor < 0 {
		floor = 0
	}
	nextSequence := floor + 1
	if _, err := r.db.ExecContext(
		ctx,
		`INSERT INTO dns_binding_counters (zone, record_prefix, next_sequence)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE next_sequence = GREATEST(next_sequence, ?)`,
		zone,
		prefix,
		nextSequence,
		nextSequence,
	); err != nil {
		return fmt.Errorf("ensure dns binding counter floor: %w", err)
	}
	return nil
}

func scanDNSBinding(src scanner, item *dnsbinding.Binding) error {
	return src.Scan(
		&item.NodeUUID,
		&item.DNSLabel,
		&item.DNSName,
		&item.Zone,
		&item.RecordPrefix,
		&item.Sequence,
		&item.LastPublicIP,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
}

func getBindingByNodeUUIDTx(ctx context.Context, tx *sql.Tx, nodeUUID string) (*dnsbinding.Binding, error) {
	const query = `
SELECT node_uuid, dns_label, dns_name, zone, record_prefix, sequence, last_public_ip, created_at, updated_at
FROM dns_bindings
WHERE node_uuid = ?
LIMIT 1`

	var item dnsbinding.Binding
	if err := scanDNSBinding(tx.QueryRowContext(ctx, query, strings.TrimSpace(nodeUUID)), &item); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find dns binding by node uuid in tx: %w", err)
	}
	return &item, nil
}

func ensureCounterRowTx(ctx context.Context, tx *sql.Tx, zone string, prefix string) error {
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO dns_binding_counters (zone, record_prefix, next_sequence)
		 VALUES (?, ?, 1)
		 ON DUPLICATE KEY UPDATE next_sequence = next_sequence`,
		zone,
		prefix,
	); err != nil {
		return fmt.Errorf("ensure dns binding counter row: %w", err)
	}
	return nil
}

func getNextSequenceForUpdateTx(ctx context.Context, tx *sql.Tx, zone string, prefix string) (int, error) {
	var nextSequence int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT next_sequence
		 FROM dns_binding_counters
		 WHERE zone = ? AND record_prefix = ?
		 FOR UPDATE`,
		zone,
		prefix,
	).Scan(&nextSequence); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("dns binding counter row not found")
		}
		return 0, fmt.Errorf("select dns binding next sequence for update: %w", err)
	}
	return nextSequence, nil
}

func setNextSequenceTx(ctx context.Context, tx *sql.Tx, zone string, prefix string, nextSequence int) error {
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE dns_binding_counters
		 SET next_sequence = ?
		 WHERE zone = ? AND record_prefix = ?`,
		nextSequence,
		zone,
		prefix,
	); err != nil {
		return fmt.Errorf("update dns binding next sequence: %w", err)
	}
	return nil
}

func maxSequenceFromDNSBindings(ctx context.Context, db *sql.DB, zone string, prefix string) (int, error) {
	var value sql.NullInt64
	if err := db.QueryRowContext(
		ctx,
		`SELECT MAX(sequence)
		 FROM dns_bindings
		 WHERE zone = ? AND record_prefix = ?`,
		zone,
		prefix,
	).Scan(&value); err != nil {
		return 0, fmt.Errorf("query max dns binding sequence: %w", err)
	}
	if !value.Valid {
		return 0, nil
	}
	return int(value.Int64), nil
}

func isDuplicateEntryError(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
