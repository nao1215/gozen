package command

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/nao1215/gozen/internal/promptgen"
)

// batchScenes builds each composition sharing the settings. It also prints individual failures and
// reports them with a non-zero exit at the end.
func batchScenes(c *promptgen.Catalog, path string, out io.Writer) error {
	b, err := os.ReadFile(path) //nolint:gosec // Explicit user-supplied scene-list file; the command does not write to it.
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	failed := false
	for line := range strings.SplitSeq(string(b), "\n") {
		id := strings.TrimSpace(line)
		if id == "" {
			continue
		}
		row := struct {
			ID     string           `json:"id"`
			Result promptgen.Result `json:"result"`
			Error  string           `json:"error,omitempty"`
		}{ID: id}
		plan, err := c.Scene(id)
		if err == nil {
			row.Result, err = c.Build(plan)
		}
		if err != nil {
			row.Error, failed = err.Error(), true
		}
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	if failed {
		return errors.New("one or more scenes failed")
	}
	return nil
}
