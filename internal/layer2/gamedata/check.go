package gamedata

import (
	"fmt"
	"io/fs"
	"maps"
	"math"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/diag"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer2/puppetdef"
	lang "github.com/nao1215/gozen/internal/text"
	"github.com/nao1215/gozen/internal/text/catalog"
)

// The names an expression may use, by where it is written.
var (
	// FieldNames are known to the conditions of events, locations and the field music.
	FieldNames = []string{"flag", "var", "gold", "party", "item", "actor", "location", "rand", "chance", "steps", "members", "pos", "pages", "spot"}
	// BattleNames are known in battle: the AI, the troop events, the battle music.
	BattleNames = append(slices.Clone(FieldNames), "turn", "self", "allies", "foes", "enemies", "result", "phase", "boss")
	// EffectNames are known to the formulas of skills and items: a is the user, b the target.
	EffectNames = append(slices.Clone(BattleNames), "a", "b")
	// TargetNames are known to a target's where and by: t is the candidate.
	TargetNames = append(slices.Clone(BattleNames), "t")
	// LevelNames are known to a class's growth: the level.
	LevelNames = []string{"level"}
)

type checker struct {
	stateNames map[string]StateName
	db         *DB
	errs       []error
	// script checks the command list fed from Go (gozen's Session.Do). The spec version is a data contract, so it is not checked.
	script bool
	// noDev makes setting a party member's stats or level an error in script (a non-dev Session; author decision 5).
	noDev bool
	// warns are warnings that do not stop loading (unreachable places in scenes and maps, etc.; returned by SceneWarnings).
	warns []error
	// puppetNames are the expression names of animated portraits (puppets), per speaker (nil if the portrait cannot be read).
	puppetNames map[string][]string
}

// errorf records a problem with its diagnostic code (ADR 0024). A few checks
// that report warnings (AIWarnings, SetWarnings) use it with a GZN01xx code.
func (c *checker) errorf(code diag.Code, where, format string, args ...any) {
	c.errs = append(c.errs, coded(c.db.diagnostic(where, fmt.Sprintf(format, args...)), code))
}

func (c *checker) warnf(code diag.Code, where, format string, args ...any) {
	c.warns = append(c.warns, coded(c.db.diagnostic(where, fmt.Sprintf(format, args...)), code))
}

func (c *checker) expr(where string, e Expr, names []string) {
	c.exprIn(where, e, names, nil)
}

// exprIn is expr plus a type for names that exist only at that location (over).
func (c *checker) exprIn(where string, e Expr, names []string, over map[string]*vtype) {
	if !e.Set() {
		return
	}
	for _, n := range e.Names() {
		if !slices.Contains(names, n) {
			c.errorf(diag.Expression, where, "%q uses the unknown name %q (known here: %s)", e.String(), n, strings.Join(names, ", "))
		} else if n == "spot" {
			c.needsVersion(where, "the name spot", SchemaVersion3)
		}
	}
	c.fields(where, e, names, over)
}

func (c *checker) ref(where, kind, id string, ok bool) {
	if id != "" && !ok {
		c.errorf(diag.UnknownReference, where, "unknown %s %q", kind, id)
	}
}

func (c *checker) picture(where string, p Picture) {
	if p.File != "" && !c.db.Exists("assets/"+p.File) {
		c.errorf(diag.MissingAsset, where, "no picture assets/%s", p.File)
	}
}

func (c *checker) file(where, p string) {
	if p != "" && !c.db.Exists("assets/"+p) {
		c.errorf(diag.MissingAsset, where, "no file assets/%s", p)
	}
}

// puppet checks a moving portrait's YAML and that its pictures exist. Picture sizes
// and positions are checked when the game loads it (and by gozen art puppet).
func (c *checker) puppet(where string, a *Actor) {
	if a.Puppet == "" {
		return
	}
	b, err := fs.ReadFile(c.db.FS, "assets/"+a.Puppet)
	if err != nil {
		c.errorf(diag.MissingAsset, where, "no file assets/%s", a.Puppet)
		return
	}
	spec, err := puppetdef.Parse(b)
	if err != nil {
		c.errorf(diag.Shape, where, "%s: %v", a.Puppet, err)
		return
	}
	for _, p := range spec.Pictures() {
		c.file(where, path.Join(path.Dir(a.Puppet), p))
	}
	if face := a.Face; face != "" && !slices.Contains(spec.FaceNames(), face) {
		c.errorf(diag.UnknownReference, where, "%s has no face %q (the actor's face)", a.Puppet, face)
	}
}

// puppetFaces returns the expression names of a speaker's animated portrait; false if the portrait cannot be read.
func (c *checker) puppetFaces(who string, a *Actor) ([]string, bool) {
	if names, ok := c.puppetNames[who]; ok {
		return names, names != nil
	}
	if c.puppetNames == nil {
		c.puppetNames = map[string][]string{}
	}
	var names []string
	if b, err := fs.ReadFile(c.db.FS, "assets/"+a.Puppet); err == nil {
		if spec, err := puppetdef.Parse(b); err == nil {
			names = spec.FaceNames()
		}
	}
	c.puppetNames[who] = names
	return names, names != nil
}

// requiredDefinitions checks, before completion, the required values that must not be filled with defaults.
// Checks that use completed values, such as map characters and reference targets, stay in Check.
func (c *checker) requiredDefinitions() {
	for _, id := range c.db.Ordered("classes") {
		cl, w := c.db.Classes[id], "classes."+id
		for _, st := range StatNames {
			if e, ok := cl.Stats[st]; !ok || !e.Set() {
				c.errorf(diag.Shape, w, "stats.%s is missing", st)
			}
		}
		if !cl.ExpFor.Set() {
			c.errorf(diag.Shape, w, "exp_for is missing")
		}
	}
	for _, id := range c.db.Ordered("actors") {
		if c.db.Actors[id].Class == "" {
			c.errorf(diag.Shape, "actors."+id, "class is missing")
		}
	}
	for _, id := range c.db.Ordered("enemies") {
		e, w := c.db.Enemies[id], "enemies."+id
		if e.Image.File == "" {
			c.errorf(diag.Shape, w, "image is missing")
		}
		for _, st := range StatNames {
			if _, ok := e.Stats[st]; !ok {
				c.errorf(diag.Shape, w, "stats.%s is missing", st)
			}
		}
	}
	for _, id := range c.db.Ordered("troops") {
		if len(c.db.Troops[id].Members) == 0 {
			c.errorf(diag.Shape, "troops."+id, "no members")
		}
	}
	for _, id := range c.db.Ordered("locations") {
		if d := c.db.Locations[id].Dungeon; d != nil && d.Generate == nil && strings.TrimSpace(d.Map) == "" {
			c.errorf(diag.Shape, "locations."+id+".dungeon.map", "the map is empty")
		}
	}
}

