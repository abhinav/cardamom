package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/rovak/clu/internal/store"
	"github.com/uptrace/bun"
)

type ImportCmd struct {
	File    string `arg:"" optional:"" help:"Path to JSONL file (default: stdin)."`
	Lenient bool   `name:"lenient" help:"Skip lines that fail to parse instead of erroring."`
}

func (c *ImportCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		var rdr io.Reader = os.Stdin
		if c.File != "" {
			f, err := os.Open(c.File)
			if err != nil {
				return err
			}
			defer f.Close()
			rdr = f
		}

		// Buffer the whole file so we can run the SQL writes inside a
		// single transaction without holding file I/O concurrent with
		// the DB write. Scanner buffer = 16 MiB per line.
		scan := bufio.NewScanner(rdr)
		scan.Buffer(make([]byte, 1<<20), 16<<20)
		type record struct {
			ln   int
			line []byte
		}
		var records []record
		for ln := 1; scan.Scan(); ln++ {
			line := scan.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			// Copy out of the scanner's reused buffer.
			cp := make([]byte, len(line))
			copy(cp, line)
			records = append(records, record{ln, cp})
		}
		if err := scan.Err(); err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var issues, deps, comments, kvs, crons int
		// One transaction for the whole file. A FK violation on a dep
		// line rolls back any issue rows already inserted in the same
		// file — no half-imported state survives a failed run.
		err := s.RunInTx(r.ctx, func(ctx context.Context, tx bun.Tx) error {
			for _, rec := range records {
				ln := rec.ln
				var hdr exportLine
				if err := json.Unmarshal(rec.line, &hdr); err != nil {
					if c.Lenient {
						continue
					}
					return fmt.Errorf("line %d: %w", ln, err)
				}
				if err := applyImportLine(ctx, tx, &hdr, ln, c.Lenient, &issues, &deps, &comments, &kvs, &crons); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		r.notice("imported %d issues, %d deps, %d comments, %d kv, %d cron\n", issues, deps, comments, kvs, crons)
		return nil
	})
}

// applyImportLine handles one JSONL line inside the import transaction.
// All writes go through the tx-bound store helpers so a failure rolls
// back everything earlier in the same import.
func applyImportLine(ctx context.Context, tx bun.Tx, hdr *exportLine, ln int, lenient bool, issues, deps, comments, kvs, crons *int) error {
	switch hdr.Kind {
	case "issue":
		var rec issueExport
		if err := json.Unmarshal(hdr.Data, &rec); err != nil {
			if lenient {
				return nil
			}
			return fmt.Errorf("line %d (issue): %w", ln, err)
		}
		if err := store.UpsertIssueTx(ctx, tx, rec.Issue); err != nil {
			return fmt.Errorf("line %d: upsert issue %s: %w", ln, rec.ID, err)
		}
		if _, err := store.AddLabelsTx(ctx, tx, rec.ID, rec.Labels); err != nil {
			return fmt.Errorf("line %d: labels for %s: %w", ln, rec.ID, err)
		}
		*issues++
	case "dep":
		var rec depExport
		if err := json.Unmarshal(hdr.Data, &rec); err != nil {
			if lenient {
				return nil
			}
			return fmt.Errorf("line %d (dep): %w", ln, err)
		}
		if err := store.UpsertDepTx(ctx, tx, rec.Child, rec.Parent); err != nil {
			return fmt.Errorf("line %d: upsert dep %s->%s: %w", ln, rec.Child, rec.Parent, err)
		}
		*deps++
	case "comment":
		var rec store.Comment
		if err := json.Unmarshal(hdr.Data, &rec); err != nil {
			if lenient {
				return nil
			}
			return fmt.Errorf("line %d (comment): %w", ln, err)
		}
		if err := store.UpsertCommentTx(ctx, tx, rec); err != nil {
			return fmt.Errorf("line %d: upsert comment #%d: %w", ln, rec.ID, err)
		}
		*comments++
	case "kv":
		var rec store.KV
		if err := json.Unmarshal(hdr.Data, &rec); err != nil {
			if lenient {
				return nil
			}
			return fmt.Errorf("line %d (kv): %w", ln, err)
		}
		if err := store.KVSetTx(ctx, tx, rec.Key, rec.Value); err != nil {
			return fmt.Errorf("line %d: upsert kv %s: %w", ln, rec.Key, err)
		}
		*kvs++
	case "cron":
		var rec store.CronJob
		if err := json.Unmarshal(hdr.Data, &rec); err != nil {
			if lenient {
				return nil
			}
			return fmt.Errorf("line %d (cron): %w", ln, err)
		}
		if err := store.CronJobUpsertTx(ctx, tx, rec); err != nil {
			return fmt.Errorf("line %d: upsert cron %s: %w", ln, rec.Name, err)
		}
		*crons++
	default:
		if !lenient {
			return fmt.Errorf("line %d: unknown kind %q", ln, hdr.Kind)
		}
	}
	return nil
}
