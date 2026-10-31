package spine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// yamlFiles is every entry of dir named *.yaml or *.yml, in name order: hidden ones and broken
// links included.
func yamlFiles(dir string) []string {
	var out []string
	for _, e := range entries(dir) {
		if strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

// below is everything below dir, a folder before what it holds and each folder in name order,
// without following links to folders, and without what an unreadable folder holds.
func below(dir string) []string {
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

// matches reports whether name matches a shell-style pattern: `*` is any text, slashes included,
// `?` one character, and `[...]` one character in a set or `[!...]` one not in it. In a set, `a-z`
// is a range (a backwards one holds nothing), and `]` first or `-` first or last is itself; a `[`
// that is never closed is itself.
func matches(name, pattern string) bool {
	var re strings.Builder
	rs := []rune(pattern)
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '*':
			re.WriteString(`.*`)
		case '?':
			re.WriteString(`.`)
		case '[':
			if set, end, ok := charSet(rs, i+1); ok {
				re.WriteString(set)
				i = end
				continue
			}
			re.WriteString(literal('['))
		default:
			re.WriteString(literal(rs[i]))
		}
	}
	return regexp.MustCompile(`(?s)^` + re.String() + `$`).MatchString(name)
}

// charSet reads the set that starts at rs[i], just after its `[`: the set as a regular expression,
// and where its `]` is; false when the set is never closed.
func charSet(rs []rune, i int) (string, int, bool) {
	negate := i < len(rs) && rs[i] == '!'
	if negate {
		i++
	}
	var members strings.Builder
	for start := i; i < len(rs) && (rs[i] != ']' || i == start); i++ {
		if i+2 < len(rs) && rs[i+1] == '-' && rs[i+2] != ']' {
			if rs[i] <= rs[i+2] {
				members.WriteString(literal(rs[i]) + "-" + literal(rs[i+2]))
			}
			i += 2
			continue
		}
		members.WriteString(literal(rs[i]))
	}
	switch {
	case i == len(rs):
		return "", 0, false
	case members.Len() == 0 && negate:
		return `.`, i, true // nothing is excluded
	case members.Len() == 0:
		return `[^\x00-\x{10ffff}]`, i, true // nothing matches
	case negate:
		return "[^" + members.String() + "]", i, true
	}
	return "[" + members.String() + "]", i, true
}

// literal is r as a regular expression that matches only r.
func literal(r rune) string { return fmt.Sprintf(`\x{%x}`, r) }
