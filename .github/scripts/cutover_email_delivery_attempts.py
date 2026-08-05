from pathlib import Path

migration_022 = Path("migrations/022_create_email_delivery_attempts.sql")
if not migration_022.exists():
    print("email delivery attempt cutover already applied")
    raise SystemExit(0)

migration_013 = Path("migrations/013_create_email_messages.sql")
source = migration_013.read_text()
old = "    provider_message_id TEXT,\n    error_code TEXT,"
new = "    provider_message_id TEXT,\n    current_delivery_attempt_id UUID,\n    error_code TEXT,"
if source.count(old) != 1:
    raise SystemExit("could not add current_delivery_attempt_id to email_messages")
source = source.replace(old, new)

old = "        'queued', 'processing', 'submitted', 'delivered', 'delayed',\n        'bounced', 'complained', 'rejected', 'failed', 'canceled'"
new = "        'queued', 'processing', 'submission_unknown', 'submitted',\n        'delivered', 'delayed', 'bounced', 'complained', 'rejected',\n        'failed', 'canceled'"
if source.count(old) != 1:
    raise SystemExit("could not move submission_unknown into the canonical email status constraint")
source = source.replace(old, new)
source += "\nCREATE INDEX IF NOT EXISTS idx_email_messages_submission_unknown\n    ON email_messages (updated_at)\n    WHERE status = 'submission_unknown';\n"
migration_013.write_text(source)

Path("migrations/028_create_message_delivery_attempts.sql").write_text("""-- Create the channel-neutral provider-attempt ledger. Sender trust-plane tables
-- are created earlier so message records and attempts can reference canonical
-- sender assets and provider bindings from the beginning.

CREATE TABLE IF NOT EXISTS message_delivery_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    email_message_id UUID,
    sms_message_id UUID,
    attempt_number INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'claimed',
    provider TEXT,
    provider_account TEXT NOT NULL DEFAULT 'default',
    provider_message_id TEXT,
    provider_status TEXT,
    sender_asset_id UUID REFERENCES sender_assets(id) ON DELETE SET NULL,
    sender_provider_binding_id UUID,
    error_code TEXT,
    error_message TEXT,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    request_started_at TIMESTAMPTZ,
    request_completed_at TIMESTAMPTZ,
    submitted_at TIMESTAMPTZ,
    terminal_at TIMESTAMPTZ,
    next_reconcile_at TIMESTAMPTZ,
    last_reconciled_at TIMESTAMPTZ,
    reconcile_attempts INTEGER NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_message_delivery_attempts_email_team
        FOREIGN KEY (email_message_id, team_id)
        REFERENCES email_messages (id, team_id)
        ON DELETE CASCADE,

    CONSTRAINT fk_message_delivery_attempts_sms_team
        FOREIGN KEY (sms_message_id, team_id)
        REFERENCES sms_messages (id, team_id)
        ON DELETE CASCADE,

    CONSTRAINT fk_message_delivery_attempts_sender_binding
        FOREIGN KEY (sender_provider_binding_id, sender_asset_id)
        REFERENCES sender_provider_bindings (id, sender_asset_id)
        ON DELETE RESTRICT,

    CONSTRAINT uq_message_delivery_attempts_email_reference
        UNIQUE (id, email_message_id, team_id),

    CONSTRAINT chk_message_delivery_attempts_channel
        CHECK (channel IN ('email', 'sms')),

    CONSTRAINT chk_message_delivery_attempts_message
        CHECK (
            (
                channel = 'email'
                AND email_message_id IS NOT NULL
                AND sms_message_id IS NULL
            )
            OR (
                channel = 'sms'
                AND sms_message_id IS NOT NULL
                AND email_message_id IS NULL
            )
        ),

    CONSTRAINT chk_message_delivery_attempts_number
        CHECK (attempt_number > 0),

    CONSTRAINT chk_message_delivery_attempts_status
        CHECK (status IN (
            'claimed',
            'request_started',
            'submission_unknown',
            'submitted',
            'accepted',
            'sent',
            'delivered',
            'retryable_failure',
            'permanent_failure',
            'rejected',
            'expired',
            'canceled',
            'unknown'
        )),

    CONSTRAINT chk_message_delivery_attempts_provider
        CHECK (provider IS NULL OR length(trim(provider)) > 0),

    CONSTRAINT chk_message_delivery_attempts_provider_account
        CHECK (length(trim(provider_account)) > 0),

    CONSTRAINT chk_message_delivery_attempts_sender_reference
        CHECK (
            sender_provider_binding_id IS NULL
            OR sender_asset_id IS NOT NULL
        ),

    CONSTRAINT chk_message_delivery_attempts_reconcile_attempts
        CHECK (reconcile_attempts >= 0),

    CONSTRAINT chk_message_delivery_attempts_metadata
        CHECK (jsonb_typeof(metadata) = 'object'),

    CONSTRAINT chk_message_delivery_attempts_timestamps
        CHECK (
            (request_started_at IS NULL OR request_started_at >= claimed_at)
            AND (
                request_completed_at IS NULL
                OR request_started_at IS NULL
                OR request_completed_at >= request_started_at
            )
            AND (submitted_at IS NULL OR submitted_at >= claimed_at)
            AND (terminal_at IS NULL OR terminal_at >= claimed_at)
        )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_message_delivery_attempts_email_number
    ON message_delivery_attempts (email_message_id, attempt_number)
    WHERE email_message_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_message_delivery_attempts_sms_number
    ON message_delivery_attempts (sms_message_id, attempt_number)
    WHERE sms_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_message_delivery_attempts_provider_message
    ON message_delivery_attempts (provider, provider_message_id)
    WHERE provider IS NOT NULL AND provider_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_message_delivery_attempts_message_created
    ON message_delivery_attempts (
        channel,
        COALESCE(email_message_id, sms_message_id),
        created_at DESC
    );

CREATE INDEX IF NOT EXISTS idx_message_delivery_attempts_reconciliation
    ON message_delivery_attempts (next_reconcile_at, created_at)
    WHERE status IN (
        'submission_unknown',
        'submitted',
        'accepted',
        'sent',
        'unknown'
    )
      AND next_reconcile_at IS NOT NULL;

ALTER TABLE email_messages
    ADD CONSTRAINT fk_email_messages_current_delivery_attempt
    FOREIGN KEY (current_delivery_attempt_id, id, team_id)
    REFERENCES message_delivery_attempts (id, email_message_id, team_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX IF NOT EXISTS idx_email_messages_current_delivery_attempt
    ON email_messages (current_delivery_attempt_id)
    WHERE current_delivery_attempt_id IS NOT NULL;
""")

