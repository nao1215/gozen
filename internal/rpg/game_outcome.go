package rpg

import "encoding/hex"

// GameOutcome is an accepted casual-game result retained by event cursors.
// Event and play bind it to a deterministic invocation, definition and seed.
// It contains no transient simulation state or presentation resources.
type GameOutcome struct {
	CompletedStages     int    `json:"completed_stages"`
	BestCompletedStages int    `json:"best_completed_stages"`
	Misses              int    `json:"misses"`
	Ticks               int    `json:"ticks"`
	Distance            int    `json:"distance"`
	Collected           int    `json:"collected"`
	Streak              int    `json:"streak"`
	LifeProgress        int    `json:"life_progress"`
	ID                  string `json:"id"`
	Definition          string `json:"definition"`
	Invocation          int    `json:"invocation"`
	Seed                uint64 `json:"seed"`
	Status              string `json:"status"`
	Score               int    `json:"score"`
	Lives               int    `json:"lives"`
	Stock               int    `json:"stock"`
	Stage               int    `json:"stage"`
}

// Valid checks the bounded record shape for save loading and result submission.
// Event additionally checks the pending request and authored resource limits.
func (g GameOutcome) Valid() bool {
	digest, err := hex.DecodeString(g.Definition)
	if err != nil || len(digest) != 32 || g.ID == "" || len(g.ID) > 255 || g.Invocation < 1 || g.Invocation > 4096 {
		return false
	}
	if g.Status != "completed" && g.Status != "failed" && g.Status != "cancelled" {
		return false
	}
	for _, n := range []int{g.Score, g.Lives, g.Stock, g.Stage, g.CompletedStages, g.BestCompletedStages, g.Misses, g.Ticks, g.Distance, g.Collected, g.Streak, g.LifeProgress} {
		if n < 0 || n > 1_000_000_000 {
			return false
		}
	}
	return true
}
