package gfx

import (
	"math"
	"strings"
)

const PageShrink = 0.8

func Paginate(s string, size, width float64, lines int) ([][]string, float64) {
	return paginateWith(s, size, lines, func(t string, sz float64) []string { return Wrap(t, sz, width) })
}

func paginateWith(s string, size float64, lines int, wrap func(string, float64) []string) ([][]string, float64) {
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		return nil, 0
	}
	if lines < 1 {
		lines = 1
	}
	wr := func(t string, sz float64) []string {
		ls := wrap(strings.TrimRight(t, "\n"), sz)
		for i := 1; i < len(ls); i++ {
			ls[i] = strings.TrimLeft(ls[i], " ")
		}
		return ls
	}
	sizes := []float64{size}
	for sz := size * 0.94; sz >= size*PageShrink; sz *= 0.94 {
		sizes = append(sizes, sz)
	}
	for _, sz := range sizes {
		if ls := wr(s, sz); len(ls) <= lines {
			return [][]string{ls}, sz
		}
	}

	var best []string
	bestSize := size
	for _, sz := range sizes {
		if p := splitPages(s, sz, lines, wr); p != nil && (best == nil || len(p) < len(best)) {
			best, bestSize = p, sz
		}
	}
	if best == nil {
		ls := wr(s, size)
		var out [][]string
		for i := 0; i < len(ls); i += lines {
			out = append(out, ls[i:min(len(ls), i+lines)])
		}
		return out, size
	}
	out := make([][]string, len(best))
	for i, p := range best {
		out[i] = wr(p, bestSize)
	}
	return out, bestSize
}

func splitPages(s string, size float64, lines int, wr func(string, float64) []string) []string {

	var parts []string
	for _, u := range sentences(s) {
		body := strings.TrimRight(u, "\n")
		ls := wr(body, size)
		if len(ls) <= lines {
			parts = append(parts, u)
			continue
		}
		n := (len(ls) + lines - 1) / lines
		per := (len(ls) + n - 1) / n
		for i := 0; i < len(ls); i += per {
			parts = append(parts, strings.Join(ls[i:min(len(ls), i+per)], ""))
		}
		parts[len(parts)-1] += u[len(body):]
	}
	type cell struct {
		pages, longest, from int
		ok                   bool
	}
	dp := make([]cell, len(parts)+1)
	dp[0].ok = true
	for i := 1; i <= len(parts); i++ {
		for j := i - 1; j >= 0; j-- {
			if !dp[j].ok {
				continue
			}
			page := strings.Join(parts[j:i], "")
			if len(wr(page, size)) > lines {
				break
			}
			c := cell{pages: dp[j].pages + 1, longest: max(dp[j].longest, len([]rune(page))), from: j, ok: true}
			if !dp[i].ok || c.pages < dp[i].pages || c.pages == dp[i].pages && c.longest < dp[i].longest {
				dp[i] = c
			}
		}
	}
	if !dp[len(parts)].ok {
		return nil
	}
	var out []string
	for i := len(parts); i > 0; i = dp[i].from {
		out = append([]string{strings.TrimLeft(strings.Join(parts[dp[i].from:i], ""), " \n")}, out...)
	}
	return out
}

func sentences(s string) []string {
	var out []string
	r := []rune(s)
	start := 0
	for i := 0; i < len(r); i++ {
		end := false
		switch r[i] {
		case '\n':
			end = true
		case '。', '！', '？', '!', '?', '」', '』':
			end = i+1 >= len(r) || !isClosing(r[i+1]) && r[i+1] != '」' && r[i+1] != '』' && r[i+1] != '）'
			if end {
				for i+1 < len(r) && r[i+1] == '\n' {
					i++
				}
			}
		}
		if end {
			out = append(out, string(r[start:i+1]))
			start = i + 1
		}
	}
	if start < len(r) {
		out = append(out, string(r[start:]))
	}
	return out
}
