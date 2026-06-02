package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/arjia-labs/clu/internal/store"
)

type NoteCmd struct {
	Set    NoteSetCmd    `cmd:"" help:"Replace an issue's notes."`
	Append NoteAppendCmd `cmd:"" aliases:"add" help:"Append a note to the existing notes."`
	Clear  NoteClearCmd  `cmd:"" help:"Clear an issue's notes."`
	Show   NoteShowCmd   `cmd:"" aliases:"get" help:"Print the current notes."`
}

type NoteSetCmd struct {
	ID   string   `arg:"" help:"Issue ID."`
	Text []string `arg:"" required:"" help:"Note text."`
}

func (c *NoteSetCmd) Run(r *runCtx) error {
	text := strings.TrimSpace(strings.Join(c.Text, " "))
	if text == "" {
		// Symmetric with `comment add` (which rejects empty bodies).
		// Use `clu note clear <id>` if you actually meant to wipe notes;
		// silently storing "" was confusing in scripts.
		return errors.New("note text required (use `clu note clear <id>` to clear)")
	}
	return withStore(r, func(s *store.Store) error {
		i, err := s.SetNotes(r.ctx, c.ID, text)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(issueOut{Issue: i})
		}
		r.notice("set notes on %s\n", i.ID)
		return nil
	})
}

type NoteAppendCmd struct {
	ID   string   `arg:"" help:"Issue ID."`
	Text []string `arg:"" required:"" help:"Note text to append."`
}

func (c *NoteAppendCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		text := strings.Join(c.Text, " ")
		i, err := s.AppendNote(r.ctx, c.ID, text)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(issueOut{Issue: i})
		}
		r.notice("appended note to %s\n", i.ID)
		return nil
	})
}

type NoteClearCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

func (c *NoteClearCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		i, err := s.SetNotes(r.ctx, c.ID, "")
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(issueOut{Issue: i})
		}
		r.notice("cleared notes on %s\n", i.ID)
		return nil
	})
}

type NoteShowCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

func (c *NoteShowCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		i, err := s.Get(r.ctx, c.ID)
		if err != nil {
			return err
		}
		if i.Notes == nil || *i.Notes == "" {
			fmt.Fprintln(r.stdout, "(no notes)")
			return nil
		}
		fmt.Fprintln(r.stdout, *i.Notes)
		return nil
	})
}
