CREATE TABLE IF NOT EXISTS verification_tokens (
  identifier TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  PRIMARY KEY (identifier, token_hash)
);

CREATE INDEX IF NOT EXISTS verification_tokens_expires_at_idx
ON verification_tokens(expires_at);
