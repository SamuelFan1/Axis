package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/node"
)

func (r *NodeRepository) EnsureAvailabilitySchema(ctx context.Context) error {
	const manualDDL = `
CREATE TABLE IF NOT EXISTS node_manual_maintenance (
    node_uuid VARCHAR(36) PRIMARY KEY,
    disabled TINYINT(1) NOT NULL DEFAULT 0,
    reason VARCHAR(255) NOT NULL DEFAULT 'operator',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    KEY idx_manual_maintenance_disabled (disabled)
)`
	if _, err := r.db.ExecContext(ctx, manualDDL); err != nil {
		return fmt.Errorf("create node_manual_maintenance table: %w", err)
	}

	const probeDDL = `
CREATE TABLE IF NOT EXISTS node_https_probe_state (
    observer_region VARCHAR(64) NOT NULL,
    node_uuid VARCHAR(36) NOT NULL,
    isolated TINYINT(1) NOT NULL DEFAULT 0,
    consecutive_failures INT NOT NULL DEFAULT 0,
    consecutive_successes INT NOT NULL DEFAULT 0,
    last_http_status INT NOT NULL DEFAULT 0,
    last_error VARCHAR(512) NOT NULL DEFAULT '',
    last_checked_at DATETIME(6) NULL,
    last_transition_at DATETIME(6) NULL,
    next_check_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    lease_owner VARCHAR(255) NOT NULL DEFAULT '',
    lease_expires_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (observer_region, node_uuid),
    KEY idx_https_probe_due (observer_region, next_check_at),
    KEY idx_https_probe_isolated (observer_region, isolated)
)`
	if _, err := r.runtimeDB.ExecContext(ctx, probeDDL); err != nil {
		return fmt.Errorf("create node_https_probe_state table: %w", err)
	}
	return nil
}

func (r *NodeRepository) ListIdentitiesByRegion(ctx context.Context, region string) ([]node.NodeIdentity, error) {
	const query = `SELECT` + selectNodeIdentityColumns + `
FROM managed_nodes
WHERE LOWER(TRIM(region)) = ?
ORDER BY hostname ASC, uuid ASC`
	rows, err := r.db.QueryContext(ctx, query, strings.TrimSpace(strings.ToLower(region)))
	if err != nil {
		return nil, fmt.Errorf("list managed node identities by region: %w", err)
	}
	defer rows.Close()

	var items []node.NodeIdentity
	for rows.Next() {
		var item node.NodeIdentity
		if err := scanNodeIdentity(rows, &item); err != nil {
			return nil, fmt.Errorf("scan managed node identity by region: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed node identities by region: %w", err)
	}
	return items, nil
}

func (r *NodeRepository) LoadManualDisabled(ctx context.Context, nodeUUIDs []string) (map[string]bool, error) {
	result := make(map[string]bool)
	placeholders, args := normalizedUUIDArgs(nodeUUIDs)
	if len(placeholders) == 0 {
		return result, nil
	}
	query := fmt.Sprintf(`SELECT node_uuid, disabled FROM node_manual_maintenance WHERE node_uuid IN (%s)`, strings.Join(placeholders, ","))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load manual node maintenance: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var uuid string
		var disabled bool
		if err := rows.Scan(&uuid, &disabled); err != nil {
			return nil, fmt.Errorf("scan manual node maintenance: %w", err)
		}
		result[uuid] = disabled
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate manual node maintenance: %w", err)
	}
	return result, nil
}

func (r *NodeRepository) SetManualDisabled(ctx context.Context, nodeUUID string, disabled bool) error {
	const query = `
INSERT INTO node_manual_maintenance (node_uuid, disabled, reason)
VALUES (?, ?, 'operator')
ON DUPLICATE KEY UPDATE
    disabled = VALUES(disabled),
    reason = VALUES(reason),
    updated_at = CURRENT_TIMESTAMP(6)`
	if _, err := r.db.ExecContext(ctx, query, strings.TrimSpace(nodeUUID), disabled); err != nil {
		return fmt.Errorf("set manual node maintenance: %w", err)
	}
	return nil
}