// Check reports every problem of the data: unknown ids, missing pictures, expressions
// using names unknown where they are written, commands without a verb.
func (db *DB) Check() []error {
	c := &checker{db: db}
	for _, err := range gameVersionErrors(db.Game) {
		c.errs = append(c.errs, db.locateReported(err))
	}
	if len(c.errs) > 0 {
		return sortedDiagnostics(c.errs)
	}
	c.requiredDefinitions()
	c.declarations()
	c.stateDeclarations()
	c.tacticCatalog()
	c.numericValues(&db.Game, "game.yaml")
	c.numericValues(&file{
		Classes: db.Classes, Actors: db.Actors, Items: db.Items, Skills: db.Skills,
		States: db.States, Enemies: db.Enemies, Troops: db.Troops, Locations: db.Locations,
		Events: db.Events, Shops: db.Shops, AI: db.AI, Music: &db.Music,
	}, "")
	g := &db.Game
	_, ok := db.Locations[g.Start.Location]
	c.ref("game.yaml start.location", "location", g.Start.Location, ok)
	if g.Start.Location == "" {
		c.errorf(diag.Shape, "game.yaml", "start.location is missing")
	}
	if len(g.Start.Party) == 0 {
		c.errorf(diag.Shape, "game.yaml", "start.party is empty (a game starts with someone)")
	}
	if len(g.Start.Party) > g.MaxParty {
		c.errorf(diag.OutOfRange, "game.yaml start.party", "%d members, more than max_party (%d)", len(g.Start.Party), g.MaxParty)
	}
	inParty := map[string]bool{}
	for _, a := range g.Start.Party {
		_, ok := db.Actors[a]
		c.ref("game.yaml start.party", "actor", a, ok)
		if inParty[a] {
			c.errorf(diag.Duplicate, "game.yaml start.party", "%s is written twice", a)
		}
		inParty[a] = true
	}
	for it := range g.Start.Items {
		_, ok := db.Items[it]
		c.ref("game.yaml start.items", "item", it, ok)
	}
	_, ok = db.Events[g.Start.Event]
	c.ref("game.yaml start.event", "event", g.Start.Event, ok)
	if _, err := catalog.Words(g.Language); err != nil {
		c.errorf(diag.UnknownReference, "game.yaml language", "%q is not a built-in language (use %s or %s)", g.Language, catalog.English, catalog.Japanese)
	}
	if g.UI.Balloon != 0 {
		c.needsVersion("game.yaml ui.balloon", "ui.balloon", SchemaVersion3)
		if g.UI.Balloon < 0.01 || g.UI.Balloon > 0.2 {
			c.errorf(diag.OutOfRange, "game.yaml ui.balloon", "want a fraction of the screen's height in [0.01, 0.2], got %g", g.UI.Balloon)
		}
	}
	c.file("game.yaml fonts.regular", g.Fonts.Regular)
	c.file("game.yaml fonts.pop", g.Fonts.Pop)
	c.picture("game.yaml title_background", g.TitleBG)
	c.picture("game.yaml title_art", g.TitleArt)
	for i, f := range g.TitleFigures {
		w := fmt.Sprintf("game.yaml title_figures[%d]", i)
		c.picture(w, Picture{File: f.File})
		if f.File == "" || f.X < 0 || f.X > 1 || f.Bottom < 0 || f.Height < 0 {
			c.errorf(diag.OutOfRange, w, "a figure needs a file, x from 0 to 1, and bottom and height of 0 or more")
		}
	}
	c.file("game.yaml fonts.title", g.Fonts.Title)
	c.file("game.yaml stand_ins", g.StandIns)
	c.picture("game.yaml game_over.background", g.GameOver.Background)
	_, ok = db.Events[g.GameOver.Event]
	c.ref("game.yaml game_over.event", "event", g.GameOver.Event, ok)
	c.music("game.yaml title_music", g.TitleBGM)
	for i, cg := range g.Gallery.CGs {
		w := fmt.Sprintf("game.yaml gallery.cgs[%d]", i)
		if (cg.File == "") == (len(cg.Files) == 0) {
			c.errorf(diag.Shape, w, "file or files (one of the two)")
		}
		c.stateFlag(w+".flag", cg.Flag)
		c.file(w+".file", cg.File)
		if len(cg.Files) > 0 {
			pages := cg.Pages(c.db.FS)
			if len(pages) == 0 {
				c.errorf(diag.MissingAsset, w+".files", "no picture matches %v under assets/", cg.Files)
			}
			for _, p := range pages {
				c.file(w+".files", p)
			}
		}
		if cg.Title == "" {
			c.errorf(diag.Shape, w, "a picture of the gallery needs a title")
		}
	}
	c.galleryKeys(g.Gallery)
	for i, sc := range g.Gallery.Scenes {
		w := fmt.Sprintf("game.yaml gallery.scenes[%d]", i)
		_, ok := c.db.Events[sc.Event]
		c.ref(w+".event", "event", sc.Event, ok)
		if sc.Title == "" || (sc.Flag == "" && !sc.When.Set() && sc.Seen == "") {
			c.errorf(diag.Shape, w, "a scene of the gallery needs a title and a flag, when or seen")
		}
		c.stateFlag(w+".flag", sc.Flag)
		c.expr(w+".when", sc.When, FieldNames)
		if sc.Seen != "" && !strings.ContainsAny(sc.Seen, "*?[") {
			c.file(w+".seen", sc.Seen)
		}
		c.assignments(w+".set", sc.Set, FieldNames)
		if sc.Picture != "" {
			c.file(w+".picture", sc.Picture)
		}
		if sc.Location != "" {
			_, ok := c.db.Locations[sc.Location]
			c.ref(w+".location", "location", sc.Location, ok)
		}
		c.picture(w+".background", sc.Background)
		c.music(w+".music", sc.Music)
		for j, page := range sc.Pages {
			at := fmt.Sprintf("%s.pages[%d]", w, j)
			if strings.TrimSpace(page.Title) == "" {
				c.errorf(diag.Shape, at+".title", "%s", lang.T("頁の題名が必要"))
			}
			if page.Line < 1 {
				c.errorf(diag.OutOfRange, at+".line", "%s", lang.T("頁の行番号は１以上にする"))
			}
			if page.Picture != "" {
				c.file(at+".picture", page.Picture)
			}
			for k, answer := range page.Answers {
				if answer < 0 {
					c.errorf(diag.OutOfRange, fmt.Sprintf("%s.answers[%d]", at, k), "%s", lang.T("頁の答えは０以上にする"))
				}
			}
		}
	}
	c.music("game.yaml gallery.music", g.Gallery.Music)
	for _, id := range slices.Sorted(maps.Keys(g.Gallery.MusicPictures)) {
		files := g.Gallery.MusicPictures[id]
		w := "game.yaml gallery.music_pictures." + id
		_, ok := c.db.Music.Songs[id]
		c.ref(w, "song", id, ok)
		if len(files) == 0 {
			c.errorf(diag.OutOfRange, w, "a song needs one picture or more")
		}
		for i, f := range files {
			if f == "" {
				c.errorf(diag.Shape, fmt.Sprintf("%s[%d]", w, i), "an empty picture")
			}
			c.file(fmt.Sprintf("%s[%d]", w, i), f)
		}
	}
	c.music("game.yaml game_over.music", g.GameOver.Music)
	c.music("game.yaml battle.music", g.Battle.Music)
	c.music("game.yaml battle.boss_win_music", g.Battle.BossWinMusic)
	c.expr("game.yaml join_exp", g.JoinExp, FieldNames)
	c.expr("game.yaml battle.escape", g.Battle.Escape, BattleNames)
	c.expr("game.yaml battle.critical", g.Battle.Critical, EffectNames)
	c.expr("game.yaml battle.speed", g.Battle.Speed, EffectNames)
	for i, m := range g.Meters {
		w := fmt.Sprintf("game.yaml meters[%d]", i)
		c.expr(w+".value", m.Value, FieldNames)
		c.expr(w+".max_value", m.MaxValue, FieldNames)
		c.expr(w+".when", m.When, FieldNames)
		if !m.Value.Set() || m.Max < 0 {
			c.errorf(diag.OutOfRange, w, "a meter needs value (and max 0 or more)")
		}
		if m.Actor != "" {
			_, ok := db.Actors[m.Actor]
			c.ref(w+".actor", "actor", m.Actor, ok)
		}
	}
	for i, cb := range g.CardBubbles {
		w := fmt.Sprintf("game.yaml card_bubbles[%d]", i)
		c.expr(w+".when", cb.When, FieldNames)
		if cb.Actor != "" {
			_, ok := db.Actors[cb.Actor]
			c.ref(w+".actor", "actor", cb.Actor, ok)
		}
		if cb.State != "" {
			_, ok := db.States[cb.State]
			c.ref(w+".state", "state", cb.State, ok)
		}
		if cb.Size < 0 || cb.Count < 0 || cb.Speed < 0 || cb.Rise < 0 {
			c.errorf(diag.OutOfRange, w, "size, count, speed and rise must not be negative")
		}
	}
	for i, a := range g.Asides {
		w := fmt.Sprintf("game.yaml asides[%d]", i)
		if a.Actor == "" && a.Face == "" && a.Name == "" {
			c.errorf(diag.Shape, w, "an aside needs a speaker (actor, face or name)")
		}
		if a.Actor != "" {
			_, ok := db.Actors[a.Actor]
			c.ref(w+".actor", "actor", a.Actor, ok)
		}
		if _, err := path.Match(a.Face, ""); err != nil {
			c.errorf(diag.Shape, w+".face", "bad pattern %q", a.Face)
		}
		for _, s := range a.Skip {
			if _, err := path.Match(s, ""); err != nil {
				c.errorf(diag.Shape, w+".skip", "bad pattern %q", s)
			}
		}
		c.expr(w+".when", a.When, FieldNames)
		_, ok := db.Events[a.Call]
		c.ref(w+".call", "event", a.Call, ok)
	}
	memoKinds := map[string]bool{}
	for i, k := range g.Memo.Kinds {
		w := fmt.Sprintf("game.yaml memo.kinds[%d]", i)
		if k.ID == "" || k.Label == "" || memoKinds[k.ID] {
			c.errorf(diag.Shape, w, "a kind of note needs an id of its own and a label")
		}
		memoKinds[k.ID] = true
	}
	reviewTags, reviewOK := map[string]bool{}, 0
	for i, t := range g.CGReview.Tags {
		w := fmt.Sprintf("game.yaml cg_review.tags[%d]", i)
		if t.ID == "" || t.Label == "" || reviewTags[t.ID] {
			c.errorf(diag.Shape, w, "a tag needs an id of its own and a label")
		}
		reviewTags[t.ID] = true
		if t.OK {
			reviewOK++
		}
	}
	if len(g.CGReview.Tags) > 0 && reviewOK != 1 {
		c.errorf(diag.Shape, "game.yaml cg_review.tags", "one tag (and only one) is ok: true")
	}
	for i, gr := range g.CGReview.Groups {
		w := fmt.Sprintf("game.yaml cg_review.groups[%d]", i)
		if gr.Label == "" || len(gr.Files) == 0 {
			c.errorf(diag.Shape, w, "a group needs a label and files")
		}
		for _, f := range gr.Files {
			if _, err := path.Match(f, ""); err != nil {
				c.errorf(diag.Shape, w+".files", "bad pattern %q", f)
			}
		}
	}
	for _, f := range g.CGReview.Extra {
		if _, err := path.Match(f, ""); err != nil {
			c.errorf(diag.Shape, "game.yaml cg_review.extra", "bad pattern %q", f)
		}
	}
	for i, v := range g.Memo.Vars {
		w := fmt.Sprintf("game.yaml memo.vars[%d]", i)
		if v.Name == "" || !v.Value.Set() {
			c.errorf(diag.Shape, w, "a number kept with the notes needs a name and a value")
		}
		c.expr(w+".value", v.Value, FieldNames)
	}
	for name, ev := range map[string]string{"after_win": g.Battle.AfterWin, "before": g.Battle.Before, "after": g.Battle.After} {
		if ev != "" {
			_, ok := db.Events[ev]
			c.ref("game.yaml battle."+name, "event", ev, ok)
		}
	}
	for name, w := range map[string]*float64{"front_weight": g.Battle.FrontWeight, "back_weight": g.Battle.BackWeight} {
		if w != nil {
			c.numberRange("game.yaml battle."+name, *w, 0, math.Inf(1))
		}
	}
	c.numberRange("game.yaml battle.variance", g.Battle.Variance, 0, 1)
	c.numberRange("game.yaml battle.critical_rate", g.Battle.CriticalRate, 0, math.Inf(1))
	c.numberRange("game.yaml battle.guard_rate", g.Battle.GuardRate, 0, math.Inf(1))
	c.numberRange("music.volume", db.Music.Volume, 0, 1)
	_, ok = db.Skills[g.Battle.Attack]
	c.ref("game.yaml battle.attack_skill", "skill", g.Battle.Attack, ok)
	_, ok = db.Skills[g.Battle.Guard]
	c.ref("game.yaml battle.guard_skill", "skill", g.Battle.Guard, ok)
	slots := map[string]bool{}
	for _, s := range g.Slots {
		slots[s.ID] = true
	}

	for _, id := range db.Ordered("classes") {
		cl := db.Classes[id]
		w := "classes." + id
		for st, e := range cl.Stats {
			if !slices.Contains(StatNames, st) {
				c.errorf(diag.UnknownReference, w, "unknown stat %q", st)
			}
			c.expr(w+".stats."+st, e, LevelNames)
		}
		c.expr(w+".exp_for", cl.ExpFor, LevelNames)
		for _, l := range cl.Skills {
			_, ok := db.Skills[l.Skill]
			c.ref(w+".skills", "skill", l.Skill, ok)
		}
		c.tree(w+".tree", cl.Tree)
		c.ai(w+".ai", cl.AI)
	}
	for _, id := range db.Ordered("actors") {
		a := db.Actors[id]
		w := "actors." + id
		_, ok := db.Classes[a.Class]
		c.ref(w+".class", "class", a.Class, ok)
		if a.Faces != "" && !db.Exists("assets/"+a.Faces+"/"+a.Face+".png") {
			c.errorf(diag.MissingAsset, w, "no face assets/%s/%s.png", a.Faces, a.Face)
		}
		c.puppet(w+".puppet", a)
		c.doll(w+".doll", a.Doll)
		for s, it := range a.Equip {
			if !slots[s] {
				c.errorf(diag.UnknownReference, w+".equip", "unknown slot %q", s)
			}
			item, ok := db.Items[it]
			c.ref(w+".equip."+s, "item", it, ok)
			if ok && !item.Fits(s) {
				c.errorf(diag.UnknownReference, w+".equip."+s, "%s goes in %q", it, item.Slot)
			}
			if cl := db.Classes[a.Class]; ok && cl != nil && len(cl.Equip) > 0 && !slices.Contains(cl.Equip, item.Kind) {
				c.errorf(diag.UnknownReference, w+".equip."+s, "%s is a %q, which a %s cannot wear (the class's equip)", it, item.Kind, a.Class)
			}
		}
		c.file(w+".cutin", a.Cutin)
		if len(a.FaceBox) != 0 && len(a.FaceBox) != 3 {
			c.errorf(diag.Shape, w+".face_box", "want [x, y, size] (parts of the picture)")
		}
		if len(a.CutinBox) != 0 && len(a.CutinBox) != 3 {
			c.errorf(diag.Shape, w+".cutin_box", "want [x, y, size] (parts of the picture)")
		}
		if a.FaceShade < 0 || a.FaceShade > 1 {
			c.errorf(diag.OutOfRange, w+".face_shade", "want 0 to 1 (0 as drawn, 1 black), got %g", a.FaceShade)
		}
		if a.FaceShadeTop < 0 || a.FaceShadeTop > 1 {
			c.errorf(diag.OutOfRange, w+".face_shade_top", "want 0 to 1 (0 as drawn, 1 black), got %g", a.FaceShadeTop)
		}
		if (a.FaceShade > 0 || a.FaceShadeTop > 0) && a.Faces == "" {
			c.errorf(diag.Shape, w+".face_shade", "needs faces (the shade is for the face icons)")
		}
		c.ai(w+".ai", a.AI)
		c.faces(w, a.Faces, a.FaceRule)
		for when, f := range a.Reactions { // whether the battle reaction face image exists (without it a pink square used to appear)
			if a.Faces != "" && f != "" && !db.Exists("assets/"+a.Faces+"/"+f+".png") {
				c.errorf(diag.MissingAsset, w+".reactions."+when, "no face assets/%s/%s.png", a.Faces, f)
			}
		}
		if cl := db.Classes[a.Class]; cl != nil {
			sp := cl.StartSP + cl.SP*(a.Level-1)
			for node, lv := range a.Tree {
				i := slices.IndexFunc(cl.Tree, func(n TreeNode) bool { return n.ID == node })
				switch {
				case i < 0:
					c.errorf(diag.UnknownReference, w+".tree", "no node %q in the skill tree of %s", node, a.Class)
				case lv < 1 || lv > cl.Tree[i].Max:
					c.errorf(diag.OutOfRange, w+".tree."+node, "level %d (want 1..%d)", lv, cl.Tree[i].Max)
				case len(cl.Tree[i].Requires) > 0:
					c.errorf(diag.Shape, w+".tree."+node, "the node needs others first (raise only the first nodes)")
				}
				sp -= lv
			}
			if sp < 0 {
				c.errorf(diag.OutOfRange, w+".tree", "more levels than the start's SP")
			}
		}
	}
	for _, id := range db.Ordered("items") {
		it := db.Items[id]
		w := "items." + id
		if it.Slot != "" && !slices.ContainsFunc(db.Game.Slots, func(sl Slot) bool { return it.Fits(sl.ID) }) {
			c.errorf(diag.UnknownReference, w+".slot", "unknown slot %q", it.Slot)
		}
		if it.Slot != "" && !worn(db, it.Kind) {
			c.errorf(diag.UnknownReference, w+".kind", "no class can wear a %q (add it to a class's equip)", it.Kind)
		}
		for st := range it.Stats {
			if !slices.Contains(StatNames, st) {
				c.errorf(diag.UnknownReference, w+".stats", "unknown stat %q", st)
			}
		}
		if it.Use != nil {
			c.usage(w+".use", it.Use)
		}
		_, ok := db.Events[it.Event]
		c.ref(w+".event", "event", it.Event, ok)
		c.expr(w+".event_when", it.EventOK, FieldNames)
		if it.Command && it.Event == "" {
			c.errorf(diag.Shape, w+".command", "command needs an event")
		}
		c.element(w, it.Element)
		for _, gr := range it.States {
			_, ok := db.States[gr.State]
			c.ref(w+".states", "state", gr.State, ok)
			c.expr(w+".states.chance", gr.Chance, EffectNames)
		}
		for st := range it.StateResist {
			_, ok := db.States[st]
			c.ref(w+".state_resist", "state", st, ok)
		}
		if len(it.StateResist) > 0 && it.Slot == "" {
			c.errorf(diag.Shape, w+".state_resist", "state_resist is for equipment (an item with a slot)")
		}
	}
	for _, id := range db.Ordered("skills") {
		s := db.Skills[id]
		c.usage("skills."+id, &s.Usage)
	}
	for _, id := range db.Ordered("states") {
		s := db.States[id]
		for st := range s.StateResist {
			_, ok := db.States[st]
			c.ref("states."+id+".state_resist", "state", st, ok)
		}
		w := "states." + id
		if s.Restrict != "" && s.Restrict != "skip" && s.Restrict != "confuse" {
			c.errorf(diag.NotAWord, w+".restrict", "want skip or confuse, not %q", s.Restrict)
		}
		if s.RestrictChance < 0 || s.RestrictChance >= 1 || s.RestrictChance > 0 && s.Restrict != "skip" {
			c.errorf(diag.OutOfRange, w+".restrict_chance", "want 0 < chance < 1 with restrict: skip, not %g", s.RestrictChance)
		}
		for i := range s.Tick {
			c.effect(fmt.Sprintf("%s.on_turn_end[%d]", w, i), &s.Tick[i])
		}
		for st := range s.Stats {
			if !slices.Contains(StatNames, st) {
				c.errorf(diag.UnknownReference, w+".stats", "unknown stat %q", st)
			}
		}
	}
	for _, id := range db.Ordered("enemies") {
		e := db.Enemies[id]
		w := "enemies." + id
		c.picture(w+".image", e.Image)
		for _, d := range e.Drops {
			_, ok := db.Items[d.Item]
			c.ref(w+".drops", "item", d.Item, ok)
			c.expr(w+".drops.when", d.When, BattleNames)
		}
		for el := range e.Resist {
			c.element(w+".resist", el)
		}
		for st := range e.SResist {
			_, ok := db.States[st]
			c.ref(w+".state_resist", "state", st, ok)
		}
		if e.Hard < 0 {
			c.errorf(diag.OutOfRange, w+".hard", "want 0 or more")
		}
		c.ai(w+".ai", e.AI)
		for mood, f := range e.Faces {
			c.file(w+".faces."+mood, f)
		}
		for i, r := range e.FaceRule {
			c.expr(fmt.Sprintf("%s.face_rules[%d]", w, i), r.When, BattleNames)
			if _, ok := e.Faces[r.Face]; !ok {
				c.errorf(diag.UnknownReference, w+".face_rules", "unknown face %q (not in faces)", r.Face)
			}
		}
	}
	for _, id := range db.Ordered("troops") {
		t := db.Troops[id]
		w := "troops." + id
		for _, m := range t.Members {
			_, ok := db.Enemies[m]
			c.ref(w+".members", "enemy", m, ok)
		}
		c.picture(w+".background", t.Background)
		c.music(w+".music", t.Music)
		c.music(w+".win_music", t.WinMusic)
		c.text(w+".intro", t.Intro, BattleNames)
		if ls := t.LevelScale; ls != nil {
			if ls.Level < 1 {
				c.errorf(diag.OutOfRange, w+".level_scale.level", "%d (want 1 or more)", ls.Level)
			}
			if ls.Rate <= 0 || ls.Rate > 0.2 {
				c.errorf(diag.OutOfRange, w+".level_scale.rate", "%g (want more than 0, up to 0.2)", ls.Rate)
			}
			if !ls.Unbounded && (ls.Max <= 0 || ls.Max > 0.5) { // do not make it unboundedly strong like FF8 (training must always make things easier)
				c.errorf(diag.OutOfRange, w+".level_scale.max", "%g (want more than 0, up to 0.5)", ls.Max)
			}
			for _, s := range ls.Stats {
				if !slices.Contains(StatNames, s) {
					c.errorf(diag.UnknownReference, w+".level_scale.stats", "unknown stat %q", s)
				}
			}
		}
		for i, ev := range t.Events {
			ww := fmt.Sprintf("%s.events[%d]", w, i)
			c.expr(ww+".when", ev.When, BattleNames)
			c.commands(ww+".do", ev.Do, BattleNames)
		}
	}
	for _, id := range db.Ordered("locations") {
		l := db.Locations[id]
		w := "locations." + id
		c.picture(w+".background", l.Background)
		c.picture(w+".battle_background", l.BattleBG)
		c.music(w+".music", l.Music)
		c.music(w+".battle_music", l.BattleMusic)
		c.text(w+".desc", l.Desc, FieldNames)
		c.text(w+".area", l.Area, FieldNames)
		if l.ResumeTo != "" {
			r, ok := db.Locations[l.ResumeTo]
			c.ref(w+".resume_to", "location", l.ResumeTo, ok)
			if ok && r.ResumeTo != "" {
				c.errorf(diag.Shape, w+".resume_to", "%s has resume_to too (one step only)", l.ResumeTo)
			}
		}
		entryIDs := map[string]bool{}
		for i, p := range l.People {
			ww := fmt.Sprintf("%s.people[%d]", w, i)
			if p.ID != "" {
				if entryIDs[p.ID] {
					c.errorf(diag.Duplicate, ww+".id", "duplicate entry id %q", p.ID)
				}
				entryIDs[p.ID] = true
			}
			c.text(ww+".name", p.Name, FieldNames)
			c.file(ww+".image", p.Image)
			c.expr(ww+".when", p.When, FieldNames)
			c.expr(ww+".new", p.New, FieldNames)
			c.expr(ww+".mark", p.Mark, FieldNames)
			c.commands(ww+".talk", p.Talk, FieldNames)
			c.bubbles(ww+".bubbles", p.Bubbles)
		}
		for i, a := range l.Actions {
			ww := fmt.Sprintf("%s.actions[%d]", w, i)
			if a.ID != "" {
				if entryIDs[a.ID] {
					c.errorf(diag.Duplicate, ww+".id", "duplicate entry id %q", a.ID)
				}
				entryIDs[a.ID] = true
			}
			c.text(ww+".label", a.Label, FieldNames)
			c.expr(ww+".when", a.When, FieldNames)
			c.expr(ww+".new", a.New, FieldNames)
			c.expr(ww+".mark", a.Mark, FieldNames)
			c.commands(ww+".do", a.Do, FieldNames)
		}
		for i, e := range l.Exits {
			ww := fmt.Sprintf("%s.exits[%d]", w, i)
			c.text(ww+".label", e.Label, FieldNames)
			_, ok := db.Locations[e.To]
			c.ref(ww+".to", "location", e.To, ok)
			if e.To == "" && len(e.Do) == 0 {
				c.errorf(diag.Shape, ww, "an exit needs to or do")
			}
			c.expr(ww+".when", e.When, FieldNames)
			c.commands(ww+".do", e.Do, FieldNames)
			c.arrival(ww, e.To, e.At, e.Facing)
		}
		shows := 0
		for _, set := range []bool{l.Dungeon != nil, l.Map != nil, l.Stage != nil} {
			if set {
				shows++
			}
		}
		if shows > 1 {
			c.errorf(diag.Shape, w, "a location is shown one way: dungeon, map or stage (not two of them)")
		}
		if l.Dungeon != nil {
			c.dungeon(w, l)
		}
		if l.Map != nil {
			c.atlas(w, id, l)
		}
		if l.Stage != nil {
			c.stage(w, id, l)
		}
		c.expr(w+".encounters.rate", l.Encounters.Rate, FieldNames)
		for _, tp := range l.Encounters.Troops {
			_, ok := db.Troops[tp.Troop]
			c.ref(w+".encounters", "troop", tp.Troop, ok)
			c.expr(w+".encounters.when", tp.When, FieldNames)
		}
		c.commands(w+".on_enter", l.OnEnter, FieldNames)
	}
	for _, id := range db.Ordered("events") {
		e := db.Events[id]
		w := "events." + id
		switch e.Trigger {
		case "", "enter", "auto":
		default:
			c.errorf(diag.NotAWord, w+".trigger", "want enter or auto, not %q", e.Trigger)
		}
		_, ok := db.Locations[e.Location]
		c.ref(w+".location", "location", e.Location, ok)
		c.expr(w+".when", e.When, FieldNames)
		c.commands(w+".do", e.Do, FieldNames)
	}
	for _, id := range db.Ordered("shops") {
		s := db.Shops[id]
		for _, it := range s.Goods {
			_, ok := db.Items[it]
			c.ref("shops."+id+".goods", "item", it, ok)
		}
		c.file("shops."+id+".keeper", s.Keeper)
		if s.Spent != "" {
			zero, err := expr.Parse("0")
			if err != nil {
				c.errs = append(c.errs, err)
				continue
			}
			c.assignments("shops."+id+".spent", map[string]Expr{s.Spent: {Expr: zero}}, FieldNames)
		}
		if name, ok := strings.CutPrefix(s.Spent, "var."); s.Spent != "" && (!ok || name == "") {
			c.errorf(diag.NotAWord, "shops."+id+".spent", "want var.<name>, not %q", s.Spent)
		}
	}
	c.stones()
	for _, id := range db.Ordered("ai") {
		c.rules("ai."+id, db.AI[id])
	}
	for name, s := range db.Music.Songs {
		if s.File != "" { // a track
			c.file("music.songs."+name+".file", s.File)
			if len(s.Moods) > 0 {
				c.errorf(diag.Shape, "music.songs."+name, "a track (file) has no moods: give it dir and moods to make it modular")
			}
			continue
		}
		for mood, f := range s.Moods {
			c.file("music.songs."+name+"."+mood, s.Dir+"/"+f+".ogg")
		}
		if s.BPM <= 0 {
			c.errorf(diag.Shape, "music.songs."+name, "bpm is missing")
		}
	}
	for ctx, rules := range db.Music.Rules {
		names := FieldNames
		switch ctx {
		case "field":
		case "battle":
			names = BattleNames
		default:
			c.errorf(diag.NotAWord, "music.rules", "unknown context %q (want field or battle)", ctx)
		}
		for i, r := range rules {
			c.expr(fmt.Sprintf("music.rules.%s[%d]", ctx, i), r.When, names)
			for _, song := range []string{r.Song, r.Play} {
				if _, ok := db.Music.Songs[song]; song != "" && !ok {
					c.errorf(diag.UnknownReference, fmt.Sprintf("music.rules.%s[%d]", ctx, i), "unknown song %q", song)
				}
			}
			if r.Play == "" && r.Mood == "" {
				c.errorf(diag.Shape, fmt.Sprintf("music.rules.%s[%d]", ctx, i), "a rule needs a mood or a song to play")
			}
		}
	}
	c.faceWarnings()
	c.iconWarnings()
	db.sceneWarns, db.sceneWarned = sortedDiagnostics(c.warns), true
	return sortedDiagnostics(c.errs)
}

