// Package schema loads and validates author-supplied game definitions.
// Load accepts a game directory within an fs.FS and returns nil on any failure.
// Schema version 0 supports legacy omission rules; version 1 distinguishes
// omitted numeric fields from explicit zero. Data versions and save versions
// are independent. See https://nao1215.github.io/gozen/schema/ for every definition field.
//
// DB owns its decoded definitions and borrows its FS. Callers must keep the FS
// alive. A running game exclusively owns its DB: generated dungeon maps mutate
// definitions. Do not share a DB between concurrent games; load one per game.
// The model aliases expose authoring and inspection contracts while parsers and
// diagnostic implementation details remain internal. Definitions that no caller
// names are not aliased; see docs/decisions/0021-query-and-commands.md.
package schema