func (r *NodeRepository) DeleteManualState(ctx context.Context, nodeUUID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM node_manual_maintenance WHERE node_uuid = ?`, strings.TrimSpace(nodeUUID)); err != nil {
		return fmt.Errorf("delete manual node maintenance: %w", err)
	}
	return nil
}

func (r *NodeRepository) LoadHTTPSProbeStates(ctx context.Context, observerRegion string, nodeUUIDs []string) (map[string]node.HTTPSProbeState, error) {
	result := make(map[string]node.HTTPSProbeState)
	placeholders, uuidArgs := normalizedUUIDArgs(nodeUUIDs)
	if len(placeholders) == 0 {
		return result, nil
	}
	args := make([]interface{}, 0, len(uuidArgs)+1)
	args = append(args, strings.TrimSpace(strings.ToLower(observerRegion)))
	args = append(args, uuidArgs...)
	query := fmt.Sprintf(`SELECT
    observer_region, node_uuid, isolated, consecutive_failures, consecutive_successes,
    last_http_status, last_error, last_checked_at, last_transition_at, next_check_at,
    lease_owner, lease_expires_at, created_at, updated_at
FROM node_https_probe_state
WHERE observer_region = ? AND node_uuid IN (%s)`, strings.Join(placeholders, ","))
	rows, err := r.runtimeDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load HTTPS probe states: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		state, err := scanHTTPSProbeState(rows)
		if err != nil {
			return nil, fmt.Errorf("scan HTTPS probe state: %w", err)
		}
		result[state.NodeUUID] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate HTTPS probe states: %w", err)
	}
	return result, nil
}

func (r *NodeRepository) TryClaimHTTPSProbe(ctx context.Context, observerRegion, nodeUUID, owner string, now time.Time, leaseDuration time.Duration) (bool, error) {
	observerRegion = strings.TrimSpace(strings.ToLower(observerRegion))
	nodeUUID = strings.TrimSpace(nodeUUID)
	owner = strings.TrimSpace(owner)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if leaseDuration <= 0 {
		leaseDuration = time.Second
	}
	const insert = `
INSERT INTO node_https_probe_state (observer_region, node_uuid, next_check_at)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE node_uuid = VALUES(node_uuid)`
	if _, err := r.runtimeDB.ExecContext(ctx, insert, observerRegion, nodeUUID, now); err != nil {
		return false, fmt.Errorf("ensure HTTPS probe state: %w", err)
	}
	const claim = `
UPDATE node_https_probe_state
SET lease_owner = ?, lease_expires_at = ?, updated_at = CURRENT_TIMESTAMP(6)
WHERE observer_region = ?
  AND node_uuid = ?
  AND next_check_at <= ?
  AND (lease_expires_at IS NULL OR lease_expires_at <= ? OR lease_owner = ?)`
	result, err := r.runtimeDB.ExecContext(ctx, claim, owner, now.Add(leaseDuration), observerRegion, nodeUUID, now, now, owner)
	if err != nil {
		return false, fmt.Errorf("claim HTTPS probe state: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim HTTPS probe rows affected: %w", err)
	}
	return count > 0, nil
}

func (r *NodeRepository) RecordHTTPSProbeResult(ctx context.Context, observerRegion, nodeUUID, owner string, result node.HTTPSProbeResult, failureThreshold, recoveryThreshold int, interval time.Duration) (node.HTTPSProbeState, string, error) {
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	tx, err := r.runtimeDB.BeginTx(ctx, nil)
	if err != nil {
		return node.HTTPSProbeState{}, node.HTTPSProbeTransitionNone, fmt.Errorf("begin HTTPS probe result tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const selectQuery = `SELECT
    observer_region, node_uuid, isolated, consecutive_failures, consecutive_successes,
    last_http_status, last_error, last_checked_at, last_transition_at, next_check_at,
    lease_owner, lease_expires_at, created_at, updated_at
FROM node_https_probe_state
WHERE observer_region = ? AND node_uuid = ? AND lease_owner = ?
FOR UPDATE`
	current, err := scanHTTPSProbeState(tx.QueryRowContext(ctx, selectQuery, strings.TrimSpace(strings.ToLower(observerRegion)), strings.TrimSpace(nodeUUID), strings.TrimSpace(owner)))
	if err != nil {
		if err == sql.ErrNoRows {
			return node.HTTPSProbeState{}, node.HTTPSProbeTransitionNone, fmt.Errorf("HTTPS probe lease not held")
		}
		return node.HTTPSProbeState{}, node.HTTPSProbeTransitionNone, fmt.Errorf("load claimed HTTPS probe state: %w", err)
	}
	next, transition := node.ApplyHTTPSProbeResult(current, result, failureThreshold, recoveryThreshold)
	next.NextCheckAt = result.CheckedAt.Add(interval)
	next.LeaseOwner = ""
	next.LeaseExpiresAt = time.Time{}

	const update = `
UPDATE node_https_probe_state
SET isolated = ?, consecutive_failures = ?, consecutive_successes = ?,
    last_http_status = ?, last_error = ?, last_checked_at = ?, last_transition_at = ?,
    next_check_at = ?, lease_owner = '', lease_expires_at = NULL,
    updated_at = CURRENT_TIMESTAMP(6)
WHERE observer_region = ? AND node_uuid = ? AND lease_owner = ?`
	updateResult, err := tx.ExecContext(
		ctx,
		update,
		next.Isolated,
		next.ConsecutiveFailures,
		next.ConsecutiveSuccesses,
		next.LastHTTPStatus,
		next.LastError,
		nullTime(next.LastCheckedAt),
		nullTime(next.LastTransitionAt),
		next.NextCheckAt,
		strings.TrimSpace(strings.ToLower(observerRegion)),
		strings.TrimSpace(nodeUUID),
		strings.TrimSpace(owner),
	)
	if err != nil {
		return node.HTTPSProbeState{}, node.HTTPSProbeTransitionNone, fmt.Errorf("update HTTPS probe result: %w", err)
	}
	if count, err := updateResult.RowsAffected(); err != nil || count != 1 {
		if err != nil {
			return node.HTTPSProbeState{}, node.HTTPSProbeTransitionNone, fmt.Errorf("update HTTPS probe rows affected: %w", err)
		}
		return node.HTTPSProbeState{}, node.HTTPSProbeTransitionNone, fmt.Errorf("HTTPS probe lease lost")
	}
	if err := tx.Commit(); err != nil {
		return node.HTTPSProbeState{}, node.HTTPSProbeTransitionNone, fmt.Errorf("commit HTTPS probe result: %w", err)
	}
	return next, transition, nil
}

func (r *NodeRepository) ListIsolatedHTTPSProbeStates(ctx context.Context, observerRegion string) ([]node.HTTPSProbeState, error) {
	const query = `SELECT
    observer_region, node_uuid, isolated, consecutive_failures, consecutive_successes,
    last_http_status, last_error, last_checked_at, last_transition_at, next_check_at,
    lease_owner, lease_expires_at, created_at, updated_at
FROM node_https_probe_state
WHERE observer_region = ? AND isolated = 1
ORDER BY node_uuid ASC`
	rows, err := r.runtimeDB.QueryContext(ctx, query, strings.TrimSpace(strings.ToLower(observerRegion)))
	if err != nil {
		return nil, fmt.Errorf("list isolated HTTPS probe states: %w", err)
	}
	defer rows.Close()
	var states []node.HTTPSProbeState
	for rows.Next() {
		state, err := scanHTTPSProbeState(rows)
		if err != nil {
			return nil, fmt.Errorf("scan isolated HTTPS probe state: %w", err)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate isolated HTTPS probe states: %w", err)
	}
	return states, nil
}

func (r *NodeRepository) CleanupOrphanedHTTPSProbeRows(ctx context.Context, observerRegion string) (int, error) {
	identities, err := r.ListIdentitiesByRegion(ctx, observerRegion)
	if err != nil {
		return 0, err
	}
	valid := make(map[string]struct{}, len(identities))
	for _, item := range identities {
		valid[item.UUID] = struct{}{}
	}
	rows, err := r.runtimeDB.QueryContext(ctx, `SELECT node_uuid FROM node_https_probe_state WHERE observer_region = ?`, strings.TrimSpace(strings.ToLower(observerRegion)))
	if err != nil {
		return 0, fmt.Errorf("list HTTPS probe rows for cleanup: %w", err)
	}
	defer rows.Close()
	var orphaned []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return 0, fmt.Errorf("scan HTTPS probe cleanup row: %w", err)
		}
		if _, ok := valid[uuid]; !ok {
			orphaned = append(orphaned, uuid)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate HTTPS probe cleanup rows: %w", err)
	}
	placeholders, uuidArgs := normalizedUUIDArgs(orphaned)
	if len(placeholders) == 0 {
		return 0, nil
	}
	args := make([]interface{}, 0, len(uuidArgs)+1)
	args = append(args, strings.TrimSpace(strings.ToLower(observerRegion)))
	args = append(args, uuidArgs...)
	result, err := r.runtimeDB.ExecContext(ctx, fmt.Sprintf(`DELETE FROM node_https_probe_state WHERE observer_region = ? AND node_uuid IN (%s)`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return 0, fmt.Errorf("delete orphaned HTTPS probe rows: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete orphaned HTTPS probe rows affected: %w", err)
	}
	return int(count), nil
}

func scanHTTPSProbeState(src scanner) (node.HTTPSProbeState, error) {
	var state node.HTTPSProbeState
	var lastCheckedAt sql.NullTime
	var lastTransitionAt sql.NullTime
	var leaseExpiresAt sql.NullTime
	if err := src.Scan(
		&state.ObserverRegion,
		&state.NodeUUID,
		&state.Isolated,
		&state.ConsecutiveFailures,
		&state.ConsecutiveSuccesses,
		&state.LastHTTPStatus,
		&state.LastError,
		&lastCheckedAt,
		&lastTransitionAt,
		&state.NextCheckAt,
		&state.LeaseOwner,
		&leaseExpiresAt,
		&state.CreatedAt,
		&state.UpdatedAt,
	); err != nil {
		return node.HTTPSProbeState{}, err
	}
	if lastCheckedAt.Valid {
		state.LastCheckedAt = lastCheckedAt.Time
	}
	if lastTransitionAt.Valid {
		state.LastTransitionAt = lastTransitionAt.Time
	}
	if leaseExpiresAt.Valid {
		state.LeaseExpiresAt = leaseExpiresAt.Time
	}
	return state, nil
}

func normalizedUUIDArgs(nodeUUIDs []string) ([]string, []interface{}) {
	placeholders := make([]string, 0, len(nodeUUIDs))
	args := make([]interface{}, 0, len(nodeUUIDs))
	seen := make(map[string]struct{}, len(nodeUUIDs))
	for _, raw := range nodeUUIDs {
		uuid := strings.TrimSpace(raw)
		if uuid == "" {
			continue
		}
		if _, ok := seen[uuid]; ok {
			continue
		}
		seen[uuid] = struct{}{}
		placeholders = append(placeholders, "?")
		args = append(args, uuid)
	}
	return placeholders, args
}
