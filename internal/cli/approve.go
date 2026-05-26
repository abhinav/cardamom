package cli

// ApproveCmd is sugar for `cli checkpoint pass`. Mirrors the pattern of
// assign/tag/link — a short verb that wraps a longer command.
type ApproveCmd struct {
	ID     string `arg:"" help:"Checkpoint issue ID."`
	Reason string `name:"reason" help:"Optional note appended to the issue when approving."`
}

func (c *ApproveCmd) Run(r *runCtx) error {
	return resolveCheckpoint(r, c.ID, true, c.Reason)
}
