package assetpolicy

import (
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// References は YAML の文字列から指定拡張子の素材を列挙し、パターンを展開する。
// 制作設定で対象の種類を選び、検査記録の欠落を見つける。
func References(base fs.FS, prefix string, extensions []string) (pics, missing []string, err error) {
	assets, err := fs.Sub(base, prefix)
	if err != nil {
		return nil, nil, err
	}
	pictureExt := map[string]bool{}
	for _, ext := range extensions {
		pictureExt[ext] = true
	}
	var patternErr error
	found := map[string]bool{}
	lost := map[string]bool{}
	err = fs.WalkDir(base, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p == prefix {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") {
			return nil
		}
		// 検査対象の設定自体は素材を参照しない。
		if p == "asset-policy.yaml" {
			return nil
		}
		b, err := fs.ReadFile(base, p)
		if err != nil {
			return err
		}
		var v any
		if err := yaml.Unmarshal(b, &v); err != nil {
			return err
		}
		walkStrings(v, func(s string) {
			s = strings.TrimSpace(s)
			if !pictureExt[strings.ToLower(path.Ext(s))] || strings.ContainsAny(s, " \n") {
				return
			}
			s = strings.TrimPrefix(path.Clean(s), prefix+"/")
			if strings.ContainsAny(s, "*?[") {
				ms, globErr := fs.Glob(assets, s)
				if globErr != nil {
					patternErr = globErr
					return
				}
				for _, m := range ms {
					found[m] = true
				}
				return
			}
			if _, err := fs.Stat(assets, s); err == nil {
				found[s] = true
			} else {
				lost[s] = true
			}
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return keys(found), keys(lost), patternErr
}

func walkStrings(v any, f func(string)) {
	switch t := v.(type) {
	case string:
		f(t)
	case []any:
		for _, x := range t {
			walkStrings(x, f)
		}
	case map[string]any:
		for _, x := range t {
			walkStrings(x, f)
		}
	case map[any]any:
		for _, x := range t {
			walkStrings(x, f)
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
