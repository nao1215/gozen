package play

import (
	"context"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestPhoneStoreSelectionDoesNotAnswerStory(t *testing.T) {
	p := newPhoneView(data.PhoneCue{App: "store", Rows: []data.PhoneRow{{Text: "About"}, {Item: &data.PhoneItem{Title: "Game"}}, {Button: "Install"}}})
	p.store = &phoneStore{actions: map[int]phoneStoreAction{2: {id: "game"}}}
	if len(p.cue.Results()) != 0 {
		t.Fatal("store should not introduce story outcomes")
	}
	p.putDown(2, -1, false)
	if !p.store.selected || p.store.selection != 2 || p.closing != 0 || p.result != -1 {
		t.Fatalf("selection leaked into story: %+v", p.store)
	}
	p.store.selected = false
	p.putDown(-4, -4, false)
	if p.closing != phoneRise || p.store.selected {
		t.Fatal("host dismiss stopped working")
	}
}

func TestPhoneStoreCloseCancelsInstall(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	p := newPhoneView(data.PhoneCue{App: "store"})
	p.store = &phoneStore{cancel: cancel, done: make(chan phoneInstallResult, 1)}
	p.close()
	if ctx.Err() != context.Canceled {
		t.Fatal("closing store left download running")
	}
	p.close()
}
