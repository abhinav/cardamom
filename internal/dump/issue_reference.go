package dump

import (
	"fmt"
	"path"
	"strings"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/markdown/reference"
)

// Reference targets preserve full-board ownership while restricting
// navigation to issue pages emitted by the requested selection.
type referenceTargets struct {
	// The selected issue index contains every issue page emitted by the dump.
	selectedIssues map[string]struct{}

	// The log owner index maps every board log identity to its owning issue.
	logOwners map[issue.LogID]string
}

func newReferenceTargets(
	board BoardSnapshot,
	selectedIssues map[string]struct{},
) referenceTargets {
	logOwners := make(map[issue.LogID]string, len(board.LogEntries))
	for _, entry := range board.LogEntries {
		logOwners[entry.ID] = entry.IssueID
	}
	return referenceTargets{
		selectedIssues: selectedIssues,
		logOwners:      logOwners,
	}
}

// rewriteReferenceRecords applies dump navigation policy relative to each
// record's canonical generated-file location.
func rewriteReferenceRecords(
	snapshot Snapshot,
	records []markdownRecord,
	attachments attachmentResolutions,
) Snapshot {
	for _, record := range records {
		*record.body = markdown.RewriteReferences(
			*record.body,
			func(identity reference.Identity) string {
				switch identity.Kind {
				case reference.KindIssue:
					if !contains(snapshot.referenceTargets.selectedIssues, identity.ID) {
						return "%" + identity.ID
					}
					return markdownReferenceLink(
						"%"+identity.ID,
						record.outputPath,
						canonicalIssuePath(Issue{ID: identity.ID}),
					)
				case reference.KindLog:
					logID := issue.LogID(identity.ID)
					owner, ok := snapshot.referenceTargets.logOwners[logID]
					if !ok || !contains(snapshot.referenceTargets.selectedIssues, owner) {
						return "%" + identity.ID
					}
					return markdownReferenceLink(
						"%"+identity.ID,
						record.outputPath,
						canonicalIssuePath(Issue{ID: owner})+"#"+identity.ID,
					)
				case reference.KindAttachment:
					id := attachment.ID(identity.ID)
					resolution := attachments[id]
					if resolution.State == attachment.ResolutionActive &&
						resolution.Attachment != nil {
						return markdownReferenceLink(
							resolution.Attachment.Filename.String(),
							record.outputPath,
							attachmentMarkdownPath(*resolution.Attachment),
						)
					}
					if resolution.State == attachment.ResolutionRemoved &&
						resolution.Attachment != nil {
						return fmt.Sprintf(
							"Attachment unavailable: %s (%s)",
							code(resolution.Attachment.Filename.String()),
							code(identity.ID),
						)
					}
					return "%" + identity.ID
				default:
					return "%" + identity.ID
				}
			},
		)
	}
	return snapshot
}

func markdownReferenceLink(label, outputPath, target string) string {
	return fmt.Sprintf(
		"[%s](%s)",
		escapeText(label),
		relativeMarkdownPath(outputPath, target),
	)
}

func relativeMarkdownPath(outputPath, target string) string {
	directory := path.Dir(outputPath)
	if directory == "." {
		return target
	}
	depth := strings.Count(directory, "/") + 1
	return path.Join(strings.Repeat("../", depth), target)
}
