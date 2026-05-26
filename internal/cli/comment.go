package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/rovak/beadsv2/internal/store"
)

type CommentCmd struct {
	Add CommentAddCmd `cmd:"" help:"Add a comment to an issue."`
	Ls  CommentLsCmd  `cmd:"" aliases:"list" help:"List comments on an issue."`
	Rm  CommentRmCmd  `cmd:"" aliases:"remove" help:"Remove a comment by its numeric ID."`
}

type CommentAddCmd struct {
	ID     string   `arg:"" help:"Issue ID."`
	Body   []string `arg:"" required:"" help:"Comment body."`
	Author string   `name:"as" default:"${user}" help:"Author name (defaults to current user)."`
}

func (c *CommentAddCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		body := strings.Join(c.Body, " ")
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
			return r.emitJSON(cs)
		}
		printComments(r, cs)
		return nil
	})
}

type CommentRmCmd struct {
	ID int64 `arg:"" help:"Comment ID (numeric, as shown in 'bd comment ls')."`
}

func (c *CommentRmCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.RemoveComment(r.ctx, c.ID); err != nil {
			return err
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
