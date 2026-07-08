-- name: MailSelectPublicationSubscriptions :many
SELECT listener, pattern, created_at, expires_at
FROM subscriptions
WHERE expires_at > sqlc.arg(now)
ORDER BY listener, pattern;

-- name: MailInsertDelivery :exec
INSERT INTO mailbox (
    id,
    sender,
    recipient,
    source_topic,
    body,
    created_at,
    expires_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(sender),
    sqlc.arg(recipient),
    sqlc.arg(source_topic),
    sqlc.arg(body),
    sqlc.arg(created_at),
    sqlc.arg(expires_at)
);
