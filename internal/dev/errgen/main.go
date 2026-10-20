// Command errgen writes website/content/errors.md, the reference page of the
// diagnostic codes, from internal/diag (ADR 0024). Run it from the module root.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/nao1215/gozen/internal/diag"
)

func main() {
	if err := os.WriteFile("website/content/errors.md", diag.Markdown(), 0o600); err != nil {
		log.New(os.Stderr, "", 0).Print(fmt.Errorf("write the error codes page (run from the module root): %w", err))
		os.Exit(1)
	}
}
