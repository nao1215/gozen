// Package command implements prompt assembly without contacting a generation service.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/promptgen"
)

const requirements = "Requirements: a user-supplied catalog YAML file. No model, ONNX Runtime, display, or generation server is needed. See " + clihelp.README

var page = clihelp.Page{
	Name: "gozen prompt", Commands: [][2]string{{"lint", "Check every catalog item and scene"}}, Summary: "assemble image-generation text from a catalog",
	Usage: []string{"gozen prompt --config FILE [--scene ID | --plan FILE] [--select LAYER=ITEM ...]", "gozen prompt --config FILE --scenes FILE [--format text|json|jsonl]"},
	About: "Builds one scene, a plan, or the defaults. Repeated --select values replace the selected layer. Catalogs contain the text; command arguments contain only IDs.",
	Examples: []clihelp.Example{
		{"assemble one scene", "gozen prompt --config catalog.yaml --scene landscape --format json"},
		{"override a layer", "gozen prompt -c catalog.yaml --select setting=studio"},
		{"read scene IDs from stdin", "gozen prompt -c catalog.yaml --scenes - --format jsonl"},
	},
	Groups: []clihelp.Group{{Title: "Input", Flags: []string{"config", "scene", "plan", "scenes", "select"}}, {Title: "Output", Flags: []string{"format"}}, {Title: "General", Flags: []string{"help"}}},
	Notes:  []string{"--scenes reads one scene ID per line and cannot be combined with --scene, --plan or --select. JSON contains all results; JSONL emits an item per scene and a final summary. Failed scenes do not stop the batch. Paths are relative to the working directory; catalog rule paths are catalog-relative.", requirements},
}

// Config supplies invocation-owned streams and reporting to the root dispatcher.
// Input is used only when --scenes is -. The caller retains ownership of streams.
type Config struct {
	Input               io.Reader
	Output, Diagnostics *cliio.Output
	Report              *cliio.Report
	Lint                bool
}

// New registers fixed options without reading catalogs, files or environment.
func New(cfg Config) clihelp.Operation {
	name := "prompt"
	p := page
	if cfg.Lint {
		name += " lint"
		p = clihelp.Page{Name: "gozen prompt lint", Summary: "check every catalog item and scene", Examples: []clihelp.Example{{"check a catalog", "gozen prompt lint --config catalog.yaml"}}, Usage: []string{"gozen prompt lint --config FILE [--format text|json|jsonl]"}, Groups: []clihelp.Group{{Title: "Input", Flags: []string{"config"}}, {Title: "Output", Flags: []string{"format"}}, {Title: "General", Flags: []string{"help"}}}, Notes: []string{"Errors fail; warnings alone succeed.", requirements}}
	}
	f := clihelp.NewFlagSet("gozen " + name)
	catalog := f.StringP("config", "c", "", "read catalog `FILE`; required")
	cfg.Report.Command = name
	f.StringVar(&cfg.Report.Format, "format", "text", "result encoding: text, json, or jsonl")
	var scene, planFile, scenes string
	var selectors []string
	if !cfg.Lint {
		f.StringVar(&scene, "scene", "", "assemble the named scene `ID`")
		f.StringVar(&planFile, "plan", "", "read selections from `FILE`")
		f.StringVar(&scenes, "scenes", "", "read scene IDs from `FILE`; - reads stdin")
		f.StringArrayVar(&selectors, "select", nil, "select `LAYER=ITEM`; repeat to select several items")
	}
	return clihelp.Operation{Flags: f, Page: p, Run: func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := cfg.Report.Validate(); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return cliio.Invalid(diag.Operands, "positional arguments and free text are not accepted")
		}
		if *catalog == "" {
			return cliio.Invalid(diag.OptionRequired, "--config FILE is required")
		}
		if planFile != "" && scene != "" {
			return cliio.Invalid(diag.OptionConflict, "--plan and --scene cannot be used together")
		}
		if scenes != "" && (scene != "" || planFile != "" || len(selectors) > 0) {
			return cliio.Invalid(diag.OptionConflict, "--scenes cannot be combined with --scene, --plan or --select")
		}
		overrides := map[string][]string{}
		for _, value := range selectors {
			layer, item, ok := strings.Cut(value, "=")
			if !ok || layer == "" || item == "" {
				return cliio.Invalid(diag.BadValue, "--select requires LAYER=ITEM, not %q", value)
			}
			overrides[layer] = append(overrides[layer], item)
		}
		c, err := promptgen.Load(os.DirFS(filepath.Dir(*catalog)), filepath.Base(*catalog))
		if err != nil {
			return fmt.Errorf("%w\n%s", err, requirements)
		}
		if cfg.Lint {
			return cfg.lint(c)
		}
		if scenes != "" {
			return cfg.batchScenes(ctx, c, scenes)
		}
		p := promptgen.Plan{Select: map[string][]string{}}
		if planFile != "" {
			b, err := os.ReadFile(planFile) //nolint:gosec // Explicit user-supplied input path; this operation does not write files.
			if err != nil {
				return err
			}
			p, err = promptgen.ParsePlan(b)
			if err != nil {
				return err
			}
		}
		if scene != "" {
			p, err = c.Scene(scene)
			if err != nil {
				return cliio.Invalid(diag.UnknownName, "%s", err)
			}
		}
		for _, layer := range slices.Sorted(maps.Keys(overrides)) {
			for _, item := range overrides[layer] {
				if !c.HasItem(layer, item) {
					return cliio.Invalid(diag.UnknownName, "unknown selection %s=%s", layer, item)
				}
			}
		}
		if p.Select == nil {
			p.Select = map[string][]string{}
		}
		maps.Copy(p.Select, overrides)
		result, err := c.Build(p)
		if err != nil {
			validation, ok := errors.AsType[*promptgen.ValidationError](err)
			if !ok {
				return err
			}
			if cfg.Report.Machine() {
				if err := cfg.Report.Result(cfg.Output, "failed", result); err != nil {
					return err
				}
				return cliio.ErrReported
			}
			for _, d := range validation.Diagnostics {
				if err := d.WriteText(cfg.Diagnostics); err != nil {
					return err
				}
			}
			return cliio.ErrReported
		}
		if cfg.Report.Machine() {
			return cfg.Report.Result(cfg.Output, "ok", result)
		}
		return result.WriteText(cfg.Output)
	}}
}

func (cfg Config) lint(c *promptgen.Catalog) error {
	ds := c.Lint()
	status := "ok"
	for _, d := range ds {
		if d.Severity == "error" {
			status = "failed"
		}
	}
	if cfg.Report.Machine() {
		if err := cfg.Report.Result(cfg.Output, status, ds); err != nil {
			return err
		}
	} else {
		for _, d := range ds {
			if err := d.WriteText(cfg.Output); err != nil {
				return err
			}
		}
	}
	if status == "failed" {
		return cliio.ErrReported
	}
	return nil
}
