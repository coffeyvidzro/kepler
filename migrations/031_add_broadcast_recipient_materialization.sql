ALTER TABLE broadcasts
    ADD COLUMN IF NOT EXISTS recipients_materialized_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_broadcasts_pending_materialization
    ON broadcasts (queued_at, id)
    WHERE status = 'queued'
      AND recipients_materialized_at IS NULL
      AND deleted_at IS NULL;
