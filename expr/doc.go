// Package expr parses and evaluates game expressions without I/O.
// Game authors use Parse; hosts supply Env and Object implementations for names
// and fields, and Func values for extensions. Evaluation can call those supplied
// functions, so their side effects and concurrency are the host's responsibility.
// Parse and evaluation report invalid input as errors. A nil Expr represents
// an omitted condition (true) or numeric formula (zero); see Bool and Number.
// Parsed expressions may be read concurrently if their environments are safe.
// UnmarshalText mutates its receiver and must not race with evaluation.
// Ref, ValueType and Range support validation and conservative static analysis
// without consuming random inputs or executing extension functions.
package expr
