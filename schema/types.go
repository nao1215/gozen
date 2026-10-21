package schema

import "github.com/nao1215/gozen/internal/layer2/gamedata"

// Action is a thing to do at a location ("Search the chest", "Rest at the inn").
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Action = gamedata.Action

// Actor is a character who can join the party.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Actor = gamedata.Actor

// BattleCfg are the rules every battle shares.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type BattleCfg = gamedata.BattleCfg

// CG is an event's picture over the whole screen (the image command). Written as a path
// ("" hides the picture shown), or as a map:
//
//	image: {file: cg/x.jpg, fit: cover, tint: "#ffd8e0", brightness: 0.9, fade: 1, faces: true}
//
// files shows several pictures one after another, a tap between them (the last stays
// shown for what follows). A file may be a pattern (cg/night_*.jpg: the files it matches,
// in name order; it may match none), so a scene takes more pictures without its events
// written again.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type CG = gamedata.CG

// CGReviewCfg is the rule for browsing through the full-screen pictures. Tags are the labels attached to a picture (the first 10 are attached with the number keys 1-9 and 0).
// Extra is pictures (patterns) not in the gallery's columns but added to the browse, and Groups are picture groups that can be chosen before browsing whether to include
// (excluded by default; an Extra picture that matches Groups also follows that group's choice).
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type CGReviewCfg = gamedata.CGReviewCfg

// ChoiceOption is one answer of a choice.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type ChoiceOption = gamedata.ChoiceOption

// Command is one event command. The command tag is the single registry of verbs and their accompanying fields.
// verb is a public verb, internal is Go-only, and the others list verbs that allow the accompanying fields.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Command = gamedata.Command

// DB is a loaded game: game.yaml and every section of the data files.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type DB = gamedata.DB

// Dungeon is the game definition contract for authors and consumers of loaded data.
// Its fields and methods follow the YAML contract in the Schema page (https://nao1215.github.io/gozen/schema/).
type Dungeon = gamedata.Dungeon

// Effect is one thing a skill or an item does to each target.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Effect = gamedata.Effect

// Enemy is a kind of foe.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Enemy = gamedata.Enemy

// Event is a named list of commands: run from another event (call), or by itself when
// its trigger fires.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Event = gamedata.Event

// Exit leads to another location.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Exit = gamedata.Exit

// Expr is an expression of the expr language, parsed when the YAML is read. Its zero
// value is "no expression": a condition that holds and a formula that is 0.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Expr = gamedata.Expr

// FloorPlan describes how one floor of a generated dungeon is built (Generate.Floors).
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type FloorPlan = gamedata.FloorPlan

// Fonts are the font files (paths under assets/).
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Fonts = gamedata.Fonts

// Gallery is the title's gallery: pictures to look at again and scenes to play again.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Gallery = gamedata.Gallery

// GalleryCG is a picture of the gallery: open once it (any of its pages) has been shown by
// an image command (or its flag has been set). files takes the pages of one picture the
// way the image command does (cg/x.jpg, cg/x.*.jpg: the picture and its later stages),
// looked at one after another.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type GalleryCG = gamedata.GalleryCG

// GalleryPage is the playback position counting lines and questions from 1, and the choice answers up to it.
// If Picture is set, only lines over that picture are counted into Line.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type GalleryPage = gamedata.GalleryPage

// GalleryScene is a scene played again from the gallery: the event, open once its flag
// has been set (and when holds, and a picture matching seen has been shown; at least one
// of the three). It plays with the starting party and no lasting effect (battles count as
// won, nothing is saved): a new state with the flag set, then set (flag.x / var.x:
// expression) written over it, so a scene can be played as a later visit or with the
// partner it needs.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type GalleryScene = gamedata.GalleryScene

// Game is game.yaml: the game's title, screen and starting state.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Game = gamedata.Game

// GameOver is what is shown when the party falls.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type GameOver = gamedata.GameOver

