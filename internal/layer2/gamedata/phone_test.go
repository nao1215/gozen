package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
	lang "github.com/nao1215/gozen/internal/text"
)

// TestPhoneChecks: a phone names a known hold and app, carries the one list its
// app shows, and every picture, puppet and second it names is checked.
func TestPhoneChecks(t *testing.T) {
	if err := lang.Use(lang.English, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lang.Use(lang.Japanese, nil); err != nil {
			t.Error(err)
		}
	})
	for _, tc := range []struct{ name, command, want string }{
		{"history replies", `phone: {app: chat, history: true, messages: [{text: Hi, when: flag.met}], replies: [{button: Hello, when: '!flag.sent', set: {flag.sent: true}}]}`, ""},
		{"reply needs history", `phone: {app: chat, messages: [{text: Hi}], replies: [{button: Hello}]}`, "replies need app chat with history: true"},
		{"history only chat", `phone: {app: page, history: true, rows: [{text: Hi}]}`, "history is only for app chat"},
		{"home navigation", `phone: {app: home, rows: [{button: Contacts, item: {title: Contacts, icon: x.png}, event: e, when: flag.met}]}`, ""},
		{"contacts", `phone: {app: contacts, rows: [{item: {title: Ann}, when: flag.met}, {item: {title: Bob}, button: Chat, event: e}]}`, ""},
		{"page cannot navigate", `phone: {app: page, rows: [{button: Chat, event: e}]}`, "event"},
		{"unknown navigation", `phone: {app: home, rows: [{button: Chat, event: missing}]}`, "unknown event"},
		{"hidden assets validated", `phone: {app: chat, messages: [{picture: missing.png, when: false}]}`, "no picture"},
		{"timeout", `phone: {app: lock, var: var.timeout, timeout: 1, timeout_result: retry}`, ""},
		{"waiting banner", `phone: {app: banner, var: var.notice, timeout: 4, notices: [{text: Hello}]}`, ""},
		{"timeout no var", `phone: {app: lock, timeout: 1, timeout_result: retry}`, "needs var"},
		{"timeout no timeout", `phone: {app: lock, var: var.x, timeout_result: retry}`, "timeout must"},
		{"timeout notices", `phone: {app: lock, var: var.x, timeout: 1, timeout_result: retry, notices: [{text: Hi}]}`, ""},
		{"multiple waiting notices", `phone: {app: banner, var: var.x, timeout: 4, notices: [{text: Hi}, {text: Again}]}`, "exactly one notice"},
		{"waiting event", `phone: {app: banner, var: var.x, timeout: 4, notices: [{text: Hi, event: e}]}`, "uses var instead of event"},
		{"nonfinite timeout", `phone: {app: lock, var: var.x, timeout: .nan, timeout_result: retry}`, "timeout must"},
		{"negative timeout", `phone: {app: lock, var: var.x, timeout: -1, timeout_result: retry}`, "timeout must"},
		{"timeout wrong app", `phone: {app: chat, timeout: 1, messages: [{text: Hi}]}`, "timeout needs"},
		{"chat", `phone: {app: chat, title: Team, messages: [{from: Ann, text: Hi}, {mine: true, text: Hello}]}`, ""},
		{"chat picture and video", `phone: {app: chat, messages: [{picture: x.png}, {video: {picture: x.png, seconds: 5, bounce: [1, 4]}}]}`, ""},
		{"feed", `phone: {app: feed, posts: [{who: ann, picture: x.png, likes: 3}, {video: {picture: x.png}}]}`, ""},
		{"video", `phone: {app: video, hold: landscape, video: {picture: x.png, text: Clip}}`, ""},
		{"puppet", `phone: {app: video, video: {puppet: p/rig.yaml}}`, ""},
		{"unknown app", `phone: {app: mail, messages: [{text: Hi}]}`, `phone.app: app must be chat, feed, video, page, lock, banner, home, contacts, game or store, not "mail"`},
		{"misspelled app", `phone: {app: contact, rows: [{text: Hi}]}`, `phone.app: app must be chat, feed, video, page, lock, banner, home, contacts, game or store, not "contact" (did you mean "contacts"?)`},
		{"misspelled game", `phone: {app: gmae}`, `phone.app: app must be chat, feed, video, page, lock, banner, home, contacts, game or store, not "gmae" (did you mean "game"?)`},
		{"misspelled store", `phone: {app: stroe}`, `phone.app: app must be chat, feed, video, page, lock, banner, home, contacts, game or store, not "stroe" (did you mean "store"?)`},
		{"unknown hold", `phone: {app: chat, hold: flat, messages: [{text: Hi}]}`, `phone.hold: hold must be portrait or landscape, not "flat"`},
		{"no messages", `phone: {app: chat}`, `phone.messages: app chat needs messages`},
		{"other list", `phone: {app: chat, messages: [{text: Hi}], posts: [{picture: x.png}]}`, `phone.posts: app chat does not show posts`},
		{"two contents", `phone: {app: chat, messages: [{text: Hi, picture: x.png}]}`, `a message needs text, picture or video (one of them), or only a day`},
		{"empty message", `phone: {app: chat, messages: [{from: Ann}]}`, `a message needs text, picture or video (one of them), or only a day`},
		{"post without picture", `phone: {app: feed, posts: [{who: ann}]}`, `a post needs a picture, a video or only text (one of them)`},
		{"times and days", `phone: {app: chat, messages: [{day: Yesterday}, {from: Ann, text: Hi, time: '9:41'}, {mine: true, text: Yes, read: true, time: '9:42'}]}`, ""},
		{"post time", `phone: {app: feed, posts: [{who: ann, text: Down, time: 2h}]}`, ""},
		{"qr", `phone: {app: page, rows: [{qr: 'https://github.com/nao1215/gozen', caption: Scan to add Mia}]}`, ""},
		{"empty qr", `phone: {app: page, rows: [{qr: '', caption: Scan}]}`, `a row needs text, picture, pictures, item, button or qr (one of them)`},
		{"long qr", "phone: {app: page, rows: [{qr: '" + strings.Repeat("x", 3000) + "'}]}", `the qr text is too long for a QR code (3000 bytes)`},
		{"text post", `phone: {app: feed, posts: [{who: ann, text: 'Down again?', likes: 3, replies: 2, reposts: 1}]}`, ""},
		{"picture and video post", `phone: {app: feed, posts: [{picture: x.png, video: {picture: x.png}}]}`, `a post needs a picture, a video or only text (one of them)`},
		{"feed across, browsed", `phone: {app: feed, scroll: across, browse: true, posts: [{picture: x.png}, {text: Hi}]}`, ""},
		{"feed down", `phone: {app: feed, scroll: down, posts: [{picture: x.png}]}`, ""},
		{"unknown scroll", `phone: {app: feed, scroll: sideways, posts: [{picture: x.png}]}`, `phone.scroll: scroll must be down or across (got "sideways")`},
		{"scroll on chat", `phone: {app: chat, scroll: across, messages: [{text: Hi}]}`, `phone.scroll: scroll is only for app feed (app is chat)`},
		{"browse on page", `phone: {app: page, browse: true, rows: [{text: Hi}]}`, `phone.browse: browse is only for app feed (app is page)`},
		{"negative replies", `phone: {app: feed, posts: [{text: Hi, replies: -1}]}`, `replies and reposts must not be negative`},
		{"page results", `phone: {app: page, var: var.mfa, rows: [{button: Approve, set: {flag.approved: true}}, {button: Deny, id: deny}]}`, ""},
		{"call results", `phone: {app: lock, var: var.oncall, notices: [{title: API, call: true, set: {var.answered: 1}}]}`, ""},
		{"call rings out", `phone: {app: lock, var: var.oncall, notices: [{title: API, call: true, ring: 8, missed: {flag.missed: true}}]}`, ""},
		{"ring on a notice", `phone: {app: lock, notices: [{title: API, ring: 8}]}`, `phone.notices[0].ring: only a call rings`},
		{"negative ring", `phone: {app: lock, notices: [{title: API, call: true, ring: -1}]}`, `phone.notices[0].ring: ring is the seconds a call rings unanswered, 0 or more`},
		{"infinite ring", `phone: {app: lock, notices: [{title: API, call: true, ring: .inf}]}`, `phone.notices[0].ring`},
		{"missed without ring", `phone: {app: lock, notices: [{title: API, call: true, missed: {flag.missed: true}}]}`, `phone.notices[0].missed: missed runs when the call rings out, which needs ring`},
		{"bad missed", `phone: {app: lock, notices: [{title: API, call: true, ring: 3, missed: {nope: 1}}]}`, `phone.notices[0].missed`},
		{"feed likes", `phone: {app: feed, posts: [{picture: x.png, set: {flag.liked: true}}, {text: Hi, set: {var.n: var.n + 1}}]}`, ""},
		{"set on a browsed feed", `phone: {app: feed, browse: true, posts: [{picture: x.png}, {picture: x.png, set: {flag.liked: true}}]}`, `phone.posts[1].set: a feed with browse: true likes nothing, so this set never runs`},
		{"bad post set", `phone: {app: feed, posts: [{picture: x.png, set: {nope: 1}}]}`, `phone.posts[0].set`},
		{"var on a feed", `phone: {app: feed, var: var.x, posts: [{picture: x.png, set: {flag.liked: true}}]}`, `phone.var: var needs a page button or a call to keep (a feed keeps a like with the post's set)`},
		{"too many kept likes", "phone: {app: feed, posts: [" + strings.Repeat("{text: Hi, set: {flag.x: true}}, ", 32) + "]}", `phone.posts[31].set: a feed keeps the likes of at most 31 posts with set`},
		{"var without result", `phone: {app: chat, var: var.x, messages: [{text: Hi}]}`, `phone.var: var needs a page button or a call to keep [`},
		{"set on text", `phone: {app: page, rows: [{text: Hi, set: {var.x: 1}}]}`, `only a button has set or id`},
		{"bad set", `phone: {app: page, rows: [{button: Go, set: {nope: 1}}]}`, `phone.rows[0].set`},
		{"locked", `phone: {app: page, rows: [{pictures: [x.png], locked: '!flag.supported', label: Supporters only}, {picture: x.png, locked: true}]}`, ""},
		{"locked text", `phone: {app: page, rows: [{text: Hi, locked: true}]}`, `only a picture or pictures are locked`},
		{"banner", `phone: {app: banner, notices: [{app: Chat, title: Ann, text: Hi, event: e}]}`, ""},
		{"banner unknown event", `phone: {app: banner, notices: [{title: Ann, event: nope}]}`, `no parameterless event "nope"`},
		{"event on lock", `phone: {app: lock, notices: [{title: Ann, event: e}]}`, `only a banner opens an event`},
		{"call on banner", `phone: {app: banner, notices: [{title: Ann, call: true}]}`, `only the lock screen takes calls`},
		{"negative likes", `phone: {app: feed, posts: [{picture: x.png, likes: -1}]}`, `likes must not be negative`},
		{"missing picture", `phone: {app: feed, posts: [{picture: none.png}]}`, `phone.posts[0].picture: no picture assets/none.png`},
		{"missing icon", `phone: {app: chat, messages: [{icon: none.png, text: Hi}]}`, `phone.messages[0].icon: no picture assets/none.png`},
		{"empty video", `phone: {app: video, video: {text: Clip}}`, `a video needs a picture, frames or a puppet`},
		{"frames", `phone: {app: video, video: {frames: [f/1.png, f/2.png], fps: 8}}`, ""},
		{"frames pattern", `phone: {app: feed, posts: [{video: {picture: x.png, frames: [f/*.png]}}]}`, ""},
		{"bounce within frames", `phone: {app: video, video: {frames: [f/*.png], fps: 1, bounce: [2]}}`, ""},
		{"bounce past frames", `phone: {app: video, video: {frames: [f/*.png], fps: 1, bounce: [3]}}`, `bounce 3 is outside the video (0 to 2 seconds)`},
		{"missing frame", `phone: {app: video, video: {frames: [f/1.png, f/9.png]}}`, `phone.video.frames[1]: no picture assets/f/9.png`},
		{"pattern matching nothing", `phone: {app: chat, messages: [{video: {frames: [g/*.png]}}]}`, `phone.messages[0].video.frames[0]: no picture matches g/*.png under assets/`},
		{"page", `phone: {app: page, url: shop.example.test, rows: [{text: Hi, size: large, bold: true}, {picture: x.png, caption: C}, {pictures: [x.png, f/1.png], columns: 2}, {item: {icon: x.png, title: Rent, value: '-1,000'}}, {button: Pay}]}`, ""},
		{"page app bar", `phone: {app: page, title: Bank, color: '#1a73e8', rows: [{text: Hi}]}`, ""},
		{"lock", `phone: {app: lock, time: '2:14', date: Tuesday, auto: true, notices: [{app: Mail, icon: x.png, title: Ann, text: Hi}, {app: On-call, title: API, call: true}]}`, ""},
		{"lock without notices", `phone: {app: lock}`, `phone.notices: app lock needs notices`},
		{"page without rows", `phone: {app: page, title: Bank}`, `phone.rows: app page needs rows`},
		{"rows on a chat", `phone: {app: chat, messages: [{text: Hi}], rows: [{text: Hi}]}`, `phone.rows: app chat does not show rows`},
		{"empty row", `phone: {app: page, rows: [{caption: C}]}`, `a row needs text, picture, pictures, item, button or qr (one of them)`},
		{"two kinds in a row", `phone: {app: page, rows: [{text: Hi, button: Go}]}`, `a row needs text, picture, pictures, item, button or qr (one of them)`},
		{"unknown row kind", `phone: {app: page, rows: [{video: {picture: x.png}}]}`, `video`},
		{"bad size", `phone: {app: page, rows: [{text: Hi, size: huge}]}`, `size must be large or small, not "huge"`},
		{"bad columns", `phone: {app: page, rows: [{pictures: [x.png], columns: 4}]}`, `columns must be 2 or 3, not 4`},
		{"missing grid picture", `phone: {app: page, rows: [{pictures: [x.png, none.png]}]}`, `phone.rows[0].pictures[1]: no picture assets/none.png`},
		{"empty item", `phone: {app: page, rows: [{item: {value: '3'}}]}`, `an item needs a title or a text`},
		{"url and title", `phone: {app: page, url: a.test, title: A, rows: [{text: Hi}]}`, `a page has a url or a title, not both`},
		{"bad color", `phone: {app: page, title: A, color: blue, rows: [{text: Hi}]}`, `color must be #rrggbb, not "blue"`},
		{"empty notice", `phone: {app: lock, notices: [{app: Mail}]}`, `a notice needs a title or a text`},
		{"missing notice icon", `phone: {app: lock, notices: [{icon: none.png, title: A}]}`, `phone.notices[0].icon: no picture assets/none.png`},
		{"fade and pan", `phone: {app: video, video: {frames: [f/1.png, f/2.png], fps: 2, fade: 0.4, pan: true}}`, ""},
		{"fade too long", `phone: {app: video, video: {frames: [f/1.png], fps: 2, fade: 1}}`, `fade must be between 0 and a frame's length (0.5 seconds)`},
		{"pan without frames", `phone: {app: video, video: {picture: x.png, pan: true}}`, `fade and pan are for frames`},
		{"huge fps", `phone: {app: video, video: {frames: [f/1.png], seconds: 1, fps: 1e300}}`, "fps must not exceed 60"},
		{"negative fps", `phone: {app: video, video: {frames: [f/1.png], fps: -2}}`, `phone.video.fps: fps must not be negative`},
		{"missing puppet", `phone: {app: video, video: {puppet: none.yaml}}`, `phone.video.puppet: no puppet assets/none.yaml`},
		{"bounce outside", `phone: {app: video, video: {picture: x.png, seconds: 3, bounce: [5]}}`, `phone.video.bounce[0]: bounce 5 is outside the video (0 to 3 seconds)`},
		{"negative seconds", `phone: {app: video, video: {picture: x.png, seconds: -1}}`, `seconds must not be negative`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{
				"data/z.yaml":       "events: {e: {do: [{" + tc.command + "}]}}",
				"assets/p/rig.yaml": "base: base.png\nface: normal\nfaces: {normal: {}}\n",
				"assets/p/base.png": "x",
				"assets/f/1.png":    "x",
				"assets/f/2.png":    "x",
			})
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("want no error, got %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestPhoneFiniteVideoParameters(t *testing.T) {
	for _, field := range []string{"seconds", "zoom", "bounce", "fps", "fade"} {
		for _, value := range []string{".nan", ".inf", "-.inf"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				if field == "bounce" {
					value = "[" + value + "]"
				}
				_, err := testgame.TryLoad(map[string]string{
					"game.yaml":   "schema_version: 11\nid: phone-check\n" + testgame.Game,
					"data/z.yaml": "events: {e: [{phone: {app: video, video: {frames: [x.png], " + field + ": " + value + "}}}]}\n",
				})
				if err == nil || !strings.Contains(err.Error(), field) {
					t.Fatalf("nonfinite %s accepted or misdiagnosed: %v", field, err)
				}
			})
		}
	}
}

