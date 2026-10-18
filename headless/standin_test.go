package headless_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/nao1215/gozen/headless"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestStandInImageRecords は仮背景の立ち絵表示を実画面と同じ値で記録する。
func TestStandInImageRecords(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":               testgame.Game + "\nstand_ins: cg/stand_in.txt\n",
		"assets/cg/stand_in.txt":  "# 仮背景\ncg/draft.png\n",
		"assets/cg/draft.png":     "png",
		"assets/cg/draft.002.png": "png",
		"assets/cg/finished.png":  "png",
	})
	for _, tt := range []struct {
		name string
		cg   data.CG
		want []headless.Step
	}{
		{"仮背景", data.CG{File: "cg/draft.png"}, []headless.Step{{Kind: "image", File: "cg/draft.png", Faces: true}}},
		{"仮背景の差分", data.CG{File: "cg/draft.002.png"}, []headless.Step{{Kind: "image", File: "cg/draft.002.png", Faces: true}}},
		{"ページ選択", data.CG{Files: []string{"cg/draft.png", "cg/draft.*.png"}, Page: 2}, []headless.Step{{Kind: "image", File: "cg/draft.002.png", Faces: true}}},
		{"連続表示", data.CG{Files: []string{"cg/draft.png", "cg/draft.*.png", "cg/finished.png"}}, []headless.Step{
			{Kind: "image", File: "cg/draft.png", Faces: true},
			{Kind: "image", File: "cg/draft.002.png", Faces: true},
			{Kind: "image", File: "cg/finished.png"},
		}},
		{"完成画像", data.CG{File: "cg/finished.png"}, []headless.Step{{Kind: "image", File: "cg/finished.png"}}},
		{"明示した立ち絵表示", data.CG{File: "cg/finished.png", Faces: true}, []headless.Step{{Kind: "image", File: "cg/finished.png", Faces: true}}},
		{"画像消去", data.CG{}, []headless.Step{{Kind: "image"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.cg
			cmds := []data.Command{{Image: &tt.cg}}
			d := headless.New(db, testgame.State(db, 1))
			d.Record = true
			if err := d.Run(context.Background(), cmds); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(d.Steps, tt.want) {
				t.Errorf("通常実行の記録 %v、期待 %v", d.Steps, tt.want)
			}
			paths := headless.Explore(db, testgame.State(db, 1), cmds, headless.Config{})
			if len(paths) != 1 {
				t.Fatalf("経路数 %d、期待 1", len(paths))
			}
			if len(paths[0].Errs) != 0 {
				t.Fatal(paths[0].Errs)
			}
			if !reflect.DeepEqual(paths[0].Steps, tt.want) {
				t.Errorf("経路探索の記録 %v、期待 %v", paths[0].Steps, tt.want)
			}
			if !reflect.DeepEqual(tt.cg, original) {
				t.Error("元の画像命令を書き換えた")
			}
		})
	}
}
