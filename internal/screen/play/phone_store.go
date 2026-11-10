package play

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"

	lang "github.com/nao1215/gozen/internal/base/text"
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

// preparePhoneRows adds the rows the engine makes (a home screen's installed
// apps, a store's games) before the phone is first drawn, so they rise with it.
func (g *Game) preparePhoneRows(p *phoneView) {
	switch p.cue.App {
	case "home":
		g.addPhoneGameApps(p)
	case "store":
		if p.store == nil {
			p.store = &phoneStore{}
		}
		if !p.store.ready {
			g.phoneStoreRows(p)
		}
	}
}

func (g *Game) phoneStoreRows(p *phoneView) {
	s := p.store
	p.cue.Rows = nil
	s.actions = map[int]phoneStoreAction{}
	p.rows = nil
	p.rowsW = 0
	p.focus = -1
	if p.cue.Title == "" {
		p.cue.Title = lang.T("ゲーム")
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
		add(data.PhoneRow{Button: lang.T("更新を確認")})
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
			add(data.PhoneRow{Button: fmt.Sprintf(lang.T("%s をプレイ"), r.Title)})
			if installed.Version == r.Version {
				continue
			}
		} else if !errors.Is(openErr, fs.ErrNotExist) {
			add(data.PhoneRow{Text: openErr.Error()})
		}
		if a.Install == "bundled" && openErr != nil {
			continue
		}
		label := fmt.Sprintf(lang.T("%s をインストール"), r.Title)
		if openErr == nil {
			label = fmt.Sprintf(lang.T("%s を %s に更新"), r.Title, r.Version)
			add(data.PhoneRow{Text: lang.T("セーブは版ごとに別です。以前のセーブは残ります。")})
		}
		if s.done == nil {
			s.actions[len(p.cue.Rows)] = phoneStoreAction{id: a.ID, release: r}
			add(data.PhoneRow{Button: label})
		}
	}
	if len(p.cue.Rows) == 0 {
		add(data.PhoneRow{Text: lang.T("遊べるゲームはまだありません。")})
	}
	s.ready = true
}

func (g *Game) updatePhoneStore(p *phoneView) {
	s := p.store
	if s.done != nil {
		select {
		case result := <-s.done:
			s.finish(g, result)
		default:
		}
	}
	g.preparePhoneRows(p)
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
		s.message = fmt.Sprintf(lang.T("インストール: %v"), err)
	} else {
		if result.catalog != nil {
			s.message = lang.T("ゲームの一覧を更新しました")
		} else {
			s.message = fmt.Sprintf(lang.T("%s をインストールしました"), result.installed.Release.Title)
		}
	}
	s.ready = false
}

func (g *Game) beginPhoneInstall(s *phoneStore, action phoneStoreAction) {
	ctx, cancel := context.WithCancel(g.context)
	done := make(chan phoneInstallResult)
	s.cancel = cancel
	s.done = done
	s.message = fmt.Sprintf(lang.T("%s をインストール中…"), action.release.Title)
	if action.refresh {
		s.message = lang.T("更新を確認中…")
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

func (s *phoneStore) close() {
	if s != nil && s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}
