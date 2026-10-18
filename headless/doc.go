// Package headless drives a validated game without display or audio devices.
// Hosts create a Driver with a DB and State, call Start exactly once for a new
// game, then Run, Do, GoTo or Walk. New alone does not run starting events.
// Restore resumes a loaded state without rerunning the start or arrival events.
// Driver borrows and mutates its DB, State and random source; use it serially.
// Run observes context cancellation at event boundaries. Battles are bounded
// by MaxTurns, and settling and walking have finite iteration limits.
//
// Explore copies the saved state to inspect choices and battle outcomes. It
// expands each distinct decision site once rather than enumerating every path
// combination. Config zero limits are 300 paths and 20000 steps per path;
// negative limits are errors in Path.Errs. It preserves the input State but can
// regenerate dungeon maps in the borrowed DB. Hooks run synchronously and should
// only mutate the supplied copied state when their contract permits it.
package headless
