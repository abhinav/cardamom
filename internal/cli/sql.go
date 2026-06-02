package cli

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/Rovak/agents-clu/internal/store"
)

// SqlCmd runs an ad-hoc SQL query against the project's data.sqlite.
//
// Read-only by default — first keyword must be SELECT, WITH, EXPLAIN,
// or PRAGMA. Pass --write to allow arbitrary statements. There's no
// stricter sandbox; schema is internal and queries can break across
// migration versions.
type SqlCmd struct {
	Query string `arg:"" optional:"" help:"SQL query. Use '-' to read from stdin, or pass --file."`
	Write bool   `name:"write" help:"Allow non-SELECT statements (UPDATE/DELETE/INSERT/DDL). Default refuses anything other than read queries."`
	File  string `name:"file" short:"f" help:"Read the query from this file instead of the positional arg."`
	CSV   bool   `name:"csv" help:"Emit CSV (with header) instead of a pretty table. Mutually exclusive with --json."`
}

func (c *SqlCmd) Run(r *runCtx) error {
	if c.CSV && r.json {
		return errors.New("--csv and --json are mutually exclusive")
	}
	q, err := c.readQuery(r)
	if err != nil {
		return err
	}
	q = strings.TrimSpace(q)
	q = strings.TrimRight(q, ";")
	q = strings.TrimSpace(q)
	if q == "" {
		return errors.New("empty query")
	}
	if !c.Write {
		if err := checkReadOnly(q); err != nil {
			return err
		}
	}

	return withStore(r, func(s *store.Store) error {
		if c.Write && !isReadKeyword(leadingKeyword(q)) {
			n, err := s.RawExec(r.ctx, q)
			if err != nil {
				return err
			}
			if r.json {
				return r.emitJSON(map[string]any{"rows_affected": n})
			}
			r.notice("%d row(s) affected\n", n)
			return nil
		}
		cols, rows, err := s.RawQuery(r.ctx, q)
		if err != nil {
			return err
		}
		switch {
		case r.json:
			return emitSQLJSON(r, cols, rows)
		case c.CSV:
			return emitSQLCSV(r.stdout, cols, rows)
		default:
			return emitSQLTable(r.stdout, cols, rows)
		}
	})
}

// readQuery pulls the SQL text from --file, the positional arg, or
// stdin (when the arg is "-"). Exactly one source must be set.
func (c *SqlCmd) readQuery(r *runCtx) (string, error) {
	if c.File != "" && c.Query != "" {
		return "", errors.New("pass the query as an argument OR --file, not both")
	}
	if c.File != "" {
		b, err := os.ReadFile(c.File)
		if err != nil {
			return "", fmt.Errorf("read --file: %w", err)
		}
		return string(b), nil
	}
	if c.Query == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(b), nil
	}
	if c.Query == "" {
		return "", errors.New("no query — pass it as an argument, with --file <path>, or as '-' to read stdin")
	}
	return c.Query, nil
}

// leadingKeyword strips line / block comments and returns the first
// SQL keyword in upper case. Empty if the query is comment-only.
func leadingKeyword(q string) string {
	i := 0
	for i < len(q) {
		// Skip whitespace.
		for i < len(q) && unicode.IsSpace(rune(q[i])) {
			i++
		}
		if i >= len(q) {
			return ""
		}
		// Line comment.
		if i+1 < len(q) && q[i] == '-' && q[i+1] == '-' {
			for i < len(q) && q[i] != '\n' {
				i++
			}
			continue
		}
		// Block comment.
		if i+1 < len(q) && q[i] == '/' && q[i+1] == '*' {
			end := strings.Index(q[i+2:], "*/")
			if end == -1 {
				return ""
			}
			i += 2 + end + 2
			continue
		}
		break
	}
	start := i
	for i < len(q) && (unicode.IsLetter(rune(q[i])) || rune(q[i]) == '_') {
		i++
	}
	return strings.ToUpper(q[start:i])
}

var readOnlyLeading = map[string]bool{
	"SELECT":  true,
	"WITH":    true, // CTE-led SELECT. WITH ... UPDATE is technically allowed in SQLite — opt-in to --write for that.
	"EXPLAIN": true,
	"PRAGMA":  true,
}

func isReadKeyword(kw string) bool { return readOnlyLeading[kw] }

func checkReadOnly(q string) error {
	kw := leadingKeyword(q)
	if !readOnlyLeading[kw] {
		got := kw
		if got == "" {
			got = "<empty>"
		}
		return fmt.Errorf("read-only mode rejects %s; pass --write to allow writes", got)
	}
	return nil
}

// renderSQLValue turns one scanned column value into a display string.
// Caller picks the NULL sentinel (the pretty renderer wants "NULL", CSV
// wants ""). Boolean values are rare in SQLite (stored as 0/1) but the
// fallback handles whatever the driver hands us.
func renderSQLValue(v any, nullAs string) string {
	if v == nil {
		return nullAs
	}
	switch t := v.(type) {
	case []byte:
		return string(t)
	case string:
		return t
	case int64:
		return fmt.Sprintf("%d", t)
	case float64:
		return fmt.Sprintf("%g", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", t)
	}
}

func emitSQLTable(w io.Writer, cols []string, rows [][]any) error {
	if len(cols) == 0 {
		fmt.Fprintln(w, "(no columns)")
		return nil
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	rendered := make([][]string, len(rows))
	for i, row := range rows {
		rendered[i] = make([]string, len(cols))
		for j, v := range row {
			s := renderSQLValue(v, "NULL")
			rendered[i][j] = s
			if len(s) > widths[j] {
				widths[j] = len(s)
			}
		}
	}
	// Header.
	for i, c := range cols {
		if i > 0 {
			fmt.Fprint(w, "  ")
		}
		fmt.Fprintf(w, "%-*s", widths[i], c)
	}
	fmt.Fprintln(w)
	// Separator.
	for i, wd := range widths {
		if i > 0 {
			fmt.Fprint(w, "  ")
		}
		fmt.Fprint(w, strings.Repeat("-", wd))
	}
	fmt.Fprintln(w)
	// Rows.
	for _, row := range rendered {
		for i, s := range row {
			if i > 0 {
				fmt.Fprint(w, "  ")
			}
			fmt.Fprintf(w, "%-*s", widths[i], s)
		}
		fmt.Fprintln(w)
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "(0 rows)")
	}
	return nil
}

func emitSQLCSV(w io.Writer, cols []string, rows [][]any) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}
	for _, row := range rows {
		out := make([]string, len(row))
		for i, v := range row {
			out[i] = renderSQLValue(v, "")
		}
		if err := cw.Write(out); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// emitSQLJSON produces one JSON array of {col: value} objects on
// stdout. NULL renders as null; bytes/strings become JSON strings;
// numbers stay numeric. Matches the rest of the CLI's one-JSON-value
// rule.
func emitSQLJSON(r *runCtx, cols []string, rows [][]any) error {
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		obj := make(map[string]any, len(cols))
		for j, v := range row {
			if b, ok := v.([]byte); ok {
				obj[cols[j]] = string(b)
			} else {
				obj[cols[j]] = v
			}
		}
		out[i] = obj
	}
	return r.emitJSON(out)
}
