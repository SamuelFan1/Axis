ALTER TABLE regional_node_status_snapshots
    ADD COLUMN IF NOT EXISTS status_reason VARCHAR(512) NOT NULL DEFAULT '' AFTER status;

ALTER TABLE aggregated_node_status
    ADD COLUMN IF NOT EXISTS status_reason VARCHAR(512) NOT NULL DEFAULT '' AFTER status;
