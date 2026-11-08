package event

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// RunGame asks presentation to run a declared game. Identity is a result template;
// the frontend fills its terminal fields and returns it through AnswerGame.
// Definition is borrowed read-only from the loaded DB.
type RunGame struct {
	Definition *data.RunnerDefinition
	Identity   rpg.GameOutcome
}

func (RunGame) isRequest() {}

func (r *Runner) beginGame(cmd *data.RunGameCommand) Request {
	if len(r.games) >= 4096 {
		r.logf(errors.New("event exceeded 4096 game invocations"))
		r.Stop()
		return nil
	}
	def := r.db.Game.Runners[cmd.ID]
	if def == nil {
		r.logf(fmt.Errorf("unknown runner %q", cmd.ID))
		return nil
	}
	b, err := json.Marshal(def)
	if err != nil {
		r.logf(err)
		return nil
	}
	sum := sha256.Sum256(b)
	q := RunGame{Definition: def, Identity: rpg.GameOutcome{ID: cmd.ID, Definition: hex.EncodeToString(sum[:]), Invocation: len(r.games) + 1, Seed: r.st.Rand.Uint64()}}
	r.gamePending, r.gameCommand = &q, cmd
	return q
}

// AnswerGame accepts exactly one result for the pending game. It checks all
// output assignments on detached flag/variable maps before publishing changes.
// Invalid or duplicate submissions leave the pending request and state intact.
func (r *Runner) AnswerGame(result rpg.GameOutcome) error {
	if r.gamePending == nil || !result.Valid() {
		return errors.New("no matching valid game result")
	}
	q := r.gamePending
	id := q.Identity
	if result.ID != id.ID || result.Definition != id.Definition || result.Invocation != id.Invocation || result.Seed != id.Seed {
		return errors.New("game result does not match pending invocation")
	}
	if result.Lives > q.Definition.MaxLives || result.Stock > q.Definition.MaxStock || result.Stage >= len(q.Definition.Stages) || result.BestCompletedStages > len(q.Definition.Stages) || result.CompletedStages > result.BestCompletedStages {
		return errors.New("game result exceeds declared limits")
	}
	values := map[string]any{
		"completed_stages": result.CompletedStages, "best_completed_stages": result.BestCompletedStages, "misses": result.Misses, "ticks": result.Ticks, "distance": result.Distance, "collected": result.Collected, "streak": result.Streak, "life_progress": result.LifeProgress, "score": result.Score, "lives": result.Lives, "stock": result.Stock, "stage": result.Stage,
		"completed": result.Status == "completed", "failed": result.Status == "failed", "cancelled": result.Status == "cancelled",
	}
	copyState := *r.st
	copyState.Flags, copyState.Vars = maps.Clone(r.st.Flags), maps.Clone(r.st.Vars)
	targets := map[string]bool{}
	for _, key := range slices.Sorted(maps.Keys(r.gameCommand.Outputs)) {
		target := r.gameCommand.Outputs[key]
		if (!strings.HasPrefix(target, "var.") && !strings.HasPrefix(target, "flag.")) || targets[target] {
			return fmt.Errorf("invalid result assignment %q", target)
		}
		targets[target] = true
		value, ok := values[key]
		if !ok {
			return fmt.Errorf("unknown game output %q", key)
		}
		if err := copyState.SetPath(r.gameCommand.Outputs[key], value); err != nil {
			return err
		}
	}
	r.st.Flags, r.st.Vars = copyState.Flags, copyState.Vars
	r.games = append(r.games, result)
	r.gamePending, r.gameCommand = nil, nil
	return nil
}

// GameResults returns detached accepted outcomes for the current event cursor.
func (r *Runner) GameResults() []rpg.GameOutcome {
	if r == nil {
		return nil
	}
	return slices.Clone(r.games)
}

// ReplayGame answers an observed replay request with its next saved result.
// Callers retain transcript ordering and reject unused results at the target.
func (r *Runner) ReplayGame(results []rpg.GameOutcome) error {
	i := len(r.games)
	if i >= len(results) {
		return errors.New("load: missing game outcome")
	}
	return r.AnswerGame(results[i])
}

// GamePending reports whether a frontend must refuse an active-attempt save.
func (r *Runner) GamePending() bool { return r.gamePending != nil }
