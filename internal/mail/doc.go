// Package mail defines ephemeral messages, mailbox consumption,
// topic subscriptions, and polling behavior.
//
// Service owns store-scoped mail, mailbox, and subscription operations.
// Persistence implementations preserve fanout, consumption, and subscription
// changes as atomic operations.
package mail