// galleryKeys rejects duplicate migration targets and mappings that leak unlock records into another recollection.
func (c *checker) galleryKeys(g Gallery) {
	owners := map[string]int{}
	claim := func(key, where string, i int, explicit bool) {
		if previous, ok := owners[key]; ok && previous != i {
			if explicit || g.Scenes[previous].ID != "" {
				c.errorf(diag.Duplicate, where, lang.T("回想のキー %q が gallery.scenes[%d] と重複している"), key, previous)
			}
		} else {
			owners[key] = i
		}
	}
	for i, sc := range g.Scenes {
		w := fmt.Sprintf("game.yaml gallery.scenes[%d]", i)
		if sc.ID != "" && !gameIDPattern.MatchString(sc.ID) {
			c.errorf(diag.Shape, w+".id", "%s", lang.T("回想 ID は英数字で始まる英数字・_・- にする"))
		}
		claim(sc.Key(), w+".id", i, sc.ID != "")
	}
	for i, sc := range g.Scenes {
		w := fmt.Sprintf("game.yaml gallery.scenes[%d].legacy_keys", i)
		if len(sc.LegacyKeys) > 0 && sc.ID == "" {
			c.errorf(diag.Shape, w, "%s", lang.T("旧キーの移行には回想 ID が必要"))
		}
		for j, key := range sc.LegacyKeys {
			at := fmt.Sprintf("%s[%d]", w, j)
			if !strings.HasPrefix(key, "scene:") || key == "scene:" {
				c.errorf(diag.Shape, at, "%s", lang.T("旧キーは scene: で始まる空でない回想キーにする"))
			}
			claim(key, at, i, true)
		}
	}
}

