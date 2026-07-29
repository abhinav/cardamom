-- name: StoreGetLineageID :one
SELECT id
FROM store_lineage
WHERE singleton = 1;
