package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/rovak/beadsv2/internal/store"
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

		scan := bufio.NewScanner(rdr)
		// 16 MiB max line — enough for very large issue bodies.
		scan.Buffer(make([]byte, 1<<20), 16<<20)

		var issues, deps int
		for ln := 1; scan.Scan(); ln++ {
			line := scan.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var hdr exportLine
			if err := json.Unmarshal(line, &hdr); err != nil {
				if c.Lenient {
					continue
				}
				return fmt.Errorf("line %d: %w", ln, err)
			}
			switch hdr.Kind {
			case "issue":
				var rec issueExport
				if err := json.Unmarshal(hdr.Data, &rec); err != nil {
					if c.Lenient {
						continue
					}
					return fmt.Errorf("line %d (issue): %w", ln, err)
				}
				if err := s.UpsertIssue(r.ctx, rec.Issue); err != nil {
					return fmt.Errorf("line %d: upsert issue %s: %w", ln, rec.Issue.ID, err)
				}
				if err := s.AddLabels(r.ctx, rec.Issue.ID, rec.Labels); err != nil {
					return fmt.Errorf("line %d: labels for %s: %w", ln, rec.Issue.ID, err)
				}
				issues++
			case "dep":
				var rec depExport
				if err := json.Unmarshal(hdr.Data, &rec); err != nil {
					if c.Lenient {
						continue
					}
					return fmt.Errorf("line %d (dep): %w", ln, err)
				}
				if err := s.UpsertDep(r.ctx, rec.Child, rec.Parent); err != nil {
					return fmt.Errorf("line %d: upsert dep %s->%s: %w", ln, rec.Child, rec.Parent, err)
				}
				deps++
			default:
				if !c.Lenient {
					return fmt.Errorf("line %d: unknown kind %q", ln, hdr.Kind)
				}
			}
		}
		if err := scan.Err(); err != nil {
			return fmt.Errorf("read: %w", err)
		}
		r.notice("imported %d issues, %d deps\n", issues, deps)
		return nil
	})
}
