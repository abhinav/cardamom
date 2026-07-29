package cli

import (
	"strconv"
	"strings"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/issue"
)

// issueSummaryOutput is the structured command projection for one issue list
// record. Embedding Issue preserves its established field names while the
// adapter adds query state that is not part of the durable issue itself.
type issueSummaryOutput struct {
	issue.Issue
	Labels  []string `json:"labels"`
	Blocked bool     `json:"blocked"`
}

func newIssueSummaryOutput(summary issue.Summary) issueSummaryOutput {
	return issueSummaryOutput{
		Issue: summary.Issue, Labels: nonNilStrings(summary.Labels),
		Blocked: summary.Blocked,
	}
}

// issueDetailOutput is the structured command projection for one issue's
// current state. Durable record bodies remain available through their owning
// commands instead of expanding every issue response.
type issueDetailOutput struct {
	issue.Issue
	Keys        []string     `json:"keys"`
	Labels      []string     `json:"labels"`
	DependsOn   []string     `json:"depends_on"`
	Blocks      []string     `json:"blocks"`
	LogCount    int          `json:"log_count"`
	LatestLogID *issue.LogID `json:"latest_log_id,omitempty"`
	Blocked     bool         `json:"blocked"`
	ParentID    *string      `json:"parent_id"`
}

type issueContextEntryOutput struct {
	issue.Issue
	LogCount     int          `json:"log_count"`
	LatestLogID  *issue.LogID `json:"latest_log_id,omitempty"`
	DetailsBytes int          `json:"details_bytes,omitempty"`
}

