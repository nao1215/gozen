// Package docs holds the checks that keep README.md, the website and their
// pictures true to the code. Everything is in the tests:
//
//   - The pictures in docs/images are made from the sample games as listed in
//     docs/images/manifest.yaml. go test checks the manifest against the
//     references; GOZEN_DOCS_IMAGES=update remakes the pictures and
//     GOZEN_DOCS_IMAGES=check fails when a committed one is stale (both need a
//     display, see make docs-images and make docs-images-check).
//   - The code blocks of README.md and website/content carry a
//     <!-- verify: ... --> mark when they are not simply run. go test checks
//     the marks and loads the YAML examples; GOZEN_DOCS_STEPS=1 runs the shell
//     steps in temporary directories against this checkout (make docs-steps).
//   - GOZEN_SITE_DIR=website/public checks the built site for broken internal
//     links and pictures (make website-check).
//
// The rules are in docs/development/docs.md.
package docs
