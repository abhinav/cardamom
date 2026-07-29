// Package attachment persists finite attachment metadata and upload
// operations.
//
// The package owns durable upload staging, content-addressed blob publication,
// integrity observations, and conservative collection. SQLite statements,
// filesystem paths, transaction scopes, and byte-ordering constraints remain
// private to the repository implementation.
package attachment
