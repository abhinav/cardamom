// Package inspection owns the read model for showing one project namespace.
//
// The service resolves a project selector before reading project-scoped
// configuration and board inventory. Configuration resolution stops at the
// project layer, so callers do not need to select or fabricate a board.
package inspection
