package play

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/gamepackage"
)

type phoneInstallResult struct {
	id        string
	installed *gamepackage.Installed
	err       error
	catalog   *gamepackage.SignedCatalog
}
type phoneStore struct {
	actions   map[int]phoneStoreAction
	done      chan phoneInstallResult
	cancel    context.CancelFunc
	message   string
	ready     bool
	selected  bool
	selection int
}
type phoneStoreAction struct {
	id      string
	release gamepackage.Release
	launch  bool
	refresh bool
}

func (g *Game) phoneStoreRows(p *phoneView) {
	s := p.store
	p.cue.Rows = nil
	s.actions = map[int]phoneStoreAction{}
	p.rows = nil
	p.rowsW = 0
	p.focus = -1
	if p.cue.Title == "" {
		p.cue.Title = "Games"
	}
	add := func(r data.PhoneRow) { p.cue.Rows = append(p.cue.Rows, r) }
	if s.message != "" {
		add(data.PhoneRow{Text: s.message})
	}
	if err := g.preparePhonePackages(); err != nil {
		add(data.PhoneRow{Text: err.Error()})
		s.ready = true
		return
	}
	if g.db.Game.Phone.CatalogURL != "" && s.done == nil {
		s.actions[len(p.cue.Rows)] = phoneStoreAction{refresh: true}
		add(data.PhoneRow{Button: "Check for updates"})
	}
	for _, a := range g.db.Game.Phone.Apps {
		if _, err := g.phoneApp(a.ID); err != nil {
			continue
		}
		r, err := g.packages.catalog.Resolve(a.Package, a.Version, phoneEngineVersion())
		if err != nil {
			add(data.PhoneRow{Text: fmt.Sprintf("%s: %v", a.ID, err)})
			continue
		}
		add(data.PhoneRow{Item: &data.PhoneItem{Icon: "package:" + a.ID, Title: r.Title, Text: r.Author, Value: r.Version}})
		add(data.PhoneRow{Text: r.Description})
		installed, openErr := g.phoneListed(a.ID)
		if openErr == nil {
			s.actions[len(p.cue.Rows)] = phoneStoreAction{id: a.ID, release: installed, launch: true}
			add(data.PhoneRow{Button: "Play " + r.Title})
			if installed.Version == r.Version {
				continue
			}
		} else if !errors.Is(openErr, fs.ErrNotExist) {
			add(data.PhoneRow{Text: openErr.Error()})
		}
		if a.Install == "bundled" && openErr != nil {
			continue
		}
		label := "Install " + r.Title
		if openErr == nil {
			label = "Update " + r.Title + " to " + r.Version
			add(data.PhoneRow{Text: "Each version has separate saves. Your earlier saves are kept."})
		}
		if s.done == nil {
			s.actions[len(p.cue.Rows)] = phoneStoreAction{id: a.ID, release: r}
			add(data.PhoneRow{Button: label})
		}
	}
	if len(p.cue.Rows) == 0 {
		add(data.PhoneRow{Text: "No games are available yet."})
	}
	s.ready = true
}

func (g *Game) updatePhoneStore(p *phoneView) {
	if p.store == nil {
		p.store = &phoneStore{}
	}
	s := p.store
	if s.done != nil {
		select {
		case result := <-s.done:
			s.finish(g, result)
		default:
		}
	}
	if !s.ready {
		g.phoneStoreRows(p)
	}
	g.updatePhonePage(p)
	if !s.selected {
		return
	}
	action, ok := s.actions[s.selection]
	s.selected = false
	if !ok {
		return
	}
	if action.launch {
		if err := g.startPhoneGame(p, action.id); err != nil {
			s.message = err.Error()
			s.ready = false
		}
		return
	}
	if s.done != nil {
		return
	}
	g.beginPhoneInstall(s, action)
}

func (s *phoneStore) finish(g *Game, result phoneInstallResult) {
	s.cancel()
	s.done = nil
	s.cancel = nil
	err := result.err
	if err == nil {
		if result.catalog != nil {
			err = g.acceptPhoneCatalog(*result.catalog)
		} else {
			err = g.selectPhonePackage(result.id, result.installed)
		}
	}
	if err != nil {
		if result.installed != nil && g.packages.opened[result.id] != result.installed {
			err = errors.Join(err, result.installed.Close())
		}
		s.message = "Installation: " + err.Error()
	} else {
		if result.catalog != nil {
			s.message = "Game catalog updated"
		} else {
			s.message = "Installed " + result.installed.Release.Title
		}
	}
	s.ready = false
}

func (g *Game) beginPhoneInstall(s *phoneStore, action phoneStoreAction) {
	ctx, cancel := context.WithCancel(g.context)
	done := make(chan phoneInstallResult)
	s.cancel = cancel
	s.done = done
	s.message = "Installing " + action.release.Title + "…"
	if action.refresh {
		s.message = "Checking for updates…"
	}
	s.ready = false
	g.emit("unsupported_script", traceLine{"command": "phone_install", "reason": "network installation is not deterministic"})
	installer := g.packages.installer
	cfg := *g.db.Game.Phone
	go func() {
		result := phoneInstallResult{id: action.id}
		if action.refresh {
			catalog, err := installer.RefreshSignedCatalog(ctx, cfg.CatalogURL, cfg.CatalogSignatureURL, cfg.CatalogPublicKey)
			result.catalog = &catalog
			result.err = err
		} else {
			result.installed, result.err = installer.Install(ctx, action.release)
		}
		select {
		case done <- result:
		case <-ctx.Done():
			if result.installed != nil {
				if err := result.installed.Close(); err != nil {
					log.Printf("phone package cleanup: %v", err)
				}
			}
		}
	}()
}

func (g *Game) drawPhoneStore(p *phoneView, dst *ebiten.Image, w, h float64) {
	if p.store == nil {
		p.store = &phoneStore{}
	}
	g.drawPhonePage(p, dst, w, h)
}

func (s *phoneStore) close() {
	if s != nil && s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}
