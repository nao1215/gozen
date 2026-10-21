package gamedata

import (
	"fmt"
	"slices"

	"github.com/nao1215/gozen/internal/expr"

	"github.com/nao1215/gozen/internal/diag"
)

// Profile is a page of the character book (ADR 0031): who a character is, shown on the
// Characters screen once its when holds. A profile whose id is an actor's id belongs to
// that actor without writing actor.
type Profile struct {
	// Actor is the party member the profile shows (default: the actor of the same id).
	Actor string `yaml:"actor"`
	// Enemy is the enemy the profile shows, for a foe in the book.
	Enemy string `yaml:"enemy"`
	// Name is the name shown (default: the actor's or the enemy's name, else the id).
	Name string `yaml:"name"`
	// Title is a short epithet under the name.
	Title string `yaml:"title"`
	// Picture is a standing picture under assets/ for one who is neither an actor nor an
	// enemy (an NPC or a speaker).
	Picture string `yaml:"picture"`
	// Faces are the faces the screen can switch between: names of the actor's faces
	// (assets/<faces>/<face>.png) or of its puppet's faces. Default: the actor's face.
	Faces []string `yaml:"faces"`
	// When unlocks the profile (always unlocked without it). A locked profile is a
	// shadow named "???". Under --dev every profile is unlocked.
	When Expr `yaml:"when"`
	// Fields are lines of label and value.
	Fields []ProfileField `yaml:"fields"`
	// Text is the introduction; {expr} is filled in like a line.
	Text string `yaml:"text"`
	// Relations are how the character sees the others: what they call them, and a note.
	Relations []ProfileRelation `yaml:"relations"`
}

// ProfileField is a line of a profile: Role shield-bearer, Likes cheese. The labels are
// the game's own.
type ProfileField struct {
	Label string `yaml:"label"`
	Value string `yaml:"value"`
}

// ProfileRelation is how a character sees another: who (a profile, an actor, or
// "you"), what they call them and a note.
type ProfileRelation struct {
	Who   string `yaml:"who"`
	Calls string `yaml:"calls"`
	Text  string `yaml:"text"`
}

// ProfileActor is the actor a profile shows: actor, else the actor of the same id ("" for none).
func (db *DB) ProfileActor(id string) string {
	p := db.Profiles[id]
	if p == nil {
		return ""
	}
	if p.Actor != "" {
		return p.Actor
	}
	if db.Actors[id] != nil && p.Enemy == "" && p.Picture == "" {
		return id
	}
	return ""
}

// ProfileName is the name a profile shows.
func (db *DB) ProfileName(id string) string {
	p := db.Profiles[id]
	switch {
	case p == nil:
		return id
	case p.Name != "":
		return p.Name
	}
	if a := db.Actors[db.ProfileActor(id)]; a != nil {
		return a.Name
	}
	if e := db.Enemies[p.Enemy]; e != nil {
		return e.Name
	}
	return id
}

// ProfileFaces are the faces a profile can show, the first is shown first ("" for a
// picture that is not an actor's).
func (db *DB) ProfileFaces(id string) []string {
	p := db.Profiles[id]
	if p == nil {
		return nil
	}
	if len(p.Faces) > 0 {
		return p.Faces
	}
	if a := db.Actors[db.ProfileActor(id)]; a != nil {
		face := a.Face
		if face == "" {
			face = "normal"
		}
		return []string{face}
	}
	return []string{""}
}

