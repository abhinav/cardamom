// Package backup owns the versioned portable archive shared by board backup
// writers and loaders.
//
// The archive stores complete semantic board-copy snapshots and deduplicated
// content-addressed attachment blobs. It does not expose Cardamom's physical
// database representation.
package backup
