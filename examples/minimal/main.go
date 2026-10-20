// Command minimal loads a tiny embedded game without a graphics display.
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"

	"github.com/nao1215/gozen"
)

//go:embed game.yaml data/*.yaml
var files embed.FS

func main() {
	if err := run(); err != nil {
		log.SetFlags(0)
		log.Fatal(err)
	}
}

func run() error {
	g, err := gozen.Load(files, ".")
	if err != nil {
		return err
	}
	s, err := gozen.New(g, gozen.Config{Seed: 1})
	if err != nil {
		return err
	}
	if err := s.Begin(context.Background()); err != nil {
		return err
	}
	var location string
	var gold int
	if err := errors.Join(s.Query("location", &location), s.Query("gold", &gold)); err != nil {
		return err
	}
	_, err = fmt.Printf("location=%s gold=%d\n", location, gold)
	return err
}
