# ADR 0003: Published content and check data

Status: Accepted. Date: 2026-10-18. Corresponds to: D3.

## Context

For a public library, it is not enough to separate code dependencies. Comments, default values, tests, and generated files must also be suitable for publication.

## Decision

Fixtures and goldens are built from original, general-purpose examples, and the assembly of test data lives in `internal/testgame/`. Settings specific to a work and private production records are not used as defaults.

Leak checks inside Gozen cover general patterns such as absolute paths of personal environments and private host names. Matching against specific content is done separately, with a private list kept outside the repository.

## Reasons

It prevents the check data and check lists themselves from exposing private information, and keeps the checks reproducible without any particular production environment.

## Consequences

Besides word search, meaning and context are read, and strings in comments, settings, tests, samples, assets, embedded files, generated files, and binaries are also checked. The steps before publication are collected in the [release checklist](../development/release-checklist.md).
