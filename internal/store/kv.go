package store

import (
	"context"
	"database/sql"
	"errors"
)

// KVSet upserts a key-value pair. Replaces the value if the key exists.
func (s *Store) KVSet(ctx context.Context, key, value string) error {
	if key == "" {
		return errors.New("key required")
	}
	return KVSetTx(ctx, s.db, key, value)
}

// KVGet returns the value for a key, or ErrKVNotFound if missing.
func (s *Store) KVGet(ctx context.Context, key string) (string, error) {
	var kv KV
	err := s.db.NewSelect().Model(&kv).Where("key = ?", key).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrKVNotFound
	}
	if err != nil {
		return "", err
	}
	return kv.Value, nil
}

// KVDelete removes a key. Returns ErrKVNotFound if it wasn't present.
func (s *Store) KVDelete(ctx context.Context, key string) error {
	res, err := s.db.NewDelete().Model((*KV)(nil)).Where("key = ?", key).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrKVNotFound
	}
	return nil
}

// KVList returns every entry, alphabetised by key. Also used for export.
func (s *Store) KVList(ctx context.Context) ([]KV, error) {
	var kvs []KV
	err := s.db.NewSelect().Model(&kvs).OrderExpr("key ASC").Scan(ctx)
	return kvs, err
}
