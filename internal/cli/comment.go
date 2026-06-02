package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Rovak/agents-clu/internal/store"
)

type CommentCmd struct {
	Add  CommentAddCmd  `cmd:"" help:"Add a comment to an issue. Pass body as args, '-', or via stdin (heredoc)."`
	Edit CommentEditCmd `cmd:"" help:"Replace the body of an existing comment."`
	Ls   CommentLsCmd   `cmd:"" aliases:"list" help:"List comments on an issue, chronological."`
	Rm   CommentRmCmd   `cmd:"" aliases:"remove" help:"Remove a comment by its numeric ID."`
}

// CommentAddCmd appends a comment to an issue. The body can come from
// positional args, the literal "-" (read stdin), or — if no body args
// are given and stdin isn't a TTY — implicit stdin. The implicit form
// keeps heredoc/here-string usage tidy without needing an explicit "-".
type CommentAddCmd struct {
	ID     string   `arg:"" help:"Issue ID."`
	Body   []string `arg:"" optional:"" help:"Comment body. Use '-' or omit (with piped stdin) to read from stdin."`
	Author string   `short:"a" name:"agent" default:"${user}" help:"Agent identity used as the comment author. Defaults to $USER."`
}

func (c *CommentAddCmd) Run(r *runCtx) error {
	body, err := readBody(c.Body)
	if err != nil {
		return err
	}
	if body == "" {
		return fmt.Errorf("comment body required (positional args, '-', or pipe via stdin)")
	}
	return withStore(r, func(s *store.Store) error {
		s.SetActor(c.Author) // the comment author is the acting identity
		cm, err := s.AddComment(r.ctx, c.ID, c.Author, body)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(cm)
		}
		r.notice("commented on %s as %s (#%d)\n", cm.IssueID, cm.Author, cm.ID)
		return nil
	})
}

// readBody resolves the body source: positional args, "-" (explicit
// stdin), or implicit stdin when no args were given and stdin is piped
// in. Returns trimmed text (trailing newline + leading/trailing space).
// An empty stdin or no-args-with-tty yields "" and the caller decides
// how to handle that (Add rejects, Edit treats as clear-and-fail).
func readBody(args []string) (string, error) {
	joined := strings.TrimSpace(strings.Join(args, " "))
	// Explicit "-" → read stdin even when other args are present.
	if joined == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	if len(args) == 0 && !isStdinTTY() {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	return joined, nil
}

// CommentEditCmd replaces the body of an existing comment. The author
// + created timestamp are preserved so references to the comment ID
// stay stable.
type CommentEditCmd struct {
	ID    int64    `arg:"" help:"Comment ID (numeric, as shown in 'clu comment ls')."`
	Body  []string `arg:"" optional:"" help:"New body. Same input modes as 'comment add' (positional, '-', or stdin)."`
	Agent string   `short:"a" name:"agent" default:"${user}" help:"Agent identity recorded as the actor for this edit. Defaults to $USER."`
}

func (c *CommentEditCmd) Run(r *runCtx) error {
	body, err := readBody(c.Body)
	if err != nil {
		return err
	}
	if body == "" {
		return fmt.Errorf("comment body required (positional args, '-', or pipe via stdin)")
	}
	return withStore(r, func(s *store.Store) error {
		s.SetActor(c.Agent)
		cm, err := s.EditComment(r.ctx, c.ID, body)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(cm)
		}
		r.notice("edited comment #%d on %s\n", cm.ID, cm.IssueID)
		return nil
	})
}

type CommentLsCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

func (c *CommentLsCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		cs, err := s.Comments(r.ctx, c.ID)
		if err != nil {
			return err
		}
		if r.json {
			if cs == nil {
				cs = []store.Comment{}
			}
			return r.emitJSON(cs)
		}
		printComments(r, cs)
		return nil
	})
}

type CommentRmCmd struct {
	ID    int64  `arg:"" help:"Comment ID (numeric, as shown in 'cli comment ls')."`
	Agent string `short:"a" name:"agent" default:"${user}" help:"Agent identity recorded as the actor for this removal. Defaults to $USER."`
}

func (c *CommentRmCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		s.SetActor(c.Agent)
		if err := s.RemoveComment(r.ctx, c.ID); err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(map[string]any{"id": c.ID, "removed": true})
		}
		r.notice("removed comment #%d\n", c.ID)
		return nil
	})
}

// printComments renders the human form: one header per comment, body indented.
func printComments(r *runCtx, cs []store.Comment) {
	if len(cs) == 0 {
		fmt.Fprintln(r.stdout, "(no comments)")
		return
	}
	for _, cm := range cs {
		ts := time.Unix(cm.Created, 0).Format(time.RFC3339)
		fmt.Fprintf(r.stdout, "[#%d] %s %s\n", cm.ID, cm.Author, ts)
		for _, line := range strings.Split(cm.Body, "\n") {
			fmt.Fprintf(r.stdout, "  %s\n", line)
		}
	}
}
