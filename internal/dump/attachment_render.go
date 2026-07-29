package dump

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/goccy/go-yaml"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

// AttachmentReader supplies board-scoped metadata and verified immutable
// content for one dump operation.
type AttachmentReader interface {
	// ListAttachments returns one stable active attachment page.
	ListAttachments(context.Context, attachment.ListRequest) (attachment.Page, error)

	// ResolveAttachments batch-resolves Markdown attachment destinations.
	ResolveAttachments(context.Context, attachment.ResolveRequest) ([]attachment.Resolution, error)

	// OpenAttachmentContent returns a verified handle whose Close is owned by
	// the caller.
	OpenAttachmentContent(context.Context, attachment.OpenContentRequest) (attachment.OpenedContent, error)
}

func (s *Service) renderAttachments(
	ctx context.Context,
	snapshot Snapshot,
	records []markdownRecord,
) (Snapshot, []*GeneratedFile, attachmentResolutions, error) {
	boardID, err := board.NewID(snapshot.BoardID)
	if err != nil {
		return Snapshot{}, nil, nil, fmt.Errorf("parse dump board ID: %w", err)
	}

	referencedIDs, err := collectAttachmentReferences(records)
	if err != nil {
		return Snapshot{}, nil, nil, err
	}
	resolutions, err := s.resolveAttachmentReferences(ctx, boardID, referencedIDs)
	if err != nil {
		return Snapshot{}, nil, nil, err
	}
	resolutionsByID := make(attachmentResolutions, len(resolutions))
	byID := make(map[attachment.ID]attachment.Attachment)
	for _, resolution := range resolutions {
		resolutionsByID[resolution.AttachmentID] = resolution
		if resolution.State == attachment.ResolutionActive {
			byID[resolution.AttachmentID] = *resolution.Attachment
		}
	}
	if err := s.includeAssociatedAttachments(ctx, boardID, snapshot, byID); err != nil {
		return Snapshot{}, nil, nil, err
	}

	rewritten, err := rewriteAttachmentRecords(snapshot, records, resolutions)
	if err != nil {
		return Snapshot{}, nil, nil, err
	}
	values := make([]attachment.Attachment, 0, len(byID))
	for _, value := range byID {
		values = append(values, value)
	}
	slices.SortFunc(values, func(a, b attachment.Attachment) int {
		return cmp.Compare(a.ID.String(), b.ID.String())
	})
	files := make([]*GeneratedFile, 0, len(values)*2)
	for _, value := range values {
		generated, err := s.renderAttachmentFiles(ctx, boardID, value)
		if err != nil {
			return Snapshot{}, nil, nil, err
		}
		files = append(files, generated...)
	}
	return rewritten, files, resolutionsByID, nil
}

// Attachment resolutions index the one dump operation's board-scoped
// attachment outcomes for shorthand rewriting after collection.
type attachmentResolutions map[attachment.ID]attachment.Resolution

func cloneMarkdownSnapshot(snapshot Snapshot) Snapshot {
	result := snapshot
	result.Description = cloneMarkdown(snapshot.Description)
	result.Issues = slices.Clone(snapshot.Issues)
	for index := range result.Issues {
		result.Issues[index].Summary = cloneMarkdown(result.Issues[index].Summary)
		result.Issues[index].Details = cloneMarkdown(result.Issues[index].Details)
		result.Issues[index].State = cloneMarkdown(result.Issues[index].State)
		result.Issues[index].NextAction = cloneMarkdown(result.Issues[index].NextAction)
	}
	result.Results = slices.Clone(snapshot.Results)
	result.LogEntries = slices.Clone(snapshot.LogEntries)
	for index := range result.LogEntries {
		result.LogEntries[index].NextAction = cloneMarkdown(
			result.LogEntries[index].NextAction,
		)
	}
	return result
}

func cloneMarkdown(value *string) *string {
	if value == nil {
		return nil
	}
	return new(*value)
}

func (s *Service) resolveAttachmentReferences(
	ctx context.Context,
	boardID board.ID,
	ids []attachment.ID,
) ([]attachment.Resolution, error) {
	resolutions := make([]attachment.Resolution, 0, len(ids))
	for start := 0; start < len(ids); start += attachment.MaxResolveAttachmentIDs {
		end := min(start+attachment.MaxResolveAttachmentIDs, len(ids))
		batch, err := s.attachments.ResolveAttachments(ctx, attachment.ResolveRequest{
			BoardID: boardID, AttachmentIDs: ids[start:end],
		})
		if err != nil {
			return nil, fmt.Errorf("resolve dump attachments: %w", err)
		}
		if len(batch) != end-start {
			return nil, errors.New("attachment resolution count does not match request")
		}
		for index, resolution := range batch {
			if resolution.AttachmentID != ids[start+index] {
				return nil, errors.New("attachment resolution order does not match request")
			}
			switch resolution.State {
			case attachment.ResolutionUnknown:
				if resolution.Attachment != nil {
					return nil, fmt.Errorf("unknown attachment %q resolution includes metadata", resolution.AttachmentID)
				}
			case attachment.ResolutionActive, attachment.ResolutionRemoved:
				if resolution.Attachment == nil {
					return nil, fmt.Errorf("attachment %q resolution has no metadata", resolution.AttachmentID)
				}
				if err := validateDumpAttachment(boardID, *resolution.Attachment); err != nil {
					return nil, err
				}
				expectedLifecycle := attachment.LifecycleActive
				if resolution.State == attachment.ResolutionRemoved {
					expectedLifecycle = attachment.LifecycleRemoved
				}
				if resolution.Attachment.Lifecycle != expectedLifecycle {
					return nil, fmt.Errorf("attachment %q resolution state does not match lifecycle", resolution.AttachmentID)
				}
			default:
				return nil, fmt.Errorf("attachment %q resolution state is invalid", resolution.AttachmentID)
			}
		}
		resolutions = append(resolutions, batch...)
	}
	return resolutions, nil
}

