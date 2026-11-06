package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/diag"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	lang "github.com/nao1215/gozen/internal/text"
)

func (c *command) migrateGame() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen migrate")
	c.reporting(f)
	out := f.String("out", "", "new `DIR` to write the migrated game to")
	page := clihelp.Page{
		Name: "gozen migrate",

		Summary:  lang.T("ゲームを最新の YAML 仕様へコピーして移行"),
		Usage:    []string{"gozen migrate <game-dir> --out NEW [OPTIONS]"},
		About:    lang.T("元のゲームを検証し、新規ディレクトリへコピーして YAML を短縮します。以前の既定値は保持します。元のゲームと既存の出力先は上書きしません。コピーを確認してから置き換えてください。"),
		Examples: []clihelp.Example{{lang.T("ゲームを移行する"), "gozen migrate game --out game-v11"}},
		Groups:   []clihelp.Group{{Title: "Output", Flags: []string{"out", "format"}}, general},
		Notes:    []string{noRequirements},
	}
	return clihelp.Operation{Page: page, Flags: f, Run: func(_ context.Context) error {
		if f.NArg() != 1 || *out == "" {
			return cliio.Invalid(diag.Operands, "%s", lang.T("migrate にはゲームのディレクトリと --out NEW が必要です"))
		}
		source := f.Arg(0)
		if err := data.UseWords(os.DirFS(source)); err != nil {
			return err
		}
		changes, err := data.Migrate(os.DirFS(source))
		if err != nil {
			return err
		}
		if err := cli.Publish(*out, []string{source}, func(stage string) error {
			if err := os.CopyFS(stage, os.DirFS(source)); err != nil {
				return err
			}
			for name, b := range changes {
				if err := os.WriteFile(filepath.Join(stage, filepath.FromSlash(name)), b, 0o600); err != nil {
					return err
				}
			}
			if _, err := data.Load(os.DirFS(stage)); err != nil {
				return fmt.Errorf(lang.T("移行結果の検証に失敗しました。出力は公開していません: %w"), err)
			}
			return nil
		}); err != nil {
			return err
		}
		if c.report.Machine() {
			return c.report.Result(c.out, "ok", struct {
				Directory string `json:"directory"`
				Schema    int    `json:"schema_version"`
				Changed   int    `json:"changed_files"`
			}{*out, data.CurrentSchemaVersion, len(changes)})
		}
		c.out.Printf(lang.T("%s に仕様版 %d のゲームを作成しました（変更した YAML: %d）。元のゲームは変更していません\n"), *out, data.CurrentSchemaVersion, len(changes))
		return nil
	}}
}
