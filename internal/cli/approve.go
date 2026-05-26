package cli

// ApproveCmd is sugar for `clu checkpoint pass`. Mirrors the pattern of
// assign/tag/link — a short verb that wraps a longer command.
type ApproveCmd struct {
	ID     string `arg:"" help:"Checkpoint issue ID."`
	As     string `short:"a" name:"agent" default:"${user}" help:"Approver identity (defaults to $USER). Must match an entry in the approvers list."`
	Reason string `name:"reason" help:"Optional note appended to the issue when approving."`
}

func (c *ApproveCmd) Run(r *runCtx) error {
	return resolveCheckpoint(r, c.ID, c.As, true, c.Reason)
}