func TestPhoneChecksPuppetContents(t *testing.T) {
	for _, tc := range []struct{ name, rig, want string }{
		{"invalid definition", "base: [invalid", "rig.yaml"},
		{"missing base", "base: absent.png\nfaces: {normal: {}}\n", "absent.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{
				"data/z.yaml":           "events: {e: {do: [{phone: {app: video, video: {puppet: phone/rig.yaml}}}]}}",
				"assets/phone/rig.yaml": tc.rig,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("invalid phone puppet was accepted or misdiagnosed: %v", err)
			}
		})
	}
}

func TestPhoneGameDeclarations(t *testing.T) {
	const phone = `
phone:
  catalog: apps/catalog.yaml
  catalog_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  apps:
    - {id: puzzle, package: demo/puzzle, version: 1.0.0, install: bundled, path: apps/puzzle.zip, when: flag.unlocked}
`
	for _, tc := range []struct{ name, from, to, command, want string }{
		{name: "duplicate ID", from: "    - {id: puzzle", to: "    - {id: puzzle, package: demo/other, version: 1.0.0, install: optional}\n    - {id: puzzle", command: "{app: store}", want: "duplicate phone app"},
		{name: "unsigned catalog URL", from: "  catalog:", to: "  catalog_url: https://example.com/catalog.yaml\n  catalog:", command: "{app: store}", want: "catalog_public_key"},
		{name: "short catalog key", from: "  catalog:", to: "  catalog_public_key: YQ==\n  catalog_signature_url: https://example.com/catalog.sig\n  catalog_url: https://example.com/catalog.yaml\n  catalog:", command: "{app: store}", want: "32 bytes"},
		{name: "catalog URL", from: "  catalog:", to: "  catalog_public_key: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n  catalog_signature_url: https://example.com/catalog.sig\n  catalog_url: https://example.com/catalog.yaml\n  catalog:", command: "{app: store}"},
		{name: "insecure URL", from: "  catalog:", to: "  catalog_url: http://example.com/catalog.yaml\n  catalog:", command: "{app: store}", want: "HTTPS"},
		//nolint:gosec // Deliberately invalid example credentials must be rejected, not used for authentication.
		{name: "URL credentials", from: "  catalog:", to: "  catalog_url: https://user:password@example.com/catalog.yaml\n  catalog:", command: "{app: store}", want: "HTTPS"},
		{name: "URL fragment", from: "  catalog:", to: "  catalog_url: https://example.com/catalog.yaml#part\n  catalog:", command: "{app: store}", want: "HTTPS"},
		{name: "hidden bundle", from: "apps/puzzle.zip", to: ".cache/puzzle.zip", command: "{app: store}", want: "safe relative ZIP"},
		{name: "reserved bundle", from: "apps/puzzle.zip", to: "apps/CON.zip", command: "{app: store}", want: "safe relative ZIP"},
		{name: "game", command: "{app: game, game: puzzle}"},
		{name: "store", command: "{app: store}"},
		{name: "optional", from: "install: bundled, path: apps/puzzle.zip", to: "install: optional", command: "{app: store}"},
		{name: "range requires bundled pin", from: "version: 1.0.0", to: "version: '>=1.0.0 <2.0.0'", command: "{app: store}", want: "bundled_version"},
		{name: "bundled pin out of range", from: "version: 1.0.0", to: "version: '>=1.0.0 <2.0.0', bundled_version: 2.0.0", command: "{app: store}", want: "bundled_version"},
		{name: "optional forbids bundled pin", from: "install: bundled, path: apps/puzzle.zip", to: "install: optional, bundled_version: 1.0.0", command: "{app: store}", want: "only bundled"},
		{name: "bounded versions", from: "version: 1.0.0", to: "version: '>=1.0.0 <2.0.0', bundled_version: 1.0.0", command: "{app: game, game: puzzle}"},
		{name: "unknown game", command: "{app: game, game: missing}", want: "unknown phone app"},
		{name: "missing game", command: "{app: game}", want: "unknown phone app"},
		{name: "wrong app", command: "{app: page, game: puzzle, rows: [{text: Hi}]}", want: "game is only for app game"},
		{name: "store game key", command: "{app: store, game: puzzle}", want: "game is only for app game"},
		{name: "foreign content", command: "{app: game, game: puzzle, rows: [{text: Hi}]}", want: "do not accept other app content"},
		{name: "foreign outcome", command: "{app: game, game: puzzle, var: var.result}", want: "do not accept other app content"},
		{name: "catalog traversal", from: "apps/catalog.yaml", to: "../catalog.yaml", command: "{app: store}", want: "safe relative"},
		{name: "catalog digest", from: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", to: "abc", command: "{app: store}", want: "SHA-256"},
		{name: "bundle traversal", from: "apps/puzzle.zip", to: "../puzzle.zip", command: "{app: store}", want: "safe relative ZIP"},
		{name: "optional path", from: "install: bundled", to: "install: optional", command: "{app: store}", want: "only bundled"},
		{name: "unknown install", from: "install: bundled", to: "install: auto", command: "{app: store}", want: "bundled or optional"},
		{name: "unbounded range", from: "version: 1.0.0", to: "version: '>=1.0.0'", command: "{app: store}", want: "version"},
		{name: "reversed range", from: "version: 1.0.0", to: "version: '>=2.0.0 <1.0.0'", command: "{app: store}", want: "empty version interval"},
		{name: "bad package", from: "demo/puzzle", to: "puzzle", command: "{app: store}", want: "namespaced"},
		{name: "bad ID", from: "id: puzzle", to: "id: ../puzzle", command: "{app: store}", want: "local identifier"},
		{name: "bad condition", from: "flag.unlocked", to: "unknown.value", command: "{app: store}", want: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defs := phone
			if tc.from != "" {
				defs = strings.Replace(defs, tc.from, tc.to, 1)
			}
			_, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.Game + defs, "data/phone.yaml": "events: {phone_test: {do: [{phone: " + tc.command + "}]}}"})
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}
