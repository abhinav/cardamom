package dump

import (
	"cmp"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/markdown/reference"
)

var attachmentDestination = regexp.MustCompile(`attachment:[^\s)>]+`)

// markdownRecord identifies one authored Markdown value and its generated-file
// location in a cloned dump snapshot.
type markdownRecord struct {
	// name identifies the record in dump rewrite diagnostics.
	name string

	// outputPath is the canonical dump-root-relative generated file path.
	outputPath string

	// body points into the cloned snapshot rewritten for this dump.
	body *string

	// attachmentReferences contains parsed attachment destinations in body.
	attachmentReferences []attachmentReference
}

type attachmentReference struct {
	id    attachment.ID
	start int
	end   int
	uses  []sourceRange
}

type sourceRange struct {
	start int
	end   int
}

func collectMarkdownRecords(snapshot Snapshot) []markdownRecord {
	var records []markdownRecord
	if snapshot.Selection.Mode == SelectionWholeBoard && snapshot.Description != nil {
		records = append(records, markdownRecord{
			name: "board description", outputPath: "README.md", body: snapshot.Description,
		})
	}
	for index := range snapshot.Issues {
		value := &snapshot.Issues[index]
		outputPath := canonicalIssuePath(*value)
		if value.Summary != nil {
			records = append(records, markdownRecord{
				name:       fmt.Sprintf("issue %q summary", value.ID),
				outputPath: outputPath, body: value.Summary,
			})
		}
		if value.Details != nil {
			records = append(records, markdownRecord{
				name:       fmt.Sprintf("issue %q details", value.ID),
				outputPath: outputPath, body: value.Details,
			})
		}
		if value.State != nil {
			records = append(records, markdownRecord{
				name:       fmt.Sprintf("issue %q state", value.ID),
				outputPath: outputPath, body: value.State,
			})
		}
		if value.NextAction != nil {
			records = append(records, markdownRecord{
				name:       fmt.Sprintf("issue %q next action", value.ID),
				outputPath: outputPath, body: value.NextAction,
			})
		}
	}
	for index := range snapshot.Results {
		value := &snapshot.Results[index]
		records = append(records, markdownRecord{
			name:       fmt.Sprintf("issue %q result", value.IssueID),
			outputPath: filepath.ToSlash(filepath.Join("issues", value.IssueID+".md")),
			body:       &value.Body,
		})
	}
	for index := range snapshot.LogEntries {
		value := &snapshot.LogEntries[index]
		records = append(records, markdownRecord{
			name:       fmt.Sprintf("log entry %s on issue %q", value.ID, value.IssueID),
			outputPath: filepath.ToSlash(filepath.Join("issues", value.IssueID+".md")),
			body:       &value.Body,
		})
		if value.NextAction != nil {
			records = append(records, markdownRecord{
				name: fmt.Sprintf(
					"log entry %s next action on issue %q",
					value.ID,
					value.IssueID,
				),
				outputPath: filepath.ToSlash(
					filepath.Join("issues", value.IssueID+".md"),
				),
				body: value.NextAction,
			})
		}
	}

	return records
}

func collectAttachmentReferences(records []markdownRecord) ([]attachment.ID, error) {
	unique := make(map[attachment.ID]struct{})
	for index := range records {
		references, err := parseAttachmentReferences(*records[index].body)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", records[index].name, err)
		}
		records[index].attachmentReferences = references
		for _, reference := range references {
			unique[reference.id] = struct{}{}
		}
		_ = markdown.RewriteReferences(*records[index].body, func(identity reference.Identity) string {
			if identity.Kind == reference.KindAttachment {
				unique[attachment.ID(identity.ID)] = struct{}{}
			}
			return "%" + identity.ID
		})
	}
	ids := make([]attachment.ID, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b attachment.ID) int {
		return cmp.Compare(a.String(), b.String())
	})
	return ids, nil
}

