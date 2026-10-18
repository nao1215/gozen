# Security policy

Gozen is under development. Security fixes currently target the default branch; no released version has a separate maintenance commitment yet. Review [CHANGELOG.md](CHANGELOG.md) when updating.

Please report suspected vulnerabilities privately through [GitHub's vulnerability reporting form](https://github.com/nao1215/gozen/security/advisories/new). If private reporting is unavailable, open an issue asking for a private contact channel, without technical details. Do not put exploit details or private game data in a public issue.

Include the affected revision, Go version and OS, the entry point, expected and actual behavior, impact, and a minimal neutral reproduction. Do not send credentials, private assets, or model files. Coordinate public disclosure with the maintainer; response times are not guaranteed.

Game files, expressions, images and save files may be untrusted input. Game binaries and native libraries execute with the host user's permissions: use trusted binaries and ONNX Runtime libraries. Models and runtime libraries are user-supplied and are not bundled. See [production-tool setup](docs/development/production-tools.md) for requirements.

[Security CI](docs/development/ci.md) is configured to run govulncheck and CodeQL. Successful remote runs are not yet verified; the local vulnerability scan could not fetch its database. Follow the [release checklist](docs/development/release-checklist.md) to verify the release checks. These checks do not establish that all vulnerabilities are detected. Dependabot proposes Go module and GitHub Actions updates; updates still require review and verification.
