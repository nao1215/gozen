package rpg

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/nao1215/gozen/expr"
	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	lang "github.com/nao1215/gozen/text"
)

func (s *State) stateRegistry() map[string]data.StateName {
	if s.stateNames == nil {
		s.stateNames = s.DB.StateRegistry()
	}
	return s.stateNames
}

// initStateNames は宣言された初期値だけを補い、保存済みの値と生成名の未設定を保つ。
func (s *State) initStateNames() {
	if s.DB.Game.State.Strict {
		s.stateNames = s.DB.StateRegistry()
	}
	for path, d := range s.DB.Game.State.Names {
		kind, name, _ := strings.Cut(path, ".")
		if kind == "flag" {
			if _, ok := s.Flags[name]; !ok {
				s.Flags[name] = optional.Value[bool](d.Initial)
			}
		} else if _, ok := s.Vars[name]; !ok {
			v := d.Initial
			if d.Type == "number" {
				v, _ = expr.ToNumber(v)
			}
			s.Vars[name] = v
		}
	}
}

func (s *State) checkStateValue(path string, value any) error {
	d, declared := s.DB.Game.State.Names[path]
	if s.DB.Game.State.Strict {
		d, declared = s.stateRegistry()[path]
		if !declared {
			return fmt.Errorf(lang.T("未宣言の代入先 %q"), path)
		}
	}
	if declared && expr.TypeOfValue(value) != d.ValueType() {
		return fmt.Errorf(lang.T("%s の値は型 %s と一致しない"), path, d.Type)
	}
	return nil
}

func (s *State) validateStateNames() error {
	if !s.DB.Game.State.Strict && len(s.DB.Game.State.Names) == 0 {
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(s.Flags)) {
		if err := s.checkStateValue("flag."+name, s.Flags[name]); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(s.Vars)) {
		if err := s.checkStateValue("var."+name, s.Vars[name]); err != nil {
			return err
		}
	}
	return nil
}
