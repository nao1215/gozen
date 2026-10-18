package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

func diagnosticFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte(definitionGame)},
		"data/base.yaml": {Data: []byte(definitionData)},
	}
	for name, src := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(src)}
	}
	return fsys
}

func wantDiagnostics(t *testing.T, files map[string]string, want ...string) (*DB, string) {
	t.Helper()
	db, err := Load(diagnosticFS(files))
	if err == nil {
		t.Fatal("不正な入力を受け入れた")
	}
	for _, s := range want {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("診断に %q が無い。\n%v", s, err)
		}
	}
	return db, err.Error()
}

func TestSemanticDiagnosticLocations(t *testing.T) {
	t.Run("別ファイルと入れ子の命令・参照・式", func(t *testing.T) {
		files := map[string]string{
			"data/a.yaml": "events:\n  nested:\n    do:\n      - choice:\n          - label: test\n            do:\n              - if: true\n                then:\n                  - battle: missing_troop\n                    on_win:\n                      - call: missing_event\n                    on_lose:\n                      - gold: self.hp\n                    on_escape:\n                      - say: test\n                        fighters: [scout]\n",
			"data/z.yml":  "events:\n  other:\n    do:\n      - move: missing_place\n",
		}
		db, first := wantDiagnostics(t, files,
			"data/a.yaml:9:21: events.nested.do[0].choice[0].do[0].then[0].battle: unknown troop",
			"data/a.yaml:11:25: events.nested.do[0].choice[0].do[0].then[0].on_win[0].call: unknown event",
			"data/a.yaml:13:25: events.nested.do[0].choice[0].do[0].then[0].on_lose[0].gold:",
			"data/a.yaml:15:25: events.nested.do[0].choice[0].do[0].then[0].on_escape[0]: fighters belongs to battle",
			"data/z.yml:4:9: events.other.do[0].move: unknown location")
		if db == nil {
			t.Fatal("意味検査で DB が失われた")
		}
		// 旧データを再検査しても位置は変わらず、マップの走査順でも表示が揺れない。
		for i := 0; i < 20; i++ {
			if got := joinDiagnostics(db.Check()).Error(); got != first {
				t.Fatalf("診断の順序か位置が変わった。\n%s\n%s", first, got)
			}
		}
	})
	for _, tc := range []struct{ name, src, want string }{
		{"未知の項目", "events:\n  sample:\n    doo: []\n", "data/z.yaml:3:5: events.sample.doo: unknown field"},
		{"式の構文", "events:\n  sample:\n    do:\n      - if: '1 +'\n        then: []\n", "data/z.yaml:4:13: events.sample.do[0].if: \"1 +\": unexpected end"},
		{"独自の型のエラー", "events:\n  sample:\n    do:\n      - image:\n          file: picture.png\n          tint: wrong\n", "data/z.yaml:6:17: events.sample.do[0].image.tint: color"},
		{"空の定義", "items:\n  sample: null\n", "data/z.yaml:2:3: items.sample: expected"},
		{"型の不一致", "actors:\n  sample:\n    class: ranger\n    level: wrong\n", "data/z.yaml:4:12: actors.sample.level:"},
		{"構文", "events: [\n", "data/z.yaml:1:9: $:"},
		{"ブロックの式", "events:\n  sample:\n    when: |\n      self.hp > 0\n", "data/z.yaml:3:5: events.sample.when:"},
		{"補完前の必須値", "actors:\n  sample: {}\n", "data/z.yaml:2:3: actors.sample: class is missing"},
	} {
		t.Run(tc.name, func(t *testing.T) { wantDiagnostics(t, map[string]string{"data/z.yaml": tc.src}, tc.want) })
	}
	t.Run("全体設定と数値の型", func(t *testing.T) {
		wantDiagnostics(t, map[string]string{"game.yaml": "start:\n  location: missing\n  party: [scout]\n"}, "game.yaml:2:3: game.yaml start.location: unknown location")
		wantDiagnostics(t, map[string]string{"game.yaml": "schema_version: 1\nid: neutral\nmax_party: '4'\n" + definitionGame}, "game.yaml:3:1: game.yaml.max_party: expected")
		wantDiagnostics(t, map[string]string{"game.yaml": "schema_version: 99\n" + definitionGame}, "game.yaml:1:1: game.yaml schema_version:")
	})
	t.Run("合成の通し番号と音楽", func(t *testing.T) {
		wantDiagnostics(t, map[string]string{
			"data/a.yaml": "items: {ore: {slot: weapon, kind: sword}}\nfusions:\n  - from: [ore, ore]\n    to: ore\n",
			"data/z.yaml": "fusions:\n  - from: [ore, ore]\n    to: missing\nmusic:\n  songs:\n    tune:\n      file: missing.ogg\n",
		}, "data/z.yaml:3:5: fusions[1].to: unknown item", "data/z.yaml:7:7: music.songs.tune.file: no file")
	})
	t.Run("アンカーの参照と上書き", func(t *testing.T) {
		wantDiagnostics(t, map[string]string{"data/z.yaml": "events:\n  original: &base\n    when: self.hp\n  copied:\n    <<: *base\n"}, "data/z.yaml:3:5: events.original.when:", "data/z.yaml:3:5: events.copied.when:")
	})
	t.Run("アンカーの不正な組合せの既存診断", func(t *testing.T) {
		wantDiagnostics(t, map[string]string{"data/z.yaml": "events:\n  original: &base\n    when: true\n  copied:\n    when: self.hp\n    <<: *base\n"}, "data/z.yaml:3:11: events.copied.when: duplicate key")
		wantDiagnostics(t, map[string]string{"data/z.yaml": "events:\n  original: &base\n    when: true\n  copied:\n    <<: [*base]\n"}, "data/z.yaml:5:9: events.copied.<<: sequence was used where mapping is expected")
	})
	t.Run("マップ内の診断順序", func(t *testing.T) {
		db, first := wantDiagnostics(t, map[string]string{"data/z.yaml": "events:\n  sample:\n    do:\n      - set: {var.z: wrong_z, var.a: wrong_a, var.m: wrong_m}\n"},
			"events.sample.do[0].set.var.z:", "events.sample.do[0].set.var.a:", "events.sample.do[0].set.var.m:")
		for i := 0; i < 30; i++ {
			if got := joinDiagnostics(db.Check()).Error(); got != first {
				t.Fatalf("マップ内の診断順序が変わった。\n%s\n%s", first, got)
			}
		}
	})
	t.Run("台詞の実際のパス", func(t *testing.T) {
		wantDiagnostics(t, map[string]string{"data/z.yaml": "events:\n  sample:\n    do:\n      - say:\n          text: test\n          side: wrong\n"}, "data/z.yaml:6:11: events.sample.do[0].say.side:")
	})
}

