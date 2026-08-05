package backup

import (
	"archive/zip"
	"errors"
	"fmt"
	"strings"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	backupv1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/backup/v1"
	"go.abhg.dev/cardamom/internal/project"
)

func validateManifest(
	manifest *backupv1.Manifest,
	files map[string]*zip.File,
) ([]project.Snapshot, []Board, []attachment.BlobDescriptor, error) {
	if manifest.GetVersion() != Version {
		return nil, nil, nil, fmt.Errorf(
			"unsupported archive version %d",
			manifest.GetVersion(),
		)
	}

	// Decode projects first because every board publication must resolve its
	// source namespace within the same manifest.
	projects := make([]project.Snapshot, 0, len(manifest.GetProjects()))
	projectIDs := make(map[project.ID]struct{}, len(manifest.GetProjects()))
	for _, encoded := range manifest.GetProjects() {
		snapshot, err := projectFromProto(encoded)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid archive project: %w", err)
		}
		if _, exists := projectIDs[snapshot.ID]; exists {
			return nil, nil, nil, fmt.Errorf(
				"archive project %q is duplicated",
				snapshot.ID,
			)
		}
		projectIDs[snapshot.ID] = struct{}{}
		projects = append(projects, snapshot)
	}

	expectedMembers := map[string]struct{}{ManifestMember: {}}
	boards := make([]Board, 0, len(manifest.GetBoards()))
	boardIDs := make(map[board.ID]struct{}, len(manifest.GetBoards()))
	for _, publication := range manifest.GetBoards() {
		boardID, err := board.NewID(publication.GetSourceBoardId())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid archive board: %w", err)
		}
		if _, exists := boardIDs[boardID]; exists {
			return nil, nil, nil, fmt.Errorf(
				"archive board %q is duplicated",
				boardID,
			)
		}
		projectID, err := project.NewID(publication.GetProjectId())
		if err != nil {
			return nil, nil, nil, fmt.Errorf(
				"archive board %q has invalid project: %w",
				boardID,
				err,
			)
		}
		if _, exists := projectIDs[projectID]; !exists {
			return nil, nil, nil, fmt.Errorf(
				"archive board %q references unknown project %q",
				boardID,
				projectID,
			)
		}
		if strings.TrimSpace(publication.GetSourceLineageId()) == "" {
			return nil, nil, nil, fmt.Errorf(
				"archive board %q source lineage is required",
				boardID,
			)
		}
		if publication.GetSnapshotVersion() != uint32(boardcopy.CopySnapshotVersion) {
			return nil, nil, nil, fmt.Errorf(
				"archive board %q uses unsupported snapshot version %d",
				boardID,
				publication.GetSnapshotVersion(),
			)
		}
		name, err := boardMember(publication.GetSnapshotDigest())
		if err != nil {
			return nil, nil, nil, fmt.Errorf(
				"archive board %q has invalid snapshot digest: %w",
				boardID,
				err,
			)
		}
		indexed, err := memberFromProto(publication.GetMember())
		if err != nil {
			return nil, nil, nil, fmt.Errorf(
				"archive board %q has invalid member: %w",
				boardID,
				err,
			)
		}
		if indexed.name != name {
			return nil, nil, nil, fmt.Errorf(
				"archive board %q uses noncanonical member %q",
				boardID,
				indexed.name,
			)
		}
		if err := registerMember(expectedMembers, files, indexed); err != nil {
			return nil, nil, nil, err
		}
		boardIDs[boardID] = struct{}{}
		boards = append(boards, Board{
			ProjectID: projectID, SourceLineageID: publication.GetSourceLineageId(),
			SourceBoardID: boardID, SourceRevision: publication.GetSourceRevision(),
			SnapshotVersion: int(publication.GetSnapshotVersion()),
			SnapshotDigest:  publication.GetSnapshotDigest(), member: indexed,
		})
	}

	blobs := make([]attachment.BlobDescriptor, 0, len(manifest.GetBlobs()))
	blobDigests := make(map[attachment.Digest]struct{}, len(manifest.GetBlobs()))
	for _, encoded := range manifest.GetBlobs() {
		digest, err := attachment.NewDigest(encoded.GetDigest())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid archive blob: %w", err)
		}
		descriptor := attachment.BlobDescriptor{
			Digest: digest, SizeBytes: encoded.GetSizeBytes(),
		}
		if err := descriptor.Validate(); err != nil {
			return nil, nil, nil, fmt.Errorf("invalid archive blob: %w", err)
		}
		if _, exists := blobDigests[digest]; exists {
			return nil, nil, nil, fmt.Errorf(
				"archive blob %s is duplicated",
				digest,
			)
		}
		name := blobMember(digest)
		if encoded.GetMember() != name {
			return nil, nil, nil, fmt.Errorf(
				"archive blob %s uses noncanonical member %q",
				digest,
				encoded.GetMember(),
			)
		}
		if err := registerMember(expectedMembers, files, archiveMember{
			name: name, sizeBytes: descriptor.SizeBytes,
			digest: descriptor.Digest.String(),
		}); err != nil {
			return nil, nil, nil, err
		}
		blobDigests[digest] = struct{}{}
		blobs = append(blobs, descriptor)
	}

	// The ZIP inventory is closed: every indexed member must exist and no
	// unindexed payload may accompany the portable backup.
	for name := range files {
		if _, expected := expectedMembers[name]; !expected {
			return nil, nil, nil, fmt.Errorf(
				"archive contains unexpected member %q",
				name,
			)
		}
	}
	return projects, boards, blobs, nil
}

func memberFromProto(encoded *backupv1.Member) (archiveMember, error) {
	if encoded == nil {
		return archiveMember{}, errors.New("descriptor is required")
	}
	digest, err := attachment.NewDigest(encoded.GetDigest())
	if err != nil {
		return archiveMember{}, err
	}
	return archiveMember{
		name: encoded.GetName(), sizeBytes: encoded.GetSizeBytes(),
		digest: digest.String(),
	}, nil
}

func registerMember(
	expected map[string]struct{},
	files map[string]*zip.File,
	descriptor archiveMember,
) error {
	if _, exists := expected[descriptor.name]; exists {
		return fmt.Errorf("archive member %q is indexed more than once", descriptor.name)
	}
	file, found := files[descriptor.name]
	if !found {
		return fmt.Errorf("archive is missing member %q", descriptor.name)
	}
	if file.UncompressedSize64 != descriptor.sizeBytes {
		return fmt.Errorf(
			"archive member %q size %d does not match expected size %d",
			descriptor.name,
			file.UncompressedSize64,
			descriptor.sizeBytes,
		)
	}
	expected[descriptor.name] = struct{}{}
	return nil
}
