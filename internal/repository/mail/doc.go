// Package mail persists ephemeral mailbox deliveries and topic subscriptions.
//
// Repository operations own their complete SQLite scope and return only domain
// values after the scope has completed. Mail writes do not participate in the
// canonical board revision sequence.
package mail