func (c *checker) music(where string, m MusicCue) {
	if m.Song == "" {
		return
	}
	s, ok := c.db.Music.Songs[m.Song]
	if !ok {
		c.errorf(diag.UnknownReference, where, "unknown song %q", m.Song)
		return
	}
	if _, ok := s.Moods[m.Mood]; m.Mood != "" && !ok {
		c.errorf(diag.UnknownReference, where, "song %q has no mood %q", m.Song, m.Mood)
	}
}

func (c *checker) element(where, el string) {
	if el == "" {
		return
	}
	if _, ok := c.db.Elements[el]; !ok {
		c.errorf(diag.UnknownReference, where, "unknown element %q", el)
	}
}

func (c *checker) faces(where, dir string, rules []FaceRule) {
	for i, r := range rules {
		c.exprIn(fmt.Sprintf("%s.face_rules[%d]", where, i), r.When, BattleNames, map[string]*vtype{"self": tSelfFace})
		if dir != "" && !c.db.Exists("assets/"+dir+"/"+r.Face+".png") {
			c.errorf(diag.MissingAsset, where+".face_rules", "no face assets/%s/%s.png", dir, r.Face)
		}
	}
}

func (c *checker) usage(where string, u *Usage) {
	if !slices.Contains(Scopes, u.Scope) {
		c.errorf(diag.NotAWord, where+".scope", "unknown scope %q (want one of %s)", u.Scope, strings.Join(Scopes, ", "))
	}
	if u.Fx != "" && !slices.Contains(Fxs, u.Fx) {
		c.errorf(diag.NotAWord, where+".fx", "unknown fx %q (want one of %s)", u.Fx, strings.Join(Fxs, ", "))
	}
	for _, w := range u.Where {
		if w != "battle" && w != "menu" {
			c.errorf(diag.NotAWord, where+".where", "want battle or menu, not %q", w)
		}
	}
	for i := range u.Effects {
		c.effect(fmt.Sprintf("%s.effects[%d]", where, i), &u.Effects[i])
	}
	c.expr(where+".hit", u.Hit, EffectNames)
	c.expr(where+".crit", u.Crit, EffectNames)
	c.expr(where+".speed", u.Speed, EffectNames)
}

