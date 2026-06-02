package cli

import (
	"fmt"
	"strings"

	"github.com/Rovak/agents-clu/internal/store"
)

// contextEntry is one upstream task in a --context bundle: the dependency
// issue plus its comments. Embeds store.Issue so its fields promote to the
// top level in JSON (description, notes, status, …).
type contextEntry struct {
	store.Issue
	Comments []store.Comment `json:"comments"`
}

// loadContext returns the ordered ancestor bundle for an issue — every task
// it transitively depends on, most-upstream first — each with its
// description, notes, and comments. depth>0 caps how far up to walk.
func loadContext(r *runCtx, s *store.Store, id string, depth int) ([]contextEntry, error) {
	ids, err := s.AncestorContext(r.ctx, id, depth)
	if err != nil {
		return nil, err
	}
	out := make([]contextEntry, 0, len(ids))
	for _, aid := range ids {
		iss, err := s.Get(r.ctx, aid)
		if err != nil {
			return nil, err
		}
		cs, err := s.Comments(r.ctx, aid)
		if err != nil {
			return nil, err
		}
		if cs == nil {
			cs = []store.Comment{}
		}
		out = append(out, contextEntry{Issue: iss, Comments: cs})
	}
	return out, nil
}

// printContextHuman renders the upstream bundle as a preamble before the
// claimed/shown issue. Reads oldest-first as the story leading up to the task.
func printContextHuman(r *runCtx, entries []contextEntry) {
	w := r.stdout
	if len(entries) == 0 {
		fmt.Fprintln(w, "Context: (no upstream dependencies)")
		fmt.Fprintln(w, strings.Repeat("─", 40))
		return
	}
	fmt.Fprintf(w, "Context — %d upstream task(s), oldest first:\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(w, "\n  %s  [%s]  %s\n", e.ID, e.Status, e.Title)
		if e.Description != nil && *e.Description != "" {
			fmt.Fprintf(w, "    description: %s\n", indent(*e.Description, "      "))
		}
		if e.Notes != nil && *e.Notes != "" {
			fmt.Fprintf(w, "    notes: %s\n", indent(*e.Notes, "      "))
		}
		if len(e.Comments) > 0 {
			fmt.Fprintf(w, "    comments:\n")
			for _, cm := range e.Comments {
				fmt.Fprintf(w, "      [%s] %s\n", cm.Author, indent(cm.Body, "        "))
			}
		}
	}
	fmt.Fprintln(w, strings.Repeat("─", 40))
}

// indent re-indents the 2nd..nth lines of s by pad (the first line follows
// the label inline). Keeps multi-line descriptions/comments readable.
func indent(s, pad string) string {
	return strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n"+pad)
}

// newIssueShowOut builds the JSON shape for one issue, normalizing nil
// slices so consumers always see arrays. Shared by printIssue and the
// --context wrapper.
func newIssueShowOut(i store.Issue, parents, blocks, labels []string, comments []store.Comment, blocked bool) issueShowOut {
	if parents == nil {
		parents = []string{}
	}
	if blocks == nil {
		blocks = []string{}
	}
	if labels == nil {
		labels = []string{}
	}
	if comments == nil {
		comments = []store.Comment{}
	}
	return issueShowOut{Issue: i, Labels: labels, Depends: parents, Blocks: blocks, Comments: comments, Blocked: blocked}
}
