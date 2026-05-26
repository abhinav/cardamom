package store

import (
	"context"

	"github.com/uptrace/bun"
)

// queryRunner is the subset of bun's query-builder API that *bun.DB
// and bun.Tx both satisfy. Used so the upsert helpers can run either
// against the pool or inside an open transaction — see Import below.
type queryRunner interface {
	NewInsert() *bun.InsertQuery
	NewSelect() *bun.SelectQuery
	NewUpdate() *bun.UpdateQuery
	NewDelete() *bun.DeleteQuery
}

// RunInTx is the store's escape hatch for callers that need to wrap
// multiple writes in a single transaction (e.g. `clu import`, where
// a FK violation on a dep line should roll back any issue rows
// inserted earlier in the same file). The closure receives a Tx that
// implements queryRunner — pass it to the upsert*Tx helpers below.
//
// Errors from fn cause the tx to roll back. SQLite serialises writes,
// so do not hold the closure open while waiting on I/O.
func (s *Store) RunInTx(ctx context.Context, fn func(ctx context.Context, tx bun.Tx) error) error {
	return s.db.RunInTx(ctx, nil, fn)
}

// UpsertIssueTx, UpsertDepTx, UpsertCommentTx, KVSetTx, and
// CronJobUpsertTx implement the same write that their public-method
// counterparts do, but parameterised on a queryRunner so they can
// participate in an outer transaction.
//
// The public UpsertIssue / UpsertDep / UpsertComment / KVSet /
// CronJobUpsert just delegate here with s.db.

func UpsertIssueTx(ctx context.Context, q queryRunner, i Issue) error {
	_, err := q.NewInsert().Model(&i).
		On("CONFLICT (id) DO UPDATE").
		Set("title = EXCLUDED.title").
		Set("type = EXCLUDED.type").
		Set("status = EXCLUDED.status").
		Set("priority = EXCLUDED.priority").
		Set("agent = EXCLUDED.agent").
		Set("assignee = EXCLUDED.assignee").
		Set("created = EXCLUDED.created").
		Set("updated = EXCLUDED.updated").
		Set("closed = EXCLUDED.closed").
		Set("defer_until = EXCLUDED.defer_until").
		Set("description = EXCLUDED.description").
		Set("notes = EXCLUDED.notes").
		Exec(ctx)
	return err
}

func UpsertDepTx(ctx context.Context, q queryRunner, child, parent string) error {
	_, err := q.NewInsert().
		Model(&Dep{ChildID: child, ParentID: parent}).
		On("CONFLICT DO NOTHING").
		Exec(ctx)
	return err
}

func UpsertCommentTx(ctx context.Context, q queryRunner, c Comment) error {
	_, err := q.NewInsert().Model(&c).
		On("CONFLICT (id) DO UPDATE").
		Set("issue_id = EXCLUDED.issue_id").
		Set("author = EXCLUDED.author").
		Set("body = EXCLUDED.body").
		Set("created = EXCLUDED.created").
		Exec(ctx)
	return err
}

func KVSetTx(ctx context.Context, q queryRunner, key, value string) error {
	kv := KV{Key: key, Value: value}
	_, err := q.NewInsert().Model(&kv).
		On("CONFLICT (key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return err
}

func CronJobUpsertTx(ctx context.Context, q queryRunner, j CronJob) error {
	_, err := q.NewInsert().Model(&j).
		On("CONFLICT (name) DO UPDATE").
		Set("schedule = EXCLUDED.schedule").
		Set("job = EXCLUDED.job").
		Set("enabled = EXCLUDED.enabled").
		Set("next_run = EXCLUDED.next_run").
		Set("last_run = EXCLUDED.last_run").
		Set("last_status = EXCLUDED.last_status").
		Set("last_output = EXCLUDED.last_output").
		Exec(ctx)
	return err
}

// AddLabelsTx is the tx-bound variant of AddLabels. Used by Import
// so the labels for an issue go in the same transaction as the issue.
// Returns count of actual inserts (excludes duplicates).
func AddLabelsTx(ctx context.Context, q queryRunner, issueID string, labels []string) (int, error) {
	if len(labels) == 0 {
		return 0, nil
	}
	rows := make([]IssueLabel, len(labels))
	for i, l := range labels {
		if l == "" {
			return 0, ErrInvalid
		}
		rows[i] = IssueLabel{IssueID: issueID, Label: l}
	}
	res, err := q.NewInsert().Model(&rows).On("CONFLICT DO NOTHING").Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
