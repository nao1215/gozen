# ADR 0004: Boundary between the core and authoring support

Status: Partly changed. Date: 2026-10-18. Corresponds to: D4, D10. For the layout change see [ADR 0009](0009-single-module.md), and for the change to the CLI entry point see [ADR 0016](0016-single-cli.md).

## Context

The public library and authoring support, which has a different purpose, handle different features. Separating dependencies alone does not guarantee separating what is published.

## Decision

The library and the generic CLI provide only general-purpose features. Private work data and production information are not included, including in comments and identifiers.

Only `internal/promptgen`, `internal/mosaic`, and their subtrees (including the `command` package that implements `gozen prompt` and `gozen mosaic`) may handle features for production use that the author has permitted. Samples, default values, and tests are also original content that can be published. Detection categories and constraints are kept to what can be explained on its own as the tool's public specification. No ONNX model is bundled, and users specify one. `cmd/gozen` only dispatches and prints a neutral help listing, and stays generic ([ADR 0016](0016-single-cli.md)).

## Reasons

Building the authoring-support packages is not needed to use the library. The authoring tools are also kept in a form that can be used and checked without the secret settings of any particular work.

## Consequences

The layouts above and everything else get content checks by different criteria, and the condition of containing no specific information applies to all of them. The whole module is not treated as authoring support. The origin of models, samples, and generated files is also checked. Public examples are made specifically for that purpose.
