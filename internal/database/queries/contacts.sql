-- name: CreateContact :one
INSERT INTO contacts (
    team_id,
    email,
    first_name,
    last_name,
    unsubscribed
) VALUES (
    sqlc.arg(team_id),
    sqlc.arg(email),
    sqlc.narg(first_name),
    sqlc.narg(last_name),
    sqlc.arg(unsubscribed)
)
RETURNING *;

-- name: ListContacts :many
SELECT *
FROM contacts
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: GetContact :one
SELECT *
FROM contacts
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: GetContactByEmail :one
SELECT *
FROM contacts
WHERE team_id = sqlc.arg(team_id)
  AND lower(email) = lower(sqlc.arg(email));

-- name: UpdateContact :one
UPDATE contacts
SET email = sqlc.arg(email),
    first_name = sqlc.narg(first_name),
    last_name = sqlc.narg(last_name),
    unsubscribed = sqlc.arg(unsubscribed),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;

-- name: DeleteContact :one
DELETE FROM contacts
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;

-- name: ListContactPropertyValues :many
SELECT
    cpv.contact_id,
    cp.key,
    cp.value_type,
    cpv.string_value,
    cpv.number_value
FROM contact_property_values AS cpv
JOIN contact_properties AS cp
  ON cp.id = cpv.contact_property_id
 AND cp.team_id = cpv.team_id
WHERE cpv.team_id = sqlc.arg(team_id)
  AND cpv.contact_id = sqlc.arg(contact_id)
ORDER BY cp.key;

-- name: DeleteContactPropertyValues :exec
DELETE FROM contact_property_values
WHERE team_id = sqlc.arg(team_id)
  AND contact_id = sqlc.arg(contact_id);

-- name: UpsertContactStringPropertyValue :exec
INSERT INTO contact_property_values (
    team_id,
    contact_id,
    contact_property_id,
    value_type,
    string_value
)
SELECT
    sqlc.arg(team_id),
    sqlc.arg(contact_id),
    cp.id,
    cp.value_type,
    sqlc.arg(property_value)
FROM contact_properties AS cp
WHERE cp.team_id = sqlc.arg(team_id)
  AND cp.key = sqlc.arg(property_key)
  AND cp.value_type = 'string'
ON CONFLICT (contact_id, contact_property_id)
DO UPDATE SET
    value_type = EXCLUDED.value_type,
    string_value = EXCLUDED.string_value,
    number_value = NULL,
    updated_at = now();

-- name: UpsertContactNumberPropertyValue :exec
INSERT INTO contact_property_values (
    team_id,
    contact_id,
    contact_property_id,
    value_type,
    number_value
)
SELECT
    sqlc.arg(team_id),
    sqlc.arg(contact_id),
    cp.id,
    cp.value_type,
    sqlc.arg(property_value)
FROM contact_properties AS cp
WHERE cp.team_id = sqlc.arg(team_id)
  AND cp.key = sqlc.arg(property_key)
  AND cp.value_type = 'number'
ON CONFLICT (contact_id, contact_property_id)
DO UPDATE SET
    value_type = EXCLUDED.value_type,
    string_value = NULL,
    number_value = EXCLUDED.number_value,
    updated_at = now();

-- name: ListContactTopics :many
SELECT
    topic.id,
    topic.name,
    topic.description,
    COALESCE(subscription.subscription, topic.default_subscription) AS subscription
FROM topics AS topic
LEFT JOIN contact_topic_subscriptions AS subscription
  ON subscription.team_id = topic.team_id
 AND subscription.topic_id = topic.id
 AND subscription.contact_id = sqlc.arg(scope_contact_id)
WHERE topic.team_id = sqlc.arg(scope_team_id)
ORDER BY topic.created_at DESC, topic.id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListContactTopicsAfter :many
SELECT
    topic.id,
    topic.name,
    topic.description,
    COALESCE(subscription.subscription, topic.default_subscription) AS subscription
FROM topics AS topic
LEFT JOIN contact_topic_subscriptions AS subscription
  ON subscription.team_id = topic.team_id
 AND subscription.topic_id = topic.id
 AND subscription.contact_id = sqlc.arg(scope_contact_id)
WHERE topic.team_id = sqlc.arg(scope_team_id)
  AND (topic.created_at, topic.id) < (
      SELECT cursor_topic.created_at, cursor_topic.id
      FROM topics AS cursor_topic
      WHERE cursor_topic.id = sqlc.arg(cursor_id)
        AND cursor_topic.team_id = sqlc.arg(scope_team_id)
  )
ORDER BY topic.created_at DESC, topic.id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListContactTopicsBefore :many
SELECT
    topic.id,
    topic.name,
    topic.description,
    COALESCE(subscription.subscription, topic.default_subscription) AS subscription
FROM topics AS topic
LEFT JOIN contact_topic_subscriptions AS subscription
  ON subscription.team_id = topic.team_id
 AND subscription.topic_id = topic.id
 AND subscription.contact_id = sqlc.arg(scope_contact_id)
WHERE topic.team_id = sqlc.arg(scope_team_id)
  AND (topic.created_at, topic.id) > (
      SELECT cursor_topic.created_at, cursor_topic.id
      FROM topics AS cursor_topic
      WHERE cursor_topic.id = sqlc.arg(cursor_id)
        AND cursor_topic.team_id = sqlc.arg(scope_team_id)
  )
ORDER BY topic.created_at ASC, topic.id ASC
LIMIT sqlc.arg(page_limit);

-- name: ContactTopicCursorExists :one
SELECT EXISTS (
    SELECT 1
    FROM topics
    WHERE id = sqlc.arg(cursor_id)
      AND team_id = sqlc.arg(team_id)
);

-- name: UpsertContactTopicSubscription :one
INSERT INTO contact_topic_subscriptions (
    team_id,
    contact_id,
    topic_id,
    subscription
) VALUES (
    sqlc.arg(team_id),
    sqlc.arg(contact_id),
    sqlc.arg(topic_id),
    sqlc.arg(subscription)
)
ON CONFLICT (contact_id, topic_id)
DO UPDATE SET
    subscription = EXCLUDED.subscription,
    updated_at = now()
RETURNING topic_id;
