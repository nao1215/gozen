package command

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/internal/diag"
)

// fileSpec is one file of a pinned model. SHA256 and Size are checked after the download.
type fileSpec struct {
	Name     string
	SHA256   string
	Size     int64
	Optional bool // a 404 is not an error
}

// modelSpec is a model gozen models fetch knows. Rev is a fixed commit of the
// Hugging Face repository, so the same files come back every time.
type modelSpec struct {
	Name, Repo, Rev, Dir, License, Summary string
	Files                                  []fileSpec
}

// models lists the models that fetch can download. The first is the default detector.
var models = []modelSpec{
	{
		Name: "censor", Repo: "deepghs/anime_censor_detection", Rev: "0cf62fd6b28213b40ae0c0055f92e7ae6a96bdc2",
		Dir: "censor_detect_v1.0_s", License: "MIT", Summary: "censor_detect_v1.0_s: nipple_f, penis, pussy",
		Files: []fileSpec{
			{Name: "model.onnx", SHA256: "2c2524824d7d320c5619a0a73702a2e2186f619067c823d211e94f7cfe489cba", Size: 44586353},
			{Name: "labels.json", SHA256: "6e9f1afbfbcc49ab32628b9f0b8650527a85dd774481fade2e523a8403201a00", Size: 44},
			{Name: "threshold.json", SHA256: "15e55650b415a27479f8c1dc7492c1db6f7aebde3b52698b9b001fcd4d57f58d", Size: 48, Optional: true},
		},
	},
	{
		Name: "face", Repo: "deepghs/anime_face_detection", Rev: "784dc4c0bb692351ddcdbe6131a050b17d3025d5",
		Dir: "face_detect_v1.4_s", License: "MIT", Summary: "face_detect_v1.4_s: face",
		Files: []fileSpec{
			{Name: "model.onnx", SHA256: "403b5bc93b6ff789b7d183418df4a1364049bac00c24acd927604a7ff6891483", Size: 44583229},
			{Name: "labels.json", SHA256: "de28a7114bd98e3847dc12713b7f10c3e7ec2643fde853cb2cf220a94aa0f63c", Size: 14},
			{Name: "threshold.json", SHA256: "34bdd0e8eaefbf8ba3d882f48711e66a9b640d05ad7f9ff38cfdbd413966b962", Size: 48, Optional: true},
		},
	},
	{
		Name: "head", Repo: "deepghs/anime_head_detection", Rev: "06604feee81983792a57c21081e539c0ae229833",
		Dir: "head_detect_v2.0_s", License: "MIT", Summary: "head_detect_v2.0_s: head",
		Files: []fileSpec{
			{Name: "model.onnx", SHA256: "6679f9b71192298bbf174d82e9e5581c3237b0c3dc67deace7cdbf686b070a00", Size: 44585386},
			{Name: "labels.json", SHA256: "c72a180b765ad68c226cc247de91d76b93b712687dc9b90a3febc67622345eda", Size: 14},
			{Name: "threshold.json", SHA256: "263a9a3e17a9a0ada302cc8a49caeacd77de03585a00910a38157c7c48b0b3e9", Size: 48, Optional: true},
		},
	},
}

// Hooks for tests. Nothing here reaches the network unless fetch or a confirmed prompt does.
var (
	hubURL    = "https://huggingface.co"
	httpTimes = 30 * time.Minute
	cacheRoot = func() (string, error) {
		d, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "gozen", "mosaic"), nil
	}
	interactive           = func() bool { return isCharDevice(os.Stdin) && isCharDevice(os.Stdout) }
	promptIn    io.Reader = os.Stdin
)

func isCharDevice(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func findModel(name string) (modelSpec, bool) {
	for _, m := range models {
		if m.Name == name {
			return m, true
		}
	}
	return modelSpec{}, false
}

func (m modelSpec) url(f fileSpec) string {
	return hubURL + "/" + m.Repo + "/resolve/" + m.Rev + "/" + m.Dir + "/" + f.Name
}

func (m modelSpec) size() int64 {
	var n int64
	for _, f := range m.Files {
		n += f.Size
	}
	return n
}

func (m modelSpec) dir() (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", fmt.Errorf("no cache directory: %w", err)
	}
	return filepath.Join(root, m.Name), nil
}

// cachedModel returns the model.onnx path when the model is in the cache.
func (m modelSpec) cachedModel() (string, bool) {
	d, err := m.dir()
	if err != nil {
		return "", false
	}
	for _, f := range m.Files {
		if f.Optional {
			continue
		}
		st, err := os.Stat(filepath.Join(d, f.Name))
		if err != nil || !st.Mode().IsRegular() || st.Size() != f.Size {
			return "", false
		}
	}
	return filepath.Join(d, "model.onnx"), true
}

func megabytes(n int64) string { return fmt.Sprintf("%.0f MB", float64(n)/1e6) }

