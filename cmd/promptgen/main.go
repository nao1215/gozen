// promptgen は設定と項目 ID から画像用文面を組み立てる。
package main

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

const requirements = "Requirements: Go 1.26.4+ to build or install. A user-supplied catalog YAML file. No cgo, model, ONNX Runtime, display, or generation server. See https://github.com/nao1215/gozen/blob/main/README.md#promptgen"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err, "\n"+requirements); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}

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
				return "", errors.New("--config のファイルが必要です")
			}
			i++
			if name != "" {
				return "", errors.New("--config は一度だけ指定してください")
			}
			name = args[i]
		} else if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "-config=") {
			if name != "" {
				return "", errors.New("--config は一度だけ指定してください")
			}
			_, name, _ = strings.Cut(arg, "=")
		}
	}
	if name == "" {
		return "", errors.New("--config FILE が必要です。--scene ID または設定の層ごとの --層 ID で選択し、--format json|text で出力します")
	}
	return name, nil
}

func run(args []string, out, diagnostics io.Writer) error {
	for _, arg := range args {
		if arg == "--help" || arg == "-help" || arg == "-h" {
			_, err := fmt.Fprintln(out, "Usage: promptgen [lint] --config FILE [--scene ID | --plan FILE] [--<layer> ID ...] [--format json|text]\nLayers are defined in the catalog. Flags precede positional arguments.\n"+requirements)
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
	fs := flag.NewFlagSet("promptgen", flag.ContinueOnError)
	fs.SetOutput(diagnostics)
	fs.String("config", name, "設定ファイル")
	planFile := fs.String("plan", "", "ID だけを持つ構図ファイル")
	scene := fs.String("scene", "", "設定内の構図 ID")
	format := fs.String("format", "text", "出力形式。json または text")
	overrides := map[string][]string{}
	for _, id := range c.LayerIDs() {
		fs.Func(id, "選択する項目 ID。複数回指定可", func(value string) error {
			overrides[id] = append(overrides[id], value)
			return nil
		})
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("位置引数や自由文は指定できません")
	}
	if *format != "json" && *format != "text" {
		return errors.New("--format は json または text です")
	}
	if *planFile != "" && *scene != "" {
		return errors.New("--plan と --scene は同時に指定できません")
	}
	if lint {
		if *scene != "" || *planFile != "" || len(overrides) != 0 {
			return errors.New("lint は全項目と全構図を検査するため選択を指定できません")
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
			return errors.New("lint でエラーが見つかりました")
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
