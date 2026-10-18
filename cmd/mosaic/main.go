// Command mosaic processes user-selected images and models.
package main

import (
	"os"

	"github.com/nao1215/gozen/internal/mosaic/command"
)

func main() { os.Exit(command.Run(os.Args[1:], os.Stdout, os.Stderr)) }
