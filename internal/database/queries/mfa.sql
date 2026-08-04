-- name: PutUnverifiedTOTPCredential :execrows
INSERT INTO totp_credentials (user_id, secret_ciphertext)
VALUES (sqlc.arg(user_id), sqlc.arg(secret_ciphertext))
ON CONFLICT (user_id) DO UPDATE SET
    secret_ciphertext = EXCLUDED.secret_ciphertext,
    verified_at = NULL,
    last_used_step = NULL,
    updated_at = now()
WHERE totp_credentials.verified_at IS NULL;

-- name: GetTOTPCredential :one
SELECT secret_ciphertext, verified_at, last_used_step
FROM totp_credentials
WHERE user_id = sqlc.arg(user_id);

-- name: ConfirmTOTPCredential :execrows
UPDATE totp_credentials
SET verified_at = now(),
    last_used_step = sqlc.arg(last_used_step),
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND verified_at IS NULL;

-- name: DeleteRecoveryCodes :exec
DELETE FROM recovery_codes
WHERE user_id = sqlc.arg(user_id);

-- name: CreateRecoveryCode :exec
INSERT INTO recovery_codes (user_id, code_hash)
VALUES (sqlc.arg(user_id), sqlc.arg(code_hash));

-- name: ElevateSessionAfterMFAEnrollment :execrows
UPDATE sessions
SET assurance_level = 'aal2',
    mfa_completed_at = now()
WHERE id = sqlc.arg(session_id)
  AND user_id = sqlc.arg(user_id)
  AND revoked_at IS NULL;

-- name: VerifyTOTPAndElevateSession :execrows
WITH accepted AS (
    UPDATE totp_credentials
    SET last_used_step = sqlc.arg(last_used_step),
        updated_at = now()
    WHERE user_id = sqlc.arg(user_id)
      AND verified_at IS NOT NULL
      AND (last_used_step IS NULL OR last_used_step < sqlc.arg(last_used_step))
    RETURNING user_id
)
UPDATE sessions
SET assurance_level = 'aal2',
    mfa_completed_at = now()
WHERE sessions.id = sqlc.arg(session_id)
  AND sessions.user_id = sqlc.arg(user_id)
  AND sessions.revoked_at IS NULL
  AND EXISTS (SELECT 1 FROM accepted);

-- name: UseRecoveryCodeAndElevateSession :execrows
WITH accepted AS (
    UPDATE recovery_codes
    SET used_at = now()
    WHERE user_id = sqlc.arg(user_id)
      AND code_hash = sqlc.arg(code_hash)
      AND used_at IS NULL
    RETURNING user_id
)
UPDATE sessions
SET authentication_method = 'recovery_code',
    assurance_level = 'aal2',
    mfa_completed_at = now()
WHERE sessions.id = sqlc.arg(session_id)
  AND sessions.user_id = sqlc.arg(user_id)
  AND sessions.revoked_at IS NULL
  AND EXISTS (SELECT 1 FROM accepted);

-- name: DeleteTOTPCredential :exec
DELETE FROM totp_credentials
WHERE user_id = sqlc.arg(user_id);

-- name: RevokeOtherSessionsAfterMFADisable :exec
UPDATE sessions
SET revoked_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND id <> sqlc.arg(current_session_id)
  AND revoked_at IS NULL;

-- name: DowngradeSessionAfterMFADisable :exec
UPDATE sessions
SET authentication_method = 'password',
    assurance_level = 'aal1',
    mfa_completed_at = NULL
WHERE user_id = sqlc.arg(user_id)
  AND id = sqlc.arg(current_session_id);

-- name: IsTOTPEnabled :one
SELECT EXISTS (
    SELECT 1
    FROM totp_credentials
    WHERE user_id = sqlc.arg(user_id)
      AND verified_at IS NOT NULL
);

-- name: CreateMFALoginChallenge :exec
INSERT INTO authentication_challenges (token_hash, user_id, purpose, state, expires_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(user_id), 'mfa_login', jsonb_build_object('credential_version', sqlc.arg(credential_version)::bigint), sqlc.arg(expires_at));

