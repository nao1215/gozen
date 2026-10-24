package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"
	"github.com/nao1215/gozen/internal/layer2/gamedata"

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

	Script string `yaml:"script"`
	Wait   int    `yaml:"wait"`
}

type shotResult struct {
	N        int      `json:"n"`
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	Reason   string   `json:"reason,omitempty"`
	PNG      string   `json:"png,omitempty"`
	JPEG     string   `json:"jpeg,omitempty"`
	Size     string   `json:"size,omitempty"`
	Line     string   `json:"line,omitempty"`
	Pictures []string `json:"pictures,omitempty"`
	item     shotItem
}

func (c *command) shots(args []string) (err error) {
	fl := clihelp.NewFlagSet("gozen play shots")
	bin := fl.String("bin", "", "the game binary `FILE` built with the Gozen runner; required")
	overlay := fl.String("overlay", "", "take approved replaced assets from this `DIR` and check approval")
	raw := fl.Bool("raw", false, "take the shots with the original assets without checking approval (for local review only)")
	out := fl.StringP("out", "o", "", "write the shots to this new `DIR` (default: build/shots/<date-time>)")
	only := fl.String("only", "", "take only these shot `IDS`, separated by commas")
	timeout := fl.Duration("timeout", 4*time.Minute, "the time limit of one shot, such as 90s or 2m")
	if err := c.parse(fl, args); err != nil {
		return err
	}
	if fl.NArg() != 2 || *bin == "" || (*overlay == "") == !*raw {
		return cliio.Invalid(diag.OptionConflict, "play shots requires --bin and exactly one of --overlay DIR or --raw")
	}
	if *timeout <= 0 {
		return cliio.Invalid(diag.BadValue, "--timeout must be positive")
	}
	gameDir, listPath := fl.Arg(0), fl.Arg(1)
	*bin, err = exec.LookPath(*bin)
	if err != nil {
		return fmt.Errorf("shots: game binary unavailable: %w; see https://github.com/nao1215/gozen#gozen", err)
	}
	*bin, err = filepath.Abs(*bin)
	if err != nil {
		return err
	}
	gameDir, err = filepath.Abs(gameDir)
	if err := gamedata.UseWords(os.DirFS(gameDir)); err != nil {
		return err
	}
	if err != nil {
		return err
	}
	list, err := readShotList(listPath)
	if err != nil {
		return err
	}
	if *out == "" {
		*out = filepath.Join("build", "shots", time.Now().Format("20060102-150405"))
	}
	var gate *assetfs.Gate
	if !*raw {
		policy, err := assetpolicy.Load(os.DirFS(gameDir), "asset-policy.yaml")
		if err != nil {
			return err
		}
		records, outputs, err := policy.ReadRecords(os.DirFS(*overlay))
		if err != nil {
			return err
		}
		gate = &assetfs.Gate{Base: os.DirFS(gameDir), Top: os.DirFS(*overlay), Prefix: "assets", Records: records, Outputs: outputs, Watch: list.Watch}
		cov, err := assetfs.New(gate.Base, gate.Top, assetfs.Config{Prefix: "assets", Manifest: assetfs.Manifest{Name: policy.Report, File: policy.Records.File, Sum: policy.Records.Source}})
		if err != nil {
			return err
		}

		tmp, createErr := os.MkdirTemp("", "gozen-shots-")
		if createErr != nil {
			return createErr
		}
		defer func() { err = errors.Join(err, os.RemoveAll(tmp)) }()
		gameDir = filepath.Join(tmp, "game")
		if err := os.CopyFS(gameDir, cov); err != nil {
			return err
		}
	}
	want := map[string]bool{}
	for _, id := range split(*only) {
		want[id] = true
	}
	for id := range want {
		found := false
		for _, it := range list.Shots {
			found = found || it.ID == id
		}
		if !found {
			return cliio.Invalid(diag.UnknownName, "--only: unknown shot %q", id)
		}
	}
	if err := cli.Destination(*out, fl.Arg(0), *overlay, listPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o750); err != nil {
		return err
	}
	if err := os.Mkdir(*out, 0o750); err != nil {
		return err
	}
	var results []shotResult
	for i, it := range list.Shots {
		r := shotResult{N: i + 1, ID: it.ID, item: it}
		name := fmt.Sprintf("%02d_%s", i+1, it.ID)
		var approvalErr error
		if gate != nil && it.Picture != "" {
			approvalErr = gate.Check(it.Picture)
		}
		switch {
		case len(want) > 0 && !want[it.ID]:
			r.Status = "skipped"
		case approvalErr != nil:
			r.Status, r.Reason = "approval_wait", approvalErr.Error()
		default:
			shootOne(&r, it, list, *bin, gameDir, *out, name, *timeout, gate)
		}
		if r.Status != "skipped" {
			c.out.Printf("%-4s %-28s %-11s %s\n", strconv.Itoa(r.N), it.ID, r.Status, r.Reason)
		}
		results = append(results, r)
	}
	if err := writeShotIndex(*out, list, results, *raw); err != nil {
		return err
	}
	n := map[string]int{}
	for _, r := range results {
		n[r.Status]++
	}
	c.out.Printf("%s: %d shot, %d awaiting approval, %d failed (%s)\n", *out, n["ok"], n["approval_wait"], n["failed"], filepath.Join(*out, "index.html"))
	if n["failed"] > 0 || n["approval_wait"] > 0 {
		return fmt.Errorf("shots: %d failed, %d awaiting approval", n["failed"], n["approval_wait"])
	}
	return nil
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

func shootOne(r *shotResult, it shotItem, list *shotList, bin, gameDir, out, name string, timeout time.Duration, gate *assetfs.Gate) {
	fail := func(f string, a ...any) { r.Status, r.Reason = "failed", fmt.Sprintf(f, a...) }
	work, err := filepath.Abs(filepath.Join(out, "_work", name))
	if err != nil {
		fail("%v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(work), 0o750); err != nil {
		fail("%v", err)
		return
	}
	if err := os.Mkdir(work, 0o750); err != nil {
		fail("%v", err)
		return
	}
	wait := it.Wait
	if wait == 0 {
		wait = 120
	}
	script := strings.TrimSpace(it.Script) + fmt.Sprintf("\nwait %d\nshot shot\nquit\n", wait)
	if err := os.WriteFile(filepath.Join(work, "script.txt"), []byte(script), 0o600); err != nil {
		fail("%v", err)
		return
	}
	seed := it.Seed
	if seed == 0 {
		seed = list.Seed
	}
	a := []string{
		"--dev", "--dir", gameDir, "--mute", "--seed", strconv.FormatUint(seed, 10), "--window", "1",
		"--save-dir", filepath.Join(work, "save"), "--memos", filepath.Join(work, "memos"),
		"--script", filepath.Join(work, "script.txt"), "--shots", work, "--trace", filepath.Join(work, "trace.jsonl"),
	}
	add := func(k, v string) {
		if v != "" {
			a = append(a, k, v)
		}
	}
	add("--scene", it.Scene)
	add("--event", it.Event)
	add("--picture", it.Picture)
	if it.Line > 0 {
		add("--line", strconv.Itoa(it.Line))
	}
	var ans []string
	for _, n := range it.Answers {
		ans = append(ans, strconv.Itoa(n))
	}
	add("--answers", strings.Join(ans, ","))
	add("--start", it.Start)
	add("--party", strings.Join(it.Party, ","))
	if it.Level > 0 {
		add("--level", strconv.Itoa(it.Level))
	}
	if it.Floor > 0 {
		add("--floor", strconv.Itoa(it.Floor))
	}
	add("--flags", strings.Join(it.Flags, ","))
	add("--battle", it.Battle)
	if it.Calm {
		a = append(a, "--calm")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, a...)                       //nolint:gosec // The user explicitly selects the game executable; arguments are passed without a shell.
	stdout, err := os.Create(filepath.Join(work, "game.stdout.log")) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		fail("%v", err)
		return
	}
	stderr, err := os.Create(filepath.Join(work, "game.stderr.log")) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		fail("%v", errors.Join(err, stdout.Close()))
		return
	}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := errors.Join(cmd.Run(), stdout.Close(), stderr.Close()); err != nil {
		fail("game capture failed: %v; see %s. Graphics runtime requirements: https://github.com/nao1215/gozen#gozen", err, work)
		return
	}
	tr, err := readShotTrace(filepath.Join(work, "trace.jsonl"))
	if err != nil {
		fail("%v", err)
		return
	}
	r.Pictures, r.Line = tr.pictures, tr.line
	if (it.Scene != "" || it.Event != "") && tr.ready != "line" {
		fail("did not reach the line (%s; check line %d and picture %s)", orDash(tr.ready), it.Line, orDash(it.Picture))
		return
	}
	png0 := filepath.Join(work, "shot.png")
	if _, err := os.Stat(png0); err != nil {
		fail("could not capture the screen (%s)", filepath.Join(work, "game.stderr.log"))
		return
	}
	if gate != nil {
		for _, p := range tr.pictures {
			if err := gate.Check(p); err != nil {
				if e := os.Remove(png0); e != nil {
					fail("%v", e)
					return
				}
				r.Status, r.Reason = "approval_wait", err.Error()
				return
			}
		}
	}
	img, err := readPNG(png0)
	if err != nil {
		fail("%v", err)
		return
	}
	b := img.Bounds()
	r.Size = fmt.Sprintf("%dx%d", b.Dx(), b.Dy())
	r.PNG, r.JPEG = name+".png", name+".jpg"
	if err := os.Rename(png0, filepath.Join(out, r.PNG)); err != nil {
		fail("%v", err)
		return
	}
	if err := writeJPEG(filepath.Join(out, r.JPEG), img, 90); err != nil {
		fail("%v", err)
		return
	}
	r.Status = "ok"
}

type shotTrace struct {
	ready    string
	pictures []string
	line     string
}

func readShotTrace(p string) (result shotTrace, err error) {
	var t shotTrace
	f, err := os.Open(p) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return t, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var l struct {
			T        string   `json:"t"`
			Why      string   `json:"why"`
			Pictures []string `json:"pictures"`
			Text     string   `json:"text"`
		}
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return t, err
		}
		switch l.T {
		case "from_memo_ready":
			t.ready = l.Why
		case "view":
			for _, p := range l.Pictures {
				p = strings.TrimPrefix(p, "assets/")
				if !seen[p] {
					seen[p] = true
					t.pictures = append(t.pictures, p)
				}
			}
		case "say":
			t.line = l.Text
		}
	}
	return t, sc.Err()
}

func readPNG(p string) (result image.Image, err error) {
	f, err := os.Open(p) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	return png.Decode(f)
}

func writeJPEG(p string, img image.Image, q int) error {
	f, err := os.Create(p) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return err
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: q}); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
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
			appendf(&b, "<a href=%q><img src=%q loading=lazy></a>", r.PNG, r.JPEG)
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
			appendText(&b, "<br>"+e(r.Size)+" <a href="+strconv.Quote(r.JPEG)+">JPEG</a>")
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
