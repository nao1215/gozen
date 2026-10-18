// Package world exposes available field actions and arrival rules.
// Hosts pass validated definitions and a state at a known location. Entries are
// in author-defined order and Commands converts a selected entry into events.
// Arrive and Auto mark once-only events in the state before returning commands.
// PickTroop consumes the state's random source. These operations mutate or borrow
// the state's objects and must be called serially with its other game operations.
// Dungeon implementation and shared field logic remain internal.
package world
