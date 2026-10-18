// Package savefile reads and writes save bytes without knowing game state.
// Game hosts choose a directory with Prepare(id, Config) or supply their own
// destination to Write. Empty Config.Root uses UserConfigDir/gozen; errors are
// returned instead of falling back to the working directory. No other
// application's directories are probed or migrated.
// Load borrows an fs.FS and reads a relative path. Write and WritePreserving
// report I/O, sync and close failures. Writers to the same destination must be
// serialized by the host. Rename is not a cross-platform power-loss guarantee.
package savefile
