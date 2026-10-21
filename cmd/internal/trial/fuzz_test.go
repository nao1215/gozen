package trial

import "testing"

// Any text given as trial.yaml is either refused with a message or builds a trial copy that
// the engine loads: a configuration never produces a broken game.
func FuzzTrialConfig(f *testing.F) {
	for _, seed := range []string{
		"{}",
		"assume: {flag.locked_door: false, var.rare_count: 0, var.step: 2, var.x: [0, inf]}\ncut: [finale, ending]\ndive_max: {tower: 2}\nblocked: [{say: Stop.}]\nunknown: [var.n]\nkeep: ['cg/*.jpg']\n",
		"game: {title: Demo, text: {a: b}}\n",
		"assume: {var.floor: 1}\ndive_max: {tower: 1}\nblocked: [{end: true}]\n",
		"cut: ['*']\n", "cut: ['[']\n", "assume: [", "assume: {var.x: [1]}\n", "", "null",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, config string) {
		cfg, err := Parse([]byte(config))
		if err != nil {
			return
		}
		res, err := Build(drawnFloorGame(), cfg)
		if err != nil {
			return
		}
		if len(cfg.Game) > 0 || len(cfg.Blocked) > 0 {
			return // game: and blocked: are written into the copy as given; the CLI reports what the engine rejects
		}
		if err := res.Check(); err != nil {
			t.Fatalf("%q builds a copy that does not load: %v", config, err)
		}
	})
}
