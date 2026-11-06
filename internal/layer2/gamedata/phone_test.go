package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestPhoneChecks: a phone names a known hold and app, carries the one list its
// app shows, and every picture, puppet and second it names is checked.
func TestPhoneChecks(t *testing.T) {
	for _, tc := range []struct{ name, command, want string }{
		{"chat", `phone: {app: chat, title: Team, messages: [{from: Ann, text: Hi}, {mine: true, text: Hello}]}`, ""},
		{"chat picture and video", `phone: {app: chat, messages: [{picture: x.png}, {video: {picture: x.png, seconds: 5, bounce: [1, 4]}}]}`, ""},
		{"feed", `phone: {app: feed, posts: [{who: ann, picture: x.png, likes: 3}, {video: {picture: x.png}}]}`, ""},
		{"video", `phone: {app: video, hold: landscape, video: {picture: x.png, text: Clip}}`, ""},
		{"puppet", `phone: {app: video, video: {puppet: p/rig.yaml}}`, ""},
		{"unknown app", `phone: {app: mail, messages: [{text: Hi}]}`, `phone.app: app must be chat, feed, video, page, lock or banner, not "mail"`},
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
		{"negative replies", `phone: {app: feed, posts: [{text: Hi, replies: -1}]}`, `replies and reposts must not be negative`},
		{"page results", `phone: {app: page, var: var.mfa, rows: [{button: Approve, set: {flag.approved: true}}, {button: Deny, id: deny}]}`, ""},
		{"call results", `phone: {app: lock, var: var.oncall, notices: [{title: API, call: true, set: {var.answered: 1}}]}`, ""},
		{"var without result", `phone: {app: chat, var: var.x, messages: [{text: Hi}]}`, `phone.var: var needs a page button or a call to keep`},
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
