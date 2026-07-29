-- name: MailAcknowledgeDelivery :execresult
UPDATE mailbox
SET read_at = sqlc.arg(read_at)
WHERE local_sequence = sqlc.arg(local_sequence)
    AND read_at IS NULL;
