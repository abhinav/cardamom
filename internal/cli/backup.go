package cli

import (
	"context"
	"fmt"
)

// BackupRequest carries archive and board selectors to process composition.
type BackupRequest struct {
	// Destination is the archive output path supplied by the caller.
	Destination string

	// SourceStore is the global physical store selector.
	SourceStore string

	// DefaultBoard is the explicit or ambient board used when no aggregate
	// selection mode is requested.
	DefaultBoard string

	// IncludeBoards contains explicit stable ID or exact-name selectors.
	IncludeBoards []string

	// All selects every board in the source store.
	All bool
}

// BackupResult is the concise public summary of one archive publication.
type BackupResult struct {
	// Source is the resolved physical source store directory.
	Source string `json:"source"`

	// Destination is the archive path published by the operation.
	Destination string `json:"destination"`

	// Projects is the number of archived projects.
	Projects int `json:"projects"`

	// Boards is the number of archived complete boards.
	Boards int `json:"boards"`

	// Blobs is the number of archived unique attachment blobs.
	Blobs int `json:"blobs"`
}

// BackupOperation writes one portable backup through process-owned resources.
type BackupOperation interface {
	Backup(context.Context, BackupRequest) (BackupResult, error)
}

type backupCommand struct {
	Destination  string   `arg:"" name:"output" placeholder:"OUTPUT" help:"Portable backup archive to create."`
	IncludeBoard []string `name:"include-board" placeholder:"BOARD" predictor:"boards" help:"Complete board ID or exact name to include. May be repeated."`
	All          bool     `name:"all" help:"Include every board in the selected store."`
}

// Help describes source selection and atomic archive publication.
func (*backupCommand) Help() string {
	return `Write complete boards and their attachment blobs to one portable archive.

By default, the ambient or global --board selection supplies the source board.
Repeat --include-board to select complete boards by stable ID or exact name, or
use --all for every board. --include-board and --all replace the default board
selection and cannot be combined with an explicitly supplied global --board.`
}

// Run validates selection syntax, publishes the archive, and renders one
// summary.
func (c *backupCommand) Run(
	invocation *Invocation,
	operation BackupOperation,
) error {
	if len(c.IncludeBoard) > 0 && c.All {
		return UsageErrorf("--include-board cannot be combined with --all")
	}
	if invocation.BoardExplicit && (len(c.IncludeBoard) > 0 || c.All) {
		return UsageErrorf(
			"--board cannot be combined with --include-board or --all",
		)
	}

	defaultBoard := invocation.Board
	if len(c.IncludeBoard) > 0 || c.All {
		defaultBoard = ""
	}
	result, err := operation.Backup(invocation.Context, BackupRequest{
		Destination:   c.Destination,
		SourceStore:   invocation.Store,
		DefaultBoard:  defaultBoard,
		IncludeBoards: c.IncludeBoard,
		All:           c.All,
	})
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(result)
	}
	return invocation.Output.WriteString(fmt.Sprintf(
		"backed up %d %s, %d %s, and %d %s from %s to %s\n",
		result.Projects,
		plural(result.Projects, "project", "projects"),
		result.Boards,
		plural(result.Boards, "board", "boards"),
		result.Blobs,
		plural(result.Blobs, "blob", "blobs"),
		result.Source,
		result.Destination,
	))
}

// RestoreRequest carries archive and destination selection to process
// composition.
type RestoreRequest struct {
	// Source is the portable backup archive path supplied by the caller.
	Source string

	// DestinationStore is the global physical store selector.
	DestinationStore string

	// DestinationStoreExplicit reports whether --store supplied the selector.
	DestinationStoreExplicit bool
}

// RestoreResult is the concise public summary of one complete archive load.
type RestoreResult struct {
	// Source is the resolved portable archive path.
	Source string `json:"source"`

	// Destination is the resolved physical destination store directory.
	Destination string `json:"destination"`

	// Projects is the number of archived projects reconciled.
	Projects int `json:"projects"`

	// Boards is the number of archived boards restored or already completed.
	Boards int `json:"boards"`

	// Blobs is the number of unique archive blobs published idempotently.
	Blobs int `json:"blobs"`

	// AlreadyCompletedBoards is the number of retained restore receipts.
	AlreadyCompletedBoards int `json:"already_completed_boards"`
}

// RestoreOperation loads one portable backup through process-owned resources.
type RestoreOperation interface {
	Restore(context.Context, RestoreRequest) (RestoreResult, error)
}

type restoreCommand struct {
	Source string `arg:"" name:"input" placeholder:"INPUT" help:"Portable backup archive to restore."`
}

// Help describes complete, non-destructive archive restoration.
func (*restoreCommand) Help() string {
	return `Restore every board in a portable backup archive.

The global --store selects the destination. An explicitly supplied missing
store directory is created without writing config.yaml or checkout bindings.
Existing unrelated projects and boards remain unchanged. Reapplying the same
archive resumes or reports boards whose restore already completed.`
}

// Run restores the archive and renders one summary.
func (c *restoreCommand) Run(
	invocation *Invocation,
	operation RestoreOperation,
) error {
	result, err := operation.Restore(invocation.Context, RestoreRequest{
		Source:                   c.Source,
		DestinationStore:         invocation.Store,
		DestinationStoreExplicit: invocation.StoreExplicit,
	})
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(result)
	}
	return invocation.Output.WriteString(fmt.Sprintf(
		"restored %d %s, %d %s, and %d %s from %s to %s (%d already completed)\n",
		result.Projects,
		plural(result.Projects, "project", "projects"),
		result.Boards,
		plural(result.Boards, "board", "boards"),
		result.Blobs,
		plural(result.Blobs, "blob", "blobs"),
		result.Source,
		result.Destination,
		result.AlreadyCompletedBoards,
	))
}
