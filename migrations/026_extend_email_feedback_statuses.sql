ALTER TABLE email_messages
    DROP CONSTRAINT IF EXISTS chk_email_status;

ALTER TABLE email_messages
    ADD CONSTRAINT chk_email_status CHECK (status IN (
        'queued',
        'processing',
        'submission_unknown',
        'submitted',
        'delivered',
        'partially_delivered',
        'delayed',
        'partially_failed',
        'bounced',
        'complained',
        'rejected',
        'failed',
        'canceled'
    ));