func (s *Service) includeAssociatedAttachments(
	ctx context.Context,
	boardID board.ID,
	snapshot Snapshot,
	byID map[attachment.ID]attachment.Attachment,
) error {
	if snapshot.Selection.Mode == SelectionWholeBoard {
		return s.listDumpAttachments(ctx, attachment.ListRequest{BoardID: boardID}, byID)
	}
	for _, selected := range snapshot.Issues {
		issueID, err := issue.NewID(selected.ID)
		if err != nil {
			return fmt.Errorf("parse selected issue ID %q: %w", selected.ID, err)
		}
		if err := s.listDumpAttachments(ctx, attachment.ListRequest{
			BoardID: boardID, OriginIssueID: &issueID,
		}, byID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) listDumpAttachments(
	ctx context.Context,
	request attachment.ListRequest,
	byID map[attachment.ID]attachment.Attachment,
) error {
	for {
		page, err := s.attachments.ListAttachments(ctx, request)
		if err != nil {
			return fmt.Errorf("list dump attachments: %w", err)
		}
		for _, value := range page.Attachments {
			if value.Lifecycle != attachment.LifecycleActive {
				return fmt.Errorf("list dump attachments returned inactive attachment %q", value.ID)
			}
			if err := validateDumpAttachment(request.BoardID, value); err != nil {
				return err
			}
			if existing, ok := byID[value.ID]; ok && newAttachmentSidecar(existing) != newAttachmentSidecar(value) {
				return fmt.Errorf("attachment %q metadata changed during dump selection", value.ID)
			}
			byID[value.ID] = value
		}
		if page.NextPageToken == "" {
			return nil
		}
		if page.NextPageToken == request.PageToken {
			return errors.New("list dump attachments returned a repeated page token")
		}
		request.PageToken = page.NextPageToken
	}
}

func validateDumpAttachment(boardID board.ID, value attachment.Attachment) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("validate attachment %q for dump: %w", value.ID, err)
	}
	if value.Association.BoardID() != boardID {
		return fmt.Errorf("attachment %q belongs to board %q, not %q",
			value.ID, value.Association.BoardID(), boardID)
	}
	return nil
}

func (s *Service) renderAttachmentFiles(
	ctx context.Context,
	boardID board.ID,
	value attachment.Attachment,
) ([]*GeneratedFile, error) {
	metadata := newAttachmentSidecar(value)
	encoded, err := yaml.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode attachment %q sidecar: %w", value.ID, err)
	}
	sidecar, err := NewGeneratedFile(GeneratedFileConfig{
		Path:     attachmentSidecarPath(value.ID),
		Identity: "attachment:" + value.ID.String() + ":metadata",
		Size:     int64(len(encoded)),
		Open: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(encoded)), nil
		},
	})
	if err != nil {
		return nil, err
	}
	sidecar.attachmentRole = attachmentGeneratedSidecar
	sidecar.attachment = &metadata

	content, err := NewGeneratedFile(GeneratedFileConfig{
		Path:     attachmentContentPath(value),
		Identity: "attachment:" + value.ID.String() + ":content",
		Size:     int64(value.Blob.SizeBytes),
		Open: func() (io.ReadCloser, error) {
			opened, err := s.attachments.OpenAttachmentContent(ctx, attachment.OpenContentRequest{
				BoardID: boardID, AttachmentID: value.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("open dump attachment %q: %w", value.ID, err)
			}
			if newAttachmentSidecar(opened.Attachment) != metadata {
				return nil, errors.Join(
					fmt.Errorf("open dump attachment %q returned different metadata", value.ID),
					opened.Handle.Close(),
				)
			}
			return opened.Handle, nil
		},
	})
	if err != nil {
		return nil, err
	}
	content.attachmentRole = attachmentGeneratedContent
	content.attachment = &metadata
	return []*GeneratedFile{sidecar, content}, nil
}

func attachmentSidecarPath(id attachment.ID) string {
	return filepath.ToSlash(filepath.Join("attachments", id.String(), "metadata.yaml"))
}

func attachmentContentPath(value attachment.Attachment) string {
	return filepath.ToSlash(filepath.Join(
		"attachments", value.ID.String(), "files", value.Filename.String(),
	))
}
