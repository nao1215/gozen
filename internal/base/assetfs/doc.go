// Package assetfs overlays author-supplied assets and verifies approvals.
// New accepts two borrowed filesystems and a small Config; it never closes them.
// Only files already in the base are overlaid, and directory membership comes
// from the base. When the optional manifest is readable and valid, overlays
// require matching source hashes. Missing or invalid manifests disable this
// optional filter; use Gate for required approval enforcement. Forget clears cached manifest and hash observations after file changes.
// OverlayFS supports concurrent reads; callers synchronize mutations of its FS.
// Gate and Tree support host-controlled approval checks and distribution copies.
// Paths to Tree are selected by the host. No artwork or approval policy is bundled.
// YAML policy decoding is internal tooling, separate from this foundational FS.
package assetfs
