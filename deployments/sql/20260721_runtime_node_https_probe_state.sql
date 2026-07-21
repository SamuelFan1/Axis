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
);
