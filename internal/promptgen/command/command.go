// Package command は gozen prompt の処理を実装する。設定と項目 ID から画像用文面を組み立てる。
// プロセスを終了せず、標準入出力の代わりに呼び出し側の writer を使う。
package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/promptgen"
)

const requirements = "Requirements: Go 1.26.4 or later to build or install gozen, and a user-supplied catalog YAML file. No cgo, model, ONNX Runtime, display, or generation server is needed. See " + clihelp.README

var page = clihelp.Page{
	Name:    "gozen prompt",
	Summary: "assemble image-generation text from a catalog",
	Usage: []string{
		"gozen prompt --config FILE [--scene ID | --plan FILE] [--<layer> ID ...]",
		"gozen prompt --config FILE --scenes FILE --format json",
		"gozen prompt lint --config FILE [--format json]",
	},
	About: "Reads a version 1 catalog of layers and items and assembles the positive and negative text " +
		"for one scene, one plan, or the defaults, with the selections of --<layer> on top. Duplicates are " +
		"merged, conflicts resolved, and the limits applied; the JSON output records where each word came " +
		"from. It only assembles text and never contacts a model or a generation service.\n\n" +
		"lint checks every item, dependency, and scene of the catalog. Errors fail; warnings alone succeed.",
	Examples: []clihelp.Example{
		{"assemble one scene as JSON", "gozen prompt --config catalog.yaml --scene landscape --format json"},
		{"assemble a plan that holds only item IDs", "gozen prompt --config catalog.yaml --plan plan.yaml"},
		{"choose an item of the setting layer by hand", "gozen prompt -c catalog.yaml --setting studio"},
		{"assemble many scenes, one ID per line, as JSON Lines", "gozen prompt -c catalog.yaml --scenes ids.txt --format json"},
		{"check the catalog", "gozen prompt lint --config catalog.yaml --format json"},
	},
	Groups: []clihelp.Group{
		{Title: "Input", Flags: []string{"config", "scene", "plan", "scenes"},
			Extra: [][2]string{{"    --<layer> ID", "select this item of a layer the catalog defines; repeat it to select several"}}},
		{Title: "Output", Flags: []string{"format"}},
		{Title: "General", Flags: []string{"help"}},
	},
	Notes: []string{
		"--scene and --plan do not go together. --scenes reads one scene ID per line, needs --format json, " +
			"and writes one JSON line per scene with id, result, and error; any failed scene exits 1 after " +
			"the others are written. Free text as an argument is not accepted. Exit status: 0 for success " +
			"and help, 1 when loading, building, or lint fails, 2 for invalid arguments.",
		requirements,
	},
}

// flags defines the fixed options of gozen prompt on a new flag set.
func flags() (fs *pflag.FlagSet, config, plan, scene, scenes, format *string) {
	fs = clihelp.NewFlagSet("gozen prompt")
	config = fs.StringP("config", "c", "", "read the catalog from this `FILE`; required")
	scene = fs.String("scene", "", "assemble the scene with this `ID` in the catalog")
	plan = fs.String("plan", "", "assemble the plan in this `FILE`, which holds only item IDs")
	scenes = fs.String("scenes", "", "assemble every scene ID in this `FILE`, one per line, as JSON Lines")
	format = fs.String("format", "text", "write the result as `FORMAT`: json or text")
	return
}

// Run は gozen prompt の引数を処理し、終了値を返す。成功と help は 0、引数の誤りは 2、
// それ以外の失敗は 1（ほかのサブコマンドと同じ）。失敗の内容と前提は diagnostics に書く。
// writer は呼び出し側が所有する。
func Run(args []string, out, diagnostics io.Writer) int {
	if err := run(args, out, diagnostics); err != nil {
		var u usageError
		if errors.As(err, &u) {
			fmt.Fprintln(diagnostics, err, "\nRun \"gozen prompt --help\" for usage.") //nolint:errcheck // The exit code already reports failure.
			return 2
		}
		fmt.Fprintln(diagnostics, err, "\n"+requirements) //nolint:errcheck // The exit code already reports failure.
		return 1
	}
	return 0
}

// usageError は引数の誤り（終了値 2）。
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func invalid(msg string) error { return usageError{msg} }

