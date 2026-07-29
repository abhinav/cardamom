-- name: MailListActiveSubscriptions :many
SELECT listener, pattern, created_at, expires_at
FROM subscriptions
WHERE expires_at > sqlc.arg(now)
ORDER BY listener, pattern;

-- name: MailListListenerSubscriptions :many
SELECT listener, pattern, created_at, expires_at
FROM subscriptions
WHERE listener = sqlc.arg(listener)
    AND expires_at > sqlc.arg(now)
ORDER BY pattern;

-- name: MailRemoveSubscription :execresult
DELETE FROM subscriptions
WHERE listener = sqlc.arg(listener)
    AND pattern = sqlc.arg(pattern);

-- name: MailRefreshSubscription :one
INSERT INTO subscriptions (
    listener,
    pattern,
    created_at,
    expires_at
) VALUES (
    sqlc.arg(listener),
    sqlc.arg(pattern),
    sqlc.arg(created_at),
    sqlc.arg(expires_at)
)
ON CONFLICT(listener, pattern) DO UPDATE SET
    expires_at = excluded.expires_at
RETURNING created_at;
