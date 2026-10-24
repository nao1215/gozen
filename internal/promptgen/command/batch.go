package command

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/promptgen"
)

type sceneResult struct {
	ID     string           `json:"id"`
	Result promptgen.Result `json:"result"`
	Error  string           `json:"error,omitempty"`
}

// batchScenes preserves input order, including duplicates and failed scenes.
func (cfg Config) batchScenes(ctx context.Context, c *promptgen.Catalog, path string) error {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(cfg.Input)
	} else {
		b, err = os.ReadFile(path) //nolint:gosec // Explicit user-supplied input path; this operation does not write files.
	}
	if err != nil {
		return err
	}
	rows := []sceneResult{}
	succeeded, failed := 0, 0
	for line := range strings.SplitSeq(string(b), "\n") {
		if err := ctx.Err(); err != nil {
			return err
		}
		id := strings.TrimSpace(line)
		if id == "" {
			continue
		}
		row := sceneResult{ID: id}
		p, err := c.Scene(id)
		if err == nil {
			row.Result, err = c.Build(p)
		}
		status := "ok"
		if err != nil {
			row.Error = err.Error()
			failed++
			status = "failed"
		} else {
			succeeded++
		}
		switch cfg.Report.Format {
		case "json":
			rows = append(rows, row)
		case "jsonl":
			if err := cfg.Report.Item(cfg.Output, id, status, row); err != nil {
				return err
			}
		default:
			cfg.Output.Println(id + ":")
			if row.Error != "" {
				cfg.Output.Println(row.Error)
			} else if err := row.Result.WriteText(cfg.Output); err != nil {
				return err
			}
			if cfg.Output.Err != nil {
				return cfg.Output.Err
			}
		}
	}
	status := "ok"
	if failed > 0 {
		status = "failed"
	}
	switch cfg.Report.Format {
	case "json":
		err = cfg.Report.Result(cfg.Output, status, rows)
	case "jsonl":
		err = cfg.Report.Result(cfg.Output, status, struct {
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
		}{succeeded, failed})
	}
	if err != nil {
		return err
	}
	if failed > 0 {
		return cliio.ErrReported
	}
	return nil
}