// configArg は層名をフラグとして登録する前に、設定の場所だけを取り出す。
func configArg(args []string) (string, error) {
	var name string
	set := func(v string) error {
		if name != "" {
			return invalid("--config may be given only once")
		}
		name = v
		return nil
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var err error
		switch {
		case arg == "--":
			i = len(args)
		case arg == "--config" || arg == "-c":
			if i+1 >= len(args) {
				return "", invalid(arg + " needs a file")
			}
			i++
			err = set(args[i])
		case strings.HasPrefix(arg, "--config="):
			err = set(strings.TrimPrefix(arg, "--config="))
		case strings.HasPrefix(arg, "-config"):
			return "", invalid("single-dash long option " + arg + " is not accepted; use --" + arg[1:])
		case strings.HasPrefix(arg, "-c") && !strings.HasPrefix(arg, "--"):
			err = set(strings.TrimPrefix(arg, "-c"))
		}
		if err != nil {
			return "", err
		}
	}
	if name == "" {
		return "", invalid("--config FILE is required; select with --scene ID or --<layer> ID and write with --format json|text")
	}
	return name, nil
}

func run(args []string, out, diagnostics io.Writer) error {
	if clihelp.HelpRequested(args) {
		fs, _, _, _, _, _ := flags()
		_, err := io.WriteString(out, page.String(fs))
		return err
	}
	lint := len(args) > 0 && args[0] == "lint"
	if lint {
		args = args[1:]
	}
	name, err := configArg(args)
	if err != nil {
		return err
	}
	c, err := promptgen.Load(os.DirFS(filepath.Dir(name)), filepath.Base(name))
	if err != nil {
		return err
	}
	fs, _, planFile, scene, scenes, format := flags()
	overrides := map[string][]string{}
	for _, id := range c.LayerIDs() {
		if fs.Lookup(id) != nil {
			return fmt.Errorf("layer %q has the name of a fixed option; rename the layer", id)
		}
		fs.Func(id, "select this item `ID` of the layer; repeat it to select several", func(value string) error {
			overrides[id] = append(overrides[id], value)
			return nil
		})
	}
	if _, err := clihelp.Parse(fs, args); err != nil {
		return invalid(err.Error())
	}
	if fs.NArg() != 0 {
		return invalid("positional arguments and free text are not accepted")
	}
	if *format != "json" && *format != "text" {
		return invalid("--format must be json or text")
	}
	if *planFile != "" && *scene != "" {
		return invalid("--plan and --scene cannot be used together")
	}
	if *scenes != "" {
		if lint || *scene != "" || *planFile != "" || len(overrides) != 0 || *format != "json" {
			return invalid("--scenes needs --format json and cannot be combined with lint, --scene, --plan or layer flags")
		}
		return batchScenes(c, *scenes, out)
	}
	if lint {
		if *scene != "" || *planFile != "" || len(overrides) != 0 {
			return invalid("lint checks every item and scene; it takes no selection")
		}
		ds := c.Lint()
		failed := false
		if *format == "json" {
			if err := json.NewEncoder(out).Encode(ds); err != nil {
				return err
			}
		} else {
			for _, d := range ds {
				if err := d.WriteText(out); err != nil {
					return err
				}
			}
		}
		for _, d := range ds {
			if d.Severity == "error" {
				failed = true
			}
		}
		if failed {
			return errors.New("lint found errors")
		}
		return nil
	}
	p := promptgen.Plan{Select: map[string][]string{}}
	if *planFile != "" {
		data, err := os.ReadFile(*planFile)
		if err != nil {
			return err
		}
		p, err = promptgen.ParsePlan(data)
		if err != nil {
			return err
		}
	}
	if *scene != "" {
		p, err = c.Scene(*scene)
		if err != nil {
			return err
		}
	}
	if p.Select == nil {
		p.Select = map[string][]string{}
	}
	for id, values := range overrides {
		p.Select[id] = values
	}
	result, err := c.Build(p)
	if err != nil {
		var validation *promptgen.ValidationError
		if errors.As(err, &validation) {
			if *format == "json" {
				if writeErr := json.NewEncoder(out).Encode(result); writeErr != nil {
					return writeErr
				}
			} else {
				for _, d := range validation.Diagnostics {
					if writeErr := d.WriteText(diagnostics); writeErr != nil {
						return writeErr
					}
				}
			}
		}
		return err
	}
	if *format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	return result.WriteText(out)
}
