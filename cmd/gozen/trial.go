package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nao1215/gozen/internal/base/cliio"

	"github.com/nao1215/gozen/internal/model/gamedata"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/base/clihelp"
	"github.com/nao1215/gozen/internal/base/diag"

	"github.com/nao1215/gozen/cmd/internal/trial"
	"github.com/nao1215/gozen/internal/base/assetfs"
)

func (c *command) trialCmd() clihelp.Operation {
	fl := clihelp.NewFlagSet("gozen trial")
	var out string
	fl.StringVarP(&out, "out", "o", "", "write the trial to this new `DIR`; required unless --dry-run")
	conf := fl.String("config", "", "the range settings `FILE`, relative to the working directory (default: GAME/trial.yaml)")
	overlay := fl.String("overlay", "", "take replaced assets from this `DIR` (needs --inspect)")
	inspect := fl.String("inspect", "", "the inspection records `FILE` (JSON Lines) from production; needed with --overlay")
	list := fl.String("list", "", "write the list of what was kept and dropped to this new `FILE`")
	dry := fl.BoolP("dry-run", "n", false, "write nothing; print only how much would be kept")
	return clihelp.Operation{Flags: fl, Run: func(_ context.Context) (err error) {
		if fl.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "expected one game directory")
		}
		src := fl.Arg(0)
		if !*dry && out == "" {
			return cliio.Invalid(diag.OptionRequired, "trial requires --out DIR unless --dry-run")
		}
		if (*overlay == "") != (*inspect == "") {
			return cliio.Invalid(diag.OptionNeedsAnother, "--overlay and --inspect must be supplied together")
		}
		if !*dry {
			if err := cli.Destination(out, src, *overlay); err != nil {
				return err
			}
		}
		if err := gamedata.UseWords(os.DirFS(src)); err != nil {
			return err
		}
		// The default is empty, not trial.yaml, so help --format json does not
		// advertise a file in the working directory: it is the game's own.
		configPath := cmp.Or(*conf, filepath.Join(src, "trial.yaml"))
		cfg, err := trial.Load(os.DirFS(filepath.Dir(configPath)), filepath.Base(configPath))
		if err != nil {
			if diag.Of(err) == 0 {
				err = diag.TrialConfig.Wrap(err)
			}
			return err
		}
		res, err := trial.Build(os.DirFS(src), cfg)
		if err != nil {
			return err
		}
		if err := res.Check(); err != nil {
			return fmt.Errorf("the trial copy does not pass the engine's check: %w", err)
		}
		c.summary(res)
		if *dry {
			return nil
		}
		if *list != "" {
			if err := cli.Destination(*list, src, *overlay, out); err != nil {
				return err
			}
		}
		if *overlay == "" {
			err = publishUnapproved(src, out, res)
		} else {
			err = c.publishApproved(src, out, *overlay, *inspect, res)
		}
		if err != nil || *list == "" {
			return err
		}
		// The list comes last, and a list that cannot be written takes the tree
		// back, so a failed run leaves nothing for the next one to stumble on.
		if err := writeList(*list, res); err != nil {
			return errors.Join(err, os.RemoveAll(out))
		}
		return nil
	}}
}

// publishUnapproved writes the trial as it is. The game's asset policy goes with
// it: its pictures are as unapproved as the game's, so gozen export refuses the
// trial as it refuses the game.
func publishUnapproved(src, out string, res *trial.Result) error {
	return cli.Publish(out, []string{src}, func(stage string) error {
		if err := res.Write(stage); err != nil {
			return err
		}
		const policy = "asset-policy.yaml"
		err := cli.CopyFile(os.DirFS(src), policy, filepath.Join(stage, policy), 0o600)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	})
}

// publishApproved writes the trial with its assets replaced from the overlay,
// as gozen release does for the whole game; the result needs no policy.
func (c *command) publishApproved(src, out, overlay, inspect string, res *trial.Result) (err error) {
	stage, err := os.MkdirTemp("", "gozen-trial-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(stage)) }()
	if err := res.Write(stage); err != nil {
		return err
	}
	policy, ins, outputs, err := releaseInputs(src, &overlay, inspect)
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
	if err := checkReferences(stage, overlay, "assets", policy, mine, outputs); err != nil {
		return err
	}
	var r assetfs.Result
	err = cli.Publish(out, []string{src, overlay}, func(dst string) error {
		var e error
		r, e = (assetfs.Tree{Src: stage, Top: overlay, Prefix: "assets", Dst: dst, Outputs: outputs}).Build(mine)
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
	c.out.Printf("%s: %d assets replaced (%d required for the trial)\n", out, len(r.Replaced), need)
	return nil
}

func (c *command) summary(res *trial.Result) {
	cg := 0
	for _, a := range res.Assets {
		if strings.HasPrefix(a, "cg/") {
			cg++
		}
	}
	c.out.Printf("trial: %d events (%d removed), %d locations, %d enemies, %d troops, %d songs, %d pictures and sounds (%d event pictures), gallery %d pictures and %d scenes, %d exits closed, settled in %d passes\n",
		len(res.Kept["events"]), len(res.Removed["events"]), len(res.Kept["locations"]), len(res.Kept["enemies"]),
		len(res.Kept["troops"]), len(res.Kept["songs"]), len(res.Assets), cg, res.Gallery.CGs, res.Gallery.Scenes, res.Gates, res.Passes)
	if len(res.Unresolved) > 0 {
		c.out.Printf("  %d unresolved conditions retained; inspect --list before distributing\n", len(res.Unresolved))
	}
	if len(res.Missing) > 0 {
		c.out.Printf("  %d files named in the data are missing: %s\n", len(res.Missing), strings.Join(res.Missing, ", "))
	}
}

func writeList(p string, res *trial.Result) error {
	var b strings.Builder
	for _, sec := range []string{"events", "locations", "troops", "enemies", "skills", "ai", "shops", "songs"} {
		appendf(&b, "# %s: kept %d\n%s\n# %s: removed %d\n%s\n", sec, len(res.Kept[sec]), strings.Join(res.Kept[sec], "\n"),
			sec, len(res.Removed[sec]), strings.Join(res.Removed[sec], "\n"))
	}
	appendf(&b, "# pictures and sounds %d\n%s\n", len(res.Assets), strings.Join(res.Assets, "\n"))

	appendf(&b, "# unresolved conditions retained\n%s\n", strings.Join(res.Unresolved, "\n"))
	appendf(&b, "# why kept (kept <- referenced by)\n")

	for _, k := range sortedStrings(res.Why) {
		appendf(&b, "%s <- %s\n", k, res.Why[k])
	}
	appendf(&b, "# ranges of variables and flags\n")

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
