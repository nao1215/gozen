package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	md "github.com/nao1215/markdown"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/internal/layer2/gamedata"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"

	"github.com/nao1215/gozen/cmd/gozen/internal/designeval"
	lang "github.com/nao1215/gozen/internal/text"
)

func (c *command) design() clihelp.Operation {
	fs := clihelp.NewFlagSet("gozen sim design")
	cfgPath := fs.String("config", "", "the personas and measures `FILE` (default: GAME/designeval.yaml)")
	root := fs.String("root", filepath.Join("build", "designeval"), "read previous runs from this `DIR` for comparison")
	out := fs.StringP("out", "o", "", "also write report artifacts to this new `DIR`")
	only := fs.StringArray("only", nil, "run this persona `ID`; repeatable")
	memos := fs.String("memos", "", "write marked-place notes to this `DIR`; requires --out")
	verbose := fs.BoolP("verbose", "v", false, "write simulation details to debug.log; requires --out")
	seed := fs.Uint64("seed", 0, "override every persona's random `SEED`; must be positive when supplied")
	list := fs.Bool("list", false, "list the loaded configuration without simulating")
	c.reporting(fs)
	return clihelp.Operation{Flags: fs, Run: func(_ context.Context) (err error) {
		if fs.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "expected one game directory")
		}
		if fs.Changed("seed") && *seed == 0 {
			return cliio.Invalid(diag.BadValue, "--seed must be positive when supplied")
		}
		if *out == "" && (*verbose || (*memos != "" && *memos != "none")) {
			return cliio.Invalid(diag.OptionNeedsAnother, "--verbose and --memos require --out DIR")
		}
		if *list && *out != "" {
			return cliio.Invalid(diag.OptionConflict, "--list and --out cannot be combined")
		}
		dir := fs.Arg(0)
		if *cfgPath == "" {
			*cfgPath = filepath.Join(dir, "designeval.yaml")
		}
		fsys, err := gameFS(dir)
		if err != nil {
			return err
		}
		if err := gamedata.UseWords(fsys); err != nil {
			return err
		}
		db0, err := gamedata.LoadDir(fsys, ".")
		if err != nil {
			return err
		}
		cfg, err := designeval.Load(os.DirFS(filepath.Dir(*cfgPath)), filepath.Base(*cfgPath))
		if err != nil {
			return err
		}
		if err := cfg.Validate(db0); err != nil {
			return err
		}
		if *list {
			if c.report.Machine() {
				return c.report.Result(c.out, "ok", cfg)
			}
			for _, p := range cfg.Personas {
				c.out.Printf("%s: %s (seed %d)\n", p.ID, p.Name, p.Seed)
			}
			return nil
		}
		var personas []designeval.Persona
		for _, p := range cfg.Personas {
			if len(*only) == 0 || slices.Contains(*only, p.ID) {
				if fs.Changed("seed") {
					p.Seed = *seed
				}
				personas = append(personas, p)
			}
		}
		for _, id := range *only {
			if !slices.ContainsFunc(personas, func(p designeval.Persona) bool { return p.ID == id }) {
				return cliio.Invalid(diag.UnknownName, "--only: unknown persona %q", id)
			}
		}
		if len(personas) == 0 {
			return cliio.Invalid(diag.UnknownName, "--only matched no personas")
		}
		complete := false
		if *out != "" {
			if err := cli.Destination(*out, dir, *memos, *cfgPath); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(*out), 0o750); err != nil {
				return err
			}
			if err := os.Mkdir(*out, 0o750); err != nil {
				return err
			}
			defer func() {
				if err != nil && !complete {
					err = errors.Join(err, os.RemoveAll(*out))
				}
			}()
			if err := os.Mkdir(filepath.Join(*out, "timeline"), 0o750); err != nil {
				return err
			}
		}
		var debug *cliio.Output
		if *verbose {
			f, err := os.Create(filepath.Join(*out, "debug.log"))
			if err != nil {
				return err
			}
			defer func() {
				if closeErr := f.Close(); closeErr != nil {
					err = errors.Join(err, closeErr)
				}
			}()
			debug = &cliio.Output{Writer: f}
			var mu sync.Mutex
			cfg.Debug = func(format string, a ...any) { mu.Lock(); defer mu.Unlock(); debug.Printf(format+"\n", a...) }
		}
		runs := make([]*designeval.Run, len(personas))
		sums := make([]designeval.Summary, len(personas))
		for i, p := range personas {
			db, err := gamedata.LoadDir(fsys, ".")
			if err != nil {
				return err
			}
			cfgRun, err := designeval.Load(os.DirFS(filepath.Dir(*cfgPath)), filepath.Base(*cfgPath))
			if err != nil {
				return err
			}
			cfgRun.Debug = cfg.Debug
			runs[i] = designeval.Play(db, cfgRun, p) //nolint:contextcheck // Bounded synchronous computation has no context parameter (API guidelines).
			sums[i] = designeval.Summarize(runs[i], cfg.Release)
		}
		if debug != nil && debug.Err != nil {
			return debug.Err
		}
		hash := ""
		if *out != "" {
			for _, r := range runs {
				if err := writeTimeline(filepath.Join(*out, "timeline", r.Persona.ID+".jsonl"), r); err != nil {
					return err
				}
			}
			b, err := json.MarshalIndent(map[string]any{"format_version": 1, "name": filepath.Base(*out), "hash": hash, "personas": sums}, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(*out, "summary.json"), b, 0o600); err != nil {
				return err
			}
			if err := writeBand(filepath.Join(*out, "band.png"), runs); err != nil {
				return err
			}
			var notes []string
			if *memos != "none" && *memos != "" {
				if notes, err = designMemos(*memos, filepath.Base(*out), db0, cfg, runs, sums); err != nil {
					return err
				}
			}
			prevName, prev := previousRun(*root, filepath.Base(*out))
			report := designReport(cfg, runs, sums, filepath.Base(*out), hash, prevName, prev)
			if len(notes) > 0 {
				report = reportMarkdown(md.NewMarkdown(nil, md.WithBlockSpacing()).PlainText(strings.TrimSpace(report)).H2(lang.T("メモ")).PlainTextf(lang.T("通信簿で目立った所を、テストプレイのメモ（source bot）として %d 件残した（%s。gozen notes で一覧）。"), len(notes), *memos).BulletList(notes...))
			}

			if err := os.WriteFile(filepath.Join(*out, "report.md"), []byte(report), 0o600); err != nil {
				return err
			}

			complete = true
		}
		failed := false
		for _, r := range runs {
			failed = failed || len(r.Problems) > 0
		}
		if c.report.Machine() {
			status := "ok"
			if failed {
				status = "failed"
			}
			results := make([]designOutcome, len(runs))
			for i, r := range runs {
				results[i] = designOutcome{Summary: sums[i], Seed: r.Seed, Problems: append([]string{}, r.Problems...), ClearT: r.ClearT, End: r.End, Events: append([]designeval.Event{}, r.Events...), Segments: append([]designeval.Segment{}, r.Segments...)}
				if c.report.Format == "jsonl" {
					itemStatus := "ok"
					if len(r.Problems) > 0 {
						itemStatus = "failed"
					}
					if err := c.report.Item(c.out, r.Persona.ID, itemStatus, results[i]); err != nil {
						return err
					}
				}
			}
			if err := c.report.Result(c.out, status, struct {
				Personas []designOutcome `json:"personas"`
			}{results}); err != nil {
				return err
			}
			if failed {
				return cliio.ErrReported
			}
			return nil
		}
		if *out != "" {
			c.out.Println(filepath.Join(*out, "report.md"))
		} else {
			c.out.Print(designReport(cfg, runs, sums, "", hash, "", nil))
		}
		if failed {
			return errors.New("design reported simulation errors; see report.md or the printed report")
		}
		return nil
	}}
}

