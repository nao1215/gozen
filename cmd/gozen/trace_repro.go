package main

import "github.com/nao1215/gozen/cmd/internal/repro"

func (c *command) traceRepro(path, target, out string) error {
	b, err := repro.Generate(repro.Config{Path: path, Target: target, Out: out})
	if err != nil {
		return err
	}
	if c.report.Machine() {
		return c.report.Result(c.out, "ok", b)
	}
	c.out.Println(b.Command)
	return nil
}
