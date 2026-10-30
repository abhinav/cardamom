package cli

import (
	"fmt"
	"strconv"
	"strings"

	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
)

// formatConflictValue exposes the selected assignment with current public IDs.
// Record text is preserved verbatim for inspection or composition in a file.
func formatConflictValue(value boardconflict.Value) string {
	reference := func(uid issue.UID) string {
		for _, ref := range value.Issues {
			if ref.UID == uid {
				return ref.Reference.ID
			}
		}
		return fmt.Sprintf("UID %x", uid.Bytes())
	}
	switch body := value.Payload.(type) {
	case *boardhistory.IssueTitleSet:
		return body.Title
	case *boardhistory.BoardNameSet:
		return body.Name
	case *boardhistory.BoardDescriptionSet:
		return optionalConflictText(body.Description, "No description")
	case *boardhistory.IssueSummarySet:
		return optionalConflictText(body.Summary, "No Summary")
	case *boardhistory.IssueDetailsSet:
		return optionalConflictText(body.Details, "No Details")
	case *boardhistory.IssueResult:
		return optionalConflictText(body.Body, "No Result")
	case *boardhistory.IssueKindSet:
		return body.Kind
	case *boardhistory.IssuePrioritySet:
		return fmt.Sprintf("Priority: %d", body.Priority)
	case *boardhistory.IssueLifecycleSet:
		if body.ClosedAt == nil {
			return body.Lifecycle
		}
		return fmt.Sprintf("%s since %s", body.Lifecycle, body.ClosedAt.Format("2006-01-02T15:04:05Z07:00"))
	case *boardhistory.IssueWaitingSet:
		if body.Waiting == nil {
			return "Not waiting"
		}
		return fmt.Sprintf("Waiting since %s\n%s", body.Waiting.Since.Format("2006-01-02T15:04:05Z07:00"), body.Waiting.Reason)
	case *boardhistory.IssueState:
		if body.State == nil {
			return "No State"
		}
		var out strings.Builder
		if body.State.Author != nil {
			fmt.Fprintf(&out, "Author: %s\n", *body.State.Author)
		}
		if body.State.UpdatedAt != nil {
			fmt.Fprintf(&out, "Updated: %s\n", body.State.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		if body.State.SnapshotLogEntryID != nil {
			fmt.Fprintf(&out, "Snapshot: %s\n", *body.State.SnapshotLogEntryID)
		}
		out.WriteString(body.State.Body)
		if body.State.NextAction != nil {
			fmt.Fprintf(&out, "\n\nNext action:\n%s", *body.State.NextAction)
		}
		return out.String()
	case *boardhistory.IssueCheckpoint:
		return fmt.Sprintf("%s at %s\n%s", body.Outcome, body.DecidedAt.Format("2006-01-02T15:04:05Z07:00"), body.Reason)
	case *boardhistory.IssueLabel:
		return fmt.Sprintf("%s label %q: present=%t", reference(body.IssueUID), body.Label, body.Present)
	case *boardhistory.IssueDependency:
		return fmt.Sprintf("%s depends on %s: present=%t", reference(body.IssueUID), reference(body.PrerequisiteUID), body.Present)
	case *boardhistory.IssueContainment:
		return fmt.Sprintf("%s parent %s: present=%t", reference(body.ChildUID), reference(body.ParentUID), body.Present)
	case *boardhistory.ExternalKey:
		return fmt.Sprintf("%s owned by %s: present=%t", body.Key, reference(body.IssueUID), body.Present)
	case *boardhistory.BoardPin:
		return fmt.Sprintf("%s pinned=%t position=%d", reference(body.IssueUID), body.Present, body.Position)
	case *boardhistory.BoardArchiveSet:
		if body.Archive == nil {
			return "Active board"
		}
		return fmt.Sprintf("Archived by %s at %s", body.Archive.Actor, body.Archive.At.Format("2006-01-02T15:04:05Z07:00"))
	case *boardhistory.BoardIssueIDPrefixSet:
		return optionalConflictText(body.Prefix, "Inherit issue ID prefix")
	case *boardhistory.BoardIssueIDStrategySet:
		return optionalConflictText(body.Strategy, "Inherit issue ID strategy")
	case *boardhistory.BoardSummaryLimitSet:
		return optionalConflictLimit(body.Limit)
	case *boardhistory.BoardAttachmentLimitSet:
		return optionalConflictLimit(body.Limit)
	case *boardhistory.BoardPinLimitSet:
		return optionalConflictLimit(body.Limit)
	}
	return "Unsupported retained assignment"
}

func optionalConflictText(text *string, absent string) string {
	if text == nil {
		return absent
	}
	return *text
}

func optionalConflictLimit(value *int64) string {
	if value == nil {
		return "Inherit limit"
	}
	return strconv.FormatInt(*value, 10)
}