// download fetches every file into a temporary directory, checks each SHA-256,
// and only then replaces the cache directory. A mismatch deletes what was fetched.
func (m modelSpec) download(ctx context.Context, diag *cliio.Output) (err error) {
	// Ctrl-C ends the transfer with an error, so the partial directory is removed below.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	dir, err := m.dir()
	if err != nil {
		return err
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, m.Name+".part-")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(tmp))
		}
	}()
	client := &http.Client{Timeout: httpTimes}
	for _, f := range m.Files {
		diag.Printf("fetching %s\n", m.url(f))
		if err := fetchFile(ctx, client, m.url(f), filepath.Join(tmp, f.Name), f); err != nil {
			if errors.Is(err, errSkipOptional) {
				continue
			}
			return fmt.Errorf("%s/%s: %w", m.Name, f.Name, err)
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(tmp, dir)
}

var errSkipOptional = errors.New("optional file is absent")

func fetchFile(ctx context.Context, client *http.Client, url, dst string, f fileSpec) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := client.Do(req) // #nosec G107 G704 -- the URL is pinned in this package
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode == http.StatusNotFound && f.Optional {
		return errSkipOptional
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- inside our temporary directory
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, f.Size+1))
	if err != nil {
		return err
	}
	if sum := hex.EncodeToString(h.Sum(nil)); n != f.Size || sum != f.SHA256 {
		return fmt.Errorf("SHA-256 mismatch: expected %s (%d bytes), got %s (%d bytes); the download was deleted", f.SHA256, f.Size, sum, n)
	}
	return nil
}

type modelResult struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	License    string `json:"license"`
	Bytes      int64  `json:"bytes"`
	Cached     bool   `json:"cached"`
	Path       string `json:"path,omitempty"`
	Summary    string `json:"summary"`
}

func (cli *cli) fetchCmd(list bool) clihelp.Operation {
	name := "models fetch"
	if list {
		name = "models list"
	}
	fs := newFlagSet("gozen " + name)
	p := fetchPage
	p.Name = "gozen " + name
	p.Usage = []string{"gozen " + name + " [OPTIONS] [MODEL ...]"}
	if list {
		p.Usage = []string{"gozen models list [OPTIONS]"}
		p.Summary = "list pinned models and their cache status"
		p.About = "Reports model names, sources, licenses, sizes and cache status without downloading."
		p.Notes = []string{exitStatus, "Requirements: no network, cgo or ONNX Runtime. See " + clihelp.README}
	}
	return clihelp.Operation{Flags: fs, Page: p, Run: func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if list && fs.NArg() != 0 {
			return cliio.Invalid(diag.Operands, "models list takes no operands")
		}
		names := fs.Args()
		if len(names) == 0 {
			for _, m := range models {
				names = append(names, m.Name)
			}
		}
		todo := []modelSpec{}
		for _, name := range names {
			m, ok := findModel(name)
			if !ok {
				return cliio.Invalid(diag.UnknownName, "unknown model %q: use %s", name, modelNames())
			}
			todo = append(todo, m)
		}
		rows := []modelResult{}
		for _, m := range todo {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !list {
				if err := m.download(ctx, cli.diag); err != nil {
					return err
				}
			}
			path, cached := m.cachedModel()
			row := modelResult{m.Name, m.Repo, m.License, m.size(), cached, path, m.Summary}
			rows = append(rows, row)
			if cli.report.Format == "jsonl" {
				if err := cli.report.Item(cli.out, m.Name, "ok", row); err != nil {
					return err
				}
			}
			if !cli.report.Machine() {
				state := "not fetched"
				if cached {
					state = "fetched " + filepath.Dir(path)
				}
				cli.out.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", m.Name, m.Repo, m.License, megabytes(m.size()), state, m.Summary)
			}
		}
		return cli.result(rows)
	}}
}

func modelNames() string {
	var n []string
	for _, m := range models {
		n = append(n, m.Name)
	}
	return strings.Join(n, ", ")
}

// defaultModel finds the model for an omitted --model: the cache first, then,
// with ask, a confirmation on the terminal (or yes) before downloading.
func (cli *cli) defaultModel(ctx context.Context, yes, ask bool) (string, error) {
	m := models[0]
	if p, ok := m.cachedModel(); ok {
		return p, nil
	}
	if !ask {
		return "", nil
	}
	dir, err := m.dir()
	if err != nil {
		return "", err
	}
	if !yes {
		if !interactive() {
			return "", fmt.Errorf("missing --model, and the default model %q is not in the cache; run \"gozen models fetch %s\" (about %s from %s/%s) or pass --yes", m.Name, m.Name, megabytes(m.size()), hubURL, m.Repo)
		}
		cli.diag.Printf("Download the model? About %s from %s/%s/resolve/%s/%s/ to %s [y/N] ", megabytes(m.size()), hubURL, m.Repo, m.Rev, m.Dir, dir)
		line, err := bufio.NewReader(promptIn).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("missing --model: no answer was read; run \"gozen models fetch %s\" or pass --model", m.Name)
		}
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			return "", fmt.Errorf("missing --model: no model was downloaded; run \"gozen models fetch %s\" or pass --model", m.Name)
		}
	}
	if err := m.download(ctx, cli.diag); err != nil {
		return "", err
	}
	p, _ := m.cachedModel()
	return p, nil
}