func (c *checker) effect(where string, e *Effect) {
	if e.Variance != nil {
		c.numberRange(where+".variance", *e.Variance, 0, 1)
	}
	c.expr(where+".when", e.When, EffectNames)
	c.expr(where+".damage", e.Damage, EffectNames)
	c.expr(where+".heal", e.Heal, EffectNames)
	c.expr(where+".revive", e.Revive, EffectNames)
	c.expr(where+".chance", e.Chance, EffectNames)
	c.expr(where+".turns", e.Turns, EffectNames)
	if e.Turns.Set() && e.AddState == "" {
		c.errorf(diag.Shape, where+".turns", "turns goes with add_state")
	}
	if e.Stat != "" && e.Stat != "hp" && e.Stat != "mp" {
		c.errorf(diag.NotAWord, where+".stat", "want hp or mp, not %q", e.Stat)
	}
	c.element(where, e.Element)
	if e.AddState != "" {
		_, ok := c.db.States[e.AddState]
		c.ref(where+".add_state", "state", e.AddState, ok)
	}
	if e.Remove != "" && e.Remove != "bad" {
		_, ok := c.db.States[e.Remove]
		c.ref(where+".remove_state", "state", e.Remove, ok)
	}
	if e.Summon != "" {
		_, ok := c.db.Enemies[e.Summon]
		c.ref(where+".summon", "enemy", e.Summon, ok)
	}
	n := 0
	if e.Recoil < 0 || e.Recoil > 1 {
		c.errorf(diag.OutOfRange, where+".recoil", "recoil is a part of the damage (0 to 1)")
	}
	for _, set := range []bool{e.Damage.Set(), e.Heal.Set(), e.AddState != "", e.Remove != "", e.Revive.Set(), e.Escape, e.Summon != ""} {
		if set {
			n++
		}
	}
	if n != 1 {
		c.errorf(diag.Shape, where, "an effect needs exactly one of damage, heal, add_state, remove_state, revive, escape, summon")
	}
}

func (c *checker) ai(where string, ref AIRef) {
	if ref.Name != "" {
		if _, ok := c.db.AI[ref.Name]; !ok {
			c.errorf(diag.UnknownReference, where, "unknown ai %q", ref.Name)
		}
		return
	}
	c.rules(where, ref.Rules)
}

func (c *checker) rules(where string, rules []Rule) {
	for i, r := range rules {
		w := fmt.Sprintf("%s[%d]", where, i)
		c.expr(w+".when", r.When, BattleNames)
		c.expr(w+".chance", r.Chance, BattleNames)
		if (r.Use == "") == (r.Item == "") {
			c.errorf(diag.Shape, w, "a rule needs exactly one of use (a skill) and item")
		}
		if r.Use != "" {
			_, ok := c.db.Skills[r.Use]
			c.ref(w+".use", "skill", r.Use, ok)
		}
		if r.Item != "" {
			it := c.db.Items[r.Item]
			c.ref(w+".item", "item", r.Item, it != nil)
			if it != nil && r.Use == "" && c.db.RuleUsage(r) == nil {
				c.errorf(diag.UnknownReference, w+".item", lang.T("品 %q には use がなく、AI の行動には使えない"), r.Item)
			}
		}
		c.aiTarget(w+".target", r.Target)
	}
}

func (c *checker) aiTarget(w string, t Target) {
	if t.Preset != "" && !slices.Contains(TargetPresets, t.Preset) {
		c.errorf(diag.NotAWord, w, "unknown target %q (want one of %s)", t.Preset, strings.Join(TargetPresets, ", "))
	}
	if t.Side != "" && t.Side != "foes" && t.Side != "allies" {
		c.errorf(diag.NotAWord, w+".side", "want foes or allies, not %q", t.Side)
	}
	if t.Pick != "" && t.Pick != "min" && t.Pick != "max" && t.Pick != "random" && t.Pick != "first" {
		c.errorf(diag.NotAWord, w+".pick", "want min, max, random or first, not %q", t.Pick)
	}
	if (t.Pick == "min" || t.Pick == "max") && !t.By.Set() {
		c.errorf(diag.Shape, w, "pick %s needs by", t.Pick)
	}
	c.expr(w+".where", t.Where, TargetNames)
	c.expr(w+".by", t.By, TargetNames)
}

// say checks one line of dialogue.
func (c *checker) say(w string, s *Say, names []string) {
	db := c.db
	if a, ok := db.Actors[s.Who]; ok {
		switch {
		case s.Face == "":
		case a.Puppet != "": // a speaker with an animated portrait uses the portrait's expressions (unreadable portraits are reported by the party check)
			if names, ok := c.puppetFaces(s.Who, a); ok && !slices.Contains(names, s.Face) {
				c.errorf(diag.UnknownReference, w, "%s has no face %q (%s)", s.Who, s.Face, a.Puppet)
			}
		case a.Faces != "" && !db.Exists("assets/"+a.Faces+"/"+s.Face+".png"):
			c.errorf(diag.UnknownReference, w, "%s has no face %q", s.Who, s.Face)
		}
	} else if s.Face != "" { // for a speaker who is not a party member, face is an image path
		c.file(w+".face", s.Face)
	}
	c.text(w+".who", s.Who, names)
	if s.Side != "" && s.Side != "left" && s.Side != "right" {
		c.errorf(diag.NotAWord, w+".side", "want left or right")
	}
	c.text(w, s.Text, names)
}

