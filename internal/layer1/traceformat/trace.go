// Package traceformat owns the JSONL protocol shared by the player and reproduction tools.
package traceformat

import (
	"encoding/json"
	"errors"
	"fmt"
)

// LineLimit bounds JSONL records for the player and command scanners. Writers
// reject records at this limit so every emitted record remains readable.
const LineLimit = 16 << 20

// Viewport is the recorded layout shared by player startup and exact-replay validation.
type Viewport struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Scale  float64 `json:"scale"`
	Tall   bool    `json:"tall,omitempty"`
}

// ExactReplay validates the record stream used by the trace CLI's reproduction
// bundles and the player's replay loader. The zero value starts a stream. Call
// Check in trace order, only through the target tick; it retains viewport state.
// It validates replay support, not the trace's ordering or start prerequisites.
type ExactReplay struct {
	viewport *Viewport
}

// kinds explicitly classifies accepted and refused records. Input and act drive replay; start
// restores its context and save_write restores storage clocks. Remaining records
// describe deterministic outcomes or presentation, not additional mutations.
// repro is a seek boundary.
var kinds = map[string]bool{
	"draw_skip":           true, // Recorded presentation cadence.
	"act":                 true,
	"activate":            true,
	"ai_decision":         true,
	"answer":              true,
	"autosave":            true,
	"banner_open":         true,
	"battle":              true,
	"battle_capture":      true,
	"battle_close":        true,
	"battle_end":          true,
	"battle_fx":           true,
	"battle_music":        true,
	"battle_result":       true,
	"battle_step":         true,
	"checkpoint_restored": true,
	"clash":               true,
	"clash_hit":           true,
	"clash_skill":         true,
	"end":                 true,
	"end_turn":            true,
	"flow_enter":          true,
	"flow_next":           true,
	"focus_pause":         true, // Only before the first update; checked below.
	"from_memo":           true,
	"from_memo_ready":     true,
	"gallery":             true,
	"input":               true,
	"initial_save":        true,
	"layout":              true,
	"look":                true,
	"menu":                true,
	"pick":                true,
	"message_open":        true,
	"music":               true,
	"notice":              true,
	"order":               true,
	"ready":               true,
	"region":              true,
	"repro":               true,
	"request":             true,
	"resume":              true,
	"rollback":            true,
	"save_load":           true,
	"save_write":          true,
	"say":                 true,
	"scene":               true,
	"screen":              true, // Drawing observation only.
	"screen_action":       true,
	"screen_carry":        true,
	"screen_drop":         true,
	"screen_peek":         true,
	"screen_refused":      true,
	"shot":                true,
	"skip":                true,
	"section":             true, // Performance observation only.
	"snapshot":            true,
	"sortie":              true,
	"spot":                true,
	"stand":               true,
	"start":               true,
	"state":               true,
	"travel":              true,
	"turn":                true,
	"unlock":              true,
	"view":                true,
	"viewport":            true,
	"async_picture":       false,
	"audio_timing":        false,
	"battle_setup":        false,
	"carry":               false,
	"carry_read":          false,
	"cg_review":           false,
	"cg_review_tag":       false,
	"config_write":        false,
	"dev_do":              false,
	"dev_set":             false,
	"diverge":             false,
	"external_save":       false,
	"graphs":              false,
	"guide":               false,
	"guide_blocked":       false,
	"guide_done":          false,
	"jump":                false,
	"live_reload":         false,
	"memo":                false,
	"memo_open":           false,
	"now":                 false,
	"panel":               false, // Typed text and wall-clock frame measurements are not recorded.
	"persistence_error":   false,
	"pick_error":          false,
	"problem":             false,
	"profiles":            false,
	"save_backup":         false,
	"scene_review":        false,
	"script_error":        false,
	"stream":              false,
	"stream_comment":      false,
	"stream_error":        false,
	"stream_tip":          false,
	"text_input":          false,
	"unsupported_script":  false,
	"voice":               false,
}

// Action names are checked separately: an act record is not permission to run
// an unknown future operation. Both replay modes share the supported actions.
var exactReplayActions = map[string]bool{
	"battle_auto": true,
	"run":         true,
	"use_item":    true,
	"raise":       true,
	"equip_best":  true,
	"buy":         true,
	"walk":        true,
	"dev_do":      true,
	"dev_jump":    true,
	"guides":      true,
}

// Check rejects any record not known to replay exactly, including future kinds.
// Errors identify the first unsupported kind and tick for CLI diagnostics.
func (v *ExactReplay) Check(kind string, tick int, raw []byte) error {
	if !kinds[kind] {
		return fmt.Errorf("cannot reproduce exactly: %s at tick %d", kind, tick)
	}
	switch kind {
	case "focus_pause":
		// Replay skips focus handling. Before the first update there is no
		// movement to reset and no simulation tick has advanced.
		if tick != 0 {
			return fmt.Errorf("cannot reproduce exactly: %s at tick %d", kind, tick)
		}
	case "start":
		var start struct {
			Autopilot    bool `json:"autopilot"`
			ExactVersion int  `json:"exact_version"`
			Setup        struct {
				Unsupported []string `json:"unsupported"`
			} `json:"setup"`
		}
		if err := json.Unmarshal(raw, &start); err != nil {
			return err
		}
		if start.Autopilot {
			return errors.New("cannot reproduce exactly: autopilot changes selections without recording them")
		}
		if start.ExactVersion != 3 {
			return errors.New("cannot reproduce exactly: trace lacks the source audit revision; record again")
		}
		if len(start.Setup.Unsupported) > 0 {
			return fmt.Errorf("cannot reproduce exactly: unsupported start conditions %v", start.Setup.Unsupported)
		}
	case "act":
		var act struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &act); err != nil {
			return err
		}
		if act.Name == "window" {
			return errors.New("cannot reproduce exactly: the recording changed the window size")
		}
		if !exactReplayActions[act.Name] {
			return fmt.Errorf("cannot reproduce exactly: act at tick %d (unknown action %q)", tick, act.Name)
		}
	case "viewport":
		var current Viewport
		if err := json.Unmarshal(raw, &current); err != nil {
			return err
		}
		// Startup layout can settle around the first game tick.
		// Later scale changes can change rendered dimensions even with the same logical layout.
		if tick > 1 && v.viewport != nil && current != *v.viewport {
			return errors.New("cannot reproduce exactly: the recording changed the window size")
		}
		v.viewport = &current
	}
	return nil
}

// SupportsAction reports the actions understood by ordinary and exact replay.
// The player uses it before dispatch; unknown actions are never accepted.
func SupportsAction(name string) bool { return exactReplayActions[name] }
