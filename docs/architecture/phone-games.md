# Phone games

Phone hosting, approved packages and explicit installation are available in Gozen 0.23.0. Public catalog publication is tracked separately from the engine release.

## Hosting

A phone runs one standalone Gozen game inside another game without a VM or child process.

The implemented phone host provides portrait or landscape framing, transformed input and an exit control outside the child's viewport. It suspends its own gameplay and resumes when the child closes. Child resources and saves are isolated; only one level of embedding is supported. [ADR 0063](../decisions/0063-phone-game-packages.md) defines ownership and trust. The [authoring recipe](../examples/phone-games.md) includes YAML and actual captured examples.

## Registry and installation

Host authors choose app IDs, unlock flags, bundled or optional installation, and exact versions or bounded update ranges. Players explicitly download or update an allowed app. New catalog entries do not add apps to a host's declarations, and updates preserve earlier installations and saves.

The repository maintains reviewed release metadata, including required custom PNG icons and immutable package hashes. Signed catalog refresh authenticates new releases against the host's pinned public key. Archive and storage checks preserve existing installations when a download fails. See the [registry contract](../registry/README.md) for package limits, browser mirrors, upstream metadata, review and publication.

Authors can submit games through the [registration form](https://github.com/nao1215/gozen/issues/new?template=game-registration.yml). Submission starts review and does not publish a game automatically.