func (c *checker) commands(where string, cmds []Command, names []string) {
	db := c.db
	names = withEventNames(names)
	for i := range cmds {
		cmd := &cmds[i]
		w := fmt.Sprintf("%s[%d]", where, i)
		if c.unknownFields(w, cmd) {
			continue
		}
		verb, err := cmd.Verb()
		if err != nil {
			c.errorf(diag.Shape, w, "%v", err)
			continue
		}
		c.text(w+".prompt", cmd.Prompt, names)
		switch verb {
		case "pick", "turn", "step", "walk", "equip", "buy", "sell", "act":
			if c.needsVersion2(w+"."+verb, verb) {
				c.newVerb(w, verb, cmd)
			}
		case "use":
			if !c.needsVersion2(w+".use", verb) {
				break
			}
			it, ok := db.Items[cmd.Use]
			c.ref(w+".use", "item", cmd.Use, ok)
			if ok && (it.Use == nil || !slices.Contains(it.Use.Where, "menu")) {
				c.errorf(diag.UnknownReference, w+".use", "%s cannot be used from the menu (use.where has no menu)", cmd.Use)
			}
			if cmd.Target != "" {
				_, ok := db.Actors[cmd.Target]
				c.ref(w+".target", "actor", cmd.Target, ok)
			}
		case "say":
			c.say(w+".say", cmd.Say, names)
			for j := range cmd.Say.Instead {
				ww := fmt.Sprintf("%s.say.instead[%d]", w, j)
				alt := &cmd.Say.Instead[j]
				if _, ok := db.Actors[alt.Who]; !ok {
					c.errorf(diag.UnknownReference, ww+".who", "%q is not an actor (instead is spoken by another party member)", alt.Who)
				}
				if len(alt.Instead) > 0 {
					c.errorf(diag.Shape, ww, "instead in instead")
				}
				c.say(ww, alt, names)
			}
		case "choice":
			ids := map[string]bool{}
			for j, o := range cmd.Choice {
				ww := fmt.Sprintf("%s.choice[%d]", w, j)
				if o.ID != "" {
					if ids[o.ID] {
						c.errorf(diag.Duplicate, ww+".id", "duplicate choice id %q", o.ID)
					}
					ids[o.ID] = true
				}
				c.text(ww+".label", o.Label, names)
				c.expr(ww+".when", o.When, names)
				c.commands(ww+".do", o.Do, names)
			}
		case "notice":
			c.text(w+".notice", cmd.Notice, names)
		case "if":
			c.expr(w+".if", cmd.If, names)
			c.commands(w+".then", cmd.Then, names)
			c.commands(w+".else", cmd.Else, names)
		case "set":
			c.assignments(w+".set", cmd.SetV, names)
		case "give", "take":
			ic := cmd.Give
			if ic == nil {
				ic = cmd.Take
			}
			_, ok := db.Items[ic.Item]
			c.ref(w+"."+verb, "item", ic.Item, ok)
		case "gold":
			c.expr(w+".gold", cmd.Gold, names)
		case "exp":
			c.expr(w+".exp", cmd.Exp, names)
		case "join", "leave":
			id := cmd.Join + cmd.Leave
			_, ok := db.Actors[id]
			c.ref(w+"."+verb, "actor", id, ok)
		case "hurt", "affect":
			var who string
			if cmd.Hurt != nil {
				who = cmd.Hurt.Who
				c.expr(w+".hurt.amount", cmd.Hurt.Amount, append(slices.Clone(names), "self"))
				if !cmd.Hurt.Amount.Set() {
					c.errorf(diag.Shape, w+".hurt", "hurt needs an amount")
				}
			} else {
				who = cmd.Affect.Who
				// add accepts only existing states. bad (all bad states) can be used only with remove.
				if s := cmd.Affect.Add; s != "" {
					_, ok := db.States[s]
					c.ref(w+".affect.add", "state", s, ok)
				}
				if s := cmd.Affect.Remove; s != "" && s != "bad" {
					_, ok := db.States[s]
					c.ref(w+".affect.remove", "state (or bad)", s, ok)
				}
				if cmd.Affect.Add == "" && cmd.Affect.Remove == "" {
					c.errorf(diag.Shape, w+".affect", "affect needs add or remove")
				}
			}
			if who != "all" {
				_, ok := db.Actors[who]
				c.ref(w+"."+verb+".who", "actor (or all)", who, ok)
			}
		case "heal":
			if cmd.Heal != "all" {
				_, ok := db.Actors[cmd.Heal]
				c.ref(w+".heal", "actor (or all)", cmd.Heal, ok)
			}
		case "battle":
			_, ok := db.Troops[cmd.Battle]
			c.ref(w+".battle", "troop", cmd.Battle, ok)
			for _, f := range cmd.Fighters {
				_, ok := db.Actors[f]
				c.ref(w+".fighters", "actor", f, ok)
			}
			c.commands(w+".on_win", cmd.OnWin, names)
			c.commands(w+".on_lose", cmd.OnLose, names)
			c.commands(w+".on_escape", cmd.OnRun, names)
		case "move":
			_, ok := db.Locations[cmd.Move]
			c.ref(w+".move", "location", cmd.Move, ok)
			c.arrival(w, cmd.Move, cmd.At, cmd.Facing)
		case "music":
			c.music(w+".music", *cmd.Music)
		case "image":
			c.picture(w+".image", cmd.Image.Picture())
			if cmd.Image.Over.File != "" {
				c.picture(w+".image.over", cmd.Image.Over)
			}
			if len(cmd.Image.Files) > 0 {
				if cmd.Image.File != "" {
					c.errorf(diag.Shape, w+".image", "file or files, not both")
				}
				// a pattern may match nothing (pictures to come), but some picture must be shown
				pages := cmd.Image.Pages(c.db.FS)
				if len(pages) == 0 && !cmd.Image.Optional {
					c.errorf(diag.MissingAsset, w+".image.files", "no picture matches %v under assets/", cmd.Image.Files)
				}
				for _, p := range pages {
					c.picture(w+".image.files", Picture{File: p})
				}
			}
			if f := cmd.Image.Fit; f != "" && f != "contain" && f != "cover" {
				c.errorf(diag.NotAWord, w+".image", "fit is contain or cover, not %q", f)
			}
			if cmd.Image.Fade < 0 {
				c.errorf(diag.OutOfRange, w+".image", "fade is seconds (0 or more)")
			}
			if cmd.Image.Auto < 0 || (cmd.Image.Auto > 0 && (len(cmd.Image.Files) == 0 || cmd.Image.Page != 0 || cmd.Image.Pick.Set())) {
				c.errorf(diag.OutOfRange, w+".image.auto", "auto is seconds a picture (more than 0), with files shown one after another (not page or pick)")
			}
			if cmd.Image.Page < 0 || (cmd.Image.Page > 0 && len(cmd.Image.Files) == 0) {
				c.errorf(diag.OutOfRange, w+".image.page", "page is 1 or more, with files")
			}
			if cmd.Image.Pick.Set() {
				if len(cmd.Image.Files) == 0 || cmd.Image.Page != 0 {
					c.errorf(diag.Shape, w+".image.pick", "pick is with files, not with page")
				}
				c.expr(w+".image.pick", cmd.Image.Pick, names)
			}
			if l := cmd.Image.Look; l != "" && l != "dream" {
				c.errorf(diag.NotAWord, w+".image.look", "look is dream, not %q", l)
			}
			if cmd.Image.Haze.Valid && cmd.Image.Look == "" {
				c.errorf(diag.Shape, w+".image.haze", "haze is with look: dream")
			}
		case "shop":
			_, ok := db.Shops[cmd.Shop]
			c.ref(w+".shop", "shop", cmd.Shop, ok)
		case "call":
			_, ok := db.Events[cmd.Call]
			c.ref(w+".call", "event", cmd.Call, ok)
		case "stone_suggest", "stone_apply", "stone_choose", "stone_wearer", "stone_decline":
			name := cmd.StoneSuggest + cmd.StoneApply + cmd.StoneChoose + cmd.StoneWearer + cmd.StoneDecline
			if !varName.MatchString(name) {
				c.errorf(diag.NotAWord, w+"."+verb, "want a name for var.<name> (letters, digits, _), not %q", name)
			}
		case "sp_suggest", "sp_apply", "sp_decline":
			if name := cmd.SPSuggest + cmd.SPApply + cmd.SPDecline; !varName.MatchString(name) {
				c.errorf(diag.NotAWord, w+"."+verb, "want a name for var.<name> (letters, digits, _), not %q", name)
			}
		case "equip_stones":
			if !cmd.EquipStones.Actor.Set() || !cmd.EquipStones.Set.Set() {
				c.errorf(diag.Shape, w+".equip_stones", "actor and set are needed")
			}
			c.expr(w+".equip_stones.actor", cmd.EquipStones.Actor, names)
			c.expr(w+".equip_stones.set", cmd.EquipStones.Set, names)
		case "reset_tree":
			if cmd.ResetTree != "all" {
				_, ok := db.Actors[cmd.ResetTree]
				c.ref(w+".reset_tree", "actor", cmd.ResetTree, ok)
			}
		case "learn":
			_, ok := db.Actors[cmd.Learn.Actor]
			c.ref(w+".learn.actor", "actor", cmd.Learn.Actor, ok)
			_, ok = db.Skills[cmd.Learn.Skill]
			c.ref(w+".learn.skill", "skill", cmd.Learn.Skill, ok)
		case "open":
			if slices.Contains(db.Game.Screens, cmd.Open) {
				c.needsVersion2(w+".open", "a declared screen")
			} else if !slices.Contains(OpenScreens, cmd.Open) {
				c.errorf(diag.NotAWord, w+".open", "want one of %s (or a screen declared in game.yaml screens), not %q", strings.Join(OpenScreens, ", "), cmd.Open)
			}
		case "dive":
			if l, ok := db.Locations[cmd.Dive]; !ok {
				c.ref(w+".dive", "location", cmd.Dive, false)
			} else if l.Dungeon == nil || l.Dungeon.Generate == nil {
				c.errorf(diag.UnknownReference, w+".dive", "%s is not a made dungeon (a dungeon with generate)", cmd.Dive)
			}
			c.expr(w+".floor", cmd.Floor, names)
			c.arrival(w, cmd.Dive, cmd.At, cmd.Facing)
		case "open_wall":
			if r := []rune(cmd.OpenWall); len(r) != 1 || !c.dungeonTile(cmd.OpenWall) {
				c.errorf(diag.UnknownReference, w+".open_wall", "%q is not a tile of any dungeon (one character in tiles)", cmd.OpenWall)
			}
		case "effect":
			if cmd.Effect != "shake" && cmd.Effect != "flash" && cmd.Effect != "fade" {
				c.errorf(diag.NotAWord, w+".effect", "want shake, flash or fade, not %q", cmd.Effect)
			}
		case "mood", "wait", "sfx", "end", "clear", "save", "done", "ran", "cross", "reach":
			// Commands without extra reference or expression checks are listed explicitly too, so missed registrations are detected.
		default:
			c.errorf(diag.Shape, w, "command %q has no semantic checker", verb)
		}
	}
}

