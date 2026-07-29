package attachment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
)

// collectExpiredStaging removes bytes for upload sessions the repository has
// already selected as expired. Duplicate and missing sessions are ignored.
func (s *blobStore) collectExpiredStaging(
	uploadIDs []domainattachment.UploadID,
	dryRun bool,
) (domainattachment.CollectionSummary, error) {
	paths := make(map[string]struct{}, len(uploadIDs))
	for _, uploadID := range uploadIDs {
		if _, err := domainattachment.NewUploadID(uploadID.String()); err != nil {
			return domainattachment.CollectionSummary{}, err
		}
		paths[s.stagingPath(uploadID)] = struct{}{}
	}

	candidates := make([]collectionCandidate, 0, len(paths))
	for path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return domainattachment.CollectionSummary{}, fmt.Errorf("inspect expired attachment staging: %w", err)
		}
		if !info.Mode().IsRegular() {
			return domainattachment.CollectionSummary{}, errors.New("expired attachment staging is not a regular file")
		}
		candidates = append(candidates, collectionCandidate{path: path, sizeBytes: uint64(info.Size())})
	}
	return collectCandidates(candidates, dryRun)
}

// collectOrphanBlobs removes canonical content whose digest is absent from the
// complete retained descriptor set supplied by the repository.
func (s *blobStore) collectOrphanBlobs(
	retained []domainattachment.BlobDescriptor,
	dryRun bool,
) (domainattachment.CollectionSummary, error) {
	retainedDigests := make(map[domainattachment.Digest]struct{}, len(retained))
	for _, descriptor := range retained {
		if err := descriptor.Validate(); err != nil {
			return domainattachment.CollectionSummary{}, err
		}
		retainedDigests[descriptor.Digest] = struct{}{}
	}

	entries, err := os.ReadDir(s.contentDir())
	if errors.Is(err, os.ErrNotExist) {
		return domainattachment.CollectionSummary{Count: 0, Bytes: 0}, nil
	}
	if err != nil {
		return domainattachment.CollectionSummary{}, fmt.Errorf("list attachment blobs: %w", err)
	}
	candidates := make([]collectionCandidate, 0, len(entries))
	for _, entry := range entries {
		digest, err := domainattachment.NewDigest("sha256:" + entry.Name())
		if err != nil {
			continue
		}
		if _, ok := retainedDigests[digest]; ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return domainattachment.CollectionSummary{}, fmt.Errorf("inspect orphan attachment blob: %w", err)
		}
		if !info.Mode().IsRegular() {
			return domainattachment.CollectionSummary{}, errors.New("orphan attachment blob is not a regular file")
		}
		candidates = append(candidates, collectionCandidate{
			path:      filepath.Join(s.contentDir(), entry.Name()),
			sizeBytes: uint64(info.Size()),
		})
	}
	return collectCandidates(candidates, dryRun)
}

type collectionCandidate struct {
	path      string
	sizeBytes uint64
}

func collectCandidates(
	candidates []collectionCandidate,
	dryRun bool,
) (domainattachment.CollectionSummary, error) {
	var out domainattachment.CollectionSummary
	for _, candidate := range candidates {
		if !dryRun {
			if err := os.Remove(candidate.path); err != nil {
				return out, fmt.Errorf("remove collected attachment content: %w", err)
			}
		}
		out.Count++
		out.Bytes += candidate.sizeBytes
	}
	return out, nil
}
