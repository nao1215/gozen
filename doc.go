// Package gozen loads a game written in YAML and plays it without a screen: read the
// state with an expression (Session.Query) and change it with YAML commands
// (Session.Do). It is the one Go entry of Gozen (ADR 0021, "read with an expression,
// change with a command").
//
// The flow is Load → New (or Restore) → Do / Start-Next-Answer / Explore → Query → Save.
// A Game is immutable after Load and may be shared between goroutines. A Session belongs
// to one goroutine. The words of Do, Query, Request.Kind and Effect.Kind are those of the YAML
// schema (https://nao1215.github.io/gozen/schema/), versioned by schema_version; the Go API promises only their
// shape.
//
// The game-specific Go hooks are few: a Policy answers choices and battles, a Store
// keeps saves. A game declares its own commands and screens in game.yaml (verbs,
// screens); their Go side comes later.
package gozen
