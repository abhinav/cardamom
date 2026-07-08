package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

type attachmentAddCommand struct {
	Source string `arg:"" name:"source" help:"File path, or - to read standard input."`
	Name   string `name:"name" placeholder:"NAME" help:"Portable attachment filename. Required for standard input; otherwise defaults to the source basename."`
	Issue  string `name:"issue" predictor:"issues" placeholder:"ISSUE" help:"Originating issue ID. The association is organizational and does not restrict same-board references."`
}

func (c *attachmentAddCommand) referencedIssueIDs() []string {
	if c.Issue == "" {
		return nil
	}
	return []string{c.Issue}
}

// Help describes bounded upload input and the emitted Markdown references.
func (*attachmentAddCommand) Help() string {
	return `Upload a file or standard input in bounded chunks and publish it as one
board attachment. Use - to read standard input; --name is required in that
form. On success, the command prints the attachment ID, canonical Markdown
link, and canonical Markdown image reference.`
}

func (c *attachmentAddCommand) Run(
	invocation *Invocation,
	selected *board.State,
	service *attachment.Service,
) error {
	filenameValue := c.Name
	if c.Source == "-" {
		if filenameValue == "" {
			return UsageErrorf("attachment add: --name is required when reading standard input")
		}
	} else if filenameValue == "" {
		filenameValue = filepath.Base(c.Source)
	}
	filename, err := attachment.NewFilename(filenameValue)
	if err != nil {
		return UsageErrorf("attachment add: --name: %s", err)
	}

	association, err := attachment.NewBoardAssociation(selected.ID())
	if err != nil {
		return err
	}
	if c.Issue != "" {
		originIssueID, parseErr := issue.NewID(c.Issue)
		if parseErr != nil {
			return UsageErrorf("attachment add: --issue: %s", parseErr)
		}
		association, err = attachment.NewIssueAssociation(selected.ID(), originIssueID)
		if err != nil {
			return err
		}
	}

	reader := invocation.Stdin
	var expectedSize *uint64
	var source *os.File
	if c.Source != "-" {
		file, openErr := os.Open(c.Source)
		if openErr != nil {
			return fmt.Errorf("open attachment source %q: %w", c.Source, openErr)
		}
		source = file
		reader = file

		info, statErr := file.Stat()
		if statErr != nil {
			return errors.Join(
				fmt.Errorf("inspect attachment source %q: %w", c.Source, statErr),
				file.Close(),
			)
		}
		if info.Mode().IsRegular() {
			size := uint64(info.Size())
			expectedSize = &size
		}
	}

	created, err := service.AddAttachment(invocation.Context, attachment.AddRequest{
		Invocation:        attachment.NewInvocation(invocation.Actor),
		Association:       association,
		Filename:          filename,
		ExpectedSizeBytes: expectedSize,
		Content:           reader,
	})
	if source != nil {
		err = errors.Join(err, source.Close())
	}
	if err != nil {
		return err
	}
	return writeAddedAttachment(invocation.Output, created)
}

type attachmentAddOutput struct {
	ID            string `json:"id"`
	MarkdownLink  string `json:"markdown_link"`
	MarkdownImage string `json:"markdown_image"`
}

func writeAddedAttachment(output *Output, value attachment.Attachment) error {
	label := escapeMarkdownLabel(value.Filename.String())
	target := "attachment:" + value.ID.String()
	result := attachmentAddOutput{
		ID:            value.ID.String(),
		MarkdownLink:  fmt.Sprintf("[%s](%s)", label, target),
		MarkdownImage: fmt.Sprintf("![%s](%s)", label, target),
	}
	if output.JSON() {
		return output.WriteJSON(result)
	}
	return output.WriteString(
		result.ID + "\n" + result.MarkdownLink + "\n" + result.MarkdownImage + "\n",
	)
}

func escapeMarkdownLabel(value string) string {
	return strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(value)
}
