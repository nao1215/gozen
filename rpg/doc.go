// Package rpg models game state, actors, inventory and versioned saves.
// Game hosts construct a State from validated data and an optional independent
// random source. A nil random source creates a new source for that state.
// State, Actor, their maps, and the associated DB are mutable and require
// exclusive ownership; methods are not safe for concurrent mutation.
//
// State.Save produces JSON without filesystem I/O. Parse reads a versioned save
// against the supplied DB without modifying the input bytes or consuming the
// supplied random source. ParseWithConfig explicitly opts into versionless JSON.
// LoadReport records repairs when definitions have changed. SaveFile preserves
// an old incompatible or repaired save before replacing it. The host selects
// all destinations; no legacy application directories are inspected.
package rpg
