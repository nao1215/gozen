package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"

	"github.com/nao1215/gozen/assetfs"
	"github.com/nao1215/gozen/cmd/internal/trial"
)

func (c *command) trialCmd(args []string) (err error) {
	fl := clihelp.NewFlagSet("gozen trial")
	conf := fl.String("config", "trial.yaml", "the range settings `FILE`, relative to the game directory and separated by /")
	overlay := fl.String("overlay", "", "take replaced assets from this `DIR` (needs --inspect)")
	inspect := fl.String("inspect", "", "the inspection records `FILE` (JSON Lines) from production; needed with --overlay")
	list := fl.String("list", "", "write the list of what was kept and dropped to this new `FILE`")
	dry := fl.BoolP("dry-run", "n", false, "write nothing; print only how much would be kept")
	if err := c.parse(fl, args); err != nil {
		return err
	}
	if fl.NArg() != 2 && (!*dry || fl.NArg() != 1) {
		return cli.Invalid("expected <game-dir> <out-dir>")
	}
	src := fl.Arg(0)
	if !cli.Relative(*conf) {
		return cli.Invalid("--config must be a portable relative FS path")
	}
	if (*overlay == "") != (*inspect == "") {
		return cli.Invalid("--overlay and --inspect must be supplied together")
	}
	if !*dry {
		if err := cli.Destination(fl.Arg(1), src, *overlay); err != nil {
			return err
		}
	}
	cfg, err := trial.Load(os.DirFS(src), *conf)
	if err != nil {
		return err
	}
	res, err := trial.Build(os.DirFS(src), cfg)
	if err != nil {
		return err
	}
	if err := res.Check(); err != nil {
		return fmt.Errorf("体験版の写しがエンジンの検査を通らない: %w", err)
	}
	c.summary(res)
	if !*dry && *list != "" {
		if err := cli.Destination(*list, src, *overlay, fl.Arg(1)); err != nil {
			return err
		}
		if err := writeList(*list, res); err != nil {
			return err
		}
	}
	if *dry {
		return nil
	}
	out := fl.Arg(1)
	if *overlay == "" {
		return cli.Publish(out, []string{src}, func(stage string) error { return res.Write(src, stage) })
	}
	if *inspect == "" {
		return fmt.Errorf("差し替えには --inspect が要る")
	}

	stage, err := os.MkdirTemp("", "gozen-trial-")
	if err != nil {
		return err
	}
	if err := res.Write(src, stage); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(stage)) }()
	policy, ins, outputs, err := releaseInputs(src, overlay, *inspect)
	if err != nil {
		return err
	}
	in := map[string]bool{}
	for _, a := range res.Assets {
		in[a] = true
	}
	var mine []assetfs.Approval
	for _, x := range ins {
		if in[x.File] {
			mine = append(mine, x)
		}
	}
	if err := checkReferences(stage, *overlay, "assets", policy, mine, outputs); err != nil {
		return err
	}
	var r assetfs.Result
	err = cli.Publish(out, []string{src, *overlay}, func(dst string) error {
		var e error
		r, e = (assetfs.Tree{Src: stage, Top: *overlay, Prefix: "assets", Dst: dst, Outputs: outputs}).Build(mine)
		return e
	})
	if err != nil {
		return err
	}
	need := 0
	for _, x := range mine {
		if x.Required {
			need++
		}
	}
	c.out.Printf("%s: 素材を置き換えた物 %d 枚（体験版の必須差し替え %d 枚）\n", out, len(r.Replaced), need)
	return nil
}

func (c *command) summary(res *trial.Result) {
	cg := 0
	for _, a := range res.Assets {
		if strings.HasPrefix(a, "cg/") {
			cg++
		}
	}
	c.out.Printf("体験版: イベント %d（外した %d）、場所 %d、敵 %d、敵グループ %d、曲 %d、絵・音 %d（一枚絵 %d）、ギャラリー 一枚絵 %d・回想 %d、出口の止め %d、%d 回で決まった\n",
		len(res.Kept["events"]), len(res.Removed["events"]), len(res.Kept["locations"]), len(res.Kept["enemies"]),
		len(res.Kept["troops"]), len(res.Kept["songs"]), len(res.Assets), cg, res.Gallery.CGs, res.Gallery.Scenes, res.Gates, res.Passes)
	if len(res.Missing) > 0 {
		c.out.Printf("  データに書かれているのに無いファイル %d: %s\n", len(res.Missing), strings.Join(res.Missing, ", "))
	}
}

func writeList(p string, res *trial.Result) error {
	var b strings.Builder
	for _, sec := range []string{"events", "locations", "troops", "enemies", "skills", "ai", "shops", "songs"} {
		appendf(&b, "# %s: 残す %d\n%s\n# %s: 外す %d\n%s\n", sec, len(res.Kept[sec]), strings.Join(res.Kept[sec], "\n"),
			sec, len(res.Removed[sec]), strings.Join(res.Removed[sec], "\n"))

	}
	appendf(&b, "# 絵・音 %d\n%s\n", len(res.Assets), strings.Join(res.Assets, "\n"))

	appendf(&b, "# なぜ残ったか（残した物 ← 参照した物）\n")

	for _, k := range sortedStrings(res.Why) {
		appendf(&b, "%s <- %s\n", k, res.Why[k])

	}
	appendf(&b, "# 変数・フラグの幅\n")

	for _, k := range sortedStrings(res.Domains) {
		appendf(&b, "%s = %s\t<- %s\n", k, res.Domains[k], strings.Join(res.DomainWhy[k], "; "))

	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	return cli.WriteNew(p, []byte(b.String()))
}

func sortedStrings(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
