-- name: CreateBroadcastRecipient :one
INSERT INTO broadcast_recipients (
    team_id, broadcast_id, contact_id, email, normalized_email,
    first_name, last_name, contact_snapshot, status, exclusion_reason
) VALUES (
    sqlc.arg(team_id), sqlc.arg(broadcast_id), sqlc.narg(contact_id),
    sqlc.arg(email), sqlc.arg(normalized_email), sqlc.narg(first_name),
    sqlc.narg(last_name), sqlc.arg(contact_snapshot), sqlc.arg(status),
    sqlc.narg(exclusion_reason)
)
RETURNING *;

-- name: ListBroadcastRecipients :many
SELECT *
FROM broadcast_recipients
WHERE team_id = sqlc.arg(team_id)
  AND broadcast_id = sqlc.arg(broadcast_id)
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: SetBroadcastRecipientQueued :one
UPDATE broadcast_recipients
SET status = 'queued',
    email_message_id = sqlc.arg(email_message_id),
    queued_at = now(),
    next_attempt_at = NULL,
    last_error_code = NULL,
    last_error_message = NULL
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND broadcast_id = sqlc.arg(broadcast_id)
  AND status = 'pending'
RETURNING *;

-- name: ClaimNextBroadcastRecipientForFanout :one
SELECT
    recipient.*,
    broadcast.template_id,
    broadcast.template_version_id,
    broadcast.variable_bindings
FROM broadcast_recipients AS recipient
JOIN broadcasts AS broadcast
  ON broadcast.id = recipient.broadcast_id
 AND broadcast.team_id = recipient.team_id
WHERE recipient.status = 'pending'
  AND (recipient.next_attempt_at IS NULL OR recipient.next_attempt_at <= now())
  AND broadcast.status = 'queued'
  AND broadcast.recipients_materialized_at IS NOT NULL
  AND broadcast.template_version_id IS NOT NULL
  AND broadcast.deleted_at IS NULL
ORDER BY recipient.next_attempt_at NULLS FIRST, recipient.broadcast_id, recipient.id
FOR UPDATE OF recipient SKIP LOCKED
LIMIT 1;

-- name: RetryBroadcastRecipientFanout :one
UPDATE broadcast_recipients
SET attempt_count = attempt_count + 1,
    next_attempt_at = sqlc.arg(next_attempt_at),
    last_error_code = sqlc.arg(error_code),
    last_error_message = sqlc.arg(error_message)
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND broadcast_id = sqlc.arg(broadcast_id)
  AND status = 'pending'
RETURNING *;

-- name: FailBroadcastRecipientFanout :one
UPDATE broadcast_recipients
SET status = 'failed',
    attempt_count = attempt_count + 1,
    next_attempt_at = NULL,
    last_error_code = sqlc.arg(error_code),
    last_error_message = sqlc.arg(error_message),
    failed_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND broadcast_id = sqlc.arg(broadcast_id)
  AND status = 'pending'
RETURNING *;

-- name: CountBroadcastRecipientFanoutState :one
SELECT
    count(*) FILTER (WHERE status = 'pending') AS pending_count,
    count(*) FILTER (WHERE status = 'queued') AS queued_count,
    count(*) FILTER (WHERE status = 'failed') AS failed_count
FROM broadcast_recipients
WHERE team_id = sqlc.arg(team_id)
  AND broadcast_id = sqlc.arg(broadcast_id);

-- name: MaterializeBroadcastRecipients :exec
WITH candidates AS (
    SELECT
        contact.id AS contact_id,
        contact.email,
        lower(btrim(contact.email)) AS normalized_email,
        contact.first_name,
        contact.last_name,
        jsonb_build_object(
            'id', contact.id,
            'email', contact.email,
            'first_name', contact.first_name,
            'last_name', contact.last_name,
            'properties', COALESCE(properties.values, '{}'::jsonb)
        ) AS contact_snapshot,
        CASE
            WHEN contact.email !~* '^[A-Z0-9.!#$%&''*+/=?^_{|}~-]+@[A-Z0-9]([A-Z0-9-]{0,61}[A-Z0-9])?(\.[A-Z0-9]([A-Z0-9-]{0,61}[A-Z0-9])?)+$'
                THEN 'invalid_email'
            WHEN contact.unsubscribed THEN 'global_unsubscribe'
            WHEN EXISTS (
                SELECT 1
                FROM suppressions AS suppression
                WHERE suppression.team_id = contact.team_id
                  AND lower(suppression.email) = lower(contact.email)
            ) THEN 'suppressed'
            WHEN sqlc.narg(topic_id)::uuid IS NOT NULL
             AND COALESCE(subscription.subscription, topic.default_subscription) = 'opt_out'
                THEN 'topic_unsubscribed'
            ELSE NULL
        END AS exclusion_reason
    FROM contact_segments AS membership
    JOIN contacts AS contact
      ON contact.id = membership.contact_id
     AND contact.team_id = membership.team_id
    LEFT JOIN topics AS topic
      ON topic.id = sqlc.narg(topic_id)
     AND topic.team_id = contact.team_id
    LEFT JOIN contact_topic_subscriptions AS subscription
      ON subscription.contact_id = contact.id
     AND subscription.topic_id = sqlc.narg(topic_id)
     AND subscription.team_id = contact.team_id
    LEFT JOIN LATERAL (
        SELECT jsonb_object_agg(
            property.key,
            CASE property_value.value_type
                WHEN 'string' THEN to_jsonb(property_value.string_value)
                WHEN 'number' THEN to_jsonb(property_value.number_value)
            END
        ) AS values
        FROM contact_property_values AS property_value
        JOIN contact_properties AS property
          ON property.id = property_value.contact_property_id
         AND property.team_id = property_value.team_id
        WHERE property_value.contact_id = contact.id
          AND property_value.team_id = contact.team_id
    ) AS properties ON true
    WHERE membership.team_id = sqlc.arg(team_id)
      AND membership.segment_id = sqlc.arg(segment_id)
)
INSERT INTO broadcast_recipients (
    team_id, broadcast_id, contact_id, email, normalized_email,
    first_name, last_name, contact_snapshot, status, exclusion_reason
)
SELECT
    sqlc.arg(team_id),
    sqlc.arg(broadcast_id),
    contact_id,
    email,
    normalized_email,
    first_name,
    last_name,
    contact_snapshot,
    CASE WHEN exclusion_reason IS NULL THEN 'pending' ELSE 'excluded' END,
    exclusion_reason
FROM candidates
ON CONFLICT DO NOTHING;
