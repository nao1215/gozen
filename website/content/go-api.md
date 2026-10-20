---
title: Go API
description: Use Gozen from Go - test a game headless with a session, run it with play, and add reacting characters and a chat log with react and chat.
weight: 80
toc: true
---

Most games need only [play.Main](#start-the-game-playmain) and YAML. The packages below are for testing a game in Go, hosting it in your own program, and the parts that YAML does not cover. The reference for every package is on [pkg.go.dev](https://pkg.go.dev/github.com/nao1215/gozen).

<!-- verify: run fixture=module -->
```shell
go get github.com/nao1215/gozen@latest
```

| Package | Use it to |
|:--|:--|
| [gozen](https://pkg.go.dev/github.com/nao1215/gozen) | Load and check a game, play it without a window (`Session`: `Do` YAML commands, `Query` expressions, `Save`), try every branch (`Explore`) |
| [play](https://pkg.go.dev/github.com/nao1215/gozen/play) | Run the game window with the standard options (`play.Main`), or put a session on a screen in your own Ebitengine loop (`play.New`) |
| [react](https://pkg.go.dev/github.com/nao1215/gozen/react), [chat](https://pkg.go.dev/github.com/nao1215/gozen/chat) | Characters that react to words and events; a flowing message log |
| [puppet](https://pkg.go.dev/github.com/nao1215/gozen/puppet), [gfx](https://pkg.go.dev/github.com/nao1215/gozen/gfx) | Moving portraits; fonts for your own screens |
| [schema](https://pkg.go.dev/github.com/nao1215/gozen/schema) | The YAML definitions as Go types, to read a game's definitions in a tool (versioned with `schema_version`) |

Gozen is before v1, so the API may still change. It is changing in the direction below.

## Where the API is going

Gozen is not a tool that can do anything. It is a framework with a fixed shape: the game is YAML, battles, dungeons and story flow follow Gozen's rules, and Go gets a few small hooks. Stay inside that shape and checking, simulation, tuning while you play, saving and the web build all work without decisions on your part (convention over configuration). When a feature or an API is in doubt, Gozen keeps things easy, even at the cost of a constraint.

The Go API follows one rule: **read with an expression, change with a command.** Everything about a game is already named in YAML and in the expression language (`flag`, `var`, `gold`, `party`, `actor`, `location`, `pos`). Instead of Go types that mirror the YAML, the root package [gozen](https://pkg.go.dev/github.com/nao1215/gozen) has:

- `Load` a game, `New` a session, and `Do` YAML commands on it (`- join: scout`, `- battle: wolves`).
- `Query` an expression to read the state (`gold`, `actor.hero.level`).
- `Start`, `Next` and `Answer` to drive a session one step at a time from your own screen, and `Save` to keep it.
- A few hooks for what YAML cannot say: a `Policy` that answers choices, a `Store` for saves, and later custom commands and screens.

```go
g, err := gozen.Load(os.DirFS("game"), ".")    // problems: errors.As(err, &gozen.Diagnostics{})
s, err := gozen.New(g, gozen.Config{Seed: 1})
err = s.Do(ctx, "- join: sela\n- pick: Climb\n") // each command, then what it sets off
var gold int
err = s.Query("gold", &gold)
paths, err := gozen.Explore(ctx, s, "- call: lantern\n", gozen.ExploreOptions{})
```

`Validate` and `Game.Check` give the same diagnostics as `gozen check --format json`. The rules themselves (state, events, battles, dungeons) are not Go API: they are reached through YAML commands and expressions. The YAML schema is versioned separately with `schema_version`. The decision record is [ADR 0021](https://github.com/nao1215/gozen/blob/main/docs/decisions/0021-query-and-commands.md).

## Test the game headless

A `Session` runs the same rules as the window, with no display or sound device. A test walks through the game the way a player would, with YAML commands: `pick` a menu entry, `walk` in a dungeon, fight on AUTO; and reads the result with expressions.

```go
func TestElderOpensTheField(t *testing.T) {
	g, err := gozen.Load(os.DirFS("game"), ".")
	if err != nil {
		t.Fatal(err)
	}
	s, err := gozen.New(g, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Begin(t.Context()); err != nil { // the start event and the first arrival
		t.Fatal(err)
	}
	if err := s.Do(t.Context(), "- pick: Elder\n- pick: Go to the field\n- pick: Look around\n"); err != nil {
		t.Fatal(err)
	}
	var gold int
	var result string
	if err := errors.Join(s.Query("gold", &gold), s.Query("result", &result)); err != nil {
		t.Fatal(err)
	}
	if result != "win" || gold != 9 {
		t.Fatalf("result %s, gold %d", result, gold)
	}
}
```

- `Config.Policy` answers choices and battles (`DefaultPolicy` takes the first option, or the options named in `Prefer`, and fights on AUTO); `Config.OnRequest` sees every line, choice and battle.
- In the development mode (`Config.Dev`) `set` also writes levels, stats and the position in a dungeon, to set up a test.
- `Explore` copies the session and tries every branch of the choices and battle outcomes once, returning each path with its requests and its end as a session to `Query`. Use it to find a branch that never ends or a line nobody can reach.

A save round trip, without touching the user's directory:

```go
b, err := s.Save()                     // JSON
store := gozen.MemStore()              // or gozen.DirStore(dir), or your own Store
err = store.Write("slot1", b)
b, err = store.Read("slot1")
restored, err := gozen.Restore(g, b, gozen.Config{}) // goes on without rerunning the start events
```

[examples/headless](https://github.com/nao1215/gozen/blob/main/examples/headless/main.go) is a complete program with a battle and a save round trip.

## Start the game: play.Main

`play.Main` is the whole game window: it reads the options, prints `--help`, applies `--dev`, and runs until the window closes.

```go
//go:embed game
var files embed.FS

func main() {
	// game/ holds game.yaml, data/ and assets/
	if err := play.Main(context.Background(), files, "game", os.Args[1:], play.Config{Output: os.Stdout}); err != nil {
		log.Fatal(err)
	}
}
```

- The arguments are the options; nil means none (`os.Args` is never read for you).
- `--dir` replaces the embedded files with a directory on disk.
- `Config.Store` chooses where saves go (`gozen.DirStore`, `gozen.MemStore`, or your own `gozen.Store`). Left nil, saves go to the user's config directory, or to the page's localStorage in a browser.
- Call it on the main thread. Closing the window returns nil; cancelling the context returns the context's error. Ebitengine allows one game loop per process.
- The library never calls `os.Exit`; your `main` decides the exit status.

## Put a session on a screen: play.New

To put the game inside your own Ebitengine program, or to run it in a browser (`GOOS=js`), make a `gozen.Session` and give it to `play.New`. The `Screen` it returns is an `ebiten.Game`: run it, or call its `Update`, `Draw` and `Layout` from yours, and `Close` it when the loop ends. A session just made starts with the game's start event; one you have played with `Do` goes on from where it is, and the session reads the screen's play with `Query`.

```go
g, err := gozen.Load(files, "game")
if err != nil {
	return err
}
s, err := gozen.New(g, gozen.Config{})
if err != nil {
	return err
}
sc, err := play.New(s, play.Config{})
if err != nil {
	return err
}
defer sc.Close()
return ebiten.RunGame(sc)
```

Only one `Screen` can be open at a time, and it must be used from the game thread. [examples/play](https://github.com/nao1215/gozen/tree/main/examples/play) runs Wayfinder with `play.Main` on the desktop and with `play.New` in a browser.

## Characters that react: react

`react` makes a character answer what the player types, or an event, without a language model. Words in a lexicon give tags; the person's rules, written as expressions, choose a line; when nothing matches, the person dodges in character. Mood and memory are kept and can be saved.

```go
const book = `
lexicon:
  cat: [cat, kitten, {meow: 0.6}]
people:
  - id: keeper
    mood: 50
    likes: {cat: 10}
    rules:
      - id: cat
        when: tag.cat >= 0.5
        say: ["A {slot.cat}? I keep one at the lighthouse."]
    dodge:
      unknown: ["Hm. Tell me about the trail instead."]
`

b, err := react.Parse([]byte(book))
// handle err
m, err := react.New(b, "keeper", react.Config{})
// handle err
r, err := m.React(react.Stimulus{Kind: react.KindSay, From: "player", Text: "I saw a kitten on the road"})
fmt.Println(r.Rule, r.Lines, r.Mood)
```

```text
cat [A kitten? I keep one at the lighthouse.] 60
```

Rules can also read numbers (`value.amount` for a tip), remember what was said (`remember`), ask a question and read the `answer`, and wait between reactions (`cooldown`). Games add their own tags to a stimulus (`Stimulus.Tags`).

## Moving portraits: puppet

![a streamer who blinks, talks, breathes, sways and changes her face](/images/puppet.gif)

`puppet` moves one picture like a streaming avatar: it blinks, moves the mouth while speaking, breathes, sways and tilts the head, lets the hair and other swaying parts (ribbons, hems, sleeves, cords) trail behind on springs, plays one-shot reactions, and fades between faces. You give a base picture, small difference pictures (closed eyes, mouth shapes, other faces), and a few numbers in YAML; the head, hair and swaying parts move by deforming a mesh over the picture, so no layered artwork or rig is needed.

```go
rig, err := puppet.Load(fsys, "streamer/puppet.yaml")
// handle err
p, err := puppet.New(rig, puppet.Config{})
// handle err

// in Update
p.Update()
p.Speak() // while a line is shown; p.Stop() afterwards
p.React("nod")
p.SetFace("surprised")

// in Draw, between the background and the UI
op := &ebiten.DrawImageOptions{}
op.GeoM.Scale(0.6, 0.6)
p.Draw(screen, op)
```

A swaying part is an area of the picture listed under `springs:` with its root (an edge or a point), what it follows (`head`, or `body` for breathing and whole-body reactions such as hop), its axis (`x`, `y` or `both`) and its spring; the end far from the root moves the most and later parts lie over earlier ones. The defaults are small, the same as the hair's.

A difference picture as large as the base is cut down to the part that differs when loading; `gozen art puppet puppet.yaml --out small` writes the small pictures for shipping. In an RPG, `puppet: walker/puppet.yaml` on an actor makes the talk portrait move. `go run ./examples/puppet` shows the streamer above; keys try each motion.

## A message log: chat

`chat.Log` keeps a flowing log of short messages from the player, bots, or a network you connect through a `Source`. It does not judge messages (that is `react`) and does not transport them. `chat.Mask` filters words out of messages from people before they are shown, and `chat.Crowd` produces background chatter from a script, by mood.

## Your own front end

For a different front end (a terminal game, a web server), drive a session one step at a time: `Start` queues YAML commands, `Next` runs until something must be shown and returns it as a `Request` (a line, a choice, a battle, a shop), and `Answer` answers a choice or a battle. `Config.OnEffect` receives the moves, music and pictures. A session is not safe for concurrent use; give each game its own.
