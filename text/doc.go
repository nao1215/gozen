// Package text translates the framework's messages using a process-wide catalog.
// Use copies the supplied map, and T returns a translation or the original key.
// Both are synchronized; mutating the caller's map after Use is safe.
// The active language is shared by all games in the process. Initialize it at
// the host boundary and do not use Use to switch languages independently in
// concurrent games. A nil catalog restores the framework's original messages.
package text
