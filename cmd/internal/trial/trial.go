package trial

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"

	"github.com/nao1215/gozen/cmd/internal/cli"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/data"
)

type Result struct {
	Files     map[string][]byte
	Assets    []string
	Kept      map[string][]string
	Removed   map[string][]string
	Gates     int
	Passes    int
	Missing   []string
	Domains   map[string]string
	DomainWhy map[string][]string
	Why       map[string]string
	Gallery   struct{ CGs, Scenes int }
	src       fs.FS
}

func Build(src fs.FS, cfg *Config) (*Result, error) {
	base, err := readFiles(src)
	if err != nil {
		return nil, err
	}

	base["game.yaml"] = del(del(base.game(), "cg_review"), "scene_review")
	all, err := listAssets(src)
	if err != nil {
		return nil, err
	}
	sourceEvents := sortedKeys(defsOf(base)["events"])
	a := newAnalysis(cfg, base)
	res := &Result{src: src}
	if res.Gates, err = a.gates(base); err != nil {
		return nil, err
	}
	a = newAnalysis(cfg, base)

	main, full := a.bottom(), a.bottom()
	var out files
	var kept map[string]map[string]bool
	for pass := 1; ; pass++ {
		res.Passes = pass
		var asides []yaml.MapSlice
		if l, ok := get(base.game(), "asides"); ok {
			ll := sequence(l)
			for _, e := range ll {
				if m, ok := e.(yaml.MapSlice); ok {
					asides = append(asides, m)
				}
			}
		}

		story := (&pruner{a: a, d: main}).prune(base)
		storyAsides := pruneList(a, main, asides)
		storyKept, storyActive := reach(story, nil, storyAsides, nil, nil)
		keepOnly(story, storyKept)
		setGallery(story, nil, storyActive)
		storyAssets, _ := collectAssets(story, src, all, cfg.Keep)
		main2 := a.domains(story, false, main, nil)

		out = (&pruner{a: a, d: full}).prune(base)
		scenes := openScenes(a, out, main2, storyAssets)
		res.Why = map[string]string{}
		var active []yaml.MapSlice
		allow := map[string]bool{}
		for id := range storyKept["events"] {
			allow[id] = true
		}
		for _, sc := range scenes {
			e, _ := get(sc, "event")
			s, _ := str(e)
			allow[s] = true
		}
		kept, active = reach(out, scenes, pruneList(a, full, asides), allow, res.Why)
		keepOnly(out, kept)
		dropCalls(out, kept["events"])
		setGallery(out, scenes, active)
		got, missing := collectAssets(out, src, all, cfg.Keep)
		res.Missing = missing
		filterGallery(out, got)
		res.DomainWhy = map[string][]string{}
		full2 := a.domains(out, true, full, res.DomainWhy)
		for k, v := range main2 {
			full2[k] = full2[k].Union(v)
		}
		if (main2.equal(main) && full2.equal(full)) || pass >= 50 {
			break
		}
		main, full = main2, full2
	}
	overrideGame(out, cfg.Game)

	res.Files = map[string][]byte{}
	for _, p := range out.names() {
		b, err := marshal(out[p])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		res.Files[p] = b
	}
	got, _ := collectAssets(out, src, all, cfg.Keep)
	for p := range got {
		res.Assets = append(res.Assets, p)
	}
	sort.Strings(res.Assets)
	res.Kept, res.Removed = map[string][]string{}, map[string][]string{}
	ds := defsOf(base)
	for _, sec := range deletable {
		for _, id := range sortedKeys(ds[sec]) {
			if kept[sec][id] {
				res.Kept[sec] = append(res.Kept[sec], id)
			} else {
				res.Removed[sec] = append(res.Removed[sec], id)
			}
		}
	}
	for _, id := range sourceEvents {
		if match(cfg.Cut, id) && !contains(res.Removed["events"], id) {
			res.Removed["events"] = append(res.Removed["events"], id)
		}
	}
	sort.Strings(res.Removed["events"])
	res.Domains = map[string]string{}
	for k, v := range full {
		res.Domains[k] = v.String()
	}
	g := galleryOf(out)
	cgs, _ := get(g, "cgs")
	cl := sequence(cgs)
	res.Gallery.CGs, res.Gallery.Scenes = len(cl), len(galleryScenes(out))
	return res, nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func pruneList(a *analysis, d domains, l []yaml.MapSlice) []yaml.MapSlice {
	var out []yaml.MapSlice
	for _, m := range l {
		if w, ok := get(m, "when"); ok && !a.eval(w, d).CanBeTrue() {
			continue
		}
		out = append(out, m)
	}
	return out
}

func galleryOf(fs files) yaml.MapSlice {
	g, _ := get(fs.game(), "gallery")
	gm := mapping(g)
	return gm
}

func openScenes(a *analysis, fs files, main domains, assets map[string]bool) []yaml.MapSlice {
	events := defsOf(fs)["events"]
	var out []yaml.MapSlice
	for _, sc := range galleryScenes(fs) {
		e, _ := get(sc, "event")
		if s, _ := str(e); events[s] == nil {
			continue
		}
		if f, ok := get(sc, "flag"); ok {
			if s, _ := str(f); s != "" {
				if v, ok := main["flag."+s]; ok && !v.CanBeTrue() {
					continue
				}
			}
		}
		if w, ok := get(sc, "when"); ok && !a.eval(w, main).CanBeTrue() {
			continue
		}
		if s, ok := get(sc, "seen"); ok {
			pat, _ := str(s)
			if !anyMatch(cleanPath(pat), assets) {
				continue
			}
		}
		out = append(out, sc)
	}
	return out
}

func anyMatch(pat string, set map[string]bool) bool {
	if !strings.ContainsAny(pat, "*?[") {
		return set[pat]
	}
	for p := range set {
		if ok, err := path.Match(pat, p); err == nil && ok {
			return true
		}
	}
	return false
}

func setGallery(fs files, scenes []yaml.MapSlice, asides []yaml.MapSlice) {
	g := fs.game()
	if gm := galleryOf(fs); gm != nil {
		l := []any{}
		for _, s := range scenes {
			l = append(l, s)
		}
		g = set(g, "gallery", set(gm, "scenes", l))
	}
	if _, ok := get(g, "asides"); ok {
		l := []any{}
		for _, s := range asides {
			l = append(l, s)
		}
		g = set(g, "asides", l)
	}
	fs["game.yaml"] = g
}

func filterGallery(fs files, assets map[string]bool) {
	gm := galleryOf(fs)
	if gm == nil {
		return
	}
	if c, ok := get(gm, "cgs"); ok {
		l := sequence(c)
		keep := []any{}
		for _, e := range l {
			m, ok := e.(yaml.MapSlice)
			if !ok {
				continue
			}
			has := false
			if f, ok := get(m, "file"); ok {
				s, _ := str(f)
				has = assets[cleanPath(s)]
			}
			if fl, ok := get(m, "files"); ok {
				ll := sequence(fl)
				pats := []any{}
				for _, x := range ll {
					s, _ := str(x)
					p := cleanPath(s)
					if anyMatch(p, assets) {
						has = true
					}
					if strings.ContainsAny(p, "*?[") || assets[p] {
						pats = append(pats, x)
					}
				}
				m = set(mapping(cloneNode(m)), "files", pats)
			}
			if has {
				keep = append(keep, m)
			}
		}
		gm = set(gm, "cgs", keep)
	}
	songs := defsOf(fs)["songs"]
	if mp, ok := get(gm, "music_pictures"); ok {
		m := mapping(mp)
		keep := yaml.MapSlice{}
		for _, it := range m {
			if _, ok := songs[keyOf(it.Key)]; !ok {
				continue
			}
			l := sequence(it.Value)
			pics := []any{}
			for _, x := range l {
				if s, _ := str(x); assets[cleanPath(s)] {
					pics = append(pics, x)
				}
			}
			if len(pics) > 0 {
				keep = append(keep, yaml.MapItem{Key: it.Key, Value: pics})
			}
		}
		gm = set(gm, "music_pictures", keep)
	}
	fs["game.yaml"] = set(fs.game(), "gallery", gm)
}

func overrideGame(fs files, over yaml.MapSlice) {
	g := fs.game()
	for _, it := range over {
		k := keyOf(it.Key)
		if k == "text" {
			old, _ := get(g, "text")
			om := mapping(old)
			om = mapping(cloneNode(om))
			if nm, ok := it.Value.(yaml.MapSlice); ok {
				for _, t := range nm {
					om = set(om, keyOf(t.Key), t.Value)
				}
			}
			g = set(g, "text", om)
			continue
		}
		g = set(g, k, cloneNode(it.Value))
	}
	fs["game.yaml"] = g
}

var assetExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".ogg": true, ".wav": true, ".mp3": true, ".ttf": true, ".otf": true, ".txt": true}

