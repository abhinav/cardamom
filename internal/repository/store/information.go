package store

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// Information reports store-owned identity and inventory values from one
// retained View snapshot.
type Information struct {
	// DatabaseSchemaVersion is the latest applied timestamped migration.
	DatabaseSchemaVersion int64

	// CodeSchemaVersion is the latest migration understood by this binary.
	CodeSchemaVersion int64

	// Revision is the latest committed logical store change.
	Revision int64
}

// ReadInformation reads store-owned information without ending the View
// snapshot.
func (v *View) ReadInformation(ctx context.Context) (Information, error) {
	must.NotBeNilf(v, "store information View is required")
	revision, err := query.New(v).StoreGetCanonicalRevision(ctx)
	if err != nil {
		return Information{}, fmt.Errorf("read store information: %w", err)
	}
	return Information{
		DatabaseSchemaVersion: v.databaseSchemaVersion,
		CodeSchemaVersion:     SchemaVersion(),
		Revision:              revision,
	}, nil
}
