CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    user_agent TEXT,
    ip_address TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    credential_version BIGINT NOT NULL DEFAULT 1,
    authentication_method TEXT NOT NULL DEFAULT 'password',
    assurance_level TEXT NOT NULL DEFAULT 'aal1',
    authenticated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    mfa_completed_at TIMESTAMPTZ,

    CONSTRAINT chk_sessions_credential_version CHECK (credential_version > 0),
    CONSTRAINT chk_sessions_authentication_method CHECK (
        authentication_method IN ('password', 'totp', 'recovery_code')
    ),
    CONSTRAINT chk_sessions_assurance_level CHECK (
        assurance_level IN ('aal1', 'aal2', 'aal3')
    ),
    CONSTRAINT chk_sessions_mfa_assurance CHECK (
        mfa_completed_at IS NULL OR assurance_level IN ('aal2', 'aal3')
    )
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id
    ON sessions (user_id);

CREATE INDEX IF NOT EXISTS idx_sessions_token_hash
    ON sessions (token_hash);

CREATE INDEX IF NOT EXISTS idx_sessions_expires_at
    ON sessions (expires_at);

CREATE INDEX IF NOT EXISTS idx_sessions_user_credential_version
    ON sessions (user_id, credential_version)
    WHERE revoked_at IS NULL;
