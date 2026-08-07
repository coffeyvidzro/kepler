ALTER TABLE broadcast_recipients
    ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at TIMESTAMPTZ,
    ADD COLUMN last_error_code TEXT,
    ADD COLUMN last_error_message TEXT,
    ADD COLUMN failed_at TIMESTAMPTZ,
    ADD CONSTRAINT chk_broadcast_recipients_attempt_count CHECK (attempt_count >= 0);

CREATE INDEX idx_broadcast_recipients_pending_fanout
    ON broadcast_recipients (next_attempt_at, broadcast_id, id)
    WHERE status = 'pending';
