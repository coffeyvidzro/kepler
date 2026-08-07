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
 AND subscription.contact_id = sqlc.arg(contact_id)
WHERE topic.team_id = sqlc.arg(team_id)
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
 AND subscription.contact_id = sqlc.arg(contact_id)
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
 AND subscription.contact_id = sqlc.arg(contact_id)
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
