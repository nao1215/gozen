package gamedata_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestExplicitZeroBattleAndMusic(t *testing.T) {
	fields := []struct {
		name               string
		fallback, positive float64
		get                func(*data.DB) float64
	}{
		{"variance", 0.1, 0.2, func(db *data.DB) float64 { return db.Game.Battle.Variance }},
		{"critical_rate", 1.5, 2, func(db *data.DB) float64 { return db.Game.Battle.CriticalRate }},
		{"guard_rate", 0.5, 0.25, func(db *data.DB) float64 { return db.Game.Battle.GuardRate }},
		{"front_weight", 4, 2, func(db *data.DB) float64 { return *db.Game.Battle.FrontWeight }},
		{"back_weight", 1, 2, func(db *data.DB) float64 { return *db.Game.Battle.BackWeight }},
		{"volume", 0.7, 0.4, func(db *data.DB) float64 { return db.Music.Volume }},
	}
	load := func(version int, field, value string) (*data.DB, error) {
		game := fmt.Sprintf("schema_version: %d\nid: neutral\n", version) + strings.Replace(testgame.Game, "  variance: 0.0001\n", "", 1)
		extra := map[string]string{}
		if field == "volume" {
			extra["data/music.yaml"] = "music:\n  songs: {}\n"
			if value != "" {
				extra["data/music.yaml"] += "  volume: " + value + "\n"
			}
		} else if value != "" {
			game = strings.Replace(game, "battle:\n", "battle:\n  "+field+": "+value+"\n", 1)
		}
		extra["game.yaml"] = game
		return testgame.TryLoad(extra)
	}
	for _, field := range fields {
		for _, version := range []int{0, 1} {
			for _, value := range []string{"", "0", fmt.Sprint(field.positive), "-0.1", ".nan", ".inf", "-.inf", "null"} {
				t.Run(fmt.Sprintf("%s/版%d/%s", field.name, version, value), func(t *testing.T) {
					db, err := load(version, field.name, value)
					if value == "-0.1" || strings.HasPrefix(value, ".") || value == "-.inf" || value == "null" {
						if err == nil || !strings.Contains(err.Error(), field.name) {
							t.Fatalf("不正値の診断がない: %v", err)
						}
						where := "game.yaml"
						if field.name == "volume" {
							where = "data/music.yaml"
						}
						if !strings.Contains(err.Error(), where) {
							t.Fatalf("診断にファイルがない: %v", err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					want := field.positive
					switch value {
					case "":
						want = field.fallback
					case "0":
						want = 0
						if version == 0 && field.name != "front_weight" && field.name != "back_weight" {
							want = field.fallback
						}
					}
					if got := field.get(db); got != want {
						t.Fatalf("実効値 %v、期待 %v", got, want)
					}
				})
			}
		}
		t.Run(field.name+"/旧版のゼロを新版へ移す", func(t *testing.T) {
			old, err := load(0, field.name, "0")
			if err != nil {
				t.Fatal(err)
			}
			migrated, err := load(1, field.name, fmt.Sprint(field.get(old)))
			if err != nil {
				t.Fatal(err)
			}
			if field.get(old) != field.get(migrated) {
				t.Fatal("移行で実効値が変わった")
			}
		})
	}
	for _, version := range []int{0, 1} {
		for _, field := range []string{"variance", "volume"} {
			if _, err := load(version, field, "1.01"); err == nil {
				t.Errorf("%s の上限超過を受け入れた", field)
			}
			if _, err := load(version, field, "1"); err != nil {
				t.Error(err)
			}
		}
	}
	t.Run("Go の実効値も検査", func(t *testing.T) {
		for _, value := range []float64{-1, math.NaN(), math.Inf(1)} {
			db := testgame.Load(t, nil)
			db.Game.Battle.Variance, db.Game.Battle.CriticalRate, db.Game.Battle.GuardRate = value, value, value
			db.Game.Battle.FrontWeight, db.Game.Battle.BackWeight = new(value), new(value)
			db.Music.Volume = value
			errs := fmt.Sprint(db.Check())
			for _, field := range fields {
				if !strings.Contains(errs, field.name) {
					t.Errorf("%s の診断がない: %s", field.name, errs)
				}
			}
		}
	})
	t.Run("効果のばらつきの範囲", func(t *testing.T) {
		for _, version := range []int{0, 1} {
			for _, value := range []string{"0", "0.2", "1", "-0.1", "1.01", ".nan", ".inf"} {
				db, err := testgame.TryLoad(map[string]string{
					"game.yaml":        fmt.Sprintf("schema_version: %d\nid: neutral\n", version) + testgame.Game,
					"data/effect.yaml": "skills: {fixed: {effects: [{damage: '10', variance: " + value + "}]}}",
				})
				valid := value == "0" || value == "0.2" || value == "1"
				if valid {
					if err != nil {
						t.Fatal(err)
					}
					if db.Skills["fixed"].Effects[0].Variance == nil {
						t.Fatal("効果の指定が消えた")
					}
				} else if err == nil || !strings.Contains(err.Error(), "data/effect.yaml") || !strings.Contains(err.Error(), "skills.fixed.effects[0].variance") {
					t.Fatalf("効果の範囲外の診断がない: %v", err)
				}
			}
		}
	})
}
