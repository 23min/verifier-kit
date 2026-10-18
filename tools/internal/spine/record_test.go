package spine

// The record (#44): what the Go spine gave on every input of the cases test, kept in
// testdata/record/ as the Go spine's reference now that tools/spine.py is gone. For each
// run it holds the exit code, the output, and in JSON runs the JUnit report, with the paths that
// differ between machines written as $ROOT and $TMP. The Go spine must give exactly what is
// recorded, and every entry must still have its case.
//
// `go -C tools test ./internal/spine/ -update` writes the record again from the Go spine.
// Review the diff before committing it: the record is the only reference.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

var update = flag.Bool("update", false, "write testdata/record/ again from the Go spine")

type entry struct {
	exit   int
	stdout string
	junit  *string // nil when the run has no JUnit report
}

var record = struct {
	sync.Mutex
	tmp  string                      // TestCases's temporary folder, written $TMP
	want map[string]map[string]entry // file, then key
	got  map[string]map[string]entry
}{}

// recordFile names the file a case's runs are kept in, from the case's name.
func recordFile(name string) string {
	for _, f := range []struct{ prefix, file string }{
		{"cases.txt", "cases"}, {"cli.txt", "cli"}, {"spines/", "files"}, {"seeds/", "files"},
		{"yaml/", "yaml"}, {"spine variant", "variants"}, {"behavior variant", "variants"},
		{"policy variant", "variants"}, {"value ", "values"}, {"filesystem", "filesystem"},
		{"stdin not UTF-8", "filesystem"}, {"example", "examples"}, {"fault ", "faults"},
		{"conditions", "conditions"},
	} {
		if strings.HasPrefix(name, f.prefix) {
			return f.file
		}
	}
	return ""
}

// normal writes the paths that differ between machines as $TMP and $ROOT, and the Python the
// test puts in an --adapter-cmd, $PYTHON, as python3, its default.
func normal(s string) string {
	if pythonPath != "python3" {
		s = strings.ReplaceAll(s, shellQuote(pythonPath), "python3")
	}
	if record.tmp != "" {
		s = strings.ReplaceAll(s, record.tmp, "$TMP")
	}
	return strings.ReplaceAll(s, repoRoot, "$ROOT")
}

// word is s as one word of a key: as it is when that is unambiguous, else Go-quoted.
func word(s string) string {
	if s != "" && plain(s) && !strings.ContainsAny(s, " \"'\\") {
		return s
	}
	return strconv.Quote(s)
}

// key names one run of a case: its folder, its command line, its input and its environment.
func (c spineCase) key(args []string) string {
	dir := normal(c.dir)
	if rel, err := filepath.Rel(repoRoot, c.dir); err == nil && !strings.HasPrefix(rel, "..") {
		dir = rel
	}
	parts := []string{"in", word(dir) + ":", "spine"}
	for _, a := range args {
		parts = append(parts, word(normal(a)))
	}
	if c.stdin != "" {
		parts = append(parts, "<", word(c.stdin))
	}
	env := append([]string(nil), c.env...)
	sort.Strings(env)
	for _, e := range env {
		parts = append(parts, "with", word(normal(e)))
	}
	return strings.Join(parts, " ")
}

// recordRun compares one run of the Go spine with the record, or keeps it to write the record.
func (c spineCase) recordRun(t *testing.T, args []string, r result, junit *string) {
	t.Helper()
	file := recordFile(c.name)
	if file == "" {
		t.Fatalf("%s: no file in testdata/record/ for this case; add its prefix to recordFile", c.name)
	}
	got := entry{exit: r.exit, stdout: normal(r.stdout)}
	if junit != nil {
		j := normal(*junit)
		got.junit = &j
	}
	k := c.key(args)
	record.Lock()
	defer record.Unlock()
	if record.got[file] == nil {
		record.got[file] = map[string]entry{}
	}
	if _, dup := record.got[file][k]; dup {
		t.Errorf("%s: two runs have the same key in testdata/record/%s.txt: %s", c.name, file, k)
	}
	record.got[file][k] = got
	if *update {
		return
	}
	want, ok := record.want[file][k]
	switch {
	case !ok:
		t.Errorf("%s: not in testdata/record/%s.txt: %s\nrun go -C tools test ./internal/spine/ -update, and review the diff", c.name, file, k)
	case want.exit != got.exit || want.stdout != got.stdout || (want.junit == nil) != (got.junit == nil) ||
		want.junit != nil && *want.junit != *got.junit:
		t.Errorf("%s: %s\nthe Go spine differs from testdata/record/%s.txt\nrecorded:\n%s\ngot:\n%s", c.name, k, file,
			formatEntry(k, want), formatEntry(k, got))
	}
}