func collectAssets(fs files, src fs.FS, all []string, keep []string) (map[string]bool, []string) {
	got := map[string]bool{}
	lost := map[string]bool{}
	exists := map[string]bool{}
	for _, p := range all {
		exists[p] = true
	}
	addPat := func(s string) {
		s = cleanPath(s)
		if strings.ContainsAny(s, "*?[") {
			for _, p := range all {
				if ok, err := path.Match(s, p); err == nil && ok {
					got[p] = true
				}
			}
			return
		}
		if exists[s] {
			got[s] = true
		} else {
			lost[s] = true
		}
	}
	addDir := func(dir string) {
		dir = strings.TrimSuffix(cleanPath(dir), "/") + "/"
		for _, p := range all {
			if strings.HasPrefix(p, dir) {
				got[p] = true
			}
		}
	}
	var walk func(v any, key string)
	walk = func(v any, key string) {
		switch x := v.(type) {
		case yaml.MapSlice:
			for _, it := range x {
				walk(it.Value, keyOf(it.Key))
			}
		case []any:
			for _, e := range x {
				walk(e, key)
			}
		case string:
			s := strings.TrimSpace(x)
			if key == "faces" && !strings.ContainsAny(s, " \n") && !assetExt[strings.ToLower(path.Ext(s))] && s != "" {
				addDir(s)
				return
			}
			if assetExt[strings.ToLower(path.Ext(s))] && !strings.ContainsAny(s, " \n{") {
				addPat(s)
			}
		}
	}
	for _, p := range fs.names() {
		v := fs[p]
		if p == "game.yaml" {
			g := fs.game()
			if gm := galleryOf(fs); gm != nil {
				g = set(mapping(cloneNode(g)), "gallery", yaml.MapSlice{})
				for _, sc := range galleryScenes(fs) {
					for _, k := range []string{"picture", "background"} {
						if pv, ok := get(sc, k); ok {
							walk(pv, k)
						}
					}
				}
			}
			v = g
		}
		walk(v, "")
	}

	for _, s := range defsOf(fs)["songs"] {
		m := mapping(s)
		d, ok := get(m, "dir")
		if !ok {
			continue
		}
		dir, _ := str(d)
		moods, _ := get(m, "moods")
		mm := mapping(moods)
		for _, it := range mm {
			f, _ := str(it.Value)
			addPat(path.Join(dir, f+".ogg"))
		}
	}
	addDir("sfx")
	for _, k := range keep {
		addPat(k)
	}
	var missing []string
	for p := range lost {
		missing = append(missing, p)
	}
	sort.Strings(missing)
	return got, missing
}