type designOutcome struct {
	designeval.Summary
	Seed     uint64               `json:"seed"`
	Problems []string             `json:"problem_list"`
	ClearT   float64              `json:"clear_t"`
	End      float64              `json:"end"`
	Events   []designeval.Event   `json:"events"`
	Segments []designeval.Segment `json:"segments"`
}

func writeTimeline(path string, r *designeval.Run) (err error) {
	f, err := os.Create(path) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	enc := json.NewEncoder(f)
	ev := append([]designeval.Event(nil), r.Events...)
	sort.SliceStable(ev, func(i, j int) bool { return ev[i].T < ev[j].T })
	if err := enc.Encode(map[string]any{"kind": "persona", "id": r.Persona.ID, "name": r.Persona.Name, "cleared": r.Cleared, "clear_t": r.ClearT, "end": r.End, "days": r.Days}); err != nil {
		return err
	}
	for _, e := range ev {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	for _, s := range r.Segments {
		if err := enc.Encode(map[string]any{"kind": "segment", "from": s.From, "to": s.To, "place": s.Place, "dungeon": s.Dungeon}); err != nil {
			return err
		}
	}
	return nil
}

func previousRun(root, name string) (string, map[string]designeval.Summary) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return "", nil
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() && e.Name() < name {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range slices.Backward(names) {
		b, err := os.ReadFile(filepath.Join(root, name, "summary.json")) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
		if err != nil {
			continue
		}
		var s struct {
			FormatVersion int                  `json:"format_version"`
			Personas      []designeval.Summary `json:"personas"`
		}
		if json.Unmarshal(b, &s) != nil || s.FormatVersion != 1 {
			continue
		}
		out := map[string]designeval.Summary{}
		for _, p := range s.Personas {
			out[p.ID] = p
		}
		return name, out
	}
	return "", nil
}

func minutes(m float64) string {
	if m >= 60 {
		return fmt.Sprintf(lang.T("%d時間%02d分"), int(m)/60, int(m)%60)
	}
	return fmt.Sprintf(lang.T("%.1f分"), m)
}

func gapText(g designeval.Gap) string {
	if g.N == 0 {
		return "—"
	}
	return fmt.Sprintf("%s / %s", minutes(g.Avg), minutes(g.Max))
}

func gapWhere(g designeval.Gap) string {
	if g.N == 0 || g.At == "" {
		return ""
	}
	return fmt.Sprintf(lang.T("%s（%d 日目、%s から）"), g.At, g.Day, minutes(g.From))
}

func kindName(cfg *designeval.Config, k string) string {
	for _, r := range cfg.Rewards {
		if r.Kind == k && r.Label != "" {
			return r.Label
		}
	}
	return k
}

func designReport(cfg *designeval.Config, runs []*designeval.Run, sums []designeval.Summary, name, hash, prevName string, prev map[string]designeval.Summary) string {
	w := md.NewMarkdown(nil, md.WithBlockSpacing())
	w.H1f(lang.T("流れの通信簿（%s）"), name)

	w.PlainTextf(lang.T("版 %s。時間は見積もり（designeval.yaml の time の係数で、歩数・戦闘のターン・台詞の文字数・操作を秒に直した物）。遊び手は %d 人。"), hash, len(runs))
	w.PlainTextf(lang.T("ご褒美は %s。期待は designeval.yaml の expects、解放は kind %q のご褒美。"), rewardKinds(cfg), cfg.Release)

	w.H2(lang.T("一覧"))

	overview := md.TableSet{EscapeCells: true, Header: []string{lang.T("遊び手"), lang.T("結末まで"), lang.T("日数"), lang.T("最深"), "Lv", lang.T("戦闘"), lang.T("負け"), lang.T("全滅"), kindName(cfg, cfg.Release), lang.T("ご褒美の間隔 平均 / 最長"), lang.T("何も起きない最長"), lang.T("期待→解放 平均 / 最長")}}
	for _, s := range sums {
		reached := minutes(s.ClearMin)
		if !s.Cleared {
			reached = fmt.Sprintf(lang.T("届かず（%s）"), minutes(s.EndMin))
		}
		overview.Rows = append(overview.Rows, reportCells(fmt.Sprintf(lang.T("%s（%s）"), s.Name, s.ID), reached, s.Days, fmt.Sprintf("B%dF", s.Deepest), s.Level, s.Battles, s.Lost, s.Wipes, s.Rewards[cfg.Release], gapText(s.Gap), minutes(s.Dry.Max), gapText(s.Release)))
	}

	w.Table(overview)
	w.H2(lang.T("噛み合い（目安。落とさず印だけ）"))

	byID := map[string]designeval.Summary{}
	for _, s := range sums {
		byID[s.ID] = s
	}
	t := cfg.Targets
	if a, ok := byID[t.Persona]; ok {
		if b, ok := byID[t.CloseTo]; ok && a.Cleared && b.Cleared {
			d := (b.ClearMin - a.ClearMin) / a.ClearMin
			mark := "○"
			if d > t.Within || d < -t.Within {
				mark = "×"
			}
			reportBullet(w, "", lang.T("%s と %s の結末までの差: %s と %s（%+.0f%%、目安 %.0f%% 以内）%s"), b.Name, a.Name, minutes(b.ClearMin), minutes(a.ClearMin), 100*d, 100*t.Within, mark)
			an, bn := a.ClearMin-a.ReleaseMin, b.ClearMin-b.ReleaseMin
			dn := (bn - an) / an
			mark = "○"
			if dn > t.Within || dn < -t.Within {
				mark = "×"
			}
			reportBullet(w, "", lang.T("報酬の場面を読む時間を除くと: %s と %s（%+.0f%%）%s。報酬の場面に %s と %s。"), minutes(bn), minutes(an), 100*dn, mark, minutes(b.ReleaseMin), minutes(a.ReleaseMin))
		} else if ok {
			reportBullet(w, "", lang.T("%s と %s の差: どちらかが結末に届かなかった"), b.Name, a.Name)
		}
	}
	if s, ok := byID[t.MustClear]; ok {
		if s.Cleared {
			reportBullet(w, "", lang.T("%s: 結末まで行けた（%s、負け %d、全滅 %d）○"), s.Name, minutes(s.ClearMin), s.Lost, s.Wipes)
		} else {
			reportBullet(w, "", lang.T("%s: 結末に届かなかった（B%dF まで、%d 日）×"), s.Name, s.Deepest, s.Days)
		}
	}

	w.PlainTextf(lang.T("番人ごとの、初めて着いたときの勝率（HP・MP を満たして %d 戦）と Lv、挑んだ回数と勝ち、勝率を上げるために足した戦闘:"), cfg.Game.Tries)

	bosses := md.TableSet{EscapeCells: true, Header: []string{lang.T("番人")}}
	for _, s := range sums {
		bosses.Header = append(bosses.Header, s.Name)
	}
	floors := []int{}
	for f := range cfg.Game.Bosses {
		floors = append(floors, f)
	}
	sort.Ints(floors)
	for _, f := range floors {
		row := []string{fmt.Sprintf("B%dF", f)}
		for _, s := range sums {
			cell := "—"
			for _, b := range s.Bosses {
				if b.Floor == f && b.Measured {
					cell = fmt.Sprintf(lang.T("%.0f%% Lv%d、%d 戦 %d 勝、+%d"), 100*b.Rate, b.Level, b.Tries, b.Wins, b.Grind)
				} else if b.Floor == f {
					cell = fmt.Sprintf(lang.T("測る前に挑んだ、%d 戦 %d 勝"), b.Tries, b.Wins)
				}
			}
			row = append(row, cell)
		}
		bosses.Rows = append(bosses.Rows, row)
	}
	w.Table(bosses)
	if len(cfg.Duels.List) > 0 {

		w.PlainTextf(lang.T("1 対 1 の相手ごとの勝率（%d 戦。戦う仲間の Lv）。出てきた日の朝（その日に戦う前）の初戦と、負けてすぐ挑み直す再戦、少し後の日の再戦。本当に戦った数と勝ち:"), cfg.Duels.Tries)
		if len(cfg.Duels.First) == 2 {
			w.PlainTextf(lang.T("（目安: 初戦 %.0f〜%.0f%%、再戦 %.0f%% 以上。平均は %s で、外れた所に ×）"), 100*cfg.Duels.First[0], 100*cfg.Duels.First[1], 100*cfg.Duels.Later,
				map[bool]string{true: lang.T("全員"), false: strings.Join(cfg.Duels.Personas, lang.T("・"))}[len(cfg.Duels.Personas) == 0])
		}

		duels := md.TableSet{EscapeCells: true, Header: append([]string{lang.T("相手")}, bosses.Header[1:]...)}
		duels.Header = append(duels.Header, lang.T("平均"))
		for i, a := range designeval.DuelAverages(cfg, sums) {
			row := []string{a.Name}
			for _, s := range sums {
				cell := "—"
				if i < len(s.Duels) && s.Duels[i].Opened {
					d := s.Duels[i]
					cell = fmt.Sprintf(lang.T("初戦 %.0f%%・挑み直し %.0f%% Lv%d"), 100*d.First, 100*d.Again, d.FirstLevel)
					if d.Grown {
						cell += fmt.Sprintf(lang.T("、後 %.0f%% Lv%d"), 100*d.Later, d.LaterLevel)
					}
					if d.Tries > 0 {
						cell += fmt.Sprintf(lang.T("、%d 戦 %d 勝"), d.Tries, d.Wins)
					}
				}
				row = append(row, cell)
			}
			mark := func(k string) string {
				if slices.Contains(a.Off, k) {
					return " ×"
				}
				return ""
			}
			avg := "—"
			if a.NFirst > 0 {
				avg = fmt.Sprintf(lang.T("初戦 %.0f%%%s・挑み直し %.0f%%%s"), 100*a.First, mark("first"), 100*a.Again, mark("again"))
			}
			if a.NLater > 0 {
				avg += fmt.Sprintf(lang.T("、後 %.0f%%%s"), 100*a.Later, mark("later"))
			}
			duels.Rows = append(duels.Rows, append(row, avg))
		}
		w.Table(duels)
	}

	w.H2(lang.T("遊び手ごと"))
	for i, s := range sums {
		r := runs[i]

		w.H3f(lang.T("%s（%s）"), s.Name, s.ID)

		w.PlainTextf("%s", r.Persona.Desc)

		if s.Cleared {
			reportBullet(w, "", lang.T("結末まで %s（%d 日目）。最深 B%dF、平均 Lv%d。"), minutes(s.ClearMin), s.Days, s.Deepest, s.Level)
		} else {
			reportBullet(w, "", lang.T("結末に届かなかった（%s、%d 日、最深 B%dF、平均 Lv%d）。"), minutes(s.EndMin), s.Days, s.Deepest, s.Level)
		}
		reportBullet(w, "", lang.T("過ごした時間: 街 %s、迷宮 %s。戦闘 %d 回、負け %d（やり直し %d）、全滅 %d、氷片なしで帰った %d。"), minutes(s.Minutes["town"]), minutes(s.Minutes["dungeon"]), s.Battles, s.Lost, s.Retries, s.Wipes, s.Stranded)
		reportBullet(w, "", lang.T("ご褒美の間隔（どれか）: 平均 %s。長い順: %s。"), minutes(s.Gap.Avg), spansText(s.Gap.Top))
		reportBullet(w, "", lang.T("何も起きない（ご褒美も期待も無い）長い順: %s。"), spansText(s.Dry.Top))
		reportBullet(w, "", lang.T("報酬の場面を読んだ時間: %s（結末までの %.0f%%）。"), minutes(s.ReleaseMin), 100*s.ReleaseMin/max(1e-9, s.ClearMin))
		if s.Release.N > 0 {
			reportBullet(w, "", lang.T("期待から解放まで: %d 回、平均 %s、最長 %s（%s）。最後の解放の後に残った期待 %d。"), s.Release.N, minutes(s.Release.Avg), minutes(s.Release.Max), gapWhere(s.Release), s.Unreleased)
		} else {
			reportBullet(w, "", lang.T("期待から解放まで: 解放が一度も無い（期待 %d 回）。"), s.Expects)
		}

		rewards := md.TableSet{EscapeCells: true, Header: []string{lang.T("ご褒美"), lang.T("回数"), lang.T("間隔 平均 / 最長"), lang.T("最長の所")}}
		kinds := []string{}
		for k := range s.Rewards {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			g := s.KindGap[k]
			rewards.Rows = append(rewards.Rows, reportCells(kindName(cfg, k), s.Rewards[k], gapText(g), gapWhere(g)))
		}
		w.Table(rewards)
		if len(s.ReleaseWho) > 0 {

			w.PlainTextf(lang.T("報酬の対象: %s"), countText(s.ReleaseWho))

			w.PlainTextf(lang.T("報酬の経路: %s"), countText(s.ReleaseFlow))
		}
		if len(r.Problems) > 0 {

			w.PlainTextf(lang.T("遊べなかった所（%d 件、先頭 5 件）:"), len(r.Problems))
			for _, e := range r.Problems[:min(5, len(r.Problems))] {
				reportBullet(w, "", "%s", e)
			}
		}
	}

	w.H2(lang.T("前の回との差"))

	if prev == nil {
		w.PlainTextf(lang.T("前の回が無い（%s に summary.json のある回が無い）。"), "build/designeval")
	} else {
		w.PlainTextf(lang.T("前の回: %s"), prevName)

		changes := md.TableSet{EscapeCells: true, Header: []string{lang.T("遊び手"), lang.T("指標"), lang.T("前"), lang.T("今"), lang.T("差")}}
		for _, s := range sums {
			o, ok := prev[s.ID]
			if !ok {
				changes.Rows = append(changes.Rows, []string{s.Name, lang.T("（前の回にいない）"), "", "", ""})
				continue
			}
			row := func(label string, a, b float64, unit string) {
				changes.Rows = append(changes.Rows, []string{s.Name, label, fmt.Sprintf("%.1f%s", a, unit), fmt.Sprintf("%.1f%s", b, unit), fmt.Sprintf("%+.1f%s", b-a, unit)})
			}
			row(lang.T("結末まで（分）"), o.ClearMin, s.ClearMin, "")
			row(lang.T("負け"), float64(o.Lost), float64(s.Lost), "")
			row(lang.T("報酬の回数"), float64(o.Rewards[cfg.Release]), float64(s.Rewards[cfg.Release]), "")
			row(lang.T("ご褒美の間隔の平均（分）"), o.Gap.Avg, s.Gap.Avg, "")
			row(lang.T("ご褒美の間隔の最長（分）"), o.Gap.Max, s.Gap.Max, "")
			row(lang.T("何も起きない最長（分）"), o.Dry.Max, s.Dry.Max, "")
			row(lang.T("期待→解放の平均（分）"), o.Release.Avg, s.Release.Avg, "")
			row(lang.T("期待→解放の最長（分）"), o.Release.Max, s.Release.Max, "")
			for _, b := range s.Bosses {
				for _, ob := range o.Bosses {
					if ob.Floor == b.Floor {
						row(fmt.Sprintf(lang.T("B%dF の着いたときの勝率"), b.Floor), 100*ob.Rate, 100*b.Rate, "%")
					}
				}
			}
		}
		w.Table(changes)
	}

	w.H2(lang.T("時間の帯"))

	w.PlainText(md.Image(lang.T("帯"), "band.png"))

	w.PlainTextf("%s", lang.T("上から一覧の順に 1 人 1 段。横は時間（縦の細線は 1 時間ごと）。各段の上の帯は居場所（灰色は街、緑・紫・青・赤は迷宮の区画、色が濃いほど深い）、"))
	w.PlainTextf("%s", lang.T("真ん中の線は設定で定義した報酬の種類、下の点は期待（紫）と負け（茶）。"))
	return reportMarkdown(w)
}

func spansText(sp []designeval.Span) string {
	var parts []string
	for _, x := range sp {
		parts = append(parts, fmt.Sprintf(lang.T("%s（%s、%d 日目、%s から）"), minutes(x.Len), x.At, x.Day, minutes(x.From)))
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, lang.T("、"))
}

func rewardKinds(cfg *designeval.Config) string {
	seen := map[string]bool{}
	var out []string
	for _, r := range cfg.Rewards {
		if !seen[r.Kind] {
			seen[r.Kind] = true
			out = append(out, kindName(cfg, r.Kind))
		}
	}
	return strings.Join(out, lang.T("・"))
}

func countText(m map[string]int) string {
	type kv struct {
		k string
		v int
	}
	var l []kv
	total := 0
	for k, v := range m {
		l = append(l, kv{k, v})
		total += v
	}
	sort.Slice(l, func(i, j int) bool {
		if l[i].v != l[j].v {
			return l[i].v > l[j].v
		}
		return l[i].k < l[j].k
	})
	var parts []string
	for _, x := range l {
		parts = append(parts, fmt.Sprintf(lang.T("%s %d（%.0f%%）"), x.k, x.v, 100*float64(x.v)/float64(max(1, total))))
	}
	return strings.Join(parts, lang.T("、"))
}

func rewardColor(kind string) color.NRGBA {
	colors := []color.NRGBA{{0xe8, 0x40, 0x7a, 255}, {0xf0, 0xa0, 0x30, 255}, {0x30, 0x70, 0xd0, 255}, {0x30, 0xa0, 0x50, 255}}
	if kind == "boss" {
		return color.NRGBA{0, 0, 0, 255}
	}
	var n uint32
	for _, c := range []byte(kind) {
		n = n*31 + uint32(c)
	}
	return colors[n%4]
}

func writeBand(path string, runs []*designeval.Run) (err error) {
	const W, rowH, pad = 1600, 46, 8
	end := 1.0
	for _, r := range runs {
		e := r.End
		if r.Cleared {
			e = r.ClearT
		}
		end = max(end, e)
	}
	H := pad + len(runs)*(rowH+pad)
	img := image.NewNRGBA(image.Rect(0, 0, W, H))
	fill := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := max(0, y0); y < min(H, y1); y++ {
			for x := max(0, x0); x < min(W, x1); x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	fill(0, 0, W, H, color.NRGBA{255, 255, 255, 255})
	xOf := func(t float64) int { return int(t / end * float64(W-1)) }
	for h := 3600.0; h < end; h += 3600 {
		x := xOf(h)
		fill(x, 0, x+1, H, color.NRGBA{0xdd, 0xdd, 0xdd, 255})
	}
	zone := []color.NRGBA{{0x5a, 0xa0, 0x6a, 255}, {0x9a, 0x6a, 0xc0, 255}, {0x5a, 0x80, 0xc0, 255}, {0xc0, 0x5a, 0x5a, 255}, {0x60, 0x60, 0x60, 255}}
	for i, r := range runs {
		y := pad + i*(rowH+pad)
		stop := r.End
		if r.Cleared {
			stop = r.ClearT
		}
		for _, s := range r.Segments {
			if s.From >= stop {
				break
			}
			c := color.NRGBA{0xc8, 0xc8, 0xc8, 255}
			if s.Dungeon {
				z := zone[min(len(zone)-1, max(0, (s.Floor-1)/5))]
				k := 0.55 + 0.45*float64((s.Floor-1)%5)/4
				c = color.NRGBA{uint8(255 - k*float64(255-int(z.R))), uint8(255 - k*float64(255-int(z.G))), uint8(255 - k*float64(255-int(z.B))), 255}
			}
			fill(xOf(s.From), y, max(xOf(s.From)+1, xOf(min(s.To, stop))), y+14, c)
		}
		for _, e := range r.Events {
			if e.T > stop {
				continue
			}
			x := xOf(e.T)
			switch {
			case e.Reward || e.Kind == "boss":
				c := rewardColor(e.Kind)
				fill(x, y+16, x+2, y+34, c)
			case e.Expect:
				fill(x, y+36, x+2, y+40, color.NRGBA{0x90, 0x60, 0xd0, 255})
			case e.Kind == "lost" || e.Kind == "wipe":
				fill(x, y+41, x+2, y+45, color.NRGBA{0x80, 0x50, 0x20, 255})
			}
		}
	}
	f, err := os.Create(path) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	return png.Encode(f, img)
}
