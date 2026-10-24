package session

import "errors"

// The errors of the package; test them with errors.Is.
var (
	// ErrNotFound is an ID or a path that the game does not have.
	ErrNotFound = errors.New("gozen: not found")
	// ErrInvalid is YAML or an expression that is not valid. The error also holds the
	// Diagnostics (errors.As).
	ErrInvalid = errors.New("gozen: invalid")
	// ErrIncompatibleSave is a save of another game, or of a format this version cannot read.
	ErrIncompatibleSave = errors.New("gozen: incompatible save")
)

// invalid is ErrInvalid with the diagnostics: its text is the diagnostics, one a line.
type invalid struct{ diags Diagnostics }

func (e invalid) Error() string   { return e.diags.Error() }
func (e invalid) Unwrap() []error { return []error{ErrInvalid, e.diags} }
