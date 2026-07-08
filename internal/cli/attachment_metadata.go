package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

type attachmentShowCommand struct {
	ID string `arg:"" name:"id" help:"Attachment ID."`
}

// Help describes metadata and replica-local availability shown by the command.
func (*attachmentShowCommand) Help() string {
	return `Show one attachment's board association, immutable blob descriptor,
lifecycle, replica-local availability, and mutation attribution.`
}

func (c *attachmentShowCommand) Run(
	invocation *Invocation,
	selected *board.State,
	service *attachment.Service,
) error {
	id, err := attachment.NewID(c.ID)
	if err != nil {
		return UsageErrorf("attachment show: %s", err)
	}
	value, err := service.GetAttachment(invocation.Context, attachment.GetRequest{
		BoardID: selected.ID(), AttachmentID: id,
	})
	if err != nil {
		return err
	}
	return writeAttachment(invocation.Output, value)
}

type attachmentListCommand struct {
	Issue          string `name:"issue" predictor:"issues" placeholder:"ISSUE" help:"Restrict results to this originating issue."`
	IncludeRemoved bool   `name:"include-removed" help:"Include permanently tombstoned attachment metadata."`
	PageSize       uint32 `name:"page-size" placeholder:"COUNT" help:"Maximum attachments in this page. Zero selects the default page size."`
	PageToken      string `name:"page-token" placeholder:"TOKEN" help:"Resume a prior stable page from its opaque next-page token."`
}

func (c *attachmentListCommand) referencedIssueIDs() []string {
	if c.Issue == "" {
		return nil
	}
	return []string{c.Issue}
}

// Help describes stable page selection and tombstone visibility.
func (*attachmentListCommand) Help() string {
	return `List one stable page of attachment metadata for the selected board.
Use --issue to restrict the originating issue, --include-removed to include
tombstones, and --page-token to continue a previous listing.`
}

func (c *attachmentListCommand) Run(
	invocation *Invocation,
	selected *board.State,
	service *attachment.Service,
) error {
	var originIssueID *issue.ID
	if c.Issue != "" {
		parsed, err := issue.NewID(c.Issue)
		if err != nil {
			return UsageErrorf("attachment list: --issue: %s", err)
		}
		originIssueID = &parsed
	}

	page, err := service.ListAttachments(invocation.Context, attachment.ListRequest{
		BoardID:        selected.ID(),
		OriginIssueID:  originIssueID,
		IncludeRemoved: c.IncludeRemoved,
		PageSize:       c.PageSize,
		PageToken:      c.PageToken,
	})
	if err != nil {
		return err
	}
	return writeAttachmentPage(invocation.Output, page)
}

type attachmentRemoveCommand struct {
	ID string `arg:"" name:"id" help:"Attachment ID."`
}

// Help describes the monotonic attachment tombstone transition.
func (*attachmentRemoveCommand) Help() string {
	return `Permanently tombstone one attachment. Removal is permanent and does
not by itself delete the retained immutable blob from local storage.`
}

func (c *attachmentRemoveCommand) Run(
	invocation *Invocation,
	selected *board.State,
	service *attachment.Service,
) error {
	id, err := attachment.NewID(c.ID)
	if err != nil {
		return UsageErrorf("attachment remove: %s", err)
	}
	removed, err := service.RemoveAttachment(invocation.Context, attachment.RemoveRequest{
		Invocation:   attachment.NewInvocation(invocation.Actor),
		BoardID:      selected.ID(),
		AttachmentID: id,
	})
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(newAttachmentOutput(removed))
	}
	return invocation.Output.Noticef("removed %s", removed.ID)
}

type attachmentOutput struct {
	ID           string                       `json:"id"`
	BoardID      string                       `json:"board_id"`
	OriginIssue  *string                      `json:"origin_issue_id"`
	Filename     string                       `json:"filename"`
	MediaType    string                       `json:"media_type"`
	Digest       string                       `json:"digest"`
	SizeBytes    uint64                       `json:"size_bytes"`
	Lifecycle    string                       `json:"lifecycle"`
	Availability string                       `json:"availability"`
	Created      attachmentAttributionOutput  `json:"created"`
	Removed      *attachmentAttributionOutput `json:"removed"`
}