// profiles checks the character book: the actor, enemy, pictures and faces it points
// at, the names its conditions use and whom its relations name.
func (c *checker) profiles() {
	db := c.db
	if len(db.Profiles) > 0 {
		c.needsVersion("profiles", "profiles", SchemaVersion4)
	}
	for _, id := range db.Order["profiles"] {
		p := db.Profiles[id]
		w := "profiles." + id
		if p.Actor != "" {
			_, ok := db.Actors[p.Actor]
			c.ref(w+".actor", "actor", p.Actor, ok)
		}
		if p.Enemy != "" {
			_, ok := db.Enemies[p.Enemy]
			c.ref(w+".enemy", "enemy", p.Enemy, ok)
		}
		if n := btoi(p.Actor != "") + btoi(p.Enemy != "") + btoi(p.Picture != ""); n > 1 {
			c.errorf(diag.Shape, w, "write one of actor, enemy and picture, not %d", n)
		}
		c.file(w+".picture", p.Picture)
		if p.Actor == "" && p.Enemy == "" && p.Picture == "" && db.Actors[id] == nil {
			c.errorf(diag.Shape, w, "no actor %q of the same id: write actor, enemy or picture", id)
		}
		c.profileFaces(w, id, p)
		c.expr(w+".when", p.When, profileNames)
		c.text(w+".title", p.Title, profileNames)
		c.text(w+".text", p.Text, profileNames)
		for i, f := range p.Fields {
			fw := fmt.Sprintf("%s.fields[%d]", w, i)
			if f.Label == "" {
				c.errorf(diag.Shape, fw, "label is missing")
			}
			c.text(fw+".value", f.Value, profileNames)
		}
		for i, r := range p.Relations {
			rw := fmt.Sprintf("%s.relations[%d]", w, i)
			_, isProfile := db.Profiles[r.Who]
			_, isActor := db.Actors[r.Who]
			if r.Who != "you" && !isProfile && !isActor {
				c.errorf(diag.UnknownReference, rw+".who", "unknown character %q (a profile, an actor, or you)", r.Who)
			}
			c.text(rw+".calls", r.Calls, profileNames)
			c.text(rw+".text", r.Text, profileNames)
		}
	}
}

func (c *checker) profileFaces(w, id string, p *Profile) {
	db := c.db
	a := db.Actors[db.ProfileActor(id)]
	if a == nil {
		if len(p.Faces) > 0 {
			c.errorf(diag.Shape, w+".faces", "faces need an actor (the faces are the actor's)")
		}
		return
	}
	if a.Faces == "" && a.Puppet == "" {
		c.errorf(diag.Shape, w, "actor %s has no faces nor puppet to show", db.ProfileActor(id))
		return
	}
	var puppet []string
	readable := false
	if a.Puppet != "" {
		puppet, readable = c.puppetFaces(db.ProfileActor(id), a)
	}
	for i, f := range db.ProfileFaces(id) {
		if slices.Contains(puppet, f) {
			continue
		}
		if a.Faces == "" {
			if readable {
				c.errorf(diag.UnknownReference, fmt.Sprintf("%s.faces[%d]", w, i), "the puppet of %s has no face %q", db.ProfileActor(id), f)
			}
			continue
		}
		c.file(fmt.Sprintf("%s.faces[%d]", w, i), a.Faces+"/"+f+".png")
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// texts are the texts of the profile a translation covers.
func (p *Profile) texts() []*string {
	out := []*string{&p.Name, &p.Title, &p.Text}
	for i := range p.Fields {
		out = append(out, &p.Fields[i].Label, &p.Fields[i].Value)
	}
	for i := range p.Relations {
		out = append(out, &p.Relations[i].Calls, &p.Relations[i].Text)
	}
	return out
}

// Holds reports whether a condition holds in env: true when it is not written, false when
// it fails to evaluate.
func holds(e Expr, env expr.Env) bool {
	if !e.Set() {
		return true
	}
	ok, err := e.Bool(env)
	return err == nil && ok
}

// ProfileHolds reports whether profile id's when holds in env (an unknown id never does).
func (db *DB) ProfileHolds(id string, env expr.Env) bool {
	p := db.Profiles[id]
	return p != nil && holds(p.When, env)
}

// profileNames are the names a profile's when and {expressions} may use: the state's,
// and ran (how often an event has run).
var profileNames = append(slices.Clone(FieldNames), "ran")

// ProfileIcon is the picture under assets/ a profile's face icon is cut from: its first
// face, its enemy's picture or its picture ("" for none).
func (db *DB) ProfileIcon(id string) string {
	p := db.Profiles[id]
	if p == nil {
		return ""
	}
	if p.Picture != "" {
		return p.Picture
	}
	if e := db.Enemies[p.Enemy]; e != nil {
		return e.Image.File
	}
	a := db.Actors[db.ProfileActor(id)]
	if a == nil || a.Faces == "" {
		return ""
	}
	return a.Faces + "/" + db.ProfileFaces(id)[0] + ".png"
}
