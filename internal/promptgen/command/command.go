// Package command は gozen prompt の処理を実装する。設定と項目 ID から画像用文面を組み立てる。
// プロセスを終了せず、標準入出力の代わりに呼び出し側の writer を使う。
package command

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nao1215/gozen/internal/promptgen"
)

const requirements = "Requirements: Go 1.26.4+ to build or install gozen. A user-supplied catalog YAML file. No cgo, model, ONNX Runtime, display, or generation server. See https://github.com/nao1215/gozen#gozen"

// Run は gozen prompt の引数を処理し、終了値を返す。成功と help は 0、引数の誤りは 2、
// それ以外の失敗は 1（ほかのサブコマンドと同じ）。失敗の内容と前提は diagnostics に書く。
// writer は呼び出し側が所有する。
func Run(args []string, out, diagnostics io.Writer) int {
	if err := run(args, out, diagnostics); err != nil {
		fmt.Fprintln(diagnostics, err, "\n"+requirements) //nolint:errcheck // The exit code already reports failure.
		var u usageError
		if errors.As(err, &u) {
			return 2
		}
		return 1
	}
	return 0
}

// usageError は引数の誤り（終了値 2）。
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func invalid(msg string) error { return usageError{msg} }

// 層名をフラグとして登録する前に設定の場所だけを取り出す。
func configArg(args []string) (string, error) {
	var name string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == "--config" || arg == "-config" {
			if i+1 >= len(args) {
				return "", invalid("--config needs a file")
			}
			i++
			if name != "" {
				return "", invalid("--config may be given only once")
			}
			name = args[i]
		} else if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "-config=") {
			if name != "" {
				return "", invalid("--config may be given only once")
			}
			_, name, _ = strings.Cut(arg, "=")
		}
	}
	if name == "" {
		return "", invalid("--config FILE is required; select with --scene ID or --<layer> ID and write with --format json|text")
	}
	return name, nil
}

func run(args []string, out, diagnostics io.Writer) error {
	for _, arg := range args {
		if arg == "--help" || arg == "-help" || arg == "-h" {
			_, err := fmt.Fprintln(out, "Usage: gozen prompt [lint] --config FILE [--scene ID | --plan FILE | --scenes FILE --format json] [--<layer> ID ...] [--format json|text]\n--scenes reads one scene ID per line and writes JSON Lines with id, result and optional error.\nLayers are defined in the catalog. Flags precede positional arguments.\n"+requirements)
			return err
		}
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
	fs := flag.NewFlagSet("gozen prompt", flag.ContinueOnError)
	fs.SetOutput(diagnostics)
	fs.String("config", name, "catalog file")
	planFile := fs.String("plan", "", "plan file holding only IDs")
	scene := fs.String("scene", "", "scene ID in the catalog")
	scenes := fs.String("scenes", "", "file of scene IDs, one per line; writes JSON Lines")
	format := fs.String("format", "text", "output format: json or text")
	overrides := map[string][]string{}
	for _, id := range c.LayerIDs() {
		fs.Func(id, "item ID to select; may be repeated", func(value string) error {
			overrides[id] = append(overrides[id], value)
			return nil
		})
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
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