-- name: GetActiveMFALoginChallenge :one
SELECT ac.user_id, tc.secret_ciphertext, tc.last_used_step
FROM authentication_challenges ac
JOIN totp_credentials tc ON tc.user_id = ac.user_id AND tc.verified_at IS NOT NULL
JOIN users u ON u.id = ac.user_id
WHERE ac.token_hash = sqlc.arg(token_hash)
  AND ac.purpose = 'mfa_login'
  AND ac.consumed_at IS NULL
  AND ac.expires_at > now()
  AND (ac.state->>'credential_version')::bigint = u.credential_version;

-- name: ConsumeMFALoginChallengeWithTOTP :execrows
WITH accepted_totp AS (
    UPDATE totp_credentials
    SET last_used_step = sqlc.arg(last_used_step), updated_at = now()
    WHERE user_id = sqlc.arg(user_id)
      AND verified_at IS NOT NULL
      AND (last_used_step IS NULL OR last_used_step < sqlc.arg(last_used_step))
      AND EXISTS (
          SELECT 1 FROM authentication_challenges ac
          JOIN users u ON u.id = ac.user_id
          WHERE ac.token_hash = sqlc.arg(token_hash)
            AND ac.user_id = sqlc.arg(user_id)
            AND ac.purpose = 'mfa_login'
            AND ac.consumed_at IS NULL
            AND ac.expires_at > now()
            AND u.credential_version = (ac.state->>'credential_version')::bigint
      )
    RETURNING user_id
)
UPDATE authentication_challenges
SET consumed_at = now()
WHERE authentication_challenges.token_hash = sqlc.arg(token_hash)
  AND authentication_challenges.user_id = sqlc.arg(user_id)
  AND authentication_challenges.purpose = 'mfa_login'
  AND authentication_challenges.consumed_at IS NULL
  AND authentication_challenges.expires_at > now()
  AND EXISTS (
      SELECT 1 FROM users
      WHERE users.id = authentication_challenges.user_id
        AND users.credential_version = (authentication_challenges.state->>'credential_version')::bigint
  )
  AND EXISTS (SELECT 1 FROM accepted_totp);

-- name: ConsumeMFALoginChallengeWithRecoveryCode :execrows
WITH accepted_code AS (
    UPDATE recovery_codes
    SET used_at = now()
    WHERE user_id = sqlc.arg(user_id)
      AND code_hash = sqlc.arg(code_hash)
      AND used_at IS NULL
      AND EXISTS (
          SELECT 1 FROM authentication_challenges ac
          JOIN users u ON u.id = ac.user_id
          WHERE ac.token_hash = sqlc.arg(token_hash)
            AND ac.user_id = sqlc.arg(user_id)
            AND ac.purpose = 'mfa_login'
            AND ac.consumed_at IS NULL
            AND ac.expires_at > now()
            AND u.credential_version = (ac.state->>'credential_version')::bigint
      )
    RETURNING user_id
)
UPDATE authentication_challenges
SET consumed_at = now()
WHERE authentication_challenges.token_hash = sqlc.arg(token_hash)
  AND authentication_challenges.user_id = sqlc.arg(user_id)
  AND authentication_challenges.purpose = 'mfa_login'
  AND authentication_challenges.consumed_at IS NULL
  AND authentication_challenges.expires_at > now()
  AND EXISTS (
      SELECT 1 FROM users
      WHERE users.id = authentication_challenges.user_id
        AND users.credential_version = (authentication_challenges.state->>'credential_version')::bigint
  )
  AND EXISTS (SELECT 1 FROM accepted_code);

-- name: RotateTOTPSecretCiphertext :exec
UPDATE totp_credentials SET secret_ciphertext=sqlc.arg(new_ciphertext),updated_at=now() WHERE user_id=sqlc.arg(user_id) AND secret_ciphertext=sqlc.arg(old_ciphertext);

-- name: ListTOTPSecretsForRotation :many
SELECT user_id,secret_ciphertext FROM totp_credentials ORDER BY user_id;
