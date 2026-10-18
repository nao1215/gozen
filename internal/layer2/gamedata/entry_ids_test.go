package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// 表示が同じでも ID が違えばよい。重複は未表示の枝も含めて検査する。
func TestStableEntryAndChoiceIDs(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"行動", `locations: {station: {name: 駅, actions: [{id: take, label: 受領, do: []}, {id: take, label: 受領, do: []}]}}`, "duplicate entry id"},
		{"人との衝突", `locations: {station: {name: 駅, people: [{id: take, name: 係員, talk: []}], actions: [{id: take, label: 受領, do: []}]}}`, "duplicate entry id"},
		{"選択肢", `events: {select: {do: [{choice: [{id: take, label: 受領, do: []}, {id: take, label: 受領, when: "false", do: []}]}]}}`, "duplicate choice id"},
		{"別の問い", `events: {select: {do: [{choice: [{id: take, label: 受領, do: []}]}, {choice: [{id: take, label: 受領, do: []}]}]}}`, ""},
		{"同じ表示", `events: {select: {do: [{choice: [{id: take, label: 受領, do: []}, {id: leave, label: 受領, do: []}]}]}}`, ""},
		{"旧設定", `events: {select: {do: [{choice: [{label: 受領, do: []}, {label: 受領, do: []}]}]}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{"data/ids.yaml": tc.src})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("診断 %v", err)
			}
		})
	}
}
