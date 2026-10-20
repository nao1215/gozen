# Checking public identifiers

`internal/dev/apilist/apilist.go:1` lists the declared public identifiers with the Go standard parser and go/ast, without loading dependencies. It adds no new external dependency. `api/api_test.go:1` compares the current result with `api/public.txt` and `api/schema.txt`. Identifiers of the `schema` package (the Go types of the YAML definitions, versioned by `schema_version`) go to `api/schema.txt`; all other public API goes to `api/public.txt` ([ADR 0021](../decisions/0021-query-and-commands.md)).

The scope is importable packages and their public types, functions, methods, variables and constants, plus public struct fields, interface methods and embedded names of public types. Fields and methods of unexported types are not listed: a caller who reaches them through a return value cannot use them by a type name ([ADR 0021](../decisions/0021-query-and-commands.md)). These are excluded: `internal/`, `cmd/`, `examples/`, `testdata/`, vendor, hidden directories, directories with the `_` prefix that Go ignores, the check's own `api/`, test files, main packages and nested modules. `x/` is listed, but is not covered by the compatibility promise.

OS and build tag constraints are not evaluated; the check takes the union of the declarations of every Go file. Declarations with the same name are merged, the order is fixed and the file is saved with LF. The test also normalizes lists that became CRLF in a Windows checkout. If a source cannot be parsed, the error includes the file name.

```sh
go test ./api -count=1
go run ./internal/dev/apigen
go test ./api -count=1
```

Run the update command at the root of the main module. After the first check shows what was added or removed, confirm the consumer, purpose and contract, then update and review the diff. The T2 list contains the declarations of the core API and the members reachable through public aliases.

A type alias inside the same module is followed to its declaration, and the public fields and methods declared directly there are listed under the public name. This includes a target in `internal/`. It does not list the internal type itself as an independent public API. Aliases of external modules and promoted methods are not expanded.

This check makes added and removed names visible. It does not guarantee signatures, constant values, type compatibility, methods promoted through unexported types or embedding, the meaning of behavior, or that every build tag combination works. For API compatibility, use godoc, examples, contract tests and review together. Production tooling stays in `cmd/` and `internal/` and adds no public API.