migration_022.unlink()
Path("internal/database/queries/email_delivery_attempts.sql").unlink()

repository = Path("internal/delivery/email/outbound/repository.go")
source = repository.read_text().replace("email_delivery_attempts", "message_delivery_attempts")
old = """\t\tINSERT INTO message_delivery_attempts (
\t\t\tid, email_message_id, team_id, attempt_number, status, provider
\t\t)
\t\tVALUES ($1, $2, $3, $4, 'claimed', $5)
"""
new = """\t\tINSERT INTO message_delivery_attempts (
\t\t\tid, team_id, channel, email_message_id, attempt_number, status, provider,
\t\t\tsender_asset_id, sender_provider_binding_id
\t\t)
\t\tSELECT $1, $3, 'email', $2, $4, 'claimed', $5,
\t\t\tbinding.sender_asset_id, message.sender_domain_id
\t\tFROM email_messages AS message
\t\tLEFT JOIN sender_provider_bindings AS binding
\t\t  ON binding.id = message.sender_domain_id
\t\tWHERE message.id = $2
\t\t  AND message.team_id = $3
"""
if source.count(old) != 1:
    raise SystemExit("could not rewrite email attempt insertion")
source = source.replace(old, new)
source = source.replace(
    "\t\t  AND attempt.team_id = $2\n\t\t  AND attempt.status = 'claimed'",
    "\t\t  AND attempt.team_id = $2\n\t\t  AND attempt.channel = 'email'\n\t\t  AND attempt.status = 'claimed'",
    1,
)
source = source.replace("completed_at = now()", "request_completed_at = now()", 1)
old = "\t\tSET status = 'submitted', provider = $4, provider_message_id = $5,\n\t\t\trequest_completed_at = now(), error_code = NULL, error_message = NULL, updated_at = now()"
new = "\t\tSET status = 'submitted', provider = $4, provider_message_id = $5,\n\t\t\trequest_completed_at = now(), submitted_at = COALESCE(submitted_at, now()),\n\t\t\terror_code = NULL, error_message = NULL, updated_at = now()"
if source.count(old) != 1:
    raise SystemExit("could not identify submitted email attempt update")