// loadRecord reads testdata/record/, unless the test is to write it.
func loadRecord(t *testing.T, td, tmp string) {
	record.tmp = tmp
	record.want, record.got = map[string]map[string]entry{}, map[string]map[string]entry{}
	if *update {
		return
	}
	paths, err := filepath.Glob(filepath.Join(td, "record", "*.txt"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no testdata/record/*.txt (%v); run go -C tools test ./internal/spine/ -update", err)
	}
	for _, p := range paths {
		entries, err := readRecord(p)
		if err != nil {
			t.Fatal(err)
		}
		record.want[strings.TrimSuffix(filepath.Base(p), ".txt")] = entries
	}
}

// filtered tells a run of only some cases, by -run with a subtest pattern or by -skip.
func filtered() bool {
	run, skip := flag.Lookup("test.run"), flag.Lookup("test.skip")
	return run != nil && strings.Contains(run.Value.String(), "/") || skip != nil && skip.Value.String() != ""
}

// finishRecord writes the record, or reports the entries no case gave.
func finishRecord(t *testing.T, td string) {
	if *update {
		if filtered() {
			t.Fatal("-update writes the whole record, so it runs every case: drop the subtest pattern or -skip")
		}
		dir := filepath.Join(td, "record")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		old, _ := filepath.Glob(filepath.Join(dir, "*.txt"))
		for _, p := range old {
			if _, ok := record.got[strings.TrimSuffix(filepath.Base(p), ".txt")]; !ok {
				os.Remove(p)
			}
		}
		for file, entries := range record.got {
			if err := writeRecord(filepath.Join(dir, file+".txt"), entries); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	if filtered() {
		return
	}
	for file, entries := range record.want {
		var stale []string
		for k := range entries {
			if _, ok := record.got[file][k]; !ok {
				stale = append(stale, k)
			}
		}
		sort.Strings(stale)
		if len(stale) > 0 {
			t.Errorf("testdata/record/%s.txt has %d entries no case gives, first: %s\nrun go -C tools test ./internal/spine/ -update, and review the diff",
				file, len(stale), stale[0])
		}
	}
}

// ---------- the record's files ----------
//
// Each entry starts "=== " and its key, then "exit N", then a section per output, "--- stdout"
// and in JSON runs "--- junit". Each line of an output is "| " and the line, or "|" alone for
// an empty line; a line that isn't plain text is "|q " and the line Go-quoted in ASCII. An
// output ending in a newline ends in an empty line.

// plain tells a line that can be written as it is: UTF-8, no control characters but tab, and
// no space or tab at the end, which editors strip.
func plain(s string) bool {
	if !utf8.ValidString(s) || strings.HasSuffix(s, " ") || strings.HasSuffix(s, "\t") {
		return false
	}
	for _, r := range s {
		if r < 0x20 && r != '\t' || r == 0x7f || r == utf8.RuneError {
			return false
		}
	}
	return true
}

func formatOutput(b *strings.Builder, name, s string) {
	fmt.Fprintf(b, "--- %s\n", name)
	for _, line := range strings.Split(s, "\n") {
		switch {
		case line == "":
			b.WriteString("|\n")
		case plain(line):
			b.WriteString("| " + line + "\n")
		default:
			b.WriteString("|q " + strconv.QuoteToASCII(line) + "\n")
		}
	}
}

func formatEntry(k string, e entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== %s\nexit %d\n", k, e.exit)
	formatOutput(&b, "stdout", e.stdout)
	if e.junit != nil {
		formatOutput(&b, "junit", *e.junit)
	}
	return b.String()
}

func writeRecord(path string, entries map[string]entry) error {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(formatEntry(k, entries[k]))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func readRecord(path string) (map[string]entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries := map[string]entry{}
	var k string
	var e *entry
	var section string
	var lines []string
	flush := func() {
		if e == nil || section == "" {
			return
		}
		s := strings.Join(lines, "\n")
		if section == "stdout" {
			e.stdout = s
		} else {
			e.junit = &s
		}
		section, lines = "", nil
	}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 1<<16), 1<<26)
	for n := 1; scan.Scan(); n++ {
		line := scan.Text()
		bad := func() error { return fmt.Errorf("%s:%d: cannot read %q", path, n, line) }
		switch {
		case strings.HasPrefix(line, "=== "):
			flush()
			if e != nil {
				entries[k] = *e
			}
			k, e = line[4:], &entry{}
			if _, dup := entries[k]; dup {
				return nil, fmt.Errorf("%s:%d: the key is there twice: %s", path, n, k)
			}
		case e == nil:
			return nil, bad()
		case strings.HasPrefix(line, "exit ") && section == "":
			if e.exit, err = strconv.Atoi(line[5:]); err != nil {
				return nil, bad()
			}
		case line == "--- stdout" || line == "--- junit":
			flush()
			section = line[4:]
		case section != "" && line == "|":
			lines = append(lines, "")
		case section != "" && strings.HasPrefix(line, "|q "):
			s, err := strconv.Unquote(line[3:])
			if err != nil {
				return nil, bad()
			}
			lines = append(lines, s)
		case section != "" && strings.HasPrefix(line, "| "):
			lines = append(lines, line[2:])
		default:
			return nil, bad()
		}
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	flush()
	if e != nil {
		entries[k] = *e
	}
	return entries, nil
}