// text checks the {expressions} embedded in a line.
func (c *checker) text(where, s string, names []string) {
	for _, part := range Interpolations(s) {
		e, err := parseExpr(part)
		if err != nil {
			c.errorf(diag.Expression, where, "in the text: %v", err)
			continue
		}
		c.expr(where, e, names)
	}
}

// ---- Checking expression fields
//
// Built-in values with a fixed type (party, self, pos, etc.) are checked down to the field name
// (catching typos such as party.min_hp_rtae). Values whose names are freely chosen, such as flag and var,
// are not checked. For values whose names are IDs, such as item, actor, states and with_, the ID must exist.
// The field lists follow the expression evaluators (rpg's ActorView, Group and fieldEnv, and battle's
// Unit and Battle.Env).

type vkind int

const (
	vAny  vkind = iota // unknown (not checked any further)
	vNum               // number
	vStr               // string
	vBool              // boolean
	vFunc              // function (rand, chance)
	vObj               // object with fixed fields
	vDyn               // value that can be looked up by any name (flag, var, item, etc.)
	vList              // list (members)
)

// vtype is the type of a value in an expression.
type vtype struct {
	kind   vkind
	desc   string            // name used in error messages
	fields map[string]*vtype // fields of a vObj
	// prefix looks up names that are not in a vObj's fields (with_poison, min_hp, etc.).
	// It returns the type, or an explanation for an error (if both are empty, the name is unknown).
	prefix func(c *checker, name string) (*vtype, string)
	elem   *vtype // contents of a vDyn or vList
	key    string // vDyn: the kind that a name must be an ID of (item, actor, state)
}

var (
	tAny  = &vtype{kind: vAny}
	tNum  = &vtype{kind: vNum, desc: "a number"}
	tStr  = &vtype{kind: vStr, desc: "a string"}
	tBool = &vtype{kind: vBool, desc: "a boolean"}
	tFunc = &vtype{kind: vFunc, desc: "a function"}
)

func objType(desc string, fields map[string]*vtype, prefix func(*checker, string) (*vtype, string)) *vtype {
	return &vtype{kind: vObj, desc: desc, fields: fields, prefix: prefix}
}

// statsFields is fields with the eight stats added as numbers.
func statsFields(m map[string]*vtype) map[string]*vtype {
	for _, s := range StatNames {
		m[s] = tNum
	}
	return m
}

var (
	tStates = &vtype{kind: vDyn, desc: "states", elem: tNum, key: "state"}
	tWears  = &vtype{kind: vDyn, desc: "wears", elem: tBool, key: "item"}
	// tActor is rpg's ActorView (a party character as seen in the field).
	tActor = objType("a party member", statsFields(map[string]*vtype{
		"id": tStr, "name": tStr, "level": tNum, "exp": tNum, "hp": tNum, "mp": tNum,
		"hp_rate": tNum, "mp_rate": tNum, "alive": tBool, "dead": tBool, "class": tStr,
		"equip": {kind: vDyn, desc: "equip", elem: tStr}, "states": tStates, "back": tBool, "sp": tNum,
		"wears": tWears, "tactics": objType("tactics", map[string]*vtype{"mode": tStr, "preset": tStr, "rows": {kind: vList, desc: "rows", elem: objType("a tactics row", map[string]*vtype{"enabled": tBool, "condition": tStr, "percent": tNum, "target": tStr, "action": tStr}, nil)}}, nil),
		"skills": {kind: vList, desc: "skills", elem: tStr}, "sets": {kind: vList, desc: "sets", elem: tStr},
	}), nil)
	// tActorIn is actor.<ID> (with in_party added).
	tActorIn = objType("an actor", func() map[string]*vtype {
		m := maps.Clone(tActor.fields)
		m["in_party"] = tBool
		return m
	}(), nil)
	// tUnit is battle's Unit (an ally or enemy in battle).
	tUnit = objType("a battler", statsFields(map[string]*vtype{
		"name": tStr, "id": tStr, "hp": tNum, "mp": tNum, "hp_rate": tNum, "mp_rate": tNum,
		"alive": tBool, "dead": tBool, "level": tNum, "is_enemy": tBool, "guarding": tBool,
		"index": tNum, "boss": tBool, "states": tStates, "fell_with": tStates, "state_count": tNum,
		"element": tStr, "class": tStr, "back": tBool, "lv": tNum, "wears": tWears,
	}), resistPrefix)
	// tSelfFace is self in expression rules (ActorView in the field, Unit in battle).
	tSelfFace = objType("a battler", func() map[string]*vtype {
		m := maps.Clone(tActor.fields)
		maps.Copy(m, tUnit.fields)
		return m
	}(), resistPrefix)
	tPos = objType("pos", map[string]*vtype{"x": tNum, "y": tNum, "facing": tStr}, nil)
	// tMenuEntry is one menu item (created by an event).
	tMenuEntry = objType("a menu entry", map[string]*vtype{"kind": tStr, "label": tStr, "new": tBool, "marked": tBool}, nil)
	// tRepair is one entry of repairs (rpg's LoadChange).
	tRepair = objType("a repair", map[string]*vtype{"id": tStr, "path": tStr, "reason": tStr}, nil)
)

// resistPrefix is resist_<element> (how many times stronger that element hits).
func resistPrefix(c *checker, name string) (*vtype, string) {
	el, ok := strings.CutPrefix(name, "resist_")
	if !ok {
		return nil, ""
	}
	if _, ok := c.db.Elements[el]; !ok {
		return nil, fmt.Sprintf("unknown element %q", el)
	}
	return tNum, ""
}

// groupOf is rpg's Group (party, allies, foes, enemies).
func groupOf(member *vtype) *vtype {
	return objType("a group", map[string]*vtype{
		"size": tNum, "alive": tNum, "dead": tNum, "hp_rate": tNum,
		"members": {kind: vList, desc: "members", elem: member},
	}, func(c *checker, name string) (*vtype, string) {
		if st, ok := strings.CutPrefix(name, "with_"); ok {
			if _, ok := c.db.States[st]; !ok {
				return nil, fmt.Sprintf("unknown state %q", st)
			}
			return tNum, ""
		}
		if it, ok := strings.CutPrefix(name, "wearing_"); ok {
			if _, ok := c.db.Items[it]; !ok {
				return nil, fmt.Sprintf("unknown item %q", it)
			}
			return tNum, ""
		}
		for _, agg := range []string{"min_", "max_", "avg_", "sum_"} {
			f, ok := strings.CutPrefix(name, agg)
			if !ok {
				continue
			}
			t, problem := c.fieldOf(member, f)
			if problem != "" {
				return nil, problem
			}
			if t.kind != vNum && t.kind != vBool {
				return nil, fmt.Sprintf("%s%s: %s is not a number", agg, f, f)
			}
			return tNum, ""
		}
		return nil, ""
	})
}

var (
	tFieldParty  = groupOf(tActor)
	tBattleGroup = groupOf(tUnit)
)

// typeOf is the type of name n among names (the names usable at that location).
func typeOf(n string, names []string) *vtype {
	battle := slices.Contains(names, "turn")
	switch n {
	case "flag":
		return &vtype{kind: vDyn, desc: "flag", elem: tBool}
	case "var":
		return &vtype{kind: vDyn, desc: "var", elem: tAny}
	case "item":
		return &vtype{kind: vDyn, desc: "item", elem: tNum, key: "item"}
	case "actor":
		return &vtype{kind: vDyn, desc: "actor", elem: tActorIn, key: "actor"}
	case "gold", "members", "steps", "turn", "level":
		return tNum
	case "location", "result", "phase", "spot":
		return tStr
	case "over", "cleared":
		return tBool
	case "cell":
		return tFunc
	case "map":
		return &vtype{kind: vList, desc: "map", elem: tStr}
	case "menu":
		return &vtype{kind: vList, desc: "menu", elem: tMenuEntry}
	case "repairs":
		return &vtype{kind: vList, desc: "repairs", elem: tRepair}
	case "ran":
		return &vtype{kind: vDyn, desc: "ran", elem: objType("a run", map[string]*vtype{"count": tNum, "order": tNum}, nil), key: "event"}
	case "completed", "moved":
		return tBool
	case "roamers":
		return &vtype{kind: vList, desc: "roamers", elem: objType("a roamer", map[string]*vtype{"kind": tNum, "name": tStr, "troop": tStr, "x": tNum, "y": tNum}, nil)}
	case "rand", "chance", "pages":
		return tFunc
	case "pos":
		return tPos
	case "party":
		if battle {
			return tBattleGroup
		}
		return tFieldParty
	case "allies", "foes", "enemies":
		return tBattleGroup
	case "self", "boss", "a", "b", "t":
		return tUnit
	}
	return tAny
}

