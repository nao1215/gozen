package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nao1215/gozen/internal/play"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer2/assetpolicy"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer1/assetfs"

	lang "github.com/nao1215/gozen/internal/text"
)

type shotList struct {
	Title string `yaml:"title"`

	Watch []string   `yaml:"watch"`
	Seed  uint64     `yaml:"seed"`
	Shots []shotItem `yaml:"shots"`
}

type shotItem struct {
	ID    string `yaml:"id"`
	Title string `yaml:"title"`
	Who   string `yaml:"who"`
	Phase string `yaml:"phase"`
	Pose  string `yaml:"pose"`
	Note  string `yaml:"note"`

	Scene   string `yaml:"scene"`
	Event   string `yaml:"event"`
	Picture string `yaml:"picture"`
	Line    int    `yaml:"line"`
	Answers []int  `yaml:"answers"`

	Start  string   `yaml:"start"`
	Party  []string `yaml:"party"`
	Level  int      `yaml:"level"`
	Floor  int      `yaml:"floor"`
	Flags  []string `yaml:"flags"`
	Battle string   `yaml:"battle"`
	Calm   bool     `yaml:"calm"`
	Seed   uint64   `yaml:"seed"`

	Setup  []map[string]any `yaml:"setup"`
	GIF    *shotGIF         `yaml:"gif"`
	Script string           `yaml:"script"`
	Wait   int              `yaml:"wait"`
}

type shotGIF struct {
	FPS      int    `yaml:"fps"`
	Duration string `yaml:"duration"`
	Width    int    `yaml:"width"`
	Script   string `yaml:"script"`
}

type shotResult struct {
	N        int      `json:"n"`
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	Reason   string   `json:"reason,omitempty"`
	Artifact string   `json:"artifact,omitempty"`
	Frames   int      `json:"frames"`
	Seed     uint64   `json:"seed"`
	Size     string   `json:"size,omitempty"`
	Line     string   `json:"line,omitempty"`
	Pictures []string `json:"pictures,omitempty"`
	item     shotItem
}

func (c *command) capture() clihelp.Operation {
	fl := clihelp.NewFlagSet("gozen capture")
	config := fl.String("config", "", "capture job YAML `FILE`; required")
	overlay := fl.String("overlay", "", "approved replacement asset `DIR`")
	raw := fl.Bool("raw", false, "skip approval checks for local review")
	out := fl.StringP("out", "o", "", "write images and reports to this new `DIR`; required")
	only := fl.StringArray("only", nil, "capture this job `ID`; repeatable")
	timeout := fl.Duration("timeout", 4*time.Minute, "cooperative time limit per job")
	c.reporting(fl)
	return clihelp.Operation{Flags: fl, Run: func(ctx context.Context) (err error) {
		if fl.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "expected one game directory")
		}
		if *config == "" || *out == "" {
			return cliio.Invalid(diag.OptionRequired, "capture requires --config FILE and --out DIR")
		}
		if *timeout <= 0 {
			return cliio.Invalid(diag.BadValue, "--timeout must be positive")
		}
		if *raw && *overlay != "" {
			return cliio.Invalid(diag.OptionConflict, "--raw and --overlay cannot be combined")
		}
		list, err := readShotList(*config)
		if err != nil {
			return err
		}
		gameDir := fl.Arg(0)
		fsys := os.DirFS(gameDir)
		var gate *assetfs.Gate
		if !*raw {
			_, policyErr := fs.Stat(fsys, "asset-policy.yaml")
			if *overlay != "" || policyErr == nil {
				policy, err := assetpolicy.Load(fsys, "asset-policy.yaml")
				if err != nil {
					return err
				}
				top := fsys
				if *overlay != "" {
					top = os.DirFS(*overlay)
				}
				records, outputs, err := policy.ReadRecords(top)
				if err != nil {
					return err
				}
				gate = &assetfs.Gate{Base: fsys, Top: top, Prefix: "assets", Records: records, Outputs: outputs, Watch: list.Watch}
				if *overlay != "" {
					fsys, err = assetfs.New(fsys, top, assetfs.Config{Prefix: "assets", Manifest: assetfs.Manifest{Name: policy.Report, File: policy.Records.File, Sum: policy.Records.Source}})
					if err != nil {
						return err
					}
				}
			} else if !errors.Is(policyErr, fs.ErrNotExist) {
				return policyErr
			}
		}
		want := map[string]bool{}
		for _, id := range *only {
			want[id] = true
		}
		for id := range want {
			found := false
			for _, it := range list.Shots {
				found = found || it.ID == id
			}
			if !found {
				return cliio.Invalid(diag.UnknownName, "--only: unknown capture %q", id)
			}
		}
		var jobs []play.CaptureJob
		var items []shotItem
		for _, it := range list.Shots {
			if len(want) > 0 && !want[it.ID] {
				continue
			}
			seed := it.Seed
			if seed == 0 {
				seed = list.Seed
			}
			wait := it.Wait
			if wait == 0 {
				wait = 120
			}
			setup := ""
			if len(it.Setup) > 0 {
				b, err := yaml.Marshal(it.Setup)
				if err != nil {
					return err
				}
				setup = string(b)
			}
			j := play.CaptureJob{ID: it.ID, Start: it.Start, Scene: it.Scene, Event: it.Event, Picture: it.Picture, Line: it.Line, Answers: it.Answers, Party: it.Party, Level: it.Level, Floor: it.Floor, Flags: it.Flags, Battle: it.Battle, Calm: it.Calm, Seed: seed, Script: it.Script, Setup: setup, WaitTicks: wait}
			if it.GIF != nil {
				duration, err := time.ParseDuration(it.GIF.Duration)
				if it.GIF.Duration == "" {
					duration, err = 0, nil
				}
				if err != nil {
					return fmt.Errorf("capture %s: %w", it.ID, err)
				}
				j.GIF = &play.CaptureGIF{FPS: it.GIF.FPS, Duration: duration, Width: it.GIF.Width, Script: it.GIF.Script}
			}
			jobs, items = append(jobs, j), append(items, it)
		}
		var results []shotResult
		failed := 0
		err = cli.Publish(*out, []string{gameDir, *config, *overlay}, func(stage string) error {
			cfg := play.CaptureConfig{Output: stage, Timeout: *timeout}
			if gate != nil {
				cfg.CheckAsset = gate.Check
			}
			rendered, err := play.Capture(ctx, fsys, jobs, cfg)
			if err != nil {
				return err
			}
			for i, r := range rendered {
				row := shotResult{N: i + 1, ID: r.ID, Status: "ok", Artifact: r.Artifact, Size: fmt.Sprintf("%dx%d", r.Width, r.Height), Frames: r.Frames, Seed: r.Seed, Pictures: r.Pictures, item: items[i]}
				if r.Err != nil {
					row.Status, row.Reason = "failed", r.Err.Error()
					failed++
				}
				results = append(results, row)
			}
			return writeShotIndex(stage, list, results, *raw)
		})
		if err != nil {
			return err
		}
		status := "ok"
		if failed > 0 {
			status = "failed"
		}
		if c.report.Machine() {
			if c.report.Format == "jsonl" {
				for _, r := range results {
					if err := c.report.Item(c.out, r.ID, r.Status, r); err != nil {
						return err
					}
				}
			}
			if err := c.report.Result(c.out, status, struct {
				Items     []shotResult `json:"items"`
				Succeeded int          `json:"succeeded"`
				Failed    int          `json:"failed"`
			}{results, len(results) - failed, failed}); err != nil {
				return err
			}
		} else {
			for _, r := range results {
				c.out.Printf("%s %s %s\n", r.ID, r.Status, r.Reason)
			}
			c.out.Printf("%s: %d captured, %d failed\n", *out, len(results)-failed, failed)
		}
		if failed > 0 {
			return cliio.ErrReported
		}
		return nil
	}}
}

