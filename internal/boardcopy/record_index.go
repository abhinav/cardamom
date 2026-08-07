package boardcopy

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.abhg.dev/cardamom/internal/attachment"
)

// RecordIndex is the compact publication metadata needed before record import.
//
// Identity slices retain canonical source order. Blobs contains one descriptor
// per digest in digest order.
type RecordIndex struct {
	// Header identifies the retained source view and board policy.
	Header RecordHeader

	// Counts reports the completed canonical section sizes.
	Counts RecordCounts

	// Digest identifies the portable semantics in versioned field framing.
	Digest string

	// IssueIDs contains every source issue identity in canonical order.
	IssueIDs []string

	// LogEntryIDs contains every source Log identity in canonical order.
	LogEntryIDs []string

	// AttachmentIDs contains every preserved attachment identity.
	AttachmentIDs []string

	// Blobs contains each referenced blob descriptor once in digest order.
	Blobs []attachment.BlobDescriptor

	// RemovedAttachments reports whether publication needs removal revisions.
	RemovedAttachments bool
}

// RecordIndexer validates and indexes one canonical record sequence.
type RecordIndexer struct {
	digest        *semanticDigest
	header        RecordHeader
	counts        RecordCounts
	last          Record
	issueIDs      []string
	logIDs        []string
	attachmentIDs []string
	blobs         map[attachment.Digest]attachment.BlobDescriptor
	removed       bool
	trailerSeen   bool
}

// NewRecordIndexer constructs a semantic record indexer.
func NewRecordIndexer() *RecordIndexer {
	return &RecordIndexer{
		digest: newSemanticDigest(),
		blobs:  make(map[attachment.Digest]attachment.BlobDescriptor),
	}
}

// Add validates and indexes the next canonical record.
func (i *RecordIndexer) Add(record Record) error {
	if record == nil {
		return errors.New("board record is required")
	}
	recordType := RecordTypeOf(record)
	if recordType == RecordTypeUnknown {
		return fmt.Errorf("unsupported board record type %T", record)
	}
	if i.trailerSeen {
		return errors.New("board record follows trailer")
	}
	if i.last == nil {
		if recordType != RecordTypeHeader {
			return errors.New("board record header is required first")
		}
	} else {
		lastType := RecordTypeOf(i.last)
		if recordType < lastType {
			return fmt.Errorf(
				"board record type %d follows type %d",
				recordType,
				lastType,
			)
		}
		if recordType == lastType && i.last.compareRecordKey(record) >= 0 {
			return fmt.Errorf("board record type %d is out of order", recordType)
		}
	}

	if err := i.index(record); err != nil {
		return err
	}
	i.digest.add(record)
	i.last = record
	return nil
}

func (i *RecordIndexer) index(record Record) error {
	if err := record.addToRecordCounts(&i.counts); err != nil {
		return err
	}
	switch value := record.(type) {
	case RecordHeader:
		if value.Version != CopySnapshotVersion {
			return fmt.Errorf("unsupported board record version %d", value.Version)
		}
		if strings.TrimSpace(value.SourceLineageID) == "" {
			return errors.New("board record source lineage is required")
		}
		if value.SourceRevision < 0 {
			return errors.New("board record source revision cannot be negative")
		}
		if strings.TrimSpace(value.Board.ID) == "" {
			return errors.New("board record source board is required")
		}
		i.header = value
	case CopyIssue:
		i.issueIDs = append(i.issueIDs, value.ID)
	case CopyLogEntry:
		i.logIDs = append(i.logIDs, value.ID)
	case CopyAttachment:
		i.attachmentIDs = append(i.attachmentIDs, value.ID)
		if prior, found := i.blobs[value.Blob.Digest]; found && prior != value.Blob {
			return fmt.Errorf(
				"board attachments disagree on blob %s",
				value.Blob.Digest,
			)
		}
		i.blobs[value.Blob.Digest] = value.Blob
		i.removed = i.removed || value.Lifecycle == attachment.LifecycleRemoved.String()
	case RecordTrailer:
		i.trailerSeen = true
	}
	return nil
}

// Finish returns the completed index after the required trailer is observed.
func (i *RecordIndexer) Finish() (RecordIndex, error) {
	if !i.trailerSeen {
		return RecordIndex{}, errors.New("board record trailer is required")
	}
	blobs := make([]attachment.BlobDescriptor, 0, len(i.blobs))
	for _, descriptor := range i.blobs {
		blobs = append(blobs, descriptor)
	}
	slices.SortFunc(blobs, func(left, right attachment.BlobDescriptor) int {
		return strings.Compare(left.Digest.String(), right.Digest.String())
	})
	return RecordIndex{
		Header:             i.header,
		Counts:             i.counts,
		Digest:             i.digest.sum(),
		IssueIDs:           i.issueIDs,
		LogEntryIDs:        i.logIDs,
		AttachmentIDs:      i.attachmentIDs,
		Blobs:              blobs,
		RemovedAttachments: i.removed,
	}, nil
}

// IndexRecords consumes and indexes one complete canonical record sequence.
func IndexRecords(
	records RecordSequence,
) (RecordIndex, error) {
	indexer := NewRecordIndexer()
	for record, err := range records {
		if err != nil {
			return RecordIndex{}, err
		}
		if err := indexer.Add(record); err != nil {
			return RecordIndex{}, err
		}
	}
	return indexer.Finish()
}
