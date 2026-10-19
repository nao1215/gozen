package main

import (
	"cmp"
	"errors"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	"image/png"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/image/draw"

	"github.com/nao1215/gozen/cmd/internal/cli"

	"github.com/nao1215/gozen/internal/layer1/memo"

	"github.com/nao1215/gozen/internal/text/catalog"
	lang "github.com/nao1215/gozen/text"
)

type memoEntry struct {
	memo.Memo
	ID    string
	Shot  string
	Thumb string
	N     int
}

func (c *command) memosCmd(args []string, destination string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: gozen memos <dir of the notes, e.g. build/memos>")
	}
	root := args[0]
	if destination == "" {
		destination = filepath.Clean(root) + "-html"
	}
	return cli.Publish(destination, []string{root}, func(stage string) error { return c.renderMemos(root, stage) })
}
func (c *command) renderMemos(root, output string) (err error) {
	images, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, images.Close()) }()
	all, err := memo.All(root)
	if err != nil {
		return err
	}
	var ms []*memoEntry
	for _, a := range all {
		e := &memoEntry{Memo: a.Memo, ID: a.ID}
		day, _, _ := strings.Cut(a.ID, "/")
		if e.Image != "" {
			if !cli.ID(e.Image) || !cli.Relative(a.ID) {
				return fmt.Errorf("invalid memo image path")
			}
			source := day + "/" + e.Image
			e.Shot = "images/" + day + "_" + e.Image
			b, err := fs.ReadFile(images.FS(), source)
			if err != nil {
				return err
			}
			if err := cli.WriteNew(filepath.Join(output, filepath.FromSlash(e.Shot)), b); err != nil {
				return err
			}
			e.Thumb = "thumbs/" + day + "_" + strings.TrimSuffix(e.Image, ".png") + ".jpg"
			if err := thumbnail(filepath.Join(output, e.Shot), filepath.Join(output, e.Thumb), 480); err != nil {
				return fmt.Errorf("%s: %w", e.Shot, err)
			}
		}
		ms = append(ms, e)
	}
	// The page speaks the language the notes were written in; notes from before the
	// language was recorded were written with the Japanese words.
	language := lang.Japanese
	for _, m := range ms {
		if m.Language != "" {
			language = m.Language
			break
		}
	}
	if _, err := catalog.Words(language); err != nil {
		language = lang.English
	}
	if err := lang.Use(language, nil); err != nil {
		return err
	}
	out := filepath.Join(output, "index.html")
	if err := os.WriteFile(out, []byte(memoHTML(ms, language)), 0o600); err != nil {
		return err
	}
	c.out.Printf("%d memos exported\n", len(ms))
	return nil
}

// sourceNames are the engine's keys for who wrote a note (put through lang.T when shown).
var sourceNames = map[string]string{memo.Human: "人", memo.LLM: "LLM", memo.Autoplay: "自動の遊び", memo.Bot: "ボット"}

// sourceTitle is the shown name of who wrote notes of source.
func sourceTitle(source string) string {
	if n, ok := sourceNames[source]; ok {
		return lang.T(n)
	}
	return source
}

func sourceName(m *memoEntry) string {
	n := sourceTitle(m.Source)
	if m.Bot != "" {
		n += fmt.Sprintf(lang.T("（%s）"), m.Bot)
	}
	return n
}

type memoGroup struct {
	key, title string
	place      string
	floor      float64
	dungeon    bool
	ms         []*memoEntry
}

func groupMemos(ms []*memoEntry) []*memoGroup {
	var gs []*memoGroup
	by := map[string]*memoGroup{}
	for _, m := range ms {
		place := cmp.Or(m.Place, m.Location, lang.T("（場所なし）"))
		key, title := place, place
		if m.Pos != nil {
			key = fmt.Sprintf("%s#%03d", place, int(m.Floor))
			title = fmt.Sprintf("%s B%d", place, int(m.Floor))
		}
		g := by[key]
		if g == nil {
			g = &memoGroup{key: key, title: title, place: place, floor: m.Floor, dungeon: m.Pos != nil}
			by[key] = g
			gs = append(gs, g)
		}
		g.ms = append(g.ms, m)
		m.N = len(g.ms)
	}
	slices.SortFunc(gs, func(a, b *memoGroup) int { return cmp.Compare(a.key, b.key) })
	return gs
}

