# ADR 0005: New examples and assets

Status: Accepted. Date: 2026-10-18. Corresponds to: D5.

## Context

Library usage examples and screen introductions need publishable assets and reproducible operations. We prepare dedicated examples whose rights and origin can be explained.

## Decision

`examples/` holds a small, general-purpose original game that exercises movement, conversation, events, battle, and saving. Assets are shapes and pixel art drawn in code, or CC0 and similar assets with the origin and license recorded. Images, sound, scenarios, and introduction images are prepared to the same standard.

The introduction GIFs and screenshots are captured by running that example under Xvfb on Linux. The CLI demos also get dedicated neutral assets: `gozen mosaic` uses a synthetic image and `gozen prompt` uses a newly written neutral catalog.

## Reasons

It makes the examples real users of the public API, and makes the checks of rights and content reproducible. Screenshots do not stand in for a successful drawing check.

## Consequences

The origin, license, and generation method of assets are kept in the example's documents. The sources and licenses of third-party assets are collected in `ASSETS.md`.

## Addendum (2026-10-19)

The Wayfinder art was changed from shapes drawn in code to generated images. The license of the art is separate from that of the code: the code is MIT, and the example art is CC BY 4.0 (`examples/play/game/assets/LICENSE`). How the art was made is not recorded in published material.
