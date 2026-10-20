package gozen

import "github.com/nao1215/gozen/internal/layer4/sessionlink"

func init() {
	sessionlink.Of = func(x any) (sessionlink.Parts, bool) {
		s, ok := x.(*Session)
		if !ok || s == nil {
			return sessionlink.Parts{}, false
		}
		return sessionlink.Parts{FS: s.g.fsys, DB: s.db, State: s.st, Dev: s.cfg.Dev, Seed: s.cfg.Seed, Fresh: s.fresh}, true
	}
}