func parseAttachmentReferences(source string) ([]attachmentReference, error) {
	matches := attachmentDestination.FindAllStringIndex(source, -1)
	if len(matches) == 0 {
		return nil, nil
	}
	annotated := []byte(source)
	markers := make(map[string]attachmentReference, len(matches))
	for index, match := range matches {
		marker := "x:" + fmt.Sprintf("%0*x", match[1]-match[0]-2, index)
		copy(annotated[match[0]:match[1]], marker)
		markers[marker] = attachmentReference{start: match[0], end: match[1]}
	}

	document := goldmark.DefaultParser().Parse(text.NewReader(annotated))
	var references []attachmentReference
	referenceIndexes := make(map[int]int)
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var destination []byte
		switch value := node.(type) {
		case *ast.Link:
			destination = value.Destination
		case *ast.Image:
			destination = value.Destination
		default:
			return ast.WalkContinue, nil
		}
		reference, ok := markers[string(destination)]
		if !ok {
			return ast.WalkContinue, nil
		}
		referenceIndex, duplicate := referenceIndexes[reference.start]
		if !duplicate {
			value := source[reference.start:reference.end]
			id, err := attachment.NewID(strings.TrimPrefix(value, "attachment:"))
			if err != nil {
				return ast.WalkStop, fmt.Errorf("invalid attachment destination %q", value)
			}
			reference.id = id
			referenceIndex = len(references)
			references = append(references, reference)
			referenceIndexes[reference.start] = referenceIndex
		}
		usage, ok := attachmentLinkUsageBounds(node, source, reference.start, reference.end)
		if ok && !slices.Contains(references[referenceIndex].uses, usage) {
			references[referenceIndex].uses = append(references[referenceIndex].uses, usage)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(references, func(a, b attachmentReference) int {
		return cmp.Compare(a.start, b.start)
	})
	return references, nil
}

func attachmentLinkUsageBounds(
	node ast.Node,
	source string,
	destinationStart int,
	destinationEnd int,
) (sourceRange, bool) {
	if start, end, ok := inlineAttachmentLinkBounds(source, destinationStart, destinationEnd); ok {
		return sourceRange{start: start, end: end}, true
	}
	labelStart, labelEnd := len(source), -1
	_ = ast.Walk(node, func(descendant ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		value, ok := descendant.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		labelStart = min(labelStart, value.Segment.Start)
		labelEnd = max(labelEnd, value.Segment.Stop)
		return ast.WalkContinue, nil
	})
	if labelEnd < 0 {
		return sourceRange{}, false
	}
	for labelStart > 0 && source[labelStart-1] != '[' {
		if source[labelStart-1] == '\n' {
			return sourceRange{}, false
		}
		labelStart--
	}
	if labelStart == 0 || source[labelStart-1] != '[' {
		return sourceRange{}, false
	}
	start := labelStart - 1
	if start > 0 && source[start-1] == '!' && !escapedAt(source, start-1) {
		start--
	}
	for labelEnd < len(source) && source[labelEnd] != ']' {
		if source[labelEnd] == '\n' {
			return sourceRange{}, false
		}
		labelEnd++
	}
	if labelEnd >= len(source) {
		return sourceRange{}, false
	}
	end := labelEnd + 1
	if end < len(source) && source[end] == '[' {
		for end++; end < len(source) && source[end] != ']'; end++ {
			if source[end] == '\n' {
				return sourceRange{}, false
			}
		}
		if end >= len(source) {
			return sourceRange{}, false
		}
		end++
	}
	return sourceRange{start: start, end: end}, true
}

func rewriteAttachmentRecords(
	snapshot Snapshot,
	records []markdownRecord,
	resolutions []attachment.Resolution,
) (Snapshot, error) {
	byID := make(map[attachment.ID]attachment.Resolution, len(resolutions))
	for _, resolution := range resolutions {
		byID[resolution.AttachmentID] = resolution
	}
	for _, record := range records {
		body := *record.body
		var edits []markdownEdit
		for _, reference := range record.attachmentReferences {
			resolution := byID[reference.id]
			switch resolution.State {
			case attachment.ResolutionActive:
				target := attachmentMarkdownPath(*resolution.Attachment)
				edits = append(edits, markdownEdit{
					start: reference.start, end: reference.end,
					replacement: relativeMarkdownPath(record.outputPath, target),
				})
			case attachment.ResolutionRemoved:
				if len(reference.uses) == 0 {
					return Snapshot{}, fmt.Errorf(
						"rewrite removed attachment %q referenced by %s: unsupported Markdown link form",
						reference.id, record.name,
					)
				}
				marker := fmt.Sprintf("Attachment unavailable: %s (%s)",
					code(resolution.Attachment.Filename.String()), code(reference.id.String()))
				destinationReplaced := false
				for _, usage := range reference.uses {
					edits = append(edits, markdownEdit{
						start: usage.start, end: usage.end, replacement: marker,
					})
					if usage.start <= reference.start && usage.end >= reference.end {
						destinationReplaced = true
					}
				}
				if !destinationReplaced {
					edits = append(edits, markdownEdit{
						start: reference.start, end: reference.end,
						replacement: "#attachment-unavailable-" + reference.id.String(),
					})
				}
			case attachment.ResolutionUnknown:
				return Snapshot{}, fmt.Errorf(
					"resolve attachment %q referenced by %s: attachment is unknown in board %q",
					reference.id, record.name, snapshot.BoardID,
				)
			default:
				return Snapshot{}, fmt.Errorf("resolve attachment %q referenced by %s: invalid resolution state",
					reference.id, record.name)
			}
		}
		slices.SortFunc(edits, func(a, b markdownEdit) int {
			return cmp.Compare(b.start, a.start)
		})
		lastStart := len(body)
		for _, edit := range edits {
			if edit.end > lastStart {
				return Snapshot{}, fmt.Errorf("rewrite %s: overlapping attachment links", record.name)
			}
			body = body[:edit.start] + edit.replacement + body[edit.end:]
			lastStart = edit.start
		}
		*record.body = body
	}
	return snapshot, nil
}

type markdownEdit struct {
	start       int
	end         int
	replacement string
}

func inlineAttachmentLinkBounds(source string, destinationStart, destinationEnd int) (int, int, bool) {
	before := destinationStart - 1
	if before >= 0 && source[before] == '<' {
		before--
	}
	for before >= 0 && (source[before] == ' ' || source[before] == '\t') {
		before--
	}
	if before < 1 || source[before] != '(' || source[before-1] != ']' {
		return 0, 0, false
	}
	labelEnd := before - 1
	depth := 1
	labelStart := -1
	for index := labelEnd - 1; index >= 0; index-- {
		if escapedAt(source, index) {
			continue
		}
		switch source[index] {
		case ']':
			depth++
		case '[':
			depth--
			if depth == 0 {
				labelStart = index
				index = -1
			}
		}
	}
	if labelStart < 0 {
		return 0, 0, false
	}
	start := labelStart
	if start > 0 && source[start-1] == '!' && !escapedAt(source, start-1) {
		start--
	}
	end := destinationEnd
	if end < len(source) && source[end] == '>' {
		end++
	}
	quote := byte(0)
	parentheses := 1
	for end < len(source) {
		character := source[end]
		if escapedAt(source, end) {
			end++
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			end++
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			end++
			continue
		}
		switch character {
		case '(':
			parentheses++
		case ')':
			parentheses--
			if parentheses == 0 {
				return start, end + 1, true
			}
		}
		end++
	}
	return 0, 0, false
}

func escapedAt(source string, index int) bool {
	backslashes := 0
	for index--; index >= 0 && source[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func attachmentMarkdownPath(value attachment.Attachment) string {
	return filepath.ToSlash(filepath.Join(
		"attachments", value.ID.String(), "files", url.PathEscape(value.Filename.String()),
	))
}