type attachmentAttributionOutput struct {
	Actor    string    `json:"actor"`
	At       time.Time `json:"at"`
	Revision int64     `json:"revision"`
}

func newAttachmentOutput(value attachment.Attachment) attachmentOutput {
	var originIssue *string
	if id, ok := value.Association.OriginIssueID(); ok {
		originIssue = new(id.String())
	}
	result := attachmentOutput{
		ID: value.ID.String(), BoardID: value.Association.BoardID().String(),
		OriginIssue: originIssue, Filename: value.Filename.String(),
		MediaType: value.MediaType.String(), Digest: value.Blob.Digest.String(),
		SizeBytes: value.Blob.SizeBytes, Lifecycle: value.Lifecycle.String(),
		Availability: value.Availability.String(),
		Created: attachmentAttributionOutput{
			Actor: value.Created.Actor, At: value.Created.At,
			Revision: int64(value.Created.Revision),
		},
	}
	if value.Removed != nil {
		result.Removed = &attachmentAttributionOutput{
			Actor: value.Removed.Actor, At: value.Removed.At,
			Revision: int64(value.Removed.Revision),
		}
	}
	return result
}

func writeAttachment(output *Output, value attachment.Attachment) error {
	if output.JSON() {
		return output.WriteJSON(newAttachmentOutput(value))
	}

	originIssue := "-"
	if id, ok := value.Association.OriginIssueID(); ok {
		originIssue = id.String()
	}
	if err := output.WriteString(fmt.Sprintf(
		"Attachment %s\nBoard: %s\nIssue: %s\nName: %s\nType: %s\nSize: %d bytes\nDigest: %s\nLifecycle: %s\nAvailability: %s\nCreated: %s by %s (revision %d)\n",
		value.ID, value.Association.BoardID(), originIssue, value.Filename,
		value.MediaType, value.Blob.SizeBytes, value.Blob.Digest,
		value.Lifecycle, value.Availability, value.Created.At.Format(time.RFC3339),
		value.Created.Actor, value.Created.Revision,
	)); err != nil {
		return err
	}
	if value.Removed != nil {
		return output.WriteString(fmt.Sprintf(
			"Removed: %s by %s (revision %d)\n",
			value.Removed.At.Format(time.RFC3339),
			value.Removed.Actor,
			value.Removed.Revision,
		))
	}
	return nil
}

type attachmentPageOutput struct {
	Attachments   []attachmentOutput `json:"attachments"`
	NextPageToken string             `json:"next_page_token,omitempty"`
}

func writeAttachmentPage(output *Output, page attachment.Page) error {
	if output.JSON() {
		values := make([]attachmentOutput, len(page.Attachments))
		for index, value := range page.Attachments {
			values[index] = newAttachmentOutput(value)
		}
		return output.WriteJSON(attachmentPageOutput{
			Attachments: values, NextPageToken: page.NextPageToken,
		})
	}

	writer := tabwriter.NewWriter(output.Stdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tNAME\tSIZE\tTYPE\tLIFECYCLE\tAVAILABILITY\tISSUE"); err != nil {
		return fmt.Errorf("write attachment list header: %w", err)
	}
	for _, value := range page.Attachments {
		originIssue := "-"
		if id, ok := value.Association.OriginIssueID(); ok {
			originIssue = id.String()
		}
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			value.ID,
			value.Filename,
			value.Blob.SizeBytes,
			value.MediaType,
			value.Lifecycle,
			value.Availability,
			originIssue,
		); err != nil {
			return fmt.Errorf("write attachment list: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush attachment list: %w", err)
	}
	if page.NextPageToken != "" {
		return output.WriteString("Next page token: " + page.NextPageToken + "\n")
	}
	return nil
}
