package store

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// LineageID returns the durable identity of the persistence history retained
// by View.
func (v *View) LineageID(ctx context.Context) (string, error) {
	lineageID, err := query.New(v).StoreGetLineageID(ctx)
	if err != nil {
		return "", fmt.Errorf("read store lineage: %w", err)
	}
	return lineageID, nil
}
