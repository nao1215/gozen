package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/model/gamepackage"
)

func TestCatalogSigning(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	b := []byte("{\"schema\":1,\"releases\":[]}")
	private := ed25519.NewKeyFromSeed(seed)
	public := base64.StdEncoding.EncodeToString(private[ed25519.SeedSize:])
	signature, e := sign(b, base64.StdEncoding.EncodeToString(seed), public)
	if e != nil {
		t.Fatal(e)
	}

	if _, e = gamepackage.AuthenticateSigned(b, signature, public); e != nil {
		t.Fatal(e)
	}
	if _, e = sign(b, "", public); e == nil {
		t.Fatal("signed without configured key")
	}
	if _, e = sign(b, base64.StdEncoding.EncodeToString(seed), base64.StdEncoding.EncodeToString(make([]byte, 32))); e == nil {
		t.Fatal("signed with mismatched reviewed public key")
	}
	if _, e = gamepackage.AuthenticateSigned(append(b, ' '), signature, public); e == nil {
		t.Fatal("accepted modified published catalog")
	}
}

func TestCatalogSourceBoundary(t *testing.T) {
	cfg := sources{Sources: []source{{ID: "sample/game", Repository: "sample/game", ManifestAsset: "gozen-package.json"}}}
	called := false
	fetch := func(context.Context, gamepackage.Release) error {
		called = true
		return errors.New("expected fixture stop")
	}
	for _, url := range []string{"https://github.com/other/game/releases/download/v1/game.zip", "http://github.com/sample/game/releases/download/v1/game.zip", "https://github.com/sample/game/releases/download/v1/game.zip?redirect=other", "https://github.com/sample/game/releases/download/v1/%2e%2e.zip"} {
		c := gamepackage.Catalog{Schema: 1, Releases: []gamepackage.Release{{ID: "sample/game", URL: url}}}
		if e := check(t.Context(), c, cfg, fetch); e == nil || called {
			t.Fatalf("fetched unapproved URL %s", url)
		}
	}
	if e := check(t.Context(), gamepackage.Catalog{Schema: 1}, cfg, fetch); e != nil || called {
		t.Fatalf("empty catalog: %v", e)
	}
	c := gamepackage.Catalog{Schema: 1, Releases: []gamepackage.Release{{ID: "sample/game", URL: "https://github.com/sample/game/releases/download/v1/game.zip"}}}
	if e := check(t.Context(), c, cfg, fetch); e == nil || !called {
		t.Fatal("approved source never checked")
	}
}

func TestRegistryUpdaterReviewBoundary(t *testing.T) {
	if _, e := exec.LookPath("python3"); e != nil {
		t.Skipf("updater requires Python: %v", e)
	}
	script, e := filepath.Abs("../../../scripts/update-game-catalog.py")
	if e != nil {
		t.Fatal(e)
	}
	const program = `
import copy, runpy, sys
m=runpy.run_path(sys.argv[1]); update=m['candidates']
pending=m['has_pending_update']
branch={'headRefName':'automation/game-catalog-12','headRepository':{'nameWithOwner':'fork/game'}}
assert not pending([branch], 'sample/game'), 'fork PR blocked updates'
branch['headRepository']['nameWithOwner']='sample/game'
assert pending([branch], 'sample/game'), 'own pending update not detected'
branch['headRepository']=None
assert not pending([branch], 'sample/game'), 'deleted fork blocked updates'

base='https://github.com/sample/game/releases/download/v1/'
entry={'id':'sample/game','version':'1.0.0','url':base+'game.zip'}
sources={'sources':[{'id':'sample/game','repository':'sample/game','manifest_asset':'gozen-package.json'}]}
release={'assets':[{'name':'gozen-package.json','browser_download_url':base+'gozen-package.json'}]}
def fetch(url,limit,api): return [release] if api else copy.deepcopy(entry)
empty={'schema':1,'releases':[]}
first=update(empty,sources,fetch)
assert first['releases']==[entry] and empty['releases']==[]
assert update(first,sources,fetch)==first
entry['browser_url']='https://raw.githubusercontent.com/foreign/game/'+'0'*40+'/game.zip'
try: update(empty,sources,fetch)
except ValueError: pass
else: raise AssertionError('foreign browser mirror accepted')
entry['browser_url']='https://raw.githubusercontent.com/sample/game/main/game.zip'
try: update(empty,sources,fetch)
except ValueError: pass
else: raise AssertionError('mutable browser mirror accepted')
del entry['browser_url']
entry['url']='https://github.com/other/game/releases/download/v1/game.zip'
try: update(empty,sources,fetch)
except ValueError: pass
else: raise AssertionError('foreign repository accepted')
entry['url']=base+'changed.zip'
try: update(first,sources,fetch)
except ValueError: pass
else: raise AssertionError('immutable version changed')
entry['version']='1.1.0';entry['url']=base.replace('v1/','v2/')+'game.zip'
try: update(first,sources,fetch)
except ValueError: pass
else: raise AssertionError('different release tag accepted')
release['draft']=True
assert update(empty,sources,fetch)==empty
# Multiple approved packages may share a repository and metadata asset name.
mono_sources={'sources':[
    {'id':name,'repository':'sample/game','manifest_asset':'gozen-package.json'}
    for name in ('sample/counter','sample/trail')]}
mono_releases=[]
mono_metadata={}
for name in ('counter','trail','unregistered'):
    tag_base=base.replace('v1/',name+'-v1/')
    address=tag_base+'gozen-package.json'
    mono_releases.append({'assets':[{'name':'gozen-package.json','browser_download_url':address}]})
    mono_metadata[address]={'id':'sample/'+name,'version':'1.0.0','url':tag_base+name+'.zip'}
def mono_fetch(url,limit,api):
    return copy.deepcopy(mono_releases if api else mono_metadata[url])
mono=update(empty,mono_sources,mono_fetch)
assert [r['id'] for r in mono['releases']]==['sample/counter','sample/trail'], 'unregistered package approved or shared source lost'
assert update(mono,mono_sources,mono_fetch)==mono, 'monorepo update not idempotent'
# Ignored packages cannot redirect the matching package's archive verification.
mono_metadata[base.replace('v1/','counter-v1/')+'gozen-package.json']['url']='https://github.com/foreign/game/releases/download/v1/game.zip'
try: update(empty,mono_sources,mono_fetch)
except ValueError: pass
else: raise AssertionError('matching monorepo package escaped approved repository')
print('updater review boundary passed')
`
	cmd := exec.CommandContext(t.Context(), "python3", "-c", program, script) //nolint:gosec // Fixed maintainer program and repository script, no network fixture.
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if e := cmd.Run(); e != nil {
		t.Fatalf("updater: %v; stdout=%s stderr=%s", e, &stdout, &stderr)
	}
}