func readShotList(p string) (*shotList, error) {
	b, err := os.ReadFile(p) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return nil, err
	}
	var l shotList
	if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions(b, &l, yaml.Strict()) }); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	seen := map[string]bool{}
	for i, it := range l.Shots {
		if !cli.ID(it.ID) || seen[it.ID] {
			return nil, fmt.Errorf("%s: shots[%d]: the id is missing or repeated (%q)", p, i, it.ID)
		}
		seen[it.ID] = true
		if it.Wait < 0 || it.Line < 0 || it.Floor < 0 || it.Level < 0 {
			return nil, fmt.Errorf("%s: negative shot count", it.ID)
		}
		for _, v := range it.Answers {
			if v < 0 {
				return nil, fmt.Errorf("%s: negative answer", it.ID)
			}
		}
		if it.Picture != "" && !cli.Relative(it.Picture) {
			return nil, fmt.Errorf("%s: invalid picture path", it.ID)
		}
		if it.Scene != "" && it.Event != "" {
			return nil, fmt.Errorf("%s: %s: give only one of scene and event", p, it.ID)
		}
		if it.Scene != "" && it.Start != "" {
			return nil, fmt.Errorf("%s: scene and start cannot be combined", it.ID)
		}
		if it.Battle != "" && (it.Start == "" || it.Scene != "" || it.Event != "") {
			return nil, fmt.Errorf("%s: battle requires start without scene or event", it.ID)
		}
		if (it.Line > 0 || len(it.Answers) > 0 || it.Picture != "") && it.Scene == "" && it.Event == "" {
			return nil, fmt.Errorf("%s: line, answers and picture require scene or event", it.ID)
		}
		if it.Scene == "" && it.Event == "" && it.Start == "" {
			return nil, fmt.Errorf("%s: %s: give scene, event or start", p, it.ID)
		}
	}
	if len(l.Shots) == 0 {
		return nil, errors.New("shots: empty list")
	}
	for _, prefix := range l.Watch {
		if prefix != "" && !cli.Relative(strings.TrimSuffix(prefix, "/")) {
			return nil, errors.New("shots: invalid watch prefix")
		}
	}
	if l.Seed == 0 {
		l.Seed = 1
	}
	return &l, nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func split(s string) []string {
	var out []string
	for f := range strings.SplitSeq(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func shotRepeats(items []shotItem) []string {
	var out []string
	for i := 1; i < len(items); i++ {
		a, b := items[i-1], items[i]
		if a.Phase != "" && a.Phase == b.Phase {
			out = append(out, fmt.Sprintf(lang.T("%s と %s が同じ段階 %s"), a.ID, b.ID, a.Phase))
		}
		if a.Pose != "" && a.Pose == b.Pose {
			out = append(out, fmt.Sprintf(lang.T("%s と %s が同じ構図 %s"), a.ID, b.ID, a.Pose))
		}
	}
	return out
}

func writeShotIndex(out string, list *shotList, rs []shotResult, raw bool) error {
	js, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "shots.json"), js, 0o600); err != nil {
		return err
	}
	var b strings.Builder
	e := html.EscapeString
	title := list.Title
	if title == "" {
		title = lang.T("スクリーンショット")
	}
	appendText(&b, "<!doctype html><meta charset=utf-8><title>"+e(title)+"</title>\n<style>body{font-family:sans-serif;background:#222;color:#ddd;margin:24px}"+
		"table{border-collapse:collapse}td,th{border:1px solid #555;padding:6px;vertical-align:top}img{width:480px}a{color:#9cf}.w{color:#fc6}.f{color:#f77}</style>\n")

	appendText(&b, "<h1>"+e(title)+"</h1>\n")

	if raw {
		appendText(&b, "<p class=f>"+e(lang.T("承認未検査の版（--raw）。手元で場面を見るだけの物で、紹介ページに使わない。"))+"</p>\n")
	}
	n := map[string]int{}
	for _, r := range rs {
		n[r.Status]++
	}
	appendText(&b, "<p>"+e(fmt.Sprintf(lang.T("撮れた %d 枚、承認待ち %d 枚、失敗 %d 枚。%s に作った。"), n["ok"], n["approval_wait"], n["failed"], time.Now().Format("2006-01-02 15:04")))+"</p>\n")

	var items []shotItem
	for _, r := range rs {
		items = append(items, r.item)
	}
	if rep := shotRepeats(items); len(rep) > 0 {
		appendText(&b, "<p class=w>"+e(fmt.Sprintf(lang.T("並びの重なり: %s"), strings.Join(rep, lang.T("、"))))+"</p>\n")
	}
	row := func(r shotResult) {
		it := r.item
		appendText(&b, "<tr><td>"+strconv.Itoa(r.N)+"</td><td>")

		if r.Status == "ok" {
			appendf(&b, "<a href=%q><img src=%q loading=lazy></a>", r.Artifact, r.Artifact)
		}
		appendText(&b, "</td><td><b>"+e(it.Title)+"</b><br>id "+e(it.ID))

		if it.Who != "" {
			appendText(&b, "<br>"+e(fmt.Sprintf(lang.T("人物 %s"), it.Who)))
		}
		if it.Phase != "" || it.Pose != "" {
			appendText(&b, "<br>"+e(fmt.Sprintf(lang.T("段階 %s・構図 %s"), orDash(it.Phase), orDash(it.Pose))))
		}
		var where []string
		if it.Scene != "" {
			where = append(where, fmt.Sprintf(lang.T("回想「%s」"), it.Scene))
		}
		if it.Event != "" {
			where = append(where, fmt.Sprintf(lang.T("イベント %s"), it.Event))
		}
		if it.Picture != "" {
			where = append(where, fmt.Sprintf(lang.T("%s の %d 行目"), it.Picture, max(1, it.Line)))
		}
		if it.Start != "" {
			s := fmt.Sprintf(lang.T("場所 %s"), it.Start)
			if it.Floor > 0 {
				s += fmt.Sprintf(" B%d", it.Floor)
			}
			if it.Battle != "" {
				s += fmt.Sprintf(lang.T(" 戦闘 %s"), it.Battle)
			}
			where = append(where, s)
		}
		appendText(&b, "<br>"+e(strings.Join(where, lang.T("、"))))

		if it.Note != "" {
			appendText(&b, "<br>"+e(it.Note))
		}
		if r.Status == "ok" {
			appendText(&b, "<br>"+e(r.Size))
		} else {
			cls := "w"
			if r.Status == "failed" {
				cls = "f"
			}
			appendText(&b, "<br><span class="+cls+">"+e(r.Status)+" "+e(r.Reason)+"</span>")

		}
		appendText(&b, "</td></tr>\n")
	}
	for _, st := range []struct{ status, head string }{{"ok", lang.T("撮れた物")}, {"approval_wait", lang.T("承認待ち（制作記録と素材を確かめて撮り直す）")}, {"failed", lang.T("撮れなかった物")}} {
		if n[st.status] == 0 {
			continue
		}
		appendText(&b, "<h2>"+e(st.head)+"</h2>\n<table>\n")

		for _, r := range rs {
			if r.Status == st.status {
				row(r)
			}
		}
		appendText(&b, "</table>\n")

	}
	return os.WriteFile(filepath.Join(out, "index.html"), []byte(b.String()), 0o600)
}
