package spine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Files and folders: listing them, and matching names against patterns.

// entries lists a folder's entries in name order, or none when it can't be read.
func entries(dir string) []os.DirEntry {
	list, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	return list
}

// glob is every entry of dir whose name matches pattern, in name order: hidden ones and broken
// links included.
func glob(dir, pattern string) []string {
	var out []string
	for _, e := range entries(dir) {
		if fnmatch(e.Name(), pattern) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

// rglob is everything below dir, a folder before what it holds and each folder in name order,
// without following links to folders, and without what an unreadable folder holds.
func rglob(dir string) []string {
	var out []string
	var walk func(string)
	walk = func(d string) {
		for _, e := range entries(d) {
			p := filepath.Join(d, e.Name())
			out = append(out, p)
			if e.IsDir() { // a DirEntry reports a link as a link, not as what it points to
				walk(p)
			}
		}
	}
	walk(filepath.Clean(dir))
	return out
}

// isFile reports whether p is a regular file, following links.
func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

var (
	fnmatchMu    sync.Mutex
	fnmatchCache = map[string]*regexp.Regexp{}
)

// fnmatch reports whether name matches a shell-style pattern: `*` matches any text, slashes
// included, `?` one character, and `[...]` and `[!...]` one character in or not in a set.
func fnmatch(name, pattern string) bool {
	fnmatchMu.Lock()
	re, ok := fnmatchCache[pattern]
	if !ok {
		re = regexp.MustCompile(`(?s)^` + translate(pattern) + `$`)
		fnmatchCache[pattern] = re
	}
	fnmatchMu.Unlock()
	return re.MatchString(name)
}

func runeClass(r rune) string { return fmt.Sprintf(`\x{%x}`, r) }

// translate is a pattern as a Go regular expression, with every literal character escaped.
func translate(pattern string) string {
	pat := []rune(pattern)
	var b strings.Builder
	star := false
	for i, n := 0, len(pat); i < n; {
		c := pat[i]
		i++
		if c == '*' {
			if !star { // consecutive stars are one
				b.WriteString(`.*`)
			}
			star = true
			continue
		}
		star = false
		switch c {
		case '?':
			b.WriteString(`.`)
		case '[':
			j := i
			if j < n && pat[j] == '!' {
				j++
			}
			if j < n && pat[j] == ']' {
				j++
			}
			for j < n && pat[j] != ']' {
				j++
			}
			if j >= n {
				b.WriteString(runeClass('['))
				continue
			}
			b.WriteString(charClass(pat, i, j))
			i = j + 1
		default:
			b.WriteString(runeClass(c))
		}
	}
	return b.String()
}

// charClass is the set fnmatch builds for pat[i:j], the text between [ and ]: its chunks
// between range hyphens, with empty ranges removed, every character in them literal.
func charClass(pat []rune, i, j int) string {
	var chunks [][]rune
	if !containsRune(pat[i:j], '-') {
		chunks = [][]rune{pat[i:j]}
	} else {
		k := i + 1
		if pat[i] == '!' {
			k = i + 2
		}
		for {
			k = indexRune(pat, '-', k, j)
			if k < 0 {
				break
			}
			chunks = append(chunks, pat[i:k])
			i = k + 1
			k = k + 3
		}
		if chunk := pat[i:j]; len(chunk) > 0 {
			chunks = append(chunks, chunk)
		} else {
			last := len(chunks) - 1
			chunks[last] = append(append([]rune(nil), chunks[last]...), '-')
		}
		for k := len(chunks) - 1; k > 0; k-- {
			prev, cur := chunks[k-1], chunks[k]
			if prev[len(prev)-1] > cur[0] {
				chunks[k-1] = append(append([]rune(nil), prev[:len(prev)-1]...), cur[1:]...)
				chunks = append(chunks[:k], chunks[k+1:]...)
			}
		}
	}
	total := 0
	for _, c := range chunks {
		total += len(c)
	}
	switch {
	case total == 0:
		return `[^\x00-\x{10ffff}]` // an empty set never matches
	case total == 1 && chunks[0][0] == '!' && len(chunks) == 1:
		return `.` // a negated empty set matches any character
	}
	var b strings.Builder
	b.WriteString("[")
	if chunks[0][0] == '!' {
		b.WriteString("^")
		chunks[0] = chunks[0][1:]
	}
	for c, chunk := range chunks {
		for x, r := range chunk {
			switch {
			case x == len(chunk)-1 && c < len(chunks)-1:
				b.WriteString(runeClass(r) + "-") // the hyphen between chunks is a range
			default:
				b.WriteString(runeClass(r))
			}
		}
	}
	b.WriteString("]")
	return b.String()
}

func containsRune(rs []rune, r rune) bool { return indexRune(rs, r, 0, len(rs)) >= 0 }

func indexRune(rs []rune, r rune, from, to int) int {
	for k := from; k < to; k++ {
		if rs[k] == r {
			return k
		}
	}
	return -1
}
