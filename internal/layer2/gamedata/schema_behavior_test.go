package gamedata_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// Compare against representative values from a real load so that list defaults are not just the type's zero value.
func TestSchemaDefaultBehavior(t *testing.T) {
	raw, err := os.ReadFile("../../../data/schema/types.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Contract struct {
					Default string `json:"default"`
				} `json:"x-contract"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{0, 1} {
		t.Run(fmt.Sprintf("版%d", version), func(t *testing.T) {
			db := testgame.Load(t, map[string]string{"game.yaml": fmt.Sprintf("schema_version: %d\nid: neutral\nstart: {location: camp, party: [scout]}\n", version)})
			for _, sample := range []struct {
				typ, key string
				value    any
			}{
				{"Game", "max_party", db.Game.MaxParty}, {"Screen", "width", db.Game.Screen.Width}, {"Screen", "height", db.Game.Screen.Height},
				{"BattleCfg", "variance", db.Game.Battle.Variance}, {"BattleCfg", "critical_rate", db.Game.Battle.CriticalRate},
				{"BattleCfg", "guard_rate", db.Game.Battle.GuardRate}, {"BattleCfg", "front_weight", *db.Game.Battle.FrontWeight}, {"BattleCfg", "back_weight", *db.Game.Battle.BackWeight},
				{"Class", "sp", db.Classes["ranger"].SP}, {"Class", "max_level", db.Classes["ranger"].Max},
				{"Actor", "level", db.Actors["scout"].Level}, {"Actor", "face", db.Actors["scout"].Face},
				{"Enemy", "actions", db.Enemies["mote"].Actions}, {"Enemy", "scale", db.Enemies["mote"].Scale},
				{"Usage", "hits", db.Skills["attack"].Hits}, {"Music", "volume", db.Music.Volume},
				{"Terms", "currency", db.Game.Terms.Currency}, {"Terms", "level", db.Game.Terms.Level},
			} {
				want := schema.Defs[sample.typ].Properties[sample.key].Contract.Default
				if got := fmt.Sprint(sample.value); got != want {
					t.Errorf("%s.%s の実効値 %s が一覧 %s と違う", sample.typ, sample.key, got, want)
				}
			}
			if db.Game.Battle.Critical.Set() || db.Game.Battle.Escape.Set() || db.Game.Battle.Speed.Set() {
				t.Fatal("省略した戦闘の式が書き換わった")
			}
			if db.Items["potion"].Use.Scope != "one_ally" || db.Skills["attack"].Scope != "one_enemy" || !reflect.DeepEqual(db.Items["potion"].Use.Where, []string{"battle", "menu"}) {
				t.Fatal("品と技の文脈別の既定値が違う")
			}
		})
	}
	// Existing detailed presets that had no tag are also kept with an explicit tag and schema.
	for _, src := range []string{"weakest", "{preset: weakest}"} {
		var target gamedata.Target
		if err := yaml.UnmarshalWithOptions([]byte(src), &target, yaml.Strict()); err != nil {
			t.Fatal(err)
		}
		if target.Preset != "weakest" {
			t.Fatalf("既存の略記を読めない %s", src)
		}
	}
}
