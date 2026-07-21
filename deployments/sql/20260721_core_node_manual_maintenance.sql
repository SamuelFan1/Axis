CREATE TABLE IF NOT EXISTS node_manual_maintenance (
    node_uuid VARCHAR(36) PRIMARY KEY,
    disabled TINYINT(1) NOT NULL DEFAULT 0,
    reason VARCHAR(255) NOT NULL DEFAULT 'operator',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    KEY idx_manual_maintenance_disabled (disabled)
);
