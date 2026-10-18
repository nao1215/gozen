package assetfs

import (
	"os"
	"path/filepath"
	"testing"
)

// 配布と撮影は、同じ承認・ハッシュ・必須差し替えで許可と拒否を決める。
func TestApprovalGateAndTree(t *testing.T) {
	for _, tc := range []struct {
		name                                                                  string
		approved, required, replacement, stale, identical, obsolete, unlisted bool
		want                                                                  bool
	}{
		{name: "差し替え済み", approved: true, required: true, replacement: true, want: true},
		{name: "元のまま承認", approved: true, want: true},
		{name: "未検査", unlisted: true},
		{name: "要確認", required: true, replacement: true},
		{name: "未処理", required: true},
		{name: "元画像更新", approved: true, required: true, replacement: true, stale: true},
		{name: "古い出力", approved: true, required: true, replacement: true, obsolete: true},
		{name: "差し替え欠落", approved: true, required: true},
		{name: "中身が元と同じ", approved: true, required: true, replacement: true, identical: true},
		{name: "元画像の承認が古い", approved: true, stale: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, top, out := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "out")
			write(t, src, map[string]string{"assets/a.png": "元画像"})
			if tc.replacement {
				body := "加工画像"
				if tc.identical {
					body = "元画像"
				}
				write(t, top, map[string]string{"a.png": body})
			}
			a := Approval{File: "a.png", Source: sum("元画像"), Approved: tc.approved, Required: tc.required}
			if tc.stale {
				a.Source = sum("前の画像")
			}
			records := []Approval{a}
			if tc.unlisted {
				records = []Approval{}
			}
			outputs := []string{"a.png"}
			if tc.obsolete {
				outputs = []string{}
			}
			gate := Gate{Base: os.DirFS(src), Top: os.DirFS(top), Prefix: "assets", Records: records, Outputs: outputs, Watch: []string{""}}
			err := gate.Check("a.png")
			if (err == nil) != tc.want {
				t.Fatalf("撮影の照合 %v", err)
			}
			// 未掲載は配布の参照一覧検査が拒否する。他は Tree 自身でも拒否する。
			if !tc.unlisted {
				_, err = (Tree{Src: src, Top: top, Dst: out, Prefix: "assets", Outputs: outputs}).Build(records)
				if (err == nil) != tc.want {
					t.Fatalf("配布の照合 %v", err)
				}
			}
		})
	}
}

// 承認の入れ子の項目名は、実際の差し替えでも同じように解釈する。
func TestOverlayNestedManifest(t *testing.T) {
	src, top := t.TempDir(), t.TempDir()
	write(t, src, map[string]string{"assets/a.png": "元画像"})
	write(t, top, map[string]string{"a.png": "加工画像", "records.json": `[{"input":{"file":"a.png","hash":"` + sum("元画像") + `"}}]`})
	o := overlay(t, os.DirFS(src), "assets", os.DirFS(top)).useManifest(Manifest{Name: "records.json", File: "input.file", Sum: "input.hash"})
	b, err := o.ReadFile("assets/a.png")
	if err != nil || string(b) != "加工画像" {
		t.Fatalf("入れ子の記録で差し替えが一致しない %q %v", b, err)
	}
}