source = source.replace(old, new)
source = source.replace(
    "\t\tWHERE id = $3 AND email_message_id = $1 AND team_id = $2\n\t\t  AND status = 'request_started'",
    "\t\tWHERE id = $3 AND email_message_id = $1 AND team_id = $2\n\t\t  AND channel = 'email'\n\t\t  AND status = 'request_started'",
    1,
)
source = source.replace(
    'return r.completeAttempt(ctx, messageID, teamID, attemptID, "failed", "failed", code, cause)',
    'return r.completeAttempt(ctx, messageID, teamID, attemptID, "permanent_failure", "failed", code, cause)',
    1,
)
old = "\t\tSET status = $4, error_code = $5, error_message = $6,\n\t\t\tcompleted_at = now(), updated_at = now()"
new = "\t\tSET status = $4, error_code = $5, error_message = $6,\n\t\t\trequest_completed_at = COALESCE(request_completed_at, now()),\n\t\t\tterminal_at = CASE\n\t\t\t\tWHEN $4 IN ('retryable_failure', 'permanent_failure')\n\t\t\t\tTHEN COALESCE(terminal_at, now())\n\t\t\t\tELSE terminal_at\n\t\t\tEND,\n\t\t\tupdated_at = now()"
if source.count(old) != 1:
    raise SystemExit("could not identify terminal email attempt update")
source = source.replace(old, new)
source = source.replace(
    "\t\tWHERE id = $3 AND email_message_id = $1 AND team_id = $2\n\t\t  AND status IN ('claimed', 'request_started')",
    "\t\tWHERE id = $3 AND email_message_id = $1 AND team_id = $2\n\t\t  AND channel = 'email'\n\t\t  AND status IN ('claimed', 'request_started')",
    1,
)
source = source.replace(
    "JOIN message_delivery_attempts AS attempt\n\t\t\t  ON attempt.id = message.current_delivery_attempt_id",
    "JOIN message_delivery_attempts AS attempt\n\t\t\t  ON attempt.id = message.current_delivery_attempt_id\n\t\t\t AND attempt.channel = 'email'",
)
source = source.replace(
    "\t\t\t\tcompleted_at = now(), updated_at = now()\n\t\t\tFROM stale\n\t\t\tWHERE attempt.id = stale.current_delivery_attempt_id",
    "\t\t\t\trequest_completed_at = COALESCE(request_completed_at, now()),\n\t\t\t\tupdated_at = now()\n\t\t\tFROM stale\n\t\t\tWHERE attempt.id = stale.current_delivery_attempt_id",
    1,
)
source = source.replace(
    "\t\t\t\tcompleted_at = now(), updated_at = now()\n\t\t\tFROM stale\n\t\t\tWHERE attempt.id = stale.current_delivery_attempt_id",
    "\t\t\t\trequest_completed_at = COALESCE(request_completed_at, now()),\n\t\t\t\tterminal_at = COALESCE(terminal_at, now()), updated_at = now()\n\t\t\tFROM stale\n\t\t\tWHERE attempt.id = stale.current_delivery_attempt_id",
    1,
)
repository.write_text(source)

recovery = Path("internal/delivery/email/outbound/recovery.go")
source = recovery.read_text().replace("email_delivery_attempts", "message_delivery_attempts")
source = source.replace(
    "\t\t  AND team_id = $3\n\t\tFOR UPDATE",
    "\t\t  AND team_id = $3\n\t\t  AND channel = 'email'\n\t\tFOR UPDATE",
    1,
)
source = source.replace(
    "\t\t\t\tcompleted_at = COALESCE(completed_at, now()),\n\t\t\t\tupdated_at = now()\n\t\t\tWHERE id = $1",
    "\t\t\t\trequest_completed_at = COALESCE(request_completed_at, now()),\n\t\t\t\tterminal_at = COALESCE(terminal_at, now()), updated_at = now()\n\t\t\tWHERE id = $1 AND channel = 'email'",
    1,
)
source = source.replace(
    "\t\t\t\tcompleted_at = COALESCE(completed_at, now()),\n\t\t\t\tupdated_at = now()\n\t\t\tWHERE id = $1",
    "\t\t\t\trequest_completed_at = COALESCE(request_completed_at, now()),\n\t\t\t\tupdated_at = now()\n\t\t\tWHERE id = $1 AND channel = 'email'",
    1,
)
recovery.write_text(source)

feedback = Path("internal/delivery/email/feedback/repository.go")
source = feedback.read_text().replace("email_delivery_attempts", "message_delivery_attempts")
source = source.replace(
    "\t\t\t\t  AND email_message_id = $2\n\t\t\t)",
    "\t\t\t\t  AND email_message_id = $2\n\t\t\t\t  AND channel = 'email'\n\t\t\t)",
    1,
)
source = source.replace(
    "\t\t\t\tcompleted_at = COALESCE(completed_at, now()),\n\t\t\t\tupdated_at = now()\n\t\t\tWHERE id = $1",
    "\t\t\t\trequest_completed_at = COALESCE(request_completed_at, now()),\n\t\t\t\tsubmitted_at = COALESCE(submitted_at, now()), updated_at = now()\n\t\t\tWHERE id = $1 AND channel = 'email'",
    1,
)
feedback.write_text(source)
