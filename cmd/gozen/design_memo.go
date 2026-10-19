package main

import (
	"fmt"

	"github.com/nao1215/gozen/cmd/gozen/internal/designeval"
	"github.com/nao1215/gozen/internal/layer1/memo"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func designMemos(root, run string, db *data.DB, cfg *designeval.Config, runs []*designeval.Run, sums []designeval.Summary) ([]string, error) {
	label := map[string]string{}
	for _, k := range db.Game.Memo.Kinds {
		label[k.ID] = k.Label
	}
	var out []string
	for i, r := range runs {
		s := sums[i]
		write := func(kind, text string, sn *designeval.Snap, d map[string]any) error {
			if kind == "" {
				return nil
			}
			m := memo.Memo{Source: memo.Bot, Bot: r.Persona.Name, Kind: kind, Label: label[kind], Text: text,
				Seed: r.Seed, Game: db.Game.ID, Data: d}
			m.Data["persona"] = r.Persona.ID
			m.Data["run"] = run
			var save []byte
			if sn != nil {
				m.Seconds = sn.T
				m.Location, m.Place, m.Floor, m.Pos, m.Vars = sn.Location, sn.Place, float64(sn.Floor), sn.Pos, sn.Vars
				if sn.Pos != nil {
					m.Facing = data.Facings[sn.Facing&3]
				}
				save, m.Rand = sn.Save, sn.Rand
				if save != nil {
					m.Resume = &memo.Resume{From: "here"}
				}
			}
			path, err := memo.Write(root, &m, nil, save)
			if err == nil {
				out = append(out, path)
			}
			return err
		}
		for _, sp := range s.Dry.Top {
			if err := write(cfg.Memos.Dry, fmt.Sprintf("何も起きない時間が %s 続いた（ご褒美も期待も無い）。%s、%d 日目、遊び始めから %s。", minutes(sp.Len), sp.At, sp.Day, minutes(sp.From)),
				snapAt(r, sp.From*60), map[string]any{"dry_min": sp.Len, "from_min": sp.From}); err != nil {
				return out, err
			}
		}
		if g := s.Gap; g.N > 0 {
			if err := write(cfg.Memos.Gap, fmt.Sprintf("ご褒美の間隔がいちばん長い所（%s）。%s、%d 日目、遊び始めから %s。", minutes(g.Max), g.At, g.Day, minutes(g.From)),
				snapAt(r, g.From*60), map[string]any{"gap_min": g.Max, "from_min": g.From}); err != nil {
				return out, err
			}
		}
		for _, b := range r.Bosses {
			if !b.Measured || b.Rate >= cfg.Memos.BossBelow {
				continue
			}
			if err := write(cfg.Memos.Boss, fmt.Sprintf("B%dF の番人 %s に、着いたときの勝率 %.0f%%（平均 Lv%d）。勝率を上げるのに %d 戦足した。", b.Floor, b.Name, 100*b.Rate, b.Level, b.Grind),
				b.At, map[string]any{"boss": b.Troop, "rate": b.Rate, "level": b.Level, "grind": b.Grind}); err != nil {
				return out, err
			}
		}
		for i := range r.Stuck[:min(5, len(r.Stuck))] {
			sn := &r.Stuck[i]
			if err := write(cfg.Memos.Stuck, fmt.Sprintf("つまずいた: %s（%s、%d 日目）。", sn.Note, sn.Place, sn.Day), sn, map[string]any{"note": sn.Note}); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

func snapAt(r *designeval.Run, t float64) *designeval.Snap {
	var best *designeval.Snap
	for i := range r.Snaps {
		if r.Snaps[i].T <= t+1 {
			best = &r.Snaps[i]
		}
	}
	if best == nil && len(r.Snaps) > 0 {
		best = &r.Snaps[0]
	}
	return best
}
