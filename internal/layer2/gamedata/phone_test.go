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
		{"unknown app", `phone: {app: mail, messages: [{text: Hi}]}`, `phone.app: app must be chat, feed or video, not "mail"`},
		{"unknown hold", `phone: {app: chat, hold: flat, messages: [{text: Hi}]}`, `phone.hold: hold must be portrait or landscape, not "flat"`},
		{"no messages", `phone: {app: chat}`, `phone.messages: app chat needs messages`},
		{"other list", `phone: {app: chat, messages: [{text: Hi}], posts: [{picture: x.png}]}`, `phone.posts: app chat does not show posts`},
		{"two contents", `phone: {app: chat, messages: [{text: Hi, picture: x.png}]}`, `a message needs text, picture or video (one of them)`},
		{"empty message", `phone: {app: chat, messages: [{from: Ann}]}`, `a message needs text, picture or video (one of them)`},
		{"post without picture", `phone: {app: feed, posts: [{who: ann}]}`, `a post needs picture or video (one of them)`},
		{"negative likes", `phone: {app: feed, posts: [{picture: x.png, likes: -1}]}`, `likes must not be negative`},
		{"missing picture", `phone: {app: feed, posts: [{picture: none.png}]}`, `phone.posts[0].picture: no picture assets/none.png`},
		{"missing icon", `phone: {app: chat, messages: [{icon: none.png, text: Hi}]}`, `phone.messages[0].icon: no picture assets/none.png`},
		{"empty video", `phone: {app: video, video: {text: Clip}}`, `a video needs a picture or a puppet`},
		{"missing puppet", `phone: {app: video, video: {puppet: none.yaml}}`, `phone.video.puppet: no puppet assets/none.yaml`},
		{"bounce outside", `phone: {app: video, video: {picture: x.png, seconds: 3, bounce: [5]}}`, `phone.video.bounce[0]: bounce 5 is outside the video (0 to 3 seconds)`},
		{"negative seconds", `phone: {app: video, video: {picture: x.png, seconds: -1}}`, `seconds must not be negative`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{
				"data/z.yaml":       "events: {e: {do: [{" + tc.command + "}]}}",
				"assets/p/rig.yaml": "base: base.png\nface: normal\nfaces: {normal: {}}\n",
				"assets/p/base.png": "x",
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
