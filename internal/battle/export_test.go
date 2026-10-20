package battle

import "github.com/nao1215/gozen/internal/layer2/gamedata"

// The entry point for the external test (battle_test) to check the contents of decisions made
// unexported (unexported at ADR 0021 stage 1).

func (b *Battle) Candidates(u *Unit) []candidate { return b.candidates(u) }

func (b *Battle) EvaluateRules(u *Unit) ([]candidate, []RuleEvaluation) { return b.explainRules(u) }

func (b *Battle) Rules(u *Unit) []gamedata.Rule { return b.rules(u) }

func (u *Unit) Restrict() (string, string) { return u.restrict() }
