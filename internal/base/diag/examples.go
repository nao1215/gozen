package diag

// Diagnostic examples are registry data consumed by errgen and executed by the
// CLI reference tests. Binary fixtures are original neutral PNGs, encoded so
// the published page contains every byte needed to reproduce the diagnostic.
var examples = map[int][]ExampleFile{
	101: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\nevents: {e: {do: [{say: 'aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp qqqq rrrr ssss tttt uuuu vvvv wwww xxxx yyyy zzzz aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp qqqq rrrr ssss tttt uuuu vvvv wwww xxxx yyyy zzzz aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp qqqq rrrr ssss tttt uuuu vvvv wwww xxxx yyyy zzzz aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp qqqq rrrr ssss tttt uuuu vvvv wwww xxxx yyyy zzzz aaaa bbbb cccc dddd'}]}}\n", Encoding: ""},
	},
	102: {
		{Name: "game.yaml", Content: "schema_version: 3\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations:\n  camp:\n    background: bg.png\n    stage:\n      scale: [{at: [50, 90], height: 40}, {at: [50, 10], height: 10}]\n      walk: [[[0, 0], [100, 0], [100, 100], [0, 100]]]\n      spots: {gate: {at: [50, 50]}}\n      people: [{id: guard, at: [20, 20]}]\n    people: [{id: guard, name: Guard, image: guard.png, bubbles: [Hello]}]\n", Encoding: ""},
		{Name: "assets/bg.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
		{Name: "assets/guard.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
	},
	103: {
		{Name: "game.yaml", Content: "schema_version: 3\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations:\n  camp:\n    background: bg.png\n    stage:\n      scale: [{at: [50, 90], height: 40}, {at: [50, 10], height: 10}]\n      walk: [[[0, 0], [100, 0], [100, 100], [0, 100]]]\n      spots: {gate: {at: [50, 50]}, corner: {at: [60, 60], name: Corner}}\n", Encoding: ""},
		{Name: "assets/bg.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
	},
	104: {
		{Name: "game.yaml", Content: "schema_version: 3\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations:\n  camp:\n    background: bg.png\n    stage:\n      scale: [{at: [50, 90], height: 40}, {at: [50, 10], height: 10}]\n      walk: [[[0, 0], [100, 0], [0, 100], [100, 100]]]\n      spots: {gate: {at: [50, 20]}}\n", Encoding: ""},
		{Name: "assets/bg.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
	},
	105: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\ntitle: \"\\u65C5\\u306E\\u5BBF\"\nstart:\n  location: camp\n  party:\n  - scout\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\n", Encoding: ""},
	},
	106: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}, boost: {scope: self}}\nlocations: {camp: {}}\nenemies: {slime: {image: s.png, stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, ai: [{use: boost, target: {pick: min, by: t.hp}}]}}\n", Encoding: ""},
		{Name: "assets/s.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
	},
	107: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {actions: [{label: Go, do: [{set: {var.a: 1, var.b: var.a + 1}}]}]}}\n", Encoding: ""},
	},
	108: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {actions: [{label: Train, do: [{set: {actor.scout.level: 5}}]}]}}\n", Encoding: ""},
	},
	109: {
		{Name: "game.yaml", Content: "schema_version: 4\nid: voiced\nstart: {location: room, party: [reader]}\n", Encoding: ""},
		{Name: "assets/reader/voice/hello.wav", Content: "a stub", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {reader: {class: walker, voices: reader/voice}}\nskills: {attack: {}, guard: {}}\nlocations: {room: {}}\nevents:\n  scene:\n    do:\n      - say: {who: reader, voice: hello, text: Hello.}\n      - say: {who: reader, text: Goodbye.}\n", Encoding: ""},
	},
	110: {
		{Name: "game.yaml", Content: "schema_version: 4\nid: twolang\nstart: {location: room, party: [reader]}\nwritten_in: en\nlanguages: [en, ja]\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {reader: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {room: {name: Room}}\nevents:\n  scene:\n    do:\n      - say: Hi.\n      - choice: [{label: Stay}]\n", Encoding: ""},
		{Name: "lang/ja.yaml", Content: "text:\n  Hi.: \"\\u3053\\u3093\\u306B\\u3061\\u306F\\u3002\"\n  Room: \"\\u90E8\\u5C4B\"\n  Gone line: \"\\u6D88\\u3048\\u305F\\u53F0\\u8A5E\"\n", Encoding: ""},
	},
	111: {
		{Name: "game.yaml", Content: "schema_version: 4\nid: twolang\nstart: {location: room, party: [reader]}\nwritten_in: en\nlanguages: [en, ja]\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {reader: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {room: {name: Room}}\nevents:\n  scene:\n    do:\n      - say: Hi.\n      - choice: [{label: Stay}]\n", Encoding: ""},
		{Name: "lang/ja.yaml", Content: "text:\n  Hi.: \"\\u3053\\u3093\\u306B\\u3061\\u306F\\u3002\"\n  Room: \"\\u90E8\\u5C4B\"\n  Gone line: \"\\u6D88\\u3048\\u305F\\u53F0\\u8A5E\"\n", Encoding: ""},
	},
	112: {
		{Name: "game.yaml", Content: "schema_version: 4\nid: flags\nstart: {location: room, party: [reader]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {reader: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {room: {name: Room}}\nevents:\n  talk:\n    do:\n      - set: {flag.met: true}\n      - if: flag.trusted\n        then: [{say: Welcome back.}]\n      - call: farewell\n  farewell:\n    trigger: enter\n    location: room\n    do: [{say: Goodbye.}, {move: room}]\n", Encoding: ""},
	},
	113: {
		{Name: "game.yaml", Content: "schema_version: 4\nid: flags\nstart: {location: room, party: [reader]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {reader: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {room: {name: Room}}\nevents:\n  talk:\n    do:\n      - set: {flag.met: true}\n      - if: flag.trusted\n        then: [{say: Welcome back.}]\n      - call: farewell\n  farewell:\n    trigger: enter\n    location: room\n    do: [{say: Goodbye.}, {move: room}]\n", Encoding: ""},
	},
	114: {
		{Name: "game.yaml", Content: "schema_version: 4\nid: fnvalue\nstart: {location: room, party: [reader]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {reader: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {room: {name: Room}}\nevents:\n  pay:\n    trigger: enter\n    location: room\n    do: [{gold: floor + 1}]\n", Encoding: ""},
	},
	115: {
		{Name: "game.yaml", Content: "schema_version: 4\nid: bestiary\nstart: {location: room, party: [reader]}\nui: {bestiary: true}\n", Encoding: ""},
		{Name: "assets/mote.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {reader: {class: walker}}\nskills: {attack: {}, guard: {}}\nenemies: {mote: {name: Mote, image: mote.png, stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}}}\ntroops: {lesson: {members: [mote]}}\nlocations: {room: {name: Room, encounters: {troops: [{troop: lesson}]}}}\n", Encoding: ""},
	},
	116: {
		{Name: "game.yaml", Content: "schema_version: 6\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "monster_families: {slime: Slime}\nclasses: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\nenemies: {mote: {name: Mote, image: x.png, family: slime, rank: 1, monster_class: walker, recruit: {chance: 1}, stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}}, wisp: {name: Wisp, image: x.png, family: slime, rank: 2, monster_class: walker, stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}}}\n", Encoding: ""},
		{Name: "assets/x.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
	}, 117: {
		{Name: "game.yaml", Content: "schema_version: 6\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "monster_families: {slime: Slime}\nclasses: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\nenemies: {mote: {name: Mote, image: x.png, family: slime, rank: 1, monster_class: walker, recruit: {chance: 1}, stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}}, wisp: {name: Wisp, image: x.png, family: slime, rank: 2, monster_class: walker, stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}}}\n", Encoding: ""},
		{Name: "assets/x.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
	},
	118: {
		{Name: "game.yaml", Content: "schema_version: 9\nid: g\nstart: {location: room}\nstreams: data/streams.yaml\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "actors: {host: {name: Host, puppet: p/rig.yaml}}\nlocations: {room: {}}\n", Encoding: ""},
		{Name: "data/streams.yaml", Content: "timing: {say: 2, per_char: 0.1, chat: 2, ask: 10}\ntips: [10]\ncrowd: {names: [leaf], lines: [Hi], tip_lines: [Here], rate: 0.5}\nchannels: [{id: host, actor: host, room: x.png, frame: {cx: 1, top: 0, bottom: 1}, episodes: [streams/one.yaml]}]\n", Encoding: ""},
		{Name: "streams/one.yaml", Content: "title: Short\nlobby: [Short]\nparts: [[{say: Hello.}]]\n", Encoding: ""},
		{Name: "assets/p/rig.yaml", Content: "base: base.png\nface: normal\nfaces: {normal: {}}\n", Encoding: ""},
		{Name: "assets/p/base.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
		{Name: "assets/x.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
	},
	119: {
		{Name: "game.yaml", Content: "schema_version: 9\nid: g\nstart: {location: room}\nstreams: data/streams.yaml\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "actors: {host: {name: Host, puppet: p/rig.yaml}}\nlocations: {room: {}}\n", Encoding: ""},
		{Name: "data/streams.yaml", Content: "voices: voice\ntiming: {say: 2, per_char: 0.1, chat: 2, ask: 10}\ntips: [10]\ncrowd: {names: [leaf], lines: [Hi], tip_lines: [Here], rate: 0.5}\nchannels: [{id: host, actor: host, room: x.png, frame: {cx: 1, top: 0, bottom: 1}, episodes: [streams/one.yaml]}]\n", Encoding: ""},
		{Name: "streams/one.yaml", Content: "title: Short\nlobby: [Short]\nparts: [[{say: Hello.}]]\n", Encoding: ""},
		{Name: "assets/p/rig.yaml", Content: "base: base.png\nface: normal\nfaces: {normal: {}}\n", Encoding: ""},
		{Name: "assets/p/base.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
		{Name: "assets/x.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
		{Name: "assets/voice/host/0000000000000000.ogg", Content: "x", Encoding: ""},
	},
	120: {
		{Name: "game.yaml", Content: "schema_version: 9\nid: g\nstart: {location: room}\nstreams: data/streams.yaml\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "actors: {host: {name: Host, puppet: p/rig.yaml}}\nlocations: {room: {}}\n", Encoding: ""},
		{Name: "data/streams.yaml", Content: "voices: voice\ntiming: {say: 2, per_char: 0.1, chat: 2, ask: 10}\ntips: [10]\ncrowd: {names: [leaf], lines: [Hi], tip_lines: [Here], rate: 0.5}\nchannels: [{id: host, actor: host, room: x.png, frame: {cx: 1, top: 0, bottom: 1}, episodes: [streams/one.yaml]}]\n", Encoding: ""},
		{Name: "streams/one.yaml", Content: "title: Short\nlobby: [Short]\nparts: [[{say: Hello.}]]\n", Encoding: ""},
		{Name: "assets/p/rig.yaml", Content: "base: base.png\nface: normal\nfaces: {normal: {}}\n", Encoding: ""},
		{Name: "assets/p/base.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
		{Name: "assets/x.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
		{Name: "assets/voice/host/9b56d519ccd9e1e5.ogg", Content: "RIFF....WAVEfmt", Encoding: ""},
	},
	121: {
		{Name: "game.yaml", Content: "schema_version: 9\nid: g\nstart: {location: room}\nweb: {url: 'https://example.org/g/', image: card.png}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "locations: {room: {}}\n", Encoding: ""},
		{Name: "assets/card.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
	},
	122: {
		{Name: "game.yaml", Content: "schema_version: 11\nid: complexity\nstart: {location: room}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "locations: {room: {}}\n", Encoding: ""},
		{Name: "check-complexity.yaml", Content: "file_lines: 1\n", Encoding: ""},
	},
	123: {
		{Name: "game.yaml", Content: "schema_version: 10\nid: g\nlanguage: en\nstart: {location: room}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "locations: {room: {name: Room}}\nactors: {potter: {name: Potter, speech: {en: {first_person: {preferred: I, avoid: [we]}}}}}\nevents: {hello: {do: [{say: {who: potter, text: 'we agree'}}]}}\n", Encoding: ""},
	},
	124: {
		{Name: "game.yaml", Content: "schema_version: 11\nid: unused\nstart: {location: camp, event: intro}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "locations: {camp: {}}\nevents: {intro: [{say: Hello}], leftover: [{say: Unused}]}\n", Encoding: ""},
	},
	1101: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\nevents: {bad: {do: [{battle: wolves}]}}\n", Encoding: ""},
	},
	1102: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\n", Encoding: ""},
		{Name: "data/more.yaml", Content: "locations: {camp: {}}\n", Encoding: ""},
	},
	1103: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {actions: [{label: Go, when: nope > 1, do: []}]}}\n", Encoding: ""},
	},
	1104: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {exits: [{label: Out}]}}\n", Encoding: ""},
	},
	1105: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\nevents: {e: {do: [{image: absent.png}]}}\n", Encoding: ""},
	},
	1106: {
		{Name: "game.yaml", Content: "schema_version: 3\nid: g\nstart: {location: camp, party: [scout]}\nui: {balloon: 5}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\n", Encoding: ""},
	},
	1107: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\nevents: {e: {trigger: sometimes, do: []}}\n", Encoding: ""},
	},
	1108: {
		{Name: "game.yaml", Content: "schema_version: 1\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {actions: [{label: Look, do: [{pick: Look}]}]}}\n", Encoding: ""},
	},
	1109: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Malformed: true, Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: [\n", Encoding: ""},
	},
	1111: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: cave, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {cave: {name: Cave, dungeon: {theme: cave, map: \"#####\\n#S..#\\n#####\", tiles: {S: {start: true}}}}}\n", Encoding: ""},
	},
	1112: {
		{Name: "game.yaml", Content: "schema_version: 6\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "monster_families: {slime: Slime}\nclasses: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\nenemies: {mote: {name: Mote, image: x.png, family: missing, rank: 1, monster_class: walker, recruit: {chance: 1}, stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}}}\n", Encoding: ""},
		{Name: "assets/x.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
	},
	1113: {
		{Name: "game.yaml", Content: "schema_version: 6\nid: g\nstart: {location: camp, party: [scout]}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "monster_families: {slime: Slime}\nclasses: {walker: {max_level: 2, stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\nenemies: {mote: {name: Mote, image: x.png, family: slime, rank: 1, monster_class: walker, recruit: {chance: 1, level: 3}, stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}}}\n", Encoding: ""},
		{Name: "assets/x.png", Content: "iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAIAAABMXPacAAAA+UlEQVR4nO3RQQ0AIAzAwElHClKQhow9ekkFNLk592mxWT+IBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtAABoBwBAOwAA2gEA0A4AgHYAALQDAKAdAADtPqk7BwnBg7bkAAAAAElFTkSuQmCC", Encoding: "base64"},
	},
	1201: {
		{Name: "game.yaml", Content: "schema_version: 2\nid: g\nstart: {location: camp, party: [scout]}\nfonts: {regular: regular.ttf}\n", Encoding: ""},
		{Name: "data/world.yaml", Content: "classes: {walker: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}\nactors: {scout: {class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {}}\n", Encoding: ""},
		{Name: "assets/regular.ttf", Content: "not a font", Encoding: ""},
	},
}
