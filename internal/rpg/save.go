package rpg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"sort"
	"strings"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/savefile"
	lang "github.com/nao1215/gozen/internal/text"
)

// saveFormatVersion is the save format version of the JSON wrapper and the state. It is managed separately from the YAML spec version.
const saveFormatVersion = 1

type saveEnvelope struct {
	FormatVersion int             `json:"format_version"`
	GameID        string          `json:"game_id"`
	DataVersion   int             `json:"data_version"`
	State         json.RawMessage `json:"state"`
}

// Config is how a caller that knows the origin accepts the legacy format.
type Config struct {
	// AllowLegacy is set only for an old slot at the same save location, or an external file the user has confirmed belongs to the same game.
	// A game cannot be identified automatically from JSON without a version.
	AllowLegacy bool
}

// LoadChange is a save item that was removed or changed to match the current data.
type LoadChange struct {
	Path   string `json:"path"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// LoadReport lists the version at save time and the migrations and corrections. Changes are ordered by path, ID, then reason.
type LoadReport struct {
	Legacy           bool         `json:"legacy"`
	GameID           string       `json:"game_id"`
	SavedDataVersion int          `json:"saved_data_version"`
	DataVersion      int          `json:"data_version"`
	Changes          []LoadChange `json:"changes,omitempty"`
}

// Changed reports whether a backup of the original JSON and a notice to the user are needed.
func (r LoadReport) Changed() bool {
	return r.Legacy || r.SavedDataVersion != r.DataVersion || len(r.Changes) != 0
}

// Save returns JSON wrapped with the game ID, save format version and data version.
// An old game without an ID keeps an empty string and does not rename the save location on its own.
func (s *State) Save() ([]byte, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New(lang.T("保存する状態にゲームデータがない"))
	}
	for id, a := range s.Actors {
		if a != nil && a.Tactics != nil {
			if err := a.Tactics.check(); err != nil {
				return nil, fmt.Errorf("actors/%s/tactics: %w", id, err)
			}
		}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(saveEnvelope{saveFormatVersion, s.DB.Game.ID, s.DB.Game.DataVersion, b}, "", "  ")
}

// Parse reads new-format saves and temporary snapshots. It does not modify the original bytes or the DB.
// Specify the origin of a legacy format with ParseWithConfig. Corrections are available from the returned State.LoadReport.
func Parse(db *data.DB, b []byte, rng *rand.Rand) (*State, error) {
	return ParseWithConfig(db, b, rng, Config{})
}

// ParseWithConfig verifies identity and structure, then migrates the legacy format and binds it to the current data.
func ParseWithConfig(db *data.DB, b []byte, rng *rand.Rand, opt Config) (*State, error) {
	if db == nil {
		return nil, errors.New(lang.T("読み込み先のゲームデータがない"))
	}
	raw, err := saveObject(b)
	if err != nil {
		return nil, err
	}
	report := LoadReport{GameID: db.Game.ID, DataVersion: db.Game.DataVersion}
	payload := b
	_, version := raw["format_version"]
	_, game := raw["game_id"]
	_, dataVersion := raw["data_version"]
	_, state := raw["state"]
	if version || game || dataVersion || state {
		var v int
		if err := requiredSaveValue(raw, "format_version", &v); err != nil {
			return nil, err
		}
		// The body of a future version is not interpreted with the current types.
		if v != saveFormatVersion {
			return nil, fmt.Errorf(lang.T("保存形式版 %d には対応していない（対応版 %d）"), v, saveFormatVersion)
		}
		var envelope saveEnvelope
		if err := decodeSave(b, &envelope); err != nil {
			return nil, err
		}
		if err := requiredSaveValue(raw, "game_id", &envelope.GameID); err != nil {
			return nil, err
		}
		if err := requiredSaveValue(raw, "data_version", &envelope.DataVersion); err != nil {
			return nil, err
		}
		if envelope.GameID != db.Game.ID {
			return nil, fmt.Errorf(lang.T("別のゲームのセーブ（保存元 %q、読み込み先 %q）"), envelope.GameID, db.Game.ID)
		}
		if envelope.DataVersion < 0 || envelope.DataVersion > db.Game.DataVersion {
			return nil, fmt.Errorf(lang.T("セーブのデータ版 %d は読み込めない（現在 %d）"), envelope.DataVersion, db.Game.DataVersion)
		}
		report.SavedDataVersion = envelope.DataVersion
		payload = envelope.State
	} else {
		if !opt.AllowLegacy {
			return nil, errors.New(lang.T("版なしのセーブは作品を識別できない。同じ保存先から読むか、同じ作品と確認して旧形式の移行を指定する"))
		}
		report.Legacy = true
	}
	fields, err := saveObject(payload)
	if err != nil {
		return nil, err
	}
	// Require the fields needed for identification that existed even in old versions. Filling in the start state must not hide missing ones.
	for _, key := range []string{"actors", "party", "location"} {
		if v, ok := fields[key]; !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return nil, fmt.Errorf(lang.T("セーブの必須項目 %s がない"), key)
		}
	}
	if err := checkTacticsVersions(fields["actors"]); err != nil {
		return nil, err
	}
	s := &State{DB: db, Rand: rng, LoadReport: report}
	if err := decodeSave(payload, s); err != nil {
		return nil, err
	}
	if err := s.checkSavedState(); err != nil {
		return nil, err
	}
	if s.Rand == nil {
		s.Rand = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) //nolint:gosec // Gameplay randomness; no security-sensitive values.
	}
	if err := s.validateStateNames(); err != nil {
		return nil, err
	}
	s.reconcileSave()
	s.initStateNames()
	if len(s.Party) == 0 {
		return nil, errors.New(lang.T("現在のデータに残る仲間がいないためロードできない"))
	}
	if db.Locations[s.Location] == nil {
		return nil, errors.New(lang.T("ロード後の場所がない"))
	}
	return s, nil
}

func (s *State) checkSavedState() error {
	bad := func(path string) error { return fmt.Errorf(lang.T("セーブの項目 %s が不正"), path) }
	if len(s.Party) == 0 || s.Location == "" {
		return bad("party/location")
	}
	seen := map[string]bool{}
	for _, id := range s.Party {
		if seen[id] || s.Actors[id] == nil {
			return bad("party/" + id)
		}
		seen[id] = true
	}
	for id, a := range s.Actors {
		if a == nil || id == "" || a.ID != id || a.Level < 1 {
			return bad("actors/" + id)
		}
		if a.Tactics != nil {
			if err := a.Tactics.check(); err != nil {
				return fmt.Errorf("actors/%s/tactics: %w", id, err)
			}
		}
	}
	return nil
}

func (s *State) changed(path, id, reason string) {
	s.LoadReport.Changes = append(s.LoadReport.Changes, LoadChange{path, id, reason})
}

func (s *State) reconcileSave() {
	db := s.DB
	// After returning to town, the position and wandering enemies of the last dungeon are kept.
	// Being in town alone does not treat a valid dungeon record as deleted.
	roamerLocation := s.Pos.In
	if roamerLocation == "" {
		roamerLocation = s.Location
	}
	for id, a := range s.Actors {
		if db.Actors[id] == nil {
			s.changed("actors", id, lang.T("仲間の定義がないため除外"))
			delete(s.Actors, id)
			continue
		}
		prefix := "actors/" + id + "/"
		if a.Tactics == nil {
			a.Tactics = &ActorTactics{Version: TacticsVersion, Mode: TacticsAuto}
			s.changed(prefix+"tactics", "", lang.T("作戦のない旧セーブをおまかせへ移行"))
		}
		if db.Game.Battle.SimpleTactics {
			a.db = db
			// The target of an old row moves to the action's standard target. If the condition cannot be read, it reverts to the type.
			draft := a.Tactics.clone()
			changed := false
			for i := range draft.Rows {
				if draft.Rows[i].Target != "" {
					changed = true
					draft.Rows[i].Target = ""
				}
			}
			if err := a.checkSimpleTactics(draft); err != nil {
				a.Tactics = &ActorTactics{Version: TacticsVersion, Mode: TacticsAuto}
				s.changed(prefix+"tactics", "", lang.T("旧作戦を５行の形で読めないため、おまかせに戻しました"))
			} else if changed {
				a.Tactics = &draft
				s.changed(prefix+"tactics", "", lang.T("作戦の対象を行動に合わせて自動で選ぶ形へ移しました"))
			}
		}
		for slot, item := range a.Equip {
			d := db.Items[item]
			if item == "" {
				continue
			}
			if d == nil || !slices.ContainsFunc(db.Game.Slots, func(s data.Slot) bool { return s.ID == slot }) || !d.Fits(slot) {
				s.changed(prefix+"equip/"+slot, item, lang.T("品または装備枠の定義が合わないため除外"))
			}
		}
		for _, skill := range a.Learnt {
			if db.Skills[skill] == nil {
				s.changed(prefix+"learnt", skill, lang.T("技の定義がないため除外"))
			}
		}
		for state := range a.States {
			if db.States[state] == nil {
				s.changed(prefix+"states", state, lang.T("状態の定義がないため除外"))
			}
		}
		a.db = db
		for node, lv := range a.Tree {
			n := a.Node(node)
			if n == nil || lv <= 0 || lv > n.Max {
				s.changed(prefix+"tree", node, lang.T("ツリーの定義に合わせて補正し、余分な割り振りを SP に返却"))
			}
		}
		a.bind(db)
	}
	s.Party = slices.DeleteFunc(s.Party, func(id string) bool { return s.Actors[id] == nil })
	for id := range s.Items {
		if db.Items[id] == nil {
			s.changed("items", id, lang.T("品の定義がないため除外"))
			delete(s.Items, id)
		}
	}
	if db.Locations[s.Location] == nil {
		s.changed("location", s.Location, lang.T("場所の定義がないため開始場所へ移動"))
		s.Location = db.Game.Start.Location
		s.Spot = ""
	}
	if s.Spot != "" && !db.Locations[s.Location].Spotted(s.Spot) {
		s.changed("spot", s.Spot, lang.T("立ち位置の定義がないため場所の入口へ移動"))
		s.Spot = ""
	}
	if s.Pos.In != "" {
		l := db.Locations[s.Pos.In]
		if l == nil || l.Dungeon == nil {
			s.changed("pos", s.Pos.In, lang.T("迷宮の定義がないため位置を初期化"))
			s.Pos = Pos{}
		}
	}
	// Map records are identified by location ID and separator. Free-named flags, variables and done marks are kept.
	for key := range s.Mapped {
		if l := db.Locations[key]; l != nil && l.Map != nil { // visited map points (in point order)
			continue
		}
		if savedDungeon(db, key) == nil {
			s.changed("mapped", key, lang.T("迷宮の定義がないため除外"))
			delete(s.Mapped, key)
		}
	}
	for key, ch := range s.Put {
		d := savedDungeon(db, key)
		if d == nil || (ch != string(data.CellFloor) && ch != string(data.CellWall) && ch != string(data.CellDoor) && d.Tiles[ch] == nil) {
			s.changed("put", key, lang.T("迷宮またはタイルの定義がないため除外"))
			delete(s.Put, key)
		}
	}
	kept := s.Roamers[:0]
	l := db.Locations[roamerLocation]
	for i, r := range s.Roamers {
		if l == nil || l.Dungeon == nil || r.Kind < 0 || r.Kind >= len(l.Dungeon.Roamers) || (r.Troop != "" && db.Troops[r.Troop] == nil) {
			s.changed(fmt.Sprintf("roamers/%d", i), r.Troop, lang.T("徘徊する敵の定義がないため除外"))
		} else {
			kept = append(kept, r)
		}
	}
	s.Roamers = kept
	if s.Flags == nil {
		s.Flags = map[string]bool{}
	}
	if s.Done == nil {
		s.Done = map[string]bool{}
	}
	if s.Vars == nil {
		s.Vars = map[string]any{}
	}
	if s.Items == nil {
		s.Items = map[string]int{}
	}
	if s.Mapped == nil {
		s.Mapped = map[string][]byte{}
	}
	sort.Slice(s.LoadReport.Changes, func(i, j int) bool {
		a, b := s.LoadReport.Changes[i], s.LoadReport.Changes[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Reason < b.Reason
	})
	s.LoadReport.Changes = slices.Compact(s.LoadReport.Changes)
}

func savedDungeon(db *data.DB, key string) *data.Dungeon {
	// Legacy games whose ID itself contains the separator also prefer the longest matching definition.
	var found *data.Dungeon
	longest := -1
	for id, loc := range db.Locations {
		if len(id) <= longest {
			continue
		}
		if key == id || strings.HasPrefix(key, id+"/") || strings.HasPrefix(key, id+"@") || strings.HasPrefix(key, id+"#") {
			found, longest = loc.Dungeon, len(id)
		}
	}
	return found
}

// SaveFile saves to a normal slot. Legacy formats, different versions, files needing correction and unreadable originals
// are saved to a savefile backup before being replaced. If creating the backup fails, the original file is left untouched.
// The return value is the path of the backup made this time (empty if not needed). The caller keeps the external import source.
func (s *State) SaveFile(path string) (string, error) {
	b, err := s.Save()
	if err != nil {
		return "", err
	}
	old, err := os.ReadFile(path) //nolint:gosec // The caller explicitly selects its save file.
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err == nil {
		previous, loadErr := ParseWithConfig(s.DB, old, rand.New(rand.NewPCG(1, 2)), Config{AllowLegacy: true}) //nolint:gosec // Gameplay randomness; no security-sensitive values.
		if loadErr != nil || previous.LoadReport.Changed() {
			return savefile.WritePreserving(path, b)
		}
	}
	return "", savefile.Write(path, b)
}
