CREATE TABLE IF NOT EXISTS email_tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE RESTRICT,
    provider TEXT NOT NULL DEFAULT 'aws_ses',
    region TEXT NOT NULL,
    external_name TEXT NOT NULL,
    external_id TEXT,
    tenant_arn TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    suppression_scope TEXT NOT NULL DEFAULT 'tenant',
    reputation_policy TEXT NOT NULL DEFAULT 'standard',
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_email_tenants_team_provider_region
        UNIQUE (team_id, provider, region),

    CONSTRAINT uq_email_tenants_provider_region_name
        UNIQUE (provider, region, external_name),

    CONSTRAINT chk_email_tenants_provider
        CHECK (length(trim(provider)) > 0 AND provider = lower(trim(provider))),

    CONSTRAINT chk_email_tenants_region
        CHECK (length(trim(region)) > 0 AND region = lower(trim(region))),

    CONSTRAINT chk_email_tenants_external_name
        CHECK (
            external_name = lower(trim(external_name))
            AND length(external_name) BETWEEN 1 AND 64
            AND external_name ~ '^[a-z0-9_-]+$'
        ),

    CONSTRAINT chk_email_tenants_external_id
        CHECK (external_id IS NULL OR length(trim(external_id)) > 0),

    CONSTRAINT chk_email_tenants_tenant_arn
        CHECK (tenant_arn IS NULL OR length(trim(tenant_arn)) > 0),

    CONSTRAINT chk_email_tenants_status
        CHECK (status IN (
            'pending',
            'provisioning',
            'active',
            'paused',
            'deleting',
            'failed'
        )),

    CONSTRAINT chk_email_tenants_suppression_scope
        CHECK (suppression_scope IN ('account', 'tenant')),

    CONSTRAINT chk_email_tenants_reputation_policy
        CHECK (reputation_policy IN ('none', 'standard', 'strict')),

    CONSTRAINT chk_email_tenants_failure_reason
        CHECK (failure_reason IS NULL OR length(trim(failure_reason)) > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_email_tenants_provider_region_external_id
    ON email_tenants (provider, region, external_id)
    WHERE external_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_email_tenants_team_id
    ON email_tenants (team_id);

CREATE INDEX IF NOT EXISTS idx_email_tenants_lifecycle
    ON email_tenants (provider, region, status, updated_at);
