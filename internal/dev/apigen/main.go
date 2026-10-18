// Command apigen refreshes the reviewed inventory of declared public identifiers.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/nao1215/gozen/internal/dev/apilist"
)

func main() {
	if err := run(); err != nil {
		log.New(os.Stderr, "", 0).Print(err)
		os.Exit(1)
	}
}

func run() error {
	snapshot, err := apilist.Snapshot(os.DirFS("."), "github.com/nao1215/gozen")
	if err != nil {
		return err
	}
	if err := os.WriteFile("api/public.txt", []byte(snapshot), 0o600); err != nil {
		return fmt.Errorf("write API inventory (run from module root): %w", err)
	}
	return nil
}
