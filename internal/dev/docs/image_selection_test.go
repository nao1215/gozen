package docs

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// imageRuns is the single source of capture dependencies for selection and sharding.
func (m manifest) imageRuns(names []string) []string {
	need := map[string]bool{}
	for _, name := range names {
		im := m.Images[name]
		need[im.Run] = true
		for _, frame := range im.Frames {
			if frame.Run != "" {
				need[frame.Run] = true
			}
		}
	}
	return sortedKeys(need)
}

func (m manifest) samplesFor(runs []string) []string {
	need := map[string]bool{}
	for _, name := range runs {
		need[m.Runs[name].Sample] = true
	}
	return sortedKeys(need)
}

func (m manifest) selectImages(filter, shard string) ([]string, []string, error) {
	var patterns []string
	for pattern := range strings.SplitSeq(filter, ",") {
		if pattern = strings.TrimSpace(pattern); pattern != "" {
			if _, err := path.Match(pattern, ""); err != nil {
				return nil, nil, fmt.Errorf("image filter %q: %w", pattern, err)
			}
			patterns = append(patterns, pattern)
		}
	}
	var images []string
	for _, name := range sortedKeys(m.Images) {
		if len(patterns) == 0 || slices.ContainsFunc(patterns, func(p string) bool { ok, err := path.Match(p, name); return err == nil && ok }) {
			images = append(images, name)
		}
	}
	if len(images) == 0 {
		return nil, nil, fmt.Errorf("GOZEN_DOCS_IMAGES_ONLY=%q matches no picture of %s", filter, manifestPath)
	}
	if shard != "" {
		part, count, ok := strings.Cut(shard, "/")
		index, e1 := strconv.Atoi(part)
		total, e2 := strconv.Atoi(count)
		if !ok || e1 != nil || e2 != nil || index < 1 || total < 1 || index > total {
			return nil, nil, fmt.Errorf("GOZEN_DOCS_SHARD=%q: want INDEX/COUNT with 1 <= INDEX <= COUNT", shard)
		}
		groups := m.captureGroups(images)
		if total > len(groups) {
			return nil, nil, fmt.Errorf("%d image shards exceed %d independent capture groups", total, len(groups))
		}
		// Longest groups first, with deterministic ties. Count captures before pictures:
		// rendering generally costs more than encoding. Phase timings guide future tuning.
		slices.SortStableFunc(groups, func(a, b []string) int {
			if d := len(m.imageRuns(b)) - len(m.imageRuns(a)); d != 0 {
				return d
			}
			return len(b) - len(a)
		})
		buckets := make([][]string, total)
		loads := make([]int, total)
		for _, group := range groups {
			target := 0
			for i := 1; i < total; i++ {
				if loads[i] < loads[target] || loads[i] == loads[target] && len(buckets[i]) < len(buckets[target]) {
					target = i
				}
			}
			buckets[target] = append(buckets[target], group...)
			loads[target] += len(m.imageRuns(group))
		}
		images = buckets[index-1]
		slices.Sort(images)
	}
	return images, m.imageRuns(images), nil
}

// captureGroups keeps images joined by any shared run together, including transitive
// montage dependencies. A run is never played twice across shards.
func (m manifest) captureGroups(images []string) [][]string {
	parent := map[string]string{}
	var find func(string) string
	find = func(s string) string {
		p, ok := parent[s]
		if !ok {
			parent[s] = s
			return s
		}
		if p != s {
			parent[s] = find(p)
		}
		return parent[s]
	}
	for _, name := range images {
		runs := m.imageRuns([]string{name})
		root := find(runs[0])
		for _, run := range runs[1:] {
			parent[find(run)] = find(root)
		}
	}
	groups := map[string][]string{}
	for _, name := range images {
		root := find(m.Images[name].Run)
		groups[root] = append(groups[root], name)
	}
	out := make([][]string, 0, len(groups))
	for _, root := range sortedKeys(groups) {
		out = append(out, groups[root])
	}
	return out
}

func TestImageShardCoverage(t *testing.T) {
	m := loadManifest(t)
	images, runs := map[string]bool{}, map[string]bool{}
	for part := 1; part <= 4; part++ {
		selection, captures, err := m.selectImages("", fmt.Sprintf("%d/4", part))
		if err != nil {
			t.Fatal(err)
		}
		again, _, err := m.selectImages("", fmt.Sprintf("%d/4", part))
		if err != nil || !slices.Equal(selection, again) {
			t.Fatal("nondeterministic selection")
		}
		for _, name := range selection {
			if images[name] {
				t.Fatalf("duplicate image %s", name)
			}
			images[name] = true
		}
		for _, name := range captures {
			if runs[name] {
				t.Fatalf("duplicate capture %s", name)
			}
			runs[name] = true
		}
		t.Logf("shard %d: %d images, %d runs, samples %v", part, len(selection), len(captures), m.samplesFor(captures))
	}
	all, allRuns, err := m.selectImages("", "")
	single, singleRuns, singleErr := m.selectImages("", "1/1")
	if singleErr != nil || !slices.Equal(single, all) || !slices.Equal(singleRuns, allRuns) {
		t.Fatal("one shard differs from the full check")
	}
	if err != nil || len(images) != len(all) || len(runs) != len(allRuns) {
		t.Fatal("shards omit work")
	}
}

func TestImageSelection(t *testing.T) {
	m := manifest{Images: map[string]image{
		"a.gif": {Run: "a", Frames: []pick{{Run: "b"}}},
		"b.gif": {Run: "b", Frames: []pick{{Run: "c"}}},
		"c.jpg": {Run: "c"}, "d.jpg": {Run: "d"},
	}, Runs: map[string]run{"a": {Sample: "one"}, "b": {Sample: "one"}, "c": {Sample: "two"}, "d": {Sample: "three"}}}
	for _, bad := range []string{"0/2", "3/2", "1/0", "1", "x/2", "1/2/3", "1/999999999"} {
		if _, _, err := m.selectImages("", bad); err == nil {
			t.Fatalf("accepted shard %q", bad)
		}
	}
	for _, bad := range []string{"absent*", "["} {
		if _, _, err := m.selectImages(bad, ""); err == nil {
			t.Fatalf("accepted filter %q", bad)
		}
	}
	a, runs, err := m.selectImages("", "1/2")
	if err != nil || !slices.Equal(a, []string{"a.gif", "b.gif", "c.jpg"}) || !slices.Equal(runs, []string{"a", "b", "c"}) {
		t.Fatalf("transitive group: %v %v %v", a, runs, err)
	}
	if got := m.samplesFor(runs); !slices.Equal(got, []string{"one", "two"}) {
		t.Fatal(got)
	}
	a, runs, err = m.selectImages(" d.*, c.jpg ", "")
	if err != nil || !slices.Equal(a, []string{"c.jpg", "d.jpg"}) || !slices.Equal(runs, []string{"c", "d"}) {
		t.Fatalf("filter: %v %v %v", a, runs, err)
	}
}
