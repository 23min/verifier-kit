package spine

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// Python's pathlib, fnmatch and json, as far as tools/spine.py relies on them.

// pyPath is str(Path(p)): no empty or "." parts, no trailing slash, and "." for nothing.
// POSIX keeps exactly two leading slashes, so Python does too.
func pyPath(p string) string {
	root := ""
	switch {
	case strings.HasPrefix(p, "//") && !strings.HasPrefix(p, "///"):
		root = "//"
	case strings.HasPrefix(p, "/"):
		root = "/"
	}
	var parts []string
	for _, part := range strings.Split(p, "/") {
		if part != "" && part != "." {
			parts = append(parts, part)
		}
	}
	if root == "" && len(parts) == 0 {
		return "."
	}
	return root + strings.Join(parts, "/")
}

// pyJoin is str(Path(dir, name)) for a name without slashes.
func pyJoin(dir, name string) string {
	return pyPath(dir + "/" + name)
}

// pathLess is Path ordering: by the parts between slashes, then by length.
func pathLess(a, b string) bool {
	pa, pb := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}

// entries lists a directory's names, or none when it can't be read, as pathlib's glob does.
func entries(dir string) []os.DirEntry {
	list, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	return list
}

// glob is sorted(Path(dir).glob(pattern)) for a pattern without slashes: every entry whose name
// matches, hidden ones and broken links included.
func glob(dir, pattern string) []string {
	var out []string
	for _, e := range entries(dir) {
		if fnmatch(e.Name(), pattern) {
			out = append(out, pyJoin(dir, e.Name()))
		}
	}
	sort.Slice(out, func(i, j int) bool { return pathLess(out[i], out[j]) })
	return out
}

// rglob is sorted(Path(dir).rglob("*")): everything below dir, without following links to
// directories, and without what an unreadable directory holds.
func rglob(dir string) []string {
	var out []string
	var walk func(string)
	walk = func(d string) {
		for _, e := range entries(d) {
			p := pyJoin(d, e.Name())
			out = append(out, p)
			if e.IsDir() { // a DirEntry reports a link as a link, not as what it points to
				walk(p)
			}
		}
	}
	walk(pyPath(dir))
	sort.Slice(out, func(i, j int) bool { return pathLess(out[i], out[j]) })
	return out
}

// isFile is Path.is_file(): a regular file, following links.
func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

var (
	fnmatchMu    sync.Mutex
	fnmatchCache = map[string]*regexp.Regexp{}
)

// fnmatch is fnmatch.fnmatch on POSIX: `*` matches any text, slashes included.
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

// translate is fnmatch.translate, written as Go regexp syntax with every literal escaped.
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

// pyJSONString is a JSON string as Python's json module writes it with ensure_ascii. A byte
// that isn't UTF-8 (from a command line or stdin) is the surrogate Python's surrogateescape
// decoding gives it.
func pyJSONString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, `\udc%02x`, s[i])
			i++
			continue
		}
		i += size
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			switch {
			case r >= ' ' && r <= '~':
				b.WriteRune(r)
			case r < 0x10000:
				fmt.Fprintf(&b, `\u%04x`, r)
			default:
				r -= 0x10000
				fmt.Fprintf(&b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