// Generate is how a dungeon's floors are made: the size of the maze (odd numbers), how
// many rooms, and the characters placed on its floor (count an expression of floor).
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Generate = gamedata.Generate

// Item is a thing the party carries: a consumable, a piece of equipment, or a key item.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Item = gamedata.Item

// LevelScale toughens a troop when the party's average level (rounded down) is above Level: the listed stats
// of its foes grow by Rate for each level above, up to Max (0.25: at most +25%). Past the cap
// every further level makes the battle easier again (training never loses).
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type LevelScale = gamedata.LevelScale

// Location is a place the party can be: a picture with people to talk to, things to do
// and ways out.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Location = gamedata.Location

// MemoCfg is what a test player's note asks and keeps: the kinds to choose from and the
// numbers kept with each note (an expression of the state's names each).
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type MemoCfg = gamedata.MemoCfg

// MemoKind is a kind of note: its id (kept in the file) and the label shown.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type MemoKind = gamedata.MemoKind

// MemoVar is a number kept with each note.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type MemoVar = gamedata.MemoVar

// Music is the songs and the rules choosing the mood.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Music = gamedata.Music

// MusicCue is a song and the mood it starts in ("song" or "song/mood", or a map).
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type MusicCue = gamedata.MusicCue

// Person is someone at a location to talk to.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Person = gamedata.Person

// Picture is a picture under assets/, optionally tinted (multiplied by a color) and
// darkened. Written as a path, or as a map.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Picture = gamedata.Picture

// Put is a character placed on a made floor: how many (an expression of floor, the floor's
// number), the farthest from the start (far: stairs down) or anywhere.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Put = gamedata.Put

// Roamer is a kind of strong foe walking a made floor (see Dungeon.Roamers).
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Roamer = gamedata.Roamer

// Say is a line of dialogue.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Say = gamedata.Say

// SceneReviewCfg is the flashback labels (there is one OK label; without it, only OK and NG).
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type SceneReviewCfg = gamedata.SceneReviewCfg

// Screen is the logical screen size.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Screen = gamedata.Screen

// Shop is a list of goods.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Shop = gamedata.Shop

// Skill is an action in battle (or from the menu).
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Skill = gamedata.Skill

// Start is the state a new game begins in.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Start = gamedata.Start

// Profile is a page of the character book (ADR 0031): data/ profiles.<id>.
type Profile = gamedata.Profile

// ProfileField is a line of label and value of a profile.
type ProfileField = gamedata.ProfileField

// ProfileRelation is how a profile's character sees another.
type ProfileRelation = gamedata.ProfileRelation

// StoneSet is a combination of magic stones worn together by one person: it grants a skill
// and stats while all its stones are worn.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type StoneSet = gamedata.StoneSet

// TacticAction is an action a party member can choose and the target picked automatically.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type TacticAction = gamedata.TacticAction

// TacticCondition is a condition choice. The template's expression, lottery and priority are edited only by the author.
// A Subject of target narrows single-target candidates; self/allies/foes express activation conditions.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type TacticCondition = gamedata.TacticCondition

// TacticPreset is a template prepared by the author. It defines up to 5 rows for each of the party members' named AIs.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type TacticPreset = gamedata.TacticPreset

// TacticRow is one row of selection-style tactics. It stores no display name or expression and refers to stable IDs in the catalog.
// The zero value of Enabled is disabled. Percent is needed only for threshold conditions, and nil is distinguished from 0.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type TacticRow = gamedata.TacticRow

// Tile is what a character of a dungeon's map is.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Tile = gamedata.Tile

// Troop is a group of enemies met together.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Troop = gamedata.Troop

// TroopEvent is the game definition contract for authors and consumers of loaded data.
// Its fields and methods follow the YAML contract in the Schema page (https://nao1215.github.io/gozen/schema/).
type TroopEvent = gamedata.TroopEvent

// Usage is what using an item or a skill does.
// This definition is exposed for game authors and hosts; see the Schema page (https://nao1215.github.io/gozen/schema/).
type Usage = gamedata.Usage