func memoHTML(ms []*memoEntry, language string) string {
	var b strings.Builder
	esc := html.EscapeString
	kinds := map[string]string{}
	var kindOrder, places, sources []string
	for _, m := range ms {
		if _, ok := kinds[m.Kind]; !ok {
			kinds[m.Kind] = m.Label
			kindOrder = append(kindOrder, m.Kind)
		}
		if !slices.Contains(sources, m.Source) {
			sources = append(sources, m.Source)
		}
		p := cmp.Or(m.Place, m.Location, lang.T("（場所なし）"))
		if !slices.Contains(places, p) {
			places = append(places, p)
		}
	}
	appendf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title>`, esc(language), esc(lang.T("テストプレイのメモ")))
	appendText(&b, `<style>
body{font-family:sans-serif;background:#1b1f27;color:#e8edf5;margin:20px}
h1{font-size:22px} h2{font-size:18px;border-bottom:1px solid #445;padding-bottom:4px;margin-top:28px}
.filters{position:sticky;top:0;background:#1b1f27;padding:8px 0;z-index:2}
.filters label{margin-right:12px} select{font-size:15px}
.cards{display:flex;flex-wrap:wrap;gap:14px}
.card{background:#262c38;border-radius:8px;padding:10px;width:480px}
.card img{width:480px;border-radius:4px;display:block}
.kind{display:inline-block;border-radius:4px;padding:1px 8px;font-weight:bold;margin-right:6px}
.text{font-size:17px;margin:6px 0} .meta{font-size:13px;color:#9aa6b8;line-height:1.5}
.line{font-size:13px;color:#c8d2e0;background:#1e232d;padding:4px 6px;border-radius:4px;margin-top:4px}
.pin{font-size:11px;font-weight:bold}
.k-good{background:#2e7d4f}.k-dull{background:#6b6b6b}.k-cold{background:#3b6ea8}.k-hot{background:#b0457a}.k-lost{background:#a07a20}.k-bug{background:#b03a3a}
</style></head><body>
`)

	appendf(&b, "<h1>%s</h1>\n<div class=\"filters\">", esc(fmt.Sprintf(lang.T("テストプレイのメモ（%d 件）"), len(ms))))

	appendf(&b, `<label>%s <select id="place"><option value="">%s</option>`, esc(lang.T("場所")), esc(lang.T("すべて")))

	for _, p := range places {
		appendf(&b, `<option>%s</option>`, esc(p))

	}
	appendText(&b, `</select></label>`)

	appendf(&b, `<label>%s <select id="source"><option value="">%s</option>`, esc(lang.T("書いた人")), esc(lang.T("すべて")))

	for _, s := range sources {
		appendf(&b, `<option value="%s">%s</option>`, esc(s), esc(sourceTitle(s)))

	}
	appendText(&b, `</select></label>`)

	for _, k := range kindOrder {
		appendf(&b, `<label><input type="checkbox" class="kf" value="%s" checked> <span class="kind k-%s">%s</span></label>`, esc(k), esc(k), esc(kinds[k]))

	}
	appendText(&b, "</div>\n")

	for _, g := range groupMemos(ms) {
		appendf(&b, `<section data-place="%s"><h2>%s</h2>`, esc(g.place), esc(fmt.Sprintf(lang.T("%s（%d 件）"), g.title, len(g.ms))))

		if g.dungeon {
			appendText(&b, memoMap(g.ms))

		}
		appendText(&b, `<div class="cards">`)

		for _, m := range slices.Backward(g.ms) {
			memoCard(&b, m, g.dungeon)
		}
		appendText(&b, "</div></section>\n")

	}
	appendText(&b, `<script>
function apply(){
  const p=document.getElementById('place').value;
  const src=document.getElementById('source').value;
  const ks=new Set([...document.querySelectorAll('.kf')].filter(c=>c.checked).map(c=>c.value));
  document.querySelectorAll('section').forEach(s=>{
    let n=0;
    s.querySelectorAll('[data-kind]').forEach(c=>{
      const on=(p===''||s.dataset.place===p)&&ks.has(c.dataset.kind)&&(src===''||c.dataset.source===src);
      c.style.display=on?'':'none'; if(on&&c.classList.contains('card'))n++;
    });
    s.style.display=n?'':'none';
  });
}
document.querySelectorAll('select,input').forEach(e=>e.addEventListener('change',apply));
</script></body></html>
`)

	return b.String()
}

func memoCard(b *strings.Builder, m *memoEntry, pin bool) {
	esc := html.EscapeString
	appendf(b, `<div class="card" data-kind="%s" data-source="%s">`, esc(m.Kind), esc(m.Source))
	if m.Thumb != "" {
		appendf(b, `<a href="%s" target="_blank"><img src="%s" loading="lazy" alt=""></a>`, esc(m.Shot), esc(m.Thumb))
	}
	appendf(b, `<div class="text"><span class="kind k-%s">%s</span>`, esc(m.Kind), esc(m.Label))
	if pin {
		appendf(b, `<span class="pin">#%d</span> `, m.N)
	}
	appendf(b, "%s</div><div class=\"meta\">", esc(cmp.Or(m.Text, lang.T("（一言なし）"))))
	where := cmp.Or(m.Place, m.Location)
	if m.Pos != nil {
		where += fmt.Sprintf(" B%d (%d,%d) %s", int(m.Floor), m.Pos[0], m.Pos[1], m.Facing)
	}
	appendf(b, "%s", esc(fmt.Sprintf(lang.T("%s・%s・%s・起動から %d 分"), sourceName(m), where, strings.Replace(m.Time, "T", " ", 1)[:min(len(m.Time), 16)], int(m.Seconds/60))))
	if m.Event != "" {
		appendf(b, "<br>%s", esc(fmt.Sprintf(lang.T("イベント %s"), m.Event)))
	}
	if len(m.Scenes) > 0 {
		appendf(b, "<br>%s", esc(fmt.Sprintf(lang.T("画面 %s"), strings.Join(m.Scenes, " > "))))
	}
	if len(m.Vars) > 0 {
		var vs []string
		for _, k := range slices.Sorted(maps.Keys(m.Vars)) {
			vs = append(vs, fmt.Sprintf("%s %v", k, m.Vars[k]))
		}
		appendf(b, "<br>%s", esc(strings.Join(vs, lang.T("、"))))
	}
	if m.Version != "" {
		appendf(b, "<br>%s", esc(fmt.Sprintf(lang.T("版 %s"), m.Version)))
	}
	if m.Resume != nil {
		appendf(b, "<br>%s", esc(fmt.Sprintf(lang.T("再開 %s %s"), m.Resume.From, m.Resume.Event)))
	}
	if len(m.Data) > 0 {
		var ds []string
		for _, k := range slices.Sorted(maps.Keys(m.Data)) {
			ds = append(ds, fmt.Sprintf("%s %v", k, m.Data[k]))
		}
		appendf(b, "<br>%s", esc(strings.Join(ds, lang.T("、"))))
	}
	appendf(b, "<br>%s", esc(m.ID))
	appendText(b, "</div>")

	if m.Line != "" {
		line := m.Line
		if m.Who != "" {
			line = m.Who + fmt.Sprintf(lang.T("「%s」"), line)
		}
		appendf(b, `<div class="line">%s</div>`, esc(line))
	}
	appendText(b, "</div>")

}

func memoMap(ms []*memoEntry) string {
	var last *memoEntry
	for _, m := range ms {
		if len(m.Map) > 0 {
			last = m
		}
	}
	if last == nil {
		return ""
	}
	const c = 18.0
	h := len(last.Map)
	w := 0
	for _, r := range last.Map {
		w = max(w, len([]rune(r)))
	}
	var b strings.Builder
	appendf(&b, `<svg width="%g" height="%g" style="background:#11141a;border-radius:6px;margin:6px 0">`, float64(w)*c, float64(h)*c)

	for y, row := range last.Map {
		for x, r := range []rune(row) {
			if r == '#' || r == ' ' {
				continue
			}
			fill := "#3a4252"
			if y < len(last.Seen) && x < len(last.Seen[y]) && last.Seen[y][x] == '1' {
				fill = "#7a8aa6"
			}
			appendf(&b, `<rect x="%g" y="%g" width="%g" height="%g" fill="%s"/>`, float64(x)*c, float64(y)*c, c-1, c-1, fill)

		}
	}
	at := map[[2]int]int{}
	for _, m := range ms {
		if m.Pos == nil {
			continue
		}
		k := [2]int{m.Pos[0], m.Pos[1]}
		x, y := (float64(m.Pos[0])+0.5)*c+float64(at[k])*9, (float64(m.Pos[1])+0.5)*c-float64(at[k])*4
		at[k]++
		appendf(&b, `<g data-kind="%s" data-source="%s"><circle cx="%g" cy="%g" r="8" class="k-%s" fill="%s" stroke="#fff"/><text x="%g" y="%g" font-size="10" fill="#fff" text-anchor="middle">%d</text></g>`,
			html.EscapeString(m.Kind), html.EscapeString(m.Source), x, y, html.EscapeString(m.Kind), pinColor(m.Kind), x, y+3, m.N)

	}
	appendText(&b, "</svg>")

	return b.String()
}

func pinColor(kind string) string {
	switch kind {
	case "good":
		return "#2e7d4f"
	case "dull":
		return "#6b6b6b"
	case "cold":
		return "#3b6ea8"
	case "hot":
		return "#b0457a"
	case "lost":
		return "#a07a20"
	case "bug":
		return "#b03a3a"
	}
	return "#e0a030"
}

func thumbnail(src, dst string, w int) error {
	f, err := os.Open(src) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return err
	}
	img, err := png.Decode(f)
	closeErr := f.Close()
	err = errors.Join(err, closeErr)
	if err != nil {
		return err
	}
	sb := img.Bounds()
	h := max(1, sb.Dy()*w/max(1, sb.Dx()))
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(out, out.Bounds(), img, sb, draw.Src, nil)
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	o, err := os.Create(dst) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return err
	}
	if err := jpeg.Encode(o, out, &jpeg.Options{Quality: 82}); err != nil {
		return errors.Join(err, o.Close())
	}
	return o.Close()
}
