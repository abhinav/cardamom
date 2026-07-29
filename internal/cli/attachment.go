package cli

// attachmentCommand groups the board attachment workflows under one
// discoverable CLI namespace.
type attachmentCommand struct {
	Add    attachmentAddCommand    `cmd:"" help:"Upload a file or standard input."`
	Show   attachmentShowCommand   `cmd:"" help:"Show attachment metadata."`
	List   attachmentListCommand   `cmd:"" help:"List attachment metadata."`
	Get    attachmentGetCommand    `cmd:"" help:"Download verified attachment content."`
	Remove attachmentRemoveCommand `cmd:"" help:"Permanently tombstone an attachment."`
	GC     attachmentGCCommand     `cmd:"" name:"gc" help:"Collect expired staging and orphan blob content."`
}

// Help describes the attachment command family without requiring separate
// workflow documentation.
func (*attachmentCommand) Help() string {
	return `Add, inspect, download, remove, and collect board attachments.
Attachment references use attachment:<id> and remain scoped to their board.`
}
