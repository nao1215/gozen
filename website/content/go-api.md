---
title: Go API
description: Use Gozen from Go - test a game headless, embed it with runner, compose the screen with play, and add reacting characters and a chat log with react and chat.
weight: 80
toc: true
---

Most games need only the [runner](#start-the-game-runner) and YAML. The packages below are for testing a game in Go, hosting it in your own program, and the parts that YAML does not cover. The reference for every package is on [pkg.go.dev](https://pkg.go.dev/github.com/nao1215/gozen).

```shell
go get github.com/nao1215/gozen@latest
```

| Package | Use it to |
|:--|:--|
| [data](https://pkg.go.dev/github.com/nao1215/gozen/data) | Load and validate `game.yaml` and `data/` from an `fs.FS` |
| [rpg](https://pkg.go.dev/github.com/nao1215/gozen/rpg) | Create, change, save and parse the game state |
| [headless](https://pkg.go.dev/github.com/nao1215/gozen/headless) | Play the game without a window: move, talk, fight, explore every branch |
| [runner](https://pkg.go.dev/github.com/nao1215/gozen/runner) | Start the game window with the standard options and `--help` |
| [play](https://pkg.go.dev/github.com/nao1215/gozen/play) | Run or compose the game screen with your own Ebitengine loop |
| [event](https://pkg.go.dev/github.com/nao1215/gozen/event), [battle](https://pkg.go.dev/github.com/nao1215/gozen/battle), [world](https://pkg.go.dev/github.com/nao1215/gozen/world), [dungeon](https://pkg.go.dev/github.com/nao1215/gozen/dungeon) | Drive the rules yourself: events, turns, the field menu, a grid dungeon |
| [react](https://pkg.go.dev/github.com/nao1215/gozen/react), [chat](https://pkg.go.dev/github.com/nao1215/gozen/chat) | Characters that react to words and events; a flowing message log |
| [savefile](https://pkg.go.dev/github.com/nao1215/gozen/savefile), [assetfs](https://pkg.go.dev/github.com/nao1215/gozen/assetfs) | Save storage; asset overlays and approval |
| [expr](https://pkg.go.dev/github.com/nao1215/gozen/expr), [text](https://pkg.go.dev/github.com/nao1215/gozen/text), [gfx](https://pkg.go.dev/github.com/nao1215/gozen/gfx), [sound](https://pkg.go.dev/github.com/nao1215/gozen/sound) | Expressions; engine text translation; fonts and sounds for your own screens |

Packages under `x/` are experiments and are not covered by compatibility. Gozen is before v1, so the API may still change.

## Test the game headless

`headless` runs the same rules as the window, with no display or sound device. A test can walk through the game the way a player would: talk, go somewhere, fight on AUTO, save and load.

```go
func TestElderOpensTheField(t *testing.T) {
	db, err := data.Load(os.DirFS("game"), ".")
	if err != nil {
		t.Fatal(err)
	}
	st := rpg.New(db, rand.New(rand.NewPCG(1, 2)))
	d := headless.New(db, st)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	if d.Has("Go to the field") {
		t.Fatal("the field is open before talking to the elder")
	}
	if err := d.Do("Elder"); err != nil { // talk
		t.Fatal(err)
	}
	if err := d.GoTo("field"); err != nil {
		t.Fatal(err)
	}
	if err := d.Do("Look around"); err != nil { // the battle action
		t.Fatal(err)
	}
	if d.Wins != 1 || st.Gold != 9 {
		t.Fatalf("wins %d, gold %d; transcript:\n%v", d.Wins, st.Gold, d.Transcript)
	}
}
```

- `Do(label)` runs the field menu entry whose label contains `label`; `GoTo(id)` walks the shortest way through the open exits.
- `Answers` (or `Choose`) answers choices; `Result` can decide a battle without fighting it.
- `Run(ctx, commands)` runs commands directly, and `Walk` moves through a dungeon.
- `headless.Explore` copies the state and tries every branch of the choices and battle outcomes once, returning each path with its lines. Use it to find a branch that never ends or a line nobody can reach.

A save round trip, without touching the user's directory:

```go
b, err := st.Save()                  // JSON
// ... write b wherever you like, or use savefile.Write
restored, err := rpg.Parse(db, b, nil)
d.Restore(restored)                  // resume without rerunning the start events
```

[examples/headless](https://github.com/nao1215/gozen/blob/main/examples/headless/main.go) is a complete program with a battle and a save round trip.

## Start the game: runner

`runner.Run` is the whole game window: it reads the options, prints `--help`, applies `--dev`, and runs until the window closes.

```go
//go:embed game
var files embed.FS

func main() {
	fsys, err := fs.Sub(files, "game") // game.yaml, data/, assets/
	if err != nil {
		log.Fatal(err)
	}
	if err := runner.Run(context.Background(), fsys, ".", runner.Config{Args: os.Args[1:], Output: os.Stdout}); err != nil {
		log.Fatal(err)
	}
}
```

- `Args` are the options; nil means none (`os.Args` is never read for you).
- `--dir` replaces the embedded files with a directory on disk.
- Call it on the main thread. Closing the window returns nil; cancelling the context returns the context's error. Ebitengine allows one game loop per process.
- The library never calls `os.Exit`; your `main` decides the exit status.

## Compose the screen: play

To set the options from code instead of the command line, call `play.Run` with a `play.Config` (seed, save directory, mute, window scale, full screen, and the development settings). To put the game inside your own Ebitengine program, `play.New` returns a `Game` with `Update`, `Draw` and `Layout` for your loop; call `Close` when it ends.

```go
g, err := play.New(fsys, play.Config{Mute: true})
if err != nil {
	return err
}
defer g.Close()
// call g.Update, g.Draw and g.Layout from your ebiten.Game
```

Only one `Game` can be open at a time, and it must be used from the game thread.

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

Rules can also read numbers (`value.amount` for a tip), remember what was said (`remember`), ask a question and read the `answer`, and wait between reactions (`cooldown`). Games add their own tags with an `Analyzer`.

## Moving portraits: puppet

![a streamer who blinks, talks, sways, hops, nods, tilts her head and changes her face](/images/puppet.gif)

`puppet` moves one picture like a streaming avatar: it blinks, moves the mouth while speaking, breathes, sways and tilts the head, lets the hair trail behind, plays one-shot reactions, and fades between faces. You give a base picture, small difference pictures (closed eyes, mouth shapes, other faces), and a few numbers in YAML; the head and hair move by deforming a mesh over the picture, so no layered artwork or rig is needed.

```go
rig, err := puppet.Load(fsys, "streamer/puppet.yaml")
// handle err
p, err := puppet.New(rig, puppet.Config{})
// handle err

// in Update
p.Update()
p.Speak() // while a line is shown; p.Stop() afterwards
p.React("hop")
p.SetFace("surprised")

// in Draw, between the background and the UI
op := &ebiten.DrawImageOptions{}
op.GeoM.Scale(0.6, 0.6)
p.Draw(screen, op)
```

A difference picture as large as the base is cut down to the part that differs when loading; `gozen puppet puppet.yaml --out small` writes the small pictures for shipping. In an RPG, `puppet: walker/puppet.yaml` on an actor makes the talk portrait move. `go run ./examples/puppet` shows the streamer above; keys try each motion.

## A message log: chat

`chat.Log` keeps a flowing log of short messages from the player, bots, or a network you connect through a `Source`. It does not judge messages (that is `react`) and does not transport them. `chat.Mask` filters words out of messages from people before they are shown, and `chat.Crowd` produces background chatter from a script, by mood.

## Rules without the screen

For a different front end (a terminal game, a web server, a test harness), use the rule packages directly:

| Step | Entry |
|:--|:--|
| Load definitions | `data.Load(fsys, dir)` |
| Run event commands | `event.Runner`; answer its `Request` values; receive effects through `event.Host` |
| Fight | `battle.Battle`: turns, actions and results; `Decision` explains the AI's choice |
| Field menu | `world.Entries` and `Entry.Commands`, `world.Arrive`, `world.Auto` |
| Dungeon | `dungeon`: step, turn, observe the map |
| Save | `State.Save`, `rpg.Parse`, `savefile` for the storage |

The state, a battle and a driver are not safe for concurrent use; give each game its own `DB`, state and random source.
