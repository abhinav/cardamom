package cli

import (
	"fmt"

	"go.abhg.dev/cardamom/internal/attachment"
)

type attachmentGCCommand struct {
	DryRun bool `name:"dry-run" help:"Report what collection would remove without deleting local content."`
}

// Help describes store-wide conservative collection and dry-run behavior.
func (*attachmentGCCommand) Help() string {
	return `Collect expired staging files and unreferenced blobs explicitly.
Collection is store-wide, reports retained content integrity problems, and
never changes attachment metadata. Use --dry-run to report what collection
would remove without deleting bytes.`
}

func (c *attachmentGCCommand) Run(
	invocation *Invocation,
	service *attachment.Service,
) error {
	result, err := service.CollectAttachments(
		invocation.Context,
		attachment.CollectRequest{DryRun: c.DryRun},
	)
	if err != nil {
		return err
	}
	return writeAttachmentCollection(invocation.Output, result)
}

type attachmentCollectionOutput struct {
	DryRun            bool                        `json:"dry_run"`
	ExpiredStaging    attachmentCollectionSummary `json:"expired_staging"`
	OrphanBlobs       attachmentCollectionSummary `json:"orphan_blobs"`
	IntegrityProblems []attachmentIntegrityOutput `json:"integrity_problems"`
}

type attachmentCollectionSummary struct {
	Count uint64 `json:"count"`
	Bytes uint64 `json:"bytes"`
}

type attachmentIntegrityOutput struct {
	BoardID      string `json:"board_id"`
	AttachmentID string `json:"attachment_id"`
	Digest       string `json:"digest"`
	SizeBytes    uint64 `json:"size_bytes"`
	Availability string `json:"availability"`
}

func writeAttachmentCollection(output *Output, result attachment.CollectionResult) error {
	problems := make([]attachmentIntegrityOutput, len(result.IntegrityProblems))
	for index, problem := range result.IntegrityProblems {
		problems[index] = attachmentIntegrityOutput{
			BoardID: problem.BoardID.String(), AttachmentID: problem.AttachmentID.String(),
			Digest: problem.Blob.Digest.String(), SizeBytes: problem.Blob.SizeBytes,
			Availability: problem.Availability.String(),
		}
	}
	if output.JSON() {
		return output.WriteJSON(attachmentCollectionOutput{
			DryRun: result.DryRun,
			ExpiredStaging: attachmentCollectionSummary{
				Count: result.ExpiredStaging.Count, Bytes: result.ExpiredStaging.Bytes,
			},
			OrphanBlobs: attachmentCollectionSummary{
				Count: result.OrphanBlobs.Count, Bytes: result.OrphanBlobs.Bytes,
			},
			IntegrityProblems: problems,
		})
	}
	if err := output.WriteString(fmt.Sprintf(
		"Dry run: %t\nExpired staging: %d items, %d bytes\nOrphan blobs: %d items, %d bytes\nIntegrity problems: %d\n",
		result.DryRun,
		result.ExpiredStaging.Count,
		result.ExpiredStaging.Bytes,
		result.OrphanBlobs.Count,
		result.OrphanBlobs.Bytes,
		len(problems),
	)); err != nil {
		return err
	}
	for _, problem := range problems {
		if err := output.WriteString(fmt.Sprintf(
			"%s\t%s\t%s\t%d\t%s\n",
			problem.BoardID,
			problem.AttachmentID,
			problem.Digest,
			problem.SizeBytes,
			problem.Availability,
		)); err != nil {
			return err
		}
	}
	return nil
}
