# Benchmarks

gozen is measured the way an author or a CI job runs it, one process per call
from start to exit, with [himorime](https://github.com/nao1215/himorime).
himorime builds gozen, runs each command in interleaved rounds, and reports
latency, CPU time and peak RSS.

```console
$ go install github.com/nao1215/himorime@latest
$ make bench            # himorime run bench
$ make bench-compare    # himorime compare --against main bench
```

## Regression suite: himorime.yaml

`make bench-compare` checks main out into a temporary Git worktree, builds it
and your working tree, and measures both in the same rounds, so a background
job slows both revisions instead of one. `BASE=<revision> make bench-compare`
compares against another revision. `himorime run --filter 'sim 50k battles'
bench` measures one benchmark.

On a pull request, `.github/workflows/bench.yml` runs `himorime ci bench` the
same way, with the base of the pull request as the base, on one runner. The job
fails when a command is slower, uses more CPU time or more memory than the base
beyond the tolerance in `himorime.yaml` with 95% confidence. A difference too
close to call is reported as inconclusive and does not fail the job. While the
repository is private the job runs only when started by hand, to save the
Actions budget (docs/development/ci.md).

The input is the neutral game of the CLI contract tests,
`e2e/atago/fixtures/game`, read from the working tree by both revisions.

| Benchmark | Commands | Measures |
|-----------|----------|----------|
| `version` | `gozen version` | starting gozen |
| `check 1 game` | `gozen check`, `gozen check --only structure` | checking a small game with and without the font and text layout |
| `check error` | `gozen check` on a missing directory | the failure path, exit 1 |
| `sim 200 battles` | `gozen sim battle ... -n 200` | the default battle simulation, where start-up dominates |
| `sim 50k battles` | `gozen sim battle ... -n 50000` | the battle rules themselves |
| `book 1 game` | `gozen book` | writing a small game as Markdown |

Not measured yet: `check` and `book` on a large game. There is no generator
for a large neutral game; add one as `bench/gen.sh` before adding those rows.
The graphical commands (`shots`) and `mosaic` with ONNX inference need a
display or user-supplied models and are not measured.

Numbers from different machines are not comparable; compare revisions on one
machine, as `make bench-compare` and CI do.
