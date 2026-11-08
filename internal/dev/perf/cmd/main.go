// The fixture generator prepares disposable inputs for bench/himorime.yaml.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/internal/dev/perf"
)

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/dev/perf/cmd OUTPUT_DIRECTORY [CASE]") //nolint:errcheck // Terminal error path still exits nonzero.
		os.Exit(2)
	}
	names := perf.Cases()
	if len(os.Args) == 3 {
		names = []string{os.Args[2]}
	}
	for _, name := range names {
		if err := perf.Generate(filepath.Join(os.Args[1], name), name, "e2e/atago/fixtures/game/assets/regular.ttf"); err != nil {
			fmt.Fprintln(os.Stderr, err) //nolint:errcheck // Terminal error path still exits nonzero.
			os.Exit(1)
		}
	}
}