func (r *Result) FS() fstest.MapFS {
	m := fstest.MapFS{}
	for p, b := range r.Files {
		m[p] = &fstest.MapFile{Data: b}
	}
	for _, a := range r.Assets {
		m["assets/"+a] = &fstest.MapFile{}
	}

	for _, a := range r.Assets {
		if strings.HasSuffix(a, ".txt") {
			if b, err := r.text(a); err == nil {
				m["assets/"+a] = &fstest.MapFile{Data: b}
			}
		}
	}
	return m
}

func (r *Result) text(a string) ([]byte, error) {
	b, err := fs.ReadFile(r.src, "assets/"+a)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, x := range r.Assets {
		have[x] = true
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") && assetExt[strings.ToLower(path.Ext(t))] && !have[cleanPath(t)] {
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n")), nil
}

func (r *Result) Check() error {
	_, err := data.Load(r.FS(), ".")
	return err
}

func (r *Result) Write(srcDir, dir string) error {
	if len(r.Missing) > 0 {
		return fmt.Errorf("trial: missing assets: %v", r.Missing)
	}
	for p := range r.Files {
		if !cli.Relative(p) {
			return fmt.Errorf("trial: invalid file path %q", p)
		}
	}
	for _, p := range r.Assets {
		if !cli.Relative(p) {
			return fmt.Errorf("trial: invalid asset path %q", p)
		}
	}
	for p, b := range r.Files {
		if err := writeFile(filepath.Join(dir, p), b); err != nil {
			return err
		}
	}
	for _, a := range r.Assets {
		var b []byte
		var err error
		if strings.HasSuffix(a, ".txt") {
			b, err = r.text(a)
		} else {
			b, err = fs.ReadFile(os.DirFS(srcDir), "assets/"+a)
		}
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(dir, "assets", filepath.FromSlash(a)), b); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}