type issueResultOutput struct {
	IssueID string `json:"issue_id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
}

func newIssueDetailOutput(detail issue.Detail) issueDetailOutput {
	return issueDetailOutput{
		Issue:       detail.Issue,
		Keys:        nonNilStrings(detail.Keys),
		Labels:      nonNilStrings(detail.Labels),
		DependsOn:   issueReferenceIDs(detail.DependsOn),
		Blocks:      issueReferenceIDs(detail.Blocks),
		LogCount:    detail.LogSummary.Count,
		LatestLogID: detail.LogSummary.LatestID,
		Blocked:     detail.Blocked,
		ParentID:    detail.ParentID,
	}
}

// issueViewOutput owns the CLI's optional inherited-context envelope. Commands
// without inherited context keep the bare issue-detail JSON shape.
type issueViewOutput struct {
	detail  issueDetailOutput
	context *issueContextOutput
}

type issueContextOutput struct {
	Board             boardDescriptionOutput    `json:"board"`
	Ancestors         []issueContextEntryOutput `json:"context"`
	DependencyResults []issueResultOutput       `json:"dependency_results"`
}

type boardDescriptionOutput struct {
	Description *string `json:"description"`
}

func newIssueViewOutput(view issue.View) issueViewOutput {
	output := issueViewOutput{detail: newIssueDetailOutput(view.Detail)}
	if view.Context == nil {
		return output
	}

	ancestors := make([]issueContextEntryOutput, len(view.Context.Ancestors))
	for index, ancestor := range view.Context.Ancestors {
		ancestors[index] = issueContextEntryOutput{
			Issue:        ancestor.Issue,
			LogCount:     ancestor.LogSummary.Count,
			LatestLogID:  ancestor.LogSummary.LatestID,
			DetailsBytes: ancestor.DetailsBytes,
		}
	}
	results := make([]issueResultOutput, len(view.Context.DependencyResults))
	for index, result := range view.Context.DependencyResults {
		results[index] = issueResultOutput{
			IssueID: result.Issue.ID,
			Title:   result.Issue.Title,
			Body:    result.Body,
		}
	}
	output.context = &issueContextOutput{
		Board:             boardDescriptionOutput{Description: view.Context.Board.Description},
		Ancestors:         ancestors,
		DependencyResults: results,
	}
	return output
}

func newIssueInspectionOutput(view attachment.IssueView) issueInspectionOutput {
	output := newIssueViewOutput(view.Issue)
	attachments := make([]attachmentOutput, len(view.Attachments))
	for index, value := range view.Attachments {
		attachments[index] = newAttachmentOutput(value)
	}
	return issueInspectionOutput{
		detail: issueInspectionDetailOutput{
			issueDetailOutput: output.detail,
			Attachments:       attachments,
		},
		context: output.context,
	}
}

// issueInspectionDetailOutput adds associated attachments to the structured
// issue detail returned by show.
type issueInspectionDetailOutput struct {
	issueDetailOutput
	Attachments []attachmentOutput `json:"attachments"`
}

// issueInspectionOutput keeps attachment inspection out of other issue-detail
// command projections while preserving the optional context envelope.
type issueInspectionOutput struct {
	detail  issueInspectionDetailOutput
	context *issueContextOutput
}

func (o issueInspectionOutput) jsonValue() any {
	if o.context == nil {
		return o.detail
	}
	return struct {
		issueContextOutput
		Issue issueInspectionDetailOutput `json:"issue"`
	}{
		issueContextOutput: *o.context,
		Issue:              o.detail,
	}
}

func (o issueViewOutput) jsonValue() any {
	if o.context == nil {
		return o.detail
	}
	return struct {
		issueContextOutput
		Issue issueDetailOutput `json:"issue"`
	}{
		issueContextOutput: *o.context,
		Issue:              o.detail,
	}
}

func writeIssueDetail(
	output *Output,
	detail issue.Detail,
	attachments []attachment.Attachment,
) error {
	assignee := "-"
	if detail.Issue.Assignee != nil {
		assignee = *detail.Issue.Assignee
	}
	parent := "-"
	if detail.ParentID != nil {
		parent = *detail.ParentID
	}
	attachmentValues := make([]string, len(attachments))
	for index, value := range attachments {
		attachmentValues[index] = value.ID.String() + " (" + value.Filename.String() + ")"
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "ID", value: detail.Issue.ID},
		{name: "Title", value: detail.Issue.Title},
		{name: "Type", value: detail.Issue.Type},
		{name: "Status", value: detail.Issue.Status},
		{name: "Priority", value: strconv.Itoa(detail.Issue.Priority)},
		{name: "Assignee", value: assignee},
		{name: "Parent", value: parent},
	}
	if len(detail.Keys) > 0 {
		fields = append(fields, struct {
			name  string
			value string
		}{name: "Keys", value: strings.Join(detail.Keys, ", ")})
	}
	fields = append(fields, []struct {
		name  string
		value string
	}{
		{name: "Labels", value: joinedOrDash(detail.Labels)},
		{name: "Depends on", value: joinedOrDash(issueReferenceIDs(detail.DependsOn))},
		{name: "Blocks", value: joinedOrDash(issueReferenceIDs(detail.Blocks))},
		{name: "Log entries", value: strconv.Itoa(detail.LogSummary.Count)},
		{name: "Attachments", value: joinedOrDash(attachmentValues)},
	}...)
	for _, field := range fields {
		if err := output.WriteString(field.name + ": " + field.value + "\n"); err != nil {
			return err
		}
	}
	if detail.Issue.Summary != nil {
		if err := output.WriteString("\nSummary:\n" + *detail.Issue.Summary + "\n"); err != nil {
			return err
		}
	}
	if detail.Issue.Details != nil {
		if err := output.WriteString("\nDetails:\n" + *detail.Issue.Details + "\n"); err != nil {
			return err
		}
	}
	if detail.Issue.State != nil {
		if err := output.WriteString("\nState:\n" + *detail.Issue.State + "\n"); err != nil {
			return err
		}
	}
	if detail.CurrentResult != nil {
		return output.WriteString("\nResult:\n" + detail.CurrentResult.Body + "\n")
	}
	return nil
}

func joinedOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func issueReferenceIDs(values []issue.Reference) []string {
	ids := make([]string, len(values))
	for index, value := range values {
		ids[index] = value.ID
	}
	return ids
}