func TestDuplicateDefinitionLocations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"別ファイル", map[string]string{"data/a.yaml": "items:\n  sample: {}\n", "data/z.yaml": "items:\n  sample: {}\n"}, []string{"data/z.yaml:2:3: items.sample: items.sample is defined twice", "previous definition at data/a.yaml:2:3"}},
		{"同じファイル", map[string]string{"data/z.yaml": "items:\n  sample: {}\n  sample: {}\n"}, []string{"data/z.yaml:3:3: items.sample: items.sample is defined twice", "previous definition at data/z.yaml:2:3"}},
		{"フローと引用したキー", map[string]string{"data/z.yaml": "items: {'sample': {}, sample: {}}\n"}, []string{"data/z.yaml:1:23: items.sample:", "previous definition at data/z.yaml:1:9"}},
		{"入れ子のキー", map[string]string{"data/z.yaml": "events:\n  sample:\n    do:\n      - if: true\n        then: []\n        then: []\n"}, []string{"data/z.yaml:6:9: events.sample.do[0].then:", "previous definition at data/z.yaml:5:9"}},
		{"空の音楽の別ファイル", map[string]string{"data/a.yaml": "music: {}\n", "data/z.yaml": "music: {}\n"}, []string{"data/z.yaml:1:1: music: music is defined twice", "previous definition at data/a.yaml:1:1"}},
		{"全体設定", map[string]string{"game.yaml": "schema_version: 0\nschema_version: 1\n"}, []string{"game.yaml:2:1: schema_version:", "previous definition at game.yaml:1:1"}},
	} {
		t.Run(tc.name, func(t *testing.T) { wantDiagnostics(t, tc.files, tc.want...) })
	}
}
