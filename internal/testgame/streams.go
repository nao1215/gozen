package testgame

// StreamsGame is the part of game.yaml that names the stream file of StreamFiles.
const StreamsGame = "schema_version: 9\nstreams: data/streams.yaml\n"

// StreamFiles are a neutral stream game (ADR 0048) to add to the expedition: two
// channels, an episode each, a duo, a lobby place and an event opening the streams. The
// puppet is a file-existence sentinel like assets/x.png.
func StreamFiles() map[string]string {
	ep := "title: Garden tour\nlobby: [Planting day]\nparts:\n  - - say: Hello, everyone.\n    - say: {face: smile, text: Today we plant beans.}\n    - chat: [sprout, Beans!]\n    - ask: {q: Which seeds?, options: [{c: Beans, r: [\"{from} picked beans.\"]}, {c: Peas, r: [Peas it is.]}], npc: [[sprout, 1]]}\n"
	return map[string]string{
		"data/cast.yaml":    "actors:\n  ann: {name: Ann, puppet: p/rig.yaml}\n  bo: {name: Bo, puppet: p/rig.yaml}\nlocations: {booth: {stream: lobby}}\nevents: {watch: {do: [{open: streams}]}}\n",
		"assets/p/rig.yaml": "base: base.png\nface: normal\nfaces: {normal: {}, smile: {}}\n",
		"assets/p/base.png": "x",
		"data/streams.yaml": `timing: {say: 1.8, per_char: 0.11, chat: 1.6, ask: 12}
live: {share: 0.8, min_share: 0.6, duo_chance: 0.25}
away: {per_hour: 100, cap_hours: 24, star_bonus: 0.1}
tips: [10, 50]
crowd: {names: [sprout, leaf], lines: {any: [Hi!], morning: [Good morning]}, tip_lines: [For seeds], rate: 0.4}
affection: {steps: [0, 10, 20], comment: 1, reply: 2, answer: 3, tip_per: 10, stayed: 4}
answer_chance: [0.5, 0.8, 1]
echo: {max_runes: 10, chance: 0.5}
bounce: {on: "!$", amount: 1}
channels:
  - id: ann
    actor: ann
    tag: Gardening
    tagline: Seeds every week
    color: "#2a8c4a"
    room: x.png
    frame: {cx: 100, top: 0, bottom: 200}
    voice: {pitch: 1.1}
    episodes: [streams/ann/1.yaml]
    greet: [[], ["{from}, you came back."]]
    first: ["Welcome, {from}."]
    streak: ["{days} days in a row, {from}!"]
    stayed: ["Thanks for staying, {from}."]
  - id: bo
    actor: bo
    room: x.png
    frame: {cx: 100, top: 0, bottom: 200}
    episodes: [streams/bo/1.yaml]
duos:
  - {host: ann, with: bo, episodes: [streams/ann-bo/1.yaml]}
`,
		"streams/ann/1.yaml":    ep,
		"streams/bo/1.yaml":     ep,
		"streams/ann-bo/1.yaml": ep + "    - bo: {face: smile, text: My turn.}\n",
	}
}
