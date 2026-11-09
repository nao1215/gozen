// Package session runs game rules without a screen for first-party integration tests.
// A loaded Game is shared read-only; each Session owns its state and randomness
// and is used from one goroutine. Authors use the CLI rather than importing it.
package session
