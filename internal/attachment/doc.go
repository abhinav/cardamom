// Package attachment owns attachment identity, metadata, upload state,
// lifecycle, local availability, and finite attachment operations.
//
// Attachment metadata is scoped to one board. An optional issue association
// records where an attachment entered the board but does not restrict
// same-board use. Blob persistence, staging, hashing, filesystem paths, and
// transactions belong to later repository-owned attachment behavior.
package attachment
