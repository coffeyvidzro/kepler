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
SET status = 'queued', email_message_id = sqlc.arg(email_message_id), queued_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND broadcast_id = sqlc.arg(broadcast_id)
  AND status = 'pending'
RETURNING *;

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
