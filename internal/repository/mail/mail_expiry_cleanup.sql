-- name: MailDeleteExpiredMailbox :exec
DELETE FROM mailbox
WHERE expires_at <= sqlc.arg(now);

-- name: MailDeleteExpiredSubscriptions :exec
DELETE FROM subscriptions
WHERE expires_at <= sqlc.arg(now);
