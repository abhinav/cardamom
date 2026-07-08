package attachment

import (
	"crypto/rand"
	"errors"
	"io"
	"time"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// Clock supplies upload expiry and attachment attribution timestamps.
type Clock interface {
	// Now returns the current wall-clock time.
	Now() time.Time
}

// Config defines repository-owned filesystem, time, and identity sources.
type Config struct {
	// StoreDirectory is the resolved directory containing private blob storage.
	StoreDirectory string // required

	// Clock supplies operation timestamps. Nil uses the system clock.
	Clock Clock

	// Entropy supplies upload and attachment identities. Nil uses crypto/rand.
	Entropy io.Reader
}

// Repository owns finite attachment metadata and upload operations.
type Repository struct {
	// store supplies the SQLite scopes that serialize blob and metadata work.
	store *store.Store // required

	// blobs owns staged and published bytes for the same physical store.
	blobs *blobStore // required

	// clock supplies whole-second durable lifecycle timestamps.
	clock Clock // required

	// entropy supplies upload and attachment identities.
	entropy io.Reader // required
}

var _ domainattachment.Repository = (*Repository)(nil)

// New binds attachment persistence and private bytes to one physical store.
func New(persistence *store.Store, cfg Config) (*Repository, error) {
	if persistence == nil {
		return nil, errors.New("attachment store is required")
	}
	blobs, err := newBlobStore(cfg.StoreDirectory)
	if err != nil {
		return nil, err
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	entropy := cfg.Entropy
	if entropy == nil {
		entropy = rand.Reader
	}
	return &Repository{
		store: persistence, blobs: blobs, clock: clock, entropy: entropy,
	}, nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

var _ Clock = systemClock{}
