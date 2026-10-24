# Check implementation boundaries

Game authors use commands, YAML definitions and saves. Go implementation packages belong under `internal/`; executable entry points may live under `cmd/` or in maintainer examples.

```sh
make boundary
make arch
```

`TestApplicationPackagesAreInternal` in `internal/dev/hygiene/hygiene_test.go` parses production Go files without applying OS or build tags. It rejects non-main packages outside an internal directory. Test files may assemble layers. Generated build directories and vendor files are excluded.

`.go-arch-lint.yml` checks the permitted directions between internal layers. Moving a type does not justify an exception. Preserve error codes, CLI output, YAML loading and save restoration with contract tests and CLI E2E.

Public identifier inventories and their generator were removed with the public Go API. Updating an inventory is no longer an approval mechanism for exposing application code.