// fieldOf is the type of field name of type t; for an error it returns an explanation.
func (c *checker) fieldOf(t *vtype, name string) (*vtype, string) {
	if t.fields != nil {
		if f, ok := t.fields[name]; ok {
			return f, ""
		}
	}
	if t.prefix != nil {
		if f, problem := t.prefix(c, name); f != nil || problem != "" {
			return f, problem
		}
	}
	known := slices.Sorted(maps.Keys(t.fields))
	return nil, fmt.Sprintf("%s has no field %q (known: %s)", t.desc, name, strings.Join(known, ", "))
}

// step is the type reached by following one step from type t; for an error it returns an explanation.
func (c *checker) step(t *vtype, st expr.Step) (*vtype, string) {
	switch t.kind {
	case vAny:
		return tAny, ""
	case vNum, vStr, vBool, vFunc:
		return nil, t.desc + " has no fields"
	case vList:
		if st.Index && st.Name == "" {
			return t.elem, ""
		}
		return nil, t.desc + " is a list: index it with a number"
	case vDyn:
		if st.Name == "" { // expression index: the name is unknown
			return t.elem, ""
		}
		if t.key != "" && !c.known(t.key, st.Name) {
			return nil, fmt.Sprintf("unknown %s %q", t.key, st.Name)
		}
		return t.elem, ""
	case vObj:
		if st.Name == "" {
			return tAny, ""
		}
		return c.fieldOf(t, st.Name)
	}
	return tAny, ""
}

// known reports whether an ID of kind exists.
func (c *checker) known(kind, id string) bool {
	switch kind {
	case "item":
		return c.db.Items[id] != nil
	case "actor":
		return c.db.Actors[id] != nil
	case "state":
		return c.db.States[id] != nil
	case "event":
		return c.db.Events[id] != nil
	}
	return true
}

// fields checks a sequence of expression fields by following the types (over is the type of names that exist only at that location).
func (c *checker) fields(where string, e Expr, names []string, over map[string]*vtype) {
	for _, r := range e.Refs() {
		if !slices.Contains(names, r.Name) {
			continue // expr reports unknown names
		}
		t := over[r.Name]
		if t == nil {
			t = typeOf(r.Name, names)
		}
		if c.db.Game.State.Strict && (r.Name == "flag" || r.Name == "var") {
			t = c.stateType(r.Name)
			if len(r.Path) == 0 || r.Path[0].Name == "" {
				c.errorf(diag.Expression, where, lang.T("%s の参照は宣言名を直接指定する必要がある"), r.Name)
				continue
			}
		}
		path := r.Name
		for _, st := range r.Path {
			next, problem := c.step(t, st)
			if problem != "" {
				c.errorf(diag.Expression, where, "%q: %s: %s", e.String(), path, problem)
				break
			}
			if st.Name == "" {
				path += "[]"
			} else {
				path += "." + st.Name
			}
			t = next
		}
	}
}

// OpenScreens are the screens the open command can open: formation, the skill tree (skills),
// stone fusion (fusion), and out-of-battle tactics editing (tactics).
// status, item, skill, equip, save, load, config and title are single menu screens (not scattered across shops and inns).
var OpenScreens = []string{"formation", "skills", "fusion", "status", "item", "skill", "equip", "save", "load", "config", "title", "tactics"}

// tree checks a class's skill tree: node IDs (or skill IDs when absent) must not collide,
// skill, stat and prerequisite nodes must exist, and prerequisites must not form a cycle.
func (c *checker) tree(where string, nodes []TreeNode) {
	byID := map[string]*TreeNode{}
	for i := range nodes {
		n := &nodes[i]
		w := fmt.Sprintf("%s[%d]", where, i)
		if n.ID == "" {
			c.errorf(diag.Shape, w, "a node needs id (or skill)")
			continue
		}
		if byID[n.ID] != nil {
			c.errorf(diag.Duplicate, w, "the node id %q is used twice", n.ID)
		}
		byID[n.ID] = n
		if n.Skill != "" {
			_, ok := c.db.Skills[n.Skill]
			c.ref(w+".skill", "skill", n.Skill, ok)
		} else if len(n.Stats) == 0 {
			c.errorf(diag.Shape, w, "a node needs skill or stats")
		}
		for st := range n.Stats {
			if !slices.Contains(StatNames, st) {
				c.errorf(diag.UnknownReference, w+".stats", "unknown stat %q", st)
			}
		}
		if n.Max < 1 {
			c.errorf(diag.OutOfRange, w+".max", "want 1 or more")
		}
	}
	for i := range nodes {
		n := &nodes[i]
		w := fmt.Sprintf("%s[%d]", where, i)
		for j, r := range n.Requires {
			ww := fmt.Sprintf("%s.requires[%d]", w, j)
			req := byID[r.Node]
			switch {
			case r.Node == "":
				c.errorf(diag.Shape, ww, "node is missing")
			case r.Node == n.ID:
				c.errorf(diag.Shape, ww, "a node cannot require itself")
			case req == nil:
				c.errorf(diag.UnknownReference, ww, "unknown node %q (not in this tree)", r.Node)
			case r.Level < 1 || r.Level > req.Max:
				c.errorf(diag.OutOfRange, ww+".level", "want 1 to %d (the max of %s), not %d", req.Max, r.Node, r.Level)
			}
		}
	}
	// Neither node of a prerequisite cycle (a waits for b, b waits for a) opens.
	state := map[string]int{} // 0 not visited, 1 on the current path, 2 done
	var visit func(id string) bool
	visit = func(id string) bool {
		switch state[id] {
		case 1:
			return false
		case 2:
			return true
		}
		state[id] = 1
		if n := byID[id]; n != nil {
			for _, r := range n.Requires {
				if r.Node != id && byID[r.Node] != nil && !visit(r.Node) {
					return false
				}
			}
		}
		state[id] = 2
		return true
	}
	for _, n := range nodes {
		if n.ID != "" && state[n.ID] == 0 && !visit(n.ID) {
			c.errorf(diag.Shape, where, "the requires go round in a circle (through %s)", n.ID)
			return
		}
	}
}

// varName is a name that can follow var. (the stone and SP commands write var.<name>_*).
var varName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// stones checks the references of stone combinations (stone_sets) and fusions (fusions).
func (c *checker) stones() {
	db := c.db
	for _, id := range db.Ordered("stone_sets") {
		s := db.StoneSets[id]
		w := "stone_sets." + id
		if s.Family != "" && !gameIDPattern.MatchString(s.Family) {
			c.errorf(diag.Shape, w+".family", "%s", lang.T("セットの系列 ID は英数字で始まる英数字・_・- にする"))
		}
		if s.Name == "" {
			c.errorf(diag.Shape, w, "name is missing")
		}
		if len(s.Stones) == 0 {
			c.errorf(diag.Shape, w, "stones is empty")
		}
		for _, st := range s.Stones {
			it, ok := db.Items[st]
			c.ref(w+".stones", "item", st, ok)
			if ok && !it.Equipment() {
				c.errorf(diag.UnknownReference, w+".stones", "%s is not equipment (it has no slot)", st)
			}
		}
		if s.Skill != "" {
			_, ok := db.Skills[s.Skill]
			c.ref(w+".skill", "skill", s.Skill, ok)
		}
		for st := range s.Stats {
			if !slices.Contains(StatNames, st) {
				c.errorf(diag.UnknownReference, w+".stats", "unknown stat %q", st)
			}
		}
		if s.Skill == "" && len(s.Stats) == 0 {
			c.errorf(diag.Shape, w, "a stone set needs skill or stats")
		}
		for _, a := range s.For {
			_, ok := db.Actors[a]
			c.ref(w+".for", "actor", a, ok)
		}
		if s.Only && len(s.For) == 0 {
			c.errorf(diag.Shape, w+".only", "only needs for")
		}
	}
	for i, f := range db.Fusions {
		w := fmt.Sprintf("fusions[%d]", i)
		if len(f.From) == 0 {
			c.errorf(diag.Shape, w+".from", "from is empty")
		}
		for _, it := range f.From {
			_, ok := db.Items[it]
			c.ref(w+".from", "item", it, ok)
		}
		if f.To == "" {
			c.errorf(diag.Shape, w, "to is missing")
		}
		_, ok := db.Items[f.To]
		c.ref(w+".to", "item", f.To, ok)
		if f.Price < 0 {
			c.errorf(diag.OutOfRange, w+".price", "want 0 or more, not %d", f.Price)
		}
	}
}

// worn reports whether some class can wear equipment of that kind (a class without equip can wear anything; no class means no constraint).
func worn(db *DB, kind string) bool {
	if len(db.Classes) == 0 {
		return true
	}
	for _, cl := range db.Classes {
		if len(cl.Equip) == 0 || slices.Contains(cl.Equip, kind) {
			return true
		}
	}
	return false
}

// dungeonTile reports whether some dungeon has the tile ch (what open_wall puts).
func (c *checker) dungeonTile(ch string) bool {
	for _, l := range c.db.Locations {
		if l.Dungeon != nil && l.Dungeon.Tiles[ch] != nil {
			return true
		}
	}
	return false
}
